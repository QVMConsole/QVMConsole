package pool

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"kvm_console/model"
)

var storageIDSanitizer = regexp.MustCompile(`[^a-zA-Z0-9_.-]+`)

func buildStoragePoolTree(devices []lsblkDevice, mounts map[string]findmntInfo, dfUsage map[string]mountUsage, aliases map[string]string, configs map[string]model.HostStoragePool) []HostStoragePoolInfo {
	result := make([]HostStoragePoolInfo, 0, len(devices))
	for _, dev := range devices {
		result = append(result, buildStoragePoolNode(dev, mounts, dfUsage, aliases, configs))
	}
	return result
}

func buildStoragePoolNode(dev lsblkDevice, mounts map[string]findmntInfo, dfUsage map[string]mountUsage, aliases map[string]string, configs map[string]model.HostStoragePool) HostStoragePoolInfo {
	devicePath := dev.Path
	if devicePath == "" && dev.KName != "" {
		devicePath = "/dev/" + dev.KName
	}
	idSource := aliases[devicePath]
	if idSource == "" {
		idSource = devicePath
	}
	id := normalizeStorageDeviceID(idSource)
	cfg, configured := configs[id]
	mountpoints := normalizeMountpoints([]string(dev.Mountpoints))

	node := HostStoragePoolInfo{
		ID:          id,
		Name:        dev.Name,
		DevicePath:  devicePath,
		KName:       dev.KName,
		Type:        dev.Type,
		Size:        dev.Size,
		FSType:      dev.FSType,
		FSVersion:   dev.FSVersion,
		Label:       dev.Label,
		UUID:        dev.UUID,
		Mountpoints: mountpoints,
		Model:       strings.TrimSpace(dev.Model),
		Serial:      strings.TrimSpace(dev.Serial),
		Rota:        dev.Rota,
		Removable:   dev.Removable,
		Readonly:    dev.Readonly,
		Tran:        dev.Tran,
		PKName:      dev.PKName,
		Configured:  configured,
	}
	if configured {
		node.DisplayName = cfg.DisplayName
		node.Enabled = cfg.Enabled
		node.IsDefault = cfg.IsDefault
		node.MountPath = cfg.MountPath
	}
	if node.DisplayName == "" {
		node.DisplayName = defaultStorageDisplayName(node)
	}
	if node.MountPath == "" && len(mountpoints) > 0 {
		node.MountPath = mountpoints[0]
	}
	if node.MountPath != "" {
		node.VMDir = filepath.Join(node.MountPath, "vm-disks")
		if cloneDir := configuredCloneDir(); isPathUnderMount(cloneDir, node.MountPath) {
			node.VMDir = cloneDir
		}
	}
	applyUsage(&node, mounts, dfUsage)

	for _, child := range dev.Children {
		node.Children = append(node.Children, buildStoragePoolNode(child, mounts, dfUsage, aliases, configs))
	}
	node.SystemDisk = isSystemStorageNode(node)
	node.CanFormat, node.StatusReason = canFormatStorageNode(node)
	node.CanUseForVM = canUseStorageNode(node)
	if !node.CanUseForVM && node.StatusReason == "" {
		node.StatusReason = "硬盘未挂载，无法作为虚拟机存储位置"
	}
	if node.Enabled && !node.CanUseForVM {
		node.Enabled = false
	}

	// 检测已有数据警告（仅对可格式化的整盘显示）
	if node.CanFormat && node.Type == "disk" {
		if hasData, warn := detectExistingData(node); hasData {
			node.HasExistingData = true
			node.ExistingDataWarning = warn
		}
	}

	return node
}

func applyUsage(node *HostStoragePoolInfo, mounts map[string]findmntInfo, dfUsage map[string]mountUsage) {
	for _, mp := range node.Mountpoints {
		if usage, ok := dfUsage[mp]; ok {
			node.Size = usage.Size
			node.Used = usage.Used
			node.Available = usage.Available
			break
		}
		if info, ok := mounts[mp]; ok {
			if info.Size > 0 {
				node.Size = info.Size
			}
			node.Used = info.Used
			node.Available = info.Avail
			break
		}
	}
	if node.Size > 0 && node.Used > 0 {
		node.UsePercent = int(float64(node.Used) / float64(node.Size) * 100)
	}
}

