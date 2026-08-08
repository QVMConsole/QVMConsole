package pool

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"kvm_console/model"
	"kvm_console/utils"
)

// ── LVM 存储卷类型定义 ──

// LVMVolumeRequest 创建 LVM 存储卷的请求参数。
type LVMVolumeRequest struct {
	DeviceIDs []string `json:"device_ids"` // 选中的 PV 设备 ID 列表
	VGName    string   `json:"vg_name"`    // 卷组名称
	PESize    string   `json:"pe_size"`    // PE 大小，默认 4M
	LVName    string   `json:"lv_name"`    // 逻辑卷名称
	LVSize    string   `json:"lv_size"`    // LV 大小，如 "10G" / "50%VG" / "100%FREE"
	LVType    string   `json:"lv_type"`    // linear / striped / mirrored
	Stripes   int      `json:"stripes"`    // 条带数（striped 模式）
	Mirrors   int      `json:"mirrors"`    // 镜像数（mirrored 模式）
	FSType    string   `json:"fs_type"`    // ext4 / xfs / btrfs / none
	MountPath string   `json:"mount_path"` // 挂载路径，留空则自动生成
	AddFstab  bool     `json:"add_fstab"`  // 是否写入 fstab
}

// VGInfo VG 信息。
type VGInfo struct {
	Name        string   `json:"name"`
	Size        int64    `json:"size"`     // 总大小（字节）
	Free        int64    `json:"free"`     // 可用空间（字节）
	LVCount     int      `json:"lv_count"` // LV 数量
	PVCount     int      `json:"pv_count"` // PV 数量
	PVNames     []string `json:"pv_names"` // PV 设备路径列表
	PExtentSize int64    `json:"pe_size"`  // PE 大小（字节）
}

// LVInfo LV 信息。
type LVInfo struct {
	Name       string `json:"name"`
	FullName   string `json:"full_name"`   // VG/LV
	DevicePath string `json:"device_path"` // /dev/VG/LV
	Size       int64  `json:"size"`
	LVType     string `json:"lv_type"` // linear / striped / mirrored
	Stripes    int    `json:"stripes"`
	Mirrors    int    `json:"mirrors"`
	FSType     string `json:"fstype"`
	MountPath  string `json:"mount_path"`
}

// PVInfo PV 信息。
type PVInfo struct {
	DevicePath string `json:"device_path"`
	Size       int64  `json:"size"`
	VGName     string `json:"vg_name"`
}

// ── 创建 LVM 存储卷 ──