func canFormatStorageNode(node HostStoragePoolInfo) (bool, string) {
	if node.Readonly {
		return false, "设备为只读状态"
	}
	if node.Type != "disk" && node.Type != "part" && node.Type != "lv" {
		return false, "只支持格式化整块硬盘、分区或逻辑卷"
	}
	if node.Type == "loop" || node.Type == "rom" || node.Removable {
		return false, "不支持格式化 loop、光驱或可移动设备"
	}
	// 排除 device-mapper 设备
	if node.Type == "dm" || strings.HasPrefix(node.Name, "dm-") {
		return false, "不支持格式化 device-mapper 设备"
	}
	// 排除内存盘
	nameLower := strings.ToLower(node.Name)
	if strings.HasPrefix(nameLower, "ram") || strings.HasPrefix(nameLower, "zram") {
		return false, "不支持格式化内存盘设备"
	}
	// 排除 LVM 物理卷
	fstype := strings.ToLower(node.FSType)
	if fstype == "lvm2_member" {
		return false, "该设备已被用作 LVM 物理卷"
	}
	// 排除 mdraid 成员
	if fstype == "linux_raid_member" {
		return false, "该设备已加入 mdraid 阵列"
	}
	// 排除 ZFS 存储池成员
	if fstype == "zfs_member" {
		return false, "该设备已加入 ZFS 存储池"
	}
	// 检查子设备中是否有 LVM/raid/zfs 成员
	if hasChildWithFSType(node, "lvm2_member", "linux_raid_member", "zfs_member") {
		reason := "该设备的子分区已加入"
		var parts []string
		if hasChildWithFSType(node, "lvm2_member") {
			parts = append(parts, "LVM")
		}
		if hasChildWithFSType(node, "linux_raid_member") {
			parts = append(parts, "mdraid")
		}
		if hasChildWithFSType(node, "zfs_member") {
			parts = append(parts, "ZFS")
		}
		return false, reason + strings.Join(parts, "/") + "，无法格式化"
	}
	// LV 允许在线格式化（格式化流程内部会先卸载再格式化再挂载）
	if node.Type != "lv" {
		if len(node.Mountpoints) > 0 || hasMountedChild(node) {
			return false, "设备或其分区当前已挂载"
		}
	}
	if node.SystemDisk {
		return false, "系统关键磁盘禁止格式化"
	}
	return true, ""
}

func canUseStorageNode(node HostStoragePoolInfo) bool {
	if node.Readonly || node.Type == "rom" || node.Type == "loop" {
		return false
	}
	return node.MountPath != "" && len(node.Mountpoints) > 0
}

func validateFormatTarget(pool HostStoragePoolInfo) error {
	if ok, reason := canFormatStorageNode(pool); !ok {
		return fmt.Errorf("该硬盘不能格式化: %s", reason)
	}
	return nil
}

func isSystemStorageNode(node HostStoragePoolInfo) bool {
	for _, mp := range node.Mountpoints {
		switch mp {
		case "/", "/boot", "/boot/efi", "/usr", "/var", "/home":
			return true
		}
	}
	for _, child := range node.Children {
		if isSystemStorageNode(child) {
			return true
		}
	}
	return false
}

func hasMountedChild(node HostStoragePoolInfo) bool {
	for _, child := range node.Children {
		if len(child.Mountpoints) > 0 || hasMountedChild(child) {
			return true
		}
	}
	return false
}

// hasChildWithFSType 递归检查节点或其子节点是否存在指定 FSType。
func hasChildWithFSType(node HostStoragePoolInfo, fstypes ...string) bool {
	for _, child := range node.Children {
		for _, ft := range fstypes {
			if strings.EqualFold(child.FSType, ft) {
				return true
			}
		}
		if hasChildWithFSType(child, fstypes...) {
			return true
		}
	}
	return false
}

// detectExistingData 检测磁盘是否已有分区表或文件系统（仅对可格式化的整盘生效）。
func detectExistingData(node HostStoragePoolInfo) (bool, string) {
	if node.Type != "disk" {
		return false, ""
	}
	if len(node.Mountpoints) > 0 || node.SystemDisk {
		return false, ""
	}
	// 跳过已被 LVM/raid/zfs 使用的磁盘（已有专门的错误提示）
	if hasChildWithFSType(node, "lvm2_member", "linux_raid_member", "zfs_member") {
		return false, ""
	}
	// 检测是否存在分区表或已有文件系统
	hasData := len(node.Children) > 0 || node.FSType != ""
	if hasData {
		return true, "该磁盘上检测到已有分区或文件系统，继续操作可能导致数据丢失。"
	}
	return false, ""
}

func normalizeMountpoints(items []string) []string {
	var result []string
	seen := make(map[string]bool)
	for _, item := range items {
		item = strings.TrimSpace(item)
		if item == "" || item == "[SWAP]" || seen[item] {
			continue
		}
		seen[item] = true
		result = append(result, item)
	}
	return result
}

func normalizeStorageDeviceID(raw string) string {
	raw = strings.TrimSpace(raw)
	raw = strings.TrimPrefix(raw, "/dev/disk/by-id/")
	raw = strings.TrimPrefix(raw, "/dev/disk/by-uuid/")
	raw = strings.TrimPrefix(raw, "/dev/")
	raw = storageIDSanitizer.ReplaceAllString(raw, "-")
	raw = strings.Trim(raw, "-")
	if raw == "" {
		raw = "unknown"
	}
	return raw
}

func defaultStorageMountPath(id string) string {
	return filepath.Join(hostStorageRoot, normalizeStorageDeviceID(id))
}

func defaultStorageDisplayName(pool HostStoragePoolInfo) string {
	if pool.Label != "" {
		return pool.Label
	}
	if pool.Model != "" {
		return strings.TrimSpace(fmt.Sprintf("%s %s", pool.Model, pool.Name))
	}
	if pool.DevicePath != "" {
		return pool.DevicePath
	}
	return pool.Name
}

func isPathUnderMount(pathValue, mountPath string) bool {
	pathValue = filepath.Clean(strings.TrimSpace(pathValue))
	mountPath = filepath.Clean(strings.TrimSpace(mountPath))
	if pathValue == "." || mountPath == "." {
		return false
	}
	if mountPath == string(os.PathSeparator) {
		return filepath.IsAbs(pathValue)
	}
	return pathValue == mountPath || strings.HasPrefix(pathValue, mountPath+string(os.PathSeparator))
}

// ── LVM 层级注入 ──