// CreateLVMVolume 创建 LVM 逻辑卷存储池，按顺序执行：
// pvcreate → vgcreate → lvcreate → mkfs → mount → 写入 fstab → 存储配置。
func CreateLVMVolume(ctx context.Context, req LVMVolumeRequest, progress func(int, string)) error {
	if len(req.DeviceIDs) == 0 {
		return fmt.Errorf("至少需要选择一个物理磁盘")
	}
	if strings.TrimSpace(req.VGName) == "" {
		return fmt.Errorf("卷组名称不能为空")
	}
	if strings.TrimSpace(req.LVName) == "" {
		return fmt.Errorf("逻辑卷名称不能为空")
	}
	if strings.TrimSpace(req.LVSize) == "" {
		return fmt.Errorf("逻辑卷大小不能为空")
	}
	if req.FSType == "" {
		req.FSType = "ext4"
	}
	if req.LVType == "" {
		req.LVType = "linear"
	}
	if req.PESize == "" {
		req.PESize = "4M"
	}
	if req.MountPath == "" {
		req.MountPath = defaultStorageMountPath(req.VGName + "-" + req.LVName)
	}

	// 1) 校验并收集设备路径
	progress(5, "正在校验选中的磁盘...")
	devicePaths, err := validateAndCollectPVTargets(req.DeviceIDs)
	if err != nil {
		return fmt.Errorf("校验磁盘失败: %w", err)
	}

	// 检查 VG 名称是否已存在
	progress(10, "检查卷组名称冲突...")
	if vgExists(req.VGName) {
		return fmt.Errorf("卷组 %s 已存在", req.VGName)
	}

	// 2) 创建物理卷
	progress(15, fmt.Sprintf("正在初始化 %d 个物理卷...", len(devicePaths)))
	if err := pvCreateAll(ctx, devicePaths, progress, 15, 30); err != nil {
		return fmt.Errorf("创建物理卷失败: %w", err)
	}

	// 3) 创建卷组
	progress(30, fmt.Sprintf("正在创建卷组 %s...", req.VGName))
	if err := vgCreate(ctx, req.VGName, devicePaths, req.PESize); err != nil {
		// 回滚 PV
		undoPVCreate(ctx, devicePaths)
		return fmt.Errorf("创建卷组失败: %w", err)
	}

	// 4) 创建逻辑卷
	progress(45, fmt.Sprintf("正在创建逻辑卷 %s（%s）...", req.LVName, req.LVSize))
	lvPath, err := lvCreate(ctx, req.VGName, req.LVName, req.LVSize, req.LVType, req.Stripes, req.Mirrors)
	if err != nil {
		return fmt.Errorf("创建逻辑卷失败: %w", err)
	}

	// 5) 格式化逻辑卷
	if strings.EqualFold(req.FSType, "none") {
		progress(70, "跳过格式化（用户指定不格式化）...")
	} else {
		progress(70, fmt.Sprintf("正在格式化为 %s...", req.FSType))
		if err := formatLV(ctx, lvPath, req.FSType); err != nil {
			return fmt.Errorf("格式化逻辑卷失败: %w", err)
		}
	}

	// 6) 挂载逻辑卷
	progress(80, "正在挂载逻辑卷...")
	if err := mountLV(ctx, lvPath, req.MountPath, req.FSType, req.AddFstab); err != nil {
		return fmt.Errorf("挂载逻辑卷失败: %w", err)
	}

	// 7) 创建虚拟机磁盘目录
	vmDir := filepath.Join(req.MountPath, "vm-disks")
	progress(90, "正在创建虚拟机磁盘目录...")
	if err := ensureVMStorageDir(vmDir); err != nil {
		return fmt.Errorf("创建虚拟机磁盘目录失败: %w", err)
	}

	// 8) 保存存储池配置
	progress(95, "正在保存存储池配置...")
	deviceID := normalizeStorageDeviceID(req.VGName + "-" + req.LVName)
	cfg := model.HostStoragePool{DeviceID: deviceID}
	if err := model.DB.Where("device_id = ?", deviceID).Assign(map[string]interface{}{
		"display_name": req.VGName + "/" + req.LVName,
		"enabled":      true,
		"mount_path":   req.MountPath,
	}).FirstOrCreate(&cfg).Error; err != nil {
		return fmt.Errorf("保存存储池配置失败: %w", err)
	}

	progress(100, "LVM 存储卷创建完成")
	return nil
}

// ── LVM 操作封装 ──

// validateAndCollectPVTargets 校验并收集 PV 设备路径。
func validateAndCollectPVTargets(deviceIDs []string) ([]string, error) {
	var devicePaths []string
	for _, id := range deviceIDs {
		pool, err := GetStoragePool(id)
		if err != nil {
			return nil, fmt.Errorf("设备 %s: %w", id, err)
		}
		if err := validatePVTarget(*pool); err != nil {
			return nil, fmt.Errorf("设备 %s: %w", pool.DevicePath, err)
		}
		devicePaths = append(devicePaths, pool.DevicePath)
	}
	return devicePaths, nil
}

// validatePVTarget 校验单个设备是否可作为 LVM PV。
func validatePVTarget(pool HostStoragePoolInfo) error {
	if pool.Readonly {
		return fmt.Errorf("设备为只读状态")
	}
	if pool.SystemDisk {
		return fmt.Errorf("系统关键磁盘禁止作为 PV")
	}
	if len(pool.Mountpoints) > 0 || hasMountedChild(pool) {
		return fmt.Errorf("设备或其分区当前已挂载")
	}
	fstype := strings.ToLower(pool.FSType)
	if fstype == "lvm2_member" {
		return fmt.Errorf("该设备已是 LVM 物理卷")
	}
	if fstype == "linux_raid_member" {
		return fmt.Errorf("该设备已加入 mdraid 阵列")
	}
	if fstype == "zfs_member" {
		return fmt.Errorf("该设备已加入 ZFS 存储池")
	}
	if hasChildWithFSType(pool, "lvm2_member", "linux_raid_member", "zfs_member") {
		return fmt.Errorf("该设备包含 LVM/RAID/ZFS 子分区")
	}
	if pool.Type != "disk" && pool.Type != "part" {
		return fmt.Errorf("仅支持整块磁盘或分区，当前类型: %s", pool.Type)
	}
	return nil
}