// injectLVMTree 将 VG/LV 层级信息注入存储池树，并移除冗余的 dm-X 节点。
func injectLVMTree(pools []HostStoragePoolInfo, vgs []VGInfo, lvs []LVInfo,
	mounts map[string]findmntInfo, dfUsage map[string]mountUsage, configs map[string]model.HostStoragePool) []HostStoragePoolInfo {

	// 构建 VG 名 → LV 列表映射
	vgLVMap := make(map[string][]LVInfo)
	for _, lv := range lvs {
		parts := strings.SplitN(lv.FullName, "/", 2)
		if len(parts) == 2 {
			vgLVMap[parts[0]] = append(vgLVMap[parts[0]], lv)
		}
	}

	// 构建 VG 名 → PV 设备路径映射
	vgPVMap := make(map[string][]string)
	for _, vg := range vgs {
		vgPVMap[vg.Name] = vg.PVNames
	}

	// 收集 LV 设备路径（用于后续从树中移除）
	// 同时添加 /dev/mapper/ 格式的路径，因为 lsblk 可能使用 mapper 格式
	lvDevicePaths := make(map[string]bool)
	for _, lv := range lvs {
		if lv.DevicePath != "" {
			lvDevicePaths[lv.DevicePath] = true
		}
		// 添加 /dev/mapper/VG-LV 格式（VG/LV 名称中的“-”转义为“--”）
		parts := strings.SplitN(lv.FullName, "/", 2)
		if len(parts) == 2 {
			mapperName := strings.ReplaceAll(parts[0], "-", "--") + "-" + strings.ReplaceAll(parts[1], "-", "--")
			lvDevicePaths["/dev/mapper/"+mapperName] = true
		}
	}

	// 标记 PV 设备路径 → VG 名 映射
	pvToVG := make(map[string]string)
	for _, vg := range vgs {
		for _, pvPath := range vg.PVNames {
			pvToVG[pvPath] = vg.Name
		}
	}

	// 从树中移除 LV 的 dm-X 节点并更新其父节点信息
	filteredPools := removeLVNodesFromTree(pools, lvDevicePaths)

	// 标记 PV 节点
	markPVNodes(filteredPools, pvToVG)

	// 为每个 VG 创建合成节点
	for _, vg := range vgs {
		vgID := normalizeStorageDeviceID("vg-" + vg.Name)
		vgNode := HostStoragePoolInfo{
			ID:          vgID,
			Name:        vg.Name,
			DisplayName: "VG: " + vg.Name,
			DevicePath:  "/dev/" + vg.Name,
			Type:        "vg",
			Size:        vg.Size,
			Available:   vg.Free,
			Used:        vg.Size - vg.Free,
			IsLVMVG:     true,
			PVCount:     vg.PVCount,
			LVCount:     vg.LVCount,
		}
		if vg.Size > 0 {
			vgNode.UsePercent = int(float64(vgNode.Used) / float64(vg.Size) * 100)
		}

		// 添加 LV 子节点
		if vgLVs, ok := vgLVMap[vg.Name]; ok {
			for _, lv := range vgLVs {
				lvNode := buildLVNode(lv, vg.Name, mounts, dfUsage, configs)
				vgNode.Children = append(vgNode.Children, lvNode)
			}
		}

		// 添加 PV 子节点（作为引用节点，不可操作）
		if pvPaths, ok := vgPVMap[vg.Name]; ok {
			for _, pvPath := range pvPaths {
				pvNode := buildPVReferenceNode(pvPath, vg.Name)
				if pvNode != nil {
					vgNode.Children = append(vgNode.Children, *pvNode)
				}
			}
		}

		// VG 节点本身不能作为 VM 存储目标（只有 LV 可以）
		vgNode.CanUseForVM = false
		vgNode.SystemDisk = isSystemStorageNode(vgNode)

		filteredPools = append(filteredPools, vgNode)
	}

	return filteredPools
}