// pvCreateAll 批量执行 pvcreate。
func pvCreateAll(ctx context.Context, devicePaths []string, progress func(int, string), progressStart, progressEnd int) error {
	total := len(devicePaths)
	for i, devPath := range devicePaths {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		pct := progressStart + (i*(progressEnd-progressStart))/total
		progress(pct, fmt.Sprintf("正在初始化 PV: %s (%d/%d)...", devPath, i+1, total))

		result := utils.ExecCommandContextWithTimeout(ctx, "pvcreate", 2*time.Minute, "-y", devPath)
		if result.Error != nil {
			return fmt.Errorf("pvcreate %s 失败: %s", devPath, result.Stderr)
		}
	}
	return nil
}

// vgCreate 创建卷组。
func vgCreate(ctx context.Context, vgName string, devicePaths []string, peSize string) error {
	args := []string{"-s", peSize, vgName}
	args = append(args, devicePaths...)
	result := utils.ExecCommandContextWithTimeout(ctx, "vgcreate", 2*time.Minute, args...)
	if result.Error != nil {
		return fmt.Errorf("vgcreate 失败: %s", result.Stderr)
	}
	return nil
}

// lvCreate 创建逻辑卷。
func lvCreate(ctx context.Context, vgName, lvName, lvSize, lvType string, stripes, mirrors int) (string, error) {
	// 判断尺寸参数类型：百分比使用 -l（小写），绝对值使用 -L（大写）
	var sizeFlag string
	cleanSize := strings.TrimSpace(lvSize)
	if strings.Contains(cleanSize, "%") {
		sizeFlag = "-l"
		// 如果用户只写了 "100%"，补全为 "100%VG"
		if !strings.Contains(cleanSize, "VG") && !strings.Contains(cleanSize, "FREE") && !strings.Contains(cleanSize, "PVS") && !strings.Contains(cleanSize, "ORIGIN") {
			cleanSize = cleanSize + "VG"
		}
	} else {
		sizeFlag = "-L"
	}
	args := []string{"-y", "-n", lvName, sizeFlag, cleanSize}

	switch lvType {
	case "striped":
		if stripes <= 0 {
			stripes = 2
		}
		args = append(args, "--type", "striped", "-i", strconv.Itoa(stripes))
	case "mirrored":
		if mirrors <= 0 {
			mirrors = 1
		}
		args = append(args, "--type", "mirror", "-m", strconv.Itoa(mirrors))
	default:
		// linear: 默认类型，无需额外参数
	}

	args = append(args, vgName)
	result := utils.ExecCommandContextWithTimeout(ctx, "lvcreate", 5*time.Minute, args...)
	if result.Error != nil {
		return "", fmt.Errorf("lvcreate 失败: %s", result.Stderr)
	}

	lvPath := "/dev/" + vgName + "/" + lvName
	return lvPath, nil
}

// formatLV 格式化逻辑卷。
func formatLV(ctx context.Context, lvPath, fsType string) error {
	// 先清除可能的旧签名
	utils.ExecCommandContextWithTimeout(ctx, "wipefs", 1*time.Minute, "-a", lvPath)

	mkfsCmd := "mkfs." + fsType
	mkfsArgs := buildMkfsArgs(fsType, lvPath)
	// 格式化大容量逻辑卷属于 IO 操作，不设置自动超时
	result := utils.ExecCommandContextWithTimeout(ctx, mkfsCmd, 0, mkfsArgs...)
	if result.Error != nil {
		return fmt.Errorf("mkfs.%s 失败: %s", fsType, result.Stderr)
	}
	return nil
}