// buildLVNode 构建 LV 子节点。
func buildLVNode(lv LVInfo, vgName string, mounts map[string]findmntInfo,
	dfUsage map[string]mountUsage, configs map[string]model.HostStoragePool) HostStoragePoolInfo {

	id := normalizeStorageDeviceID(vgName + "-" + lv.Name)
	cfg, configured := configs[id]

	// 回退：如果主 ID 未找到配置，尝试使用 mapper 格式的 ID 查找
	// （用户可能通过 lsblk 树的 dm 节点配置了该存储池）
	if !configured {
		mapperName := strings.ReplaceAll(vgName, "-", "--") + "-" + strings.ReplaceAll(lv.Name, "-", "--")
		mapperID := normalizeStorageDeviceID("/dev/mapper/" + mapperName)
		if mapperCfg, ok := configs[mapperID]; ok {
			cfg = mapperCfg
			configured = true
		}
	}

	node := HostStoragePoolInfo{
		ID:          id,
		Name:        lv.Name,
		DisplayName: lv.Name,
		DevicePath:  lv.DevicePath,
		Type:        "lv",
		Size:        lv.Size,
		FSType:      lv.FSType,
		VGName:      vgName,
		LVType:      lv.LVType,
		MountPath:   lv.MountPath,
		Children:    nil,
		Configured:  configured,
	}

	if configured {
		node.DisplayName = cfg.DisplayName
		node.Enabled = cfg.Enabled
		node.IsDefault = cfg.IsDefault
		node.MountPath = cfg.MountPath
	}

	// 设置挂载点（必须在 applyUsage 之前，否则无法查找使用量）
	mountPathForVM := lv.MountPath
	// 回退：如果运行时扫描未检测到挂载点，但 DB 配置中有 mount_path，使用配置值
	if mountPathForVM == "" && configured && cfg.MountPath != "" {
		mountPathForVM = cfg.MountPath
	}
	if mountPathForVM != "" {
		node.Mountpoints = []string{mountPathForVM}
		node.MountPath = mountPathForVM
		node.VMDir = filepath.Join(mountPathForVM, "vm-disks")
	}

	// 应用使用量（在挂载点设置之后）
	applyUsage(&node, mounts, dfUsage)

	node.CanUseForVM = node.MountPath != "" && len(node.Mountpoints) > 0
	node.SystemDisk = isSystemStorageNode(node)
	node.CanFormat, node.StatusReason = canFormatStorageNode(node)

	return node
}

// buildPVReferenceNode 构建 PV 引用子节点。
// PV 引用节点仅用于展示层级关系，容量已由 VG/LV 节点体现，因此大小始终为 0。
func buildPVReferenceNode(pvPath, vgName string) *HostStoragePoolInfo {
	return &HostStoragePoolInfo{
		ID:           normalizeStorageDeviceID("pv-" + vgName + "-" + filepath.Base(pvPath)),
		Name:         filepath.Base(pvPath),
		DisplayName:  filepath.Base(pvPath),
		DevicePath:   pvPath,
		Type:         "pv",
		Size:         0,
		VGName:       vgName,
		Readonly:     true, // PV 节点不允许单独操作
		StatusReason: "已加入卷组 " + vgName,
	}
}

// removeLVNodesFromTree 从树中移除属于 LVM LV 的 dm-X 节点。
func removeLVNodesFromTree(pools []HostStoragePoolInfo, lvDevicePaths map[string]bool) []HostStoragePoolInfo {
	result := make([]HostStoragePoolInfo, 0, len(pools))
	for _, pool := range pools {
		if lvDevicePaths[pool.DevicePath] {
			continue // 跳过 LV 设备
		}
		// 递归处理子节点
		pool.Children = removeLVNodesFromTree(pool.Children, lvDevicePaths)
		result = append(result, pool)
	}
	return result
}

// markPVNodes 标记属于 VG 的 PV 节点，并将其容量清零以避免与 VG/LV 重复计算。
func markPVNodes(pools []HostStoragePoolInfo, pvToVG map[string]string) {
	for i := range pools {
		if vgName, ok := pvToVG[pools[i].DevicePath]; ok {
			pools[i].VGName = vgName
			pools[i].Readonly = true
			pools[i].CanFormat = false
			pools[i].CanUseForVM = false
			pools[i].StatusReason = "已加入卷组 " + vgName
			// 清零容量：PV 磁盘的容量已由 VG/LV 节点体现，不应重复计入总计
			pools[i].Size = 0
			pools[i].Used = 0
			pools[i].Available = 0
			pools[i].UsePercent = 0
		}
		markPVNodes(pools[i].Children, pvToVG)
	}
}

// hasAnyMountedChild 检查节点是否有已挂载的子节点。
func hasAnyMountedChild(node HostStoragePoolInfo) bool {
	if len(node.Mountpoints) > 0 {
		return true
	}
	for _, child := range node.Children {
		if len(child.Mountpoints) > 0 || hasAnyMountedChild(child) {
			return true
		}
	}
	return false
}