// mountLV 挂载逻辑卷。
func mountLV(ctx context.Context, lvPath, mountPath, fsType string, addFstab bool) error {
	if err := os.MkdirAll(mountPath, 0755); err != nil {
		return fmt.Errorf("创建挂载目录失败: %w", err)
	}

	// 读取 UUID
	blkid := utils.ExecCommandContextWithTimeout(ctx, "blkid", 30*time.Second, "-s", "UUID", "-o", "value", lvPath)
	uuid := strings.TrimSpace(blkid.Stdout)
	if blkid.Error != nil || uuid == "" {
		// 没有 UUID 时使用设备路径
		uuid = lvPath
	}

	// 挂载
	mount := utils.ExecCommandContextWithTimeout(ctx, "mount", 2*time.Minute, lvPath, mountPath)
	if mount.Error != nil {
		return fmt.Errorf("mount 失败: %s", mount.Stderr)
	}

	// 写入 fstab
	if addFstab {
		if err := ensureFstabEntryLV(uuid, mountPath, fsType); err != nil {
			return fmt.Errorf("写入 fstab 失败: %w", err)
		}
	}

	return nil
}

// ensureFstabEntryLV 为 LVM LV 写入 fstab 条目。
func ensureFstabEntryLV(uuid, mountPath, fsType string) error {
	data, err := os.ReadFile("/etc/fstab")
	if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("读取 /etc/fstab 失败: %w", err)
	}
	var entry string
	if strings.HasPrefix(uuid, "/dev/") {
		entry = fmt.Sprintf("%s %s %s defaults,nofail 0 2", uuid, mountPath, fsType)
	} else {
		entry = fmt.Sprintf("UUID=%s %s %s defaults,nofail 0 2", uuid, mountPath, fsType)
	}

	var lines []string
	found := false
	for _, existing := range strings.Split(string(data), "\n") {
		trimmed := strings.TrimSpace(existing)
		if trimmed == "" {
			continue
		}
		fields := strings.Fields(trimmed)
		if len(fields) >= 2 && fields[1] == mountPath {
			lines = append(lines, entry)
			found = true
			continue
		}
		lines = append(lines, existing)
	}
	if !found {
		lines = append(lines, entry)
	}
	content := strings.Join(lines, "\n") + "\n"
	return os.WriteFile("/etc/fstab", []byte(content), 0644)
}

// undoPVCreate 回滚 PV 初始化（vgcreate 失败时使用）。
func undoPVCreate(ctx context.Context, devicePaths []string) {
	for _, devPath := range devicePaths {
		utils.ExecCommandContextWithTimeout(ctx, "pvremove", 1*time.Minute, "-y", devPath)
	}
}

// vgExists 检查卷组是否已存在。
func vgExists(vgName string) bool {
	result := utils.ExecCommand("vgs", "--noheadings", "-o", "vg_name", vgName)
	return result.Error == nil && strings.TrimSpace(result.Stdout) != ""
}

// ── LVM 信息扫描 ──

// ListVGs 扫描宿主机所有 VG/LV/PV 信息。
func ListVGs() ([]VGInfo, []LVInfo, []PVInfo, error) {
	vgs, err := scanVGs()
	if err != nil {
		return nil, nil, nil, err
	}
	lvs, err := scanLVs()
	if err != nil {
		return nil, nil, nil, err
	}
	pvs, err := scanPVs()
	if err != nil {
		return nil, nil, nil, err
	}
	return vgs, lvs, pvs, nil
}

func scanVGs() ([]VGInfo, error) {
	result := utils.ExecCommand("vgs", "--noheadings", "--units", "b",
		"-o", "vg_name,vg_size,vg_free,lv_count,pv_count,vg_extent_size", "--separator", "|")
	if result.Error != nil {
		return nil, nil
	}

	var vgs []VGInfo
	for _, line := range strings.Split(result.Stdout, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		fields := strings.SplitN(line, "|", 6)
		if len(fields) < 6 {
			continue
		}
		name := strings.TrimSpace(fields[0])
		size, _ := parseBytes(strings.TrimSpace(fields[1]))
		free, _ := parseBytes(strings.TrimSpace(fields[2]))
		lvCount, _ := strconv.Atoi(strings.TrimSpace(fields[3]))
		pvCount, _ := strconv.Atoi(strings.TrimSpace(fields[4]))
		peSize, _ := parseBytes(strings.TrimSpace(fields[5]))

		// 扫描该 VG 下的 PV 名称
		pvNames := scanVGPVNames(name)

		vgs = append(vgs, VGInfo{
			Name:        name,
			Size:        size,
			Free:        free,
			LVCount:     lvCount,
			PVCount:     pvCount,
			PVNames:     pvNames,
			PExtentSize: peSize,
		})
	}
	sort.Slice(vgs, func(i, j int) bool { return vgs[i].Name < vgs[j].Name })
	return vgs, nil
}

func scanVGPVNames(vgName string) []string {
	result := utils.ExecCommand("pvs", "--noheadings", "-o", "pv_name", "--select",
		fmt.Sprintf("vg_name=%s", vgName))
	if result.Error != nil {
		return nil
	}
	var names []string
	for _, line := range strings.Split(result.Stdout, "\n") {
		line = strings.TrimSpace(line)
		if line != "" {
			names = append(names, line)
		}
	}
	return names
}

func scanLVs() ([]LVInfo, error) {
	result := utils.ExecCommand("lvs", "--noheadings", "--units", "b",
		"-o", "lv_name,vg_name,lv_size,lv_attr,lv_path", "--separator", "|")
	if result.Error != nil {
		return nil, nil
	}

	// 读取当前挂载关系
	mountMap := buildLVMountMap()

	var lvs []LVInfo
	for _, line := range strings.Split(result.Stdout, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		fields := strings.SplitN(line, "|", 5)
		if len(fields) < 5 {
			continue
		}
		lvName := strings.TrimSpace(fields[0])
		vgName := strings.TrimSpace(fields[1])
		size, _ := parseBytes(strings.TrimSpace(fields[2]))
		attr := strings.TrimSpace(fields[3])
		devicePath := strings.TrimSpace(fields[4])

		lvType := detectLVType(attr)

		// 获取文件系统和挂载点
		fsType, mountPath := resolveLVFSAndMount(devicePath, mountMap)

		lvs = append(lvs, LVInfo{
			Name:       lvName,
			FullName:   vgName + "/" + lvName,
			DevicePath: devicePath,
			Size:       size,
			LVType:     lvType,
			Stripes:    detectLVStripes(attr),
			FSType:     fsType,
			MountPath:  mountPath,
		})
	}
	sort.Slice(lvs, func(i, j int) bool { return lvs[i].FullName < lvs[j].FullName })
	return lvs, nil
}

func scanPVs() ([]PVInfo, error) {
	result := utils.ExecCommand("pvs", "--noheadings", "--units", "b",
		"-o", "pv_name,pv_size,vg_name", "--separator", "|")
	if result.Error != nil {
		return nil, nil
	}

	var pvs []PVInfo
	for _, line := range strings.Split(result.Stdout, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		fields := strings.SplitN(line, "|", 3)
		if len(fields) < 3 {
			continue
		}
		devPath := strings.TrimSpace(fields[0])
		size, _ := parseBytes(strings.TrimSpace(fields[1]))
		vgName := strings.TrimSpace(fields[2])

		pvs = append(pvs, PVInfo{
			DevicePath: devPath,
			Size:       size,
			VGName:     vgName,
		})
	}
	return pvs, nil
}

// buildLVMountMap 构建 LV 设备路径到挂载点的映射。
func buildLVMountMap() map[string]string {
	m := make(map[string]string)
	result := utils.ExecCommandQuiet("findmnt", "-J", "-b", "-o", "TARGET,SOURCE,FSTYPE")
	if result.Error != nil || result.Stdout == "" {
		// 降级: 用 mount 命令
		mountResult := utils.ExecCommandQuiet("mount")
		if mountResult.Error == nil {
			for _, line := range strings.Split(mountResult.Stdout, "\n") {
				if !strings.Contains(line, "/dev/mapper/") {
					continue
				}
				fields := strings.Fields(line)
				if len(fields) >= 3 {
					m[fields[0]] = fields[2]
				}
			}
		}
		return m
	}
	// 解析 findmnt JSON
	var out struct {
		Filesystems []struct {
			Target string `json:"target"`
			Source string `json:"source"`
			FSType string `json:"fstype"`
		} `json:"filesystems"`
	}
	if err := json.Unmarshal([]byte(result.Stdout), &out); err != nil {
		return m
	}
	var walk func(items []struct {
		Target string `json:"target"`
		Source string `json:"source"`
		FSType string `json:"fstype"`
	})
	walk = func(items []struct {
		Target string `json:"target"`
		Source string `json:"source"`
		FSType string `json:"fstype"`
	}) {
		for _, item := range items {
			m[item.Source] = item.Target
		}
	}
	walk(out.Filesystems)
	return m
}

// resolveLVFSAndMount 解析 LV 的文件系统和挂载点。
func resolveLVFSAndMount(devicePath string, mountMap map[string]string) (string, string) {
	// 1) 直接从 mountMap 查找
	if mountPath, ok := mountMap[devicePath]; ok {
		return resolveFSType(devicePath), mountPath
	}

	// 2) 回退：用 findmnt --source <设备路径> 直接查询
	//    解决 lvs 返回 /dev/vg/lv 与 mount/findmnt 返回 /dev/mapper/vg--lv 格式不一致的问题
	result := utils.ExecCommandQuiet("findmnt", "--source", devicePath, "-J", "-b", "-o", "TARGET")
	if result.Error == nil && result.Stdout != "" {
		var out struct {
			Filesystems []struct {
				Target string `json:"target"`
			} `json:"filesystems"`
		}
		if err := json.Unmarshal([]byte(result.Stdout), &out); err == nil && len(out.Filesystems) > 0 {
			mountPath := strings.TrimSpace(out.Filesystems[0].Target)
			if mountPath != "" {
				return resolveFSType(devicePath), mountPath
			}
		}
	}

	// 3) 尝试 blkid 读取文件系统类型
	blkidResult := utils.ExecCommandQuiet("blkid", "-s", "TYPE", "-o", "value", devicePath)
	if blkidResult.Error == nil {
		return strings.TrimSpace(blkidResult.Stdout), ""
	}
	return "", ""
}

func resolveFSType(devicePath string) string {
	result := utils.ExecCommandQuiet("blkid", "-s", "TYPE", "-o", "value", devicePath)
	if result.Error == nil {
		return strings.TrimSpace(result.Stdout)
	}
	return ""
}

// detectLVType 从 lv_attr 字段推断 LV 类型。
// lv_attr 格式: 第一个字符表示卷类型。
func detectLVType(attr string) string {
	if len(attr) < 1 {
		return "linear"
	}
	switch attr[0] {
	case 'm', 'M':
		return "mirrored"
	case 'r', 'R':
		return "raid"
	case 's', 'S':
		return "snapshot"
	case 't', 'T':
		return "thin-pool"
	case 'V':
		return "thin"
	default:
		return "linear"
	}
}

// detectLVStripes 从 lv_attr 字段推断条带数。
func detectLVStripes(attr string) int {
	if len(attr) < 2 {
		return 0
	}
	switch attr[0] {
	case 's', 'S':
		return 2 // 条带至少需要 2 个
	}
	return 0
}

// parseBytes 解析带单位的字节数，如 "123456789B"。
func parseBytes(s string) (int64, error) {
	s = strings.TrimSuffix(strings.TrimSpace(s), "B")
	s = strings.TrimSpace(s)
	if s == "" {
		return 0, nil
	}
	return strconv.ParseInt(s, 10, 64)
}

// GetAvailablePVTargets 返回可供 LVM 使用的磁盘列表。
func GetAvailablePVTargets() ([]HostStoragePoolInfo, error) {
	pools, err := ListStoragePools()
	if err != nil {
		return nil, err
	}
	var available []HostStoragePoolInfo
	walkStoragePools(pools, func(pool HostStoragePoolInfo) {
		if pool.Type != "disk" {
			return
		}
		if pool.SystemDisk || pool.Readonly {
			return
		}
		if len(pool.Mountpoints) > 0 || hasMountedChild(pool) {
			return
		}
		fstype := strings.ToLower(pool.FSType)
		if fstype == "lvm2_member" || fstype == "linux_raid_member" || fstype == "zfs_member" {
			return
		}
		if hasChildWithFSType(pool, "lvm2_member", "linux_raid_member", "zfs_member") {
			return
		}
		nameLower := strings.ToLower(pool.Name)
		if strings.HasPrefix(nameLower, "ram") || strings.HasPrefix(nameLower, "zram") || pool.Type == "dm" || pool.Type == "rom" || pool.Type == "loop" {
			return
		}
		available = append(available, pool)
	})
	return available, nil
}

// ── 删除 LVM 存储卷 ──

// DeleteLVMVolume 删除指定卷组及其所有逻辑卷和物理卷。
// 流程：卸载 LV → 清理 fstab → 删除 LV → 删除 VG → 清除 PV → 清理 DB 配置。
func DeleteLVMVolume(ctx context.Context, vgName string, progress func(int, string)) error {
	vgName = strings.TrimSpace(vgName)
	if vgName == "" {
		return fmt.Errorf("卷组名称不能为空")
	}

	// 验证 VG 存在
	if !vgExists(vgName) {
		return fmt.Errorf("卷组 %s 不存在", vgName)
	}

	// 扫描 VG 下所有 LV
	progress(5, "正在收集卷组信息...")
	_, lvs, pvs, err := ListVGs()
	if err != nil {
		return fmt.Errorf("扫描 LVM 信息失败: %w", err)
	}

	var vgLVs []LVInfo
	for _, lv := range lvs {
		if strings.SplitN(lv.FullName, "/", 2)[0] == vgName {
			vgLVs = append(vgLVs, lv)
		}
	}
	var vgPVs []PVInfo
	for _, pv := range pvs {
		if pv.VGName == vgName {
			vgPVs = append(vgPVs, pv)
		}
	}

	// 安全检查：拒绝删除包含系统关键挂载点（/ /boot 等）的 VG
	for _, lv := range vgLVs {
		mp := lv.MountPath
		if mp == "" {
			// 如果运行时没检测到，用 findmnt 实时查
			_, mps := findMountsForDevice(lv.DevicePath)
			if len(mps) > 0 {
				mp = mps[0]
			}
		}
		switch mp {
		case "/", "/boot", "/boot/efi", "/usr", "/var", "/home":
			return fmt.Errorf("逻辑卷 %s 挂载于系统关键路径 %s，禁止删除", lv.FullName, mp)
		}
	}

	// 1) 卸载所有已挂载的 LV（逐设备强制卸载）
	progress(10, "正在卸载逻辑卷...")
	for _, lv := range vgLVs {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		unmountLVForcefully(ctx, lv.DevicePath, lv.MountPath, progress)
	}

	// 2) 清理 fstab 中 LV 设备路径和挂载点
	progress(30, "正在清理开机自动挂载配置...")
	if err := removeLVMFstabEntries(vgLVs); err != nil {
		return fmt.Errorf("清理 fstab 失败: %w", err)
	}

	// 3) 停用 VG（确保 LV 未被占用）
	progress(40, "正在停用卷组...")
	utils.ExecCommandQuietWithTimeout("vgchange", 1*time.Minute, "-an", vgName)

	// 4) 删除所有 LV
	progress(50, fmt.Sprintf("正在删除 %d 个逻辑卷...", len(vgLVs)))
	for _, lv := range vgLVs {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		progress(50, fmt.Sprintf("正在删除 LV: %s ...", lv.FullName))
		result := utils.ExecCommandContextWithTimeout(ctx, "lvremove", 2*time.Minute, "-y", lv.DevicePath)
		if result.Error != nil {
			return fmt.Errorf("删除 LV %s 失败: %s", lv.FullName, result.Stderr)
		}
	}

	// 5) 删除 VG
	progress(70, fmt.Sprintf("正在删除卷组 %s ...", vgName))
	vgRemoveResult := utils.ExecCommandContextWithTimeout(ctx, "vgremove", 2*time.Minute, "-y", vgName)
	if vgRemoveResult.Error != nil {
		return fmt.Errorf("删除卷组 %s 失败: %s", vgName, vgRemoveResult.Stderr)
	}

	// 6) 清除所有 PV
	progress(80, fmt.Sprintf("正在清除 %d 个物理卷...", len(vgPVs)))
	for _, pv := range vgPVs {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		progress(80, fmt.Sprintf("正在清除 PV: %s ...", pv.DevicePath))
		result := utils.ExecCommandContextWithTimeout(ctx, "pvremove", 2*time.Minute, "-y", pv.DevicePath)
		if result.Error != nil {
			return fmt.Errorf("清除 PV %s 失败: %s", pv.DevicePath, result.Stderr)
		}
	}

	// 7) 清除数据库中的存储池配置
	progress(90, "正在清除存储池配置...")
	clearLVMPoolConfigs(vgName, vgLVs)

	progress(100, "LVM 存储卷已删除")
	return nil
}

// unmountLVForcefully 卸载指定 LV，尝试多种策略：已知挂载点 → 设备路径 → 运行时扫描 → lazy 强制卸载。
func unmountLVForcefully(ctx context.Context, devicePath, knownMountPath string, progress func(int, string)) {
	// 如果已知挂载点，先尝试直接卸载
	if knownMountPath != "" {
		progress(10, fmt.Sprintf("正在卸载 %s ...", knownMountPath))
		result := utils.ExecCommandContextWithTimeout(ctx, "umount", 1*time.Minute, knownMountPath)
		if result.Error == nil {
			return
		}
	}

	// 尝试从 /proc/mounts 查找设备的实际挂载点
	mountPath := findMountPathForDevice(devicePath)
	if mountPath != "" && mountPath != knownMountPath {
		progress(10, fmt.Sprintf("正在卸载 %s (于 %s)...", devicePath, mountPath))
		result := utils.ExecCommandContextWithTimeout(ctx, "umount", 1*time.Minute, mountPath)
		if result.Error == nil {
			return
		}
	}

	// 直接按设备路径卸载
	progress(10, fmt.Sprintf("正在按设备卸载 %s ...", devicePath))
	result := utils.ExecCommandContextWithTimeout(ctx, "umount", 1*time.Minute, devicePath)
	if result.Error == nil {
		return
	}

	// 最终手段：lazy unmount（按挂载点和设备路径都试）
	if mountPath != "" {
		utils.ExecCommandQuietWithTimeout("umount", 1*time.Minute, "-l", mountPath)
	}
	if knownMountPath != "" && knownMountPath != mountPath {
		utils.ExecCommandQuietWithTimeout("umount", 1*time.Minute, "-l", knownMountPath)
	}
	utils.ExecCommandQuietWithTimeout("umount", 1*time.Minute, "-l", devicePath)
}

// findMountPathForDevice 从 /proc/mounts 查找设备的挂载点。
func findMountPathForDevice(devicePath string) string {
	result := utils.ExecCommandQuiet("findmnt", "-n", "-o", "TARGET", "--source", devicePath)
	if result.Error == nil && strings.TrimSpace(result.Stdout) != "" {
		return strings.TrimSpace(result.Stdout)
	}
	// 降级：grep /proc/mounts
	grepResult := utils.ExecShellQuiet(fmt.Sprintf("grep -F %s /proc/mounts 2>/dev/null | awk '{print $2}' | head -1", shellEscape(devicePath)))
	if grepResult.Error == nil && strings.TrimSpace(grepResult.Stdout) != "" {
		return strings.TrimSpace(grepResult.Stdout)
	}
	return ""
}

// shellEscape 对 shell 字符串做安全转义。
func shellEscape(s string) string {
	return utils.ShellSingleQuote(s)
}

// removeLVMFstabEntries 从 fstab 中移除指定 LV 的相关条目。
func removeLVMFstabEntries(lvs []LVInfo) error {
	data, err := os.ReadFile("/etc/fstab")
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("读取 /etc/fstab 失败: %w", err)
	}

	// 收集需要移除的设备路径和挂载点
	removeSet := make(map[string]bool)
	for _, lv := range lvs {
		if lv.DevicePath != "" {
			removeSet[lv.DevicePath] = true
		}
		if lv.MountPath != "" {
			removeSet[lv.MountPath] = true
		}
	}

	var kept []string
	for _, line := range strings.Split(string(data), "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			kept = append(kept, line)
			continue
		}
		fields := strings.Fields(trimmed)
		if len(fields) < 2 {
			kept = append(kept, line)
			continue
		}
		if removeSet[fields[0]] || removeSet[fields[1]] {
			continue
		}
		kept = append(kept, line)
	}

	content := strings.Join(kept, "\n") + "\n"
	return os.WriteFile("/etc/fstab", []byte(content), 0644)
}

// clearLVMPoolConfigs 清除数据库中 VG 和 LV 相关的存储池配置。
func clearLVMPoolConfigs(vgName string, lvs []LVInfo) {
	if model.DB == nil {
		return
	}
	ids := []string{normalizeStorageDeviceID("vg-" + vgName)}
	for _, lv := range lvs {
		parts := strings.SplitN(lv.FullName, "/", 2)
		if len(parts) == 2 {
			ids = append(ids, normalizeStorageDeviceID(parts[0]+"-"+lv.Name))
		}
	}
	for _, id := range ids {
		model.DB.Where("device_id = ?", id).Delete(&model.HostStoragePool{})
	}
}
