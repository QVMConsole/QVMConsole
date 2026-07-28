package pool

import (
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"

	"kvm_console/model"
	"kvm_console/utils"
)

// ListStoragePools 列出宿主机所有块设备，并合并管理员存储池配置及 LVM 层级。
func ListStoragePools() ([]HostStoragePoolInfo, error) {
	devices, err := readLSBLKDevices()
	if err != nil {
		return nil, err
	}
	mounts := readFindmntMap()
	dfUsage := readDFUsage()
	aliases := readDeviceAliasMap()
	configs := loadHostStoragePoolConfigs()
	// 过滤掉 loop 设备（如 /dev/loop*），它们不是物理磁盘
	devices = filterNonPhysical(devices)
	pools := buildStoragePoolTree(devices, mounts, dfUsage, aliases, configs)

	// 注入 LVM VG 层级
	vgs, lvs, _, err := ListVGs()
	if err == nil && len(vgs) > 0 {
		pools = injectLVMTree(pools, vgs, lvs, mounts, dfUsage, configs)
	}

	// 注入 VM 磁盘使用统计
	pools = injectVMUsageIntoPools(pools)

	return pools, nil
}

// GetStoragePool 获取单个宿主机存储池设备。
func GetStoragePool(id string) (*HostStoragePoolInfo, error) {
	pools, err := ListStoragePools()
	if err != nil {
		return nil, err
	}
	if pool := findStoragePoolByID(pools, id); pool != nil {
		return pool, nil
	}
	return nil, fmt.Errorf("存储池设备不存在: %s", id)
}

// ListVMStorageTargets 返回创建虚拟机可选择的落盘位置。
func ListVMStorageTargets(isAdmin bool) ([]VMStorageTarget, error) {
	pools, err := ListStoragePools()
	if err != nil {
		return nil, err
	}
	var targets []VMStorageTarget
	walkStoragePools(pools, func(pool HostStoragePoolInfo) {
		if !pool.CanUseForVM {
			return
		}
		if !isAdmin && !pool.Enabled {
			return
		}
		// 过滤掉可用空间为 0 的存储目标（对用户无意义）
		if pool.Available <= 0 {
			return
		}
		targets = append(targets, VMStorageTarget{
			ID:          pool.ID,
			DisplayName: pool.DisplayName,
			DevicePath:  pool.DevicePath,
			MountPath:   pool.MountPath,
			VMDir:       pool.VMDir,
			Size:        pool.Size,
			Used:        pool.Used,
			Available:   pool.Available,
			Enabled:     pool.Enabled || isAdmin,
			IsDefault:   pool.IsDefault,
		})
	})
	sort.SliceStable(targets, func(i, j int) bool {
		if targets[i].IsDefault != targets[j].IsDefault {
			return targets[i].IsDefault
		}
		if targets[i].Enabled != targets[j].Enabled {
			return targets[i].Enabled
		}
		return targets[i].DisplayName < targets[j].DisplayName
	})
	return targets, nil
}

func readLSBLKDevices() ([]lsblkDevice, error) {
	result := utils.ExecCommand("lsblk", "-J", "-b", "-o", "NAME,KNAME,PATH,TYPE,SIZE,FSTYPE,FSVER,LABEL,UUID,MOUNTPOINTS,MODEL,SERIAL,ROTA,RM,RO,TRAN,PKNAME")
	if result.Error != nil {
		return nil, fmt.Errorf("读取宿主机硬盘列表失败: %s", result.Stderr)
	}
	var out lsblkOutput
	if err := json.Unmarshal([]byte(result.Stdout), &out); err != nil {
		return nil, fmt.Errorf("解析硬盘列表失败: %w", err)
	}
	return out.BlockDevices, nil
}

// filterNonPhysical 递归过滤掉 loop、rom、ram 等非物理磁盘设备及其子设备。
func filterNonPhysical(devices []lsblkDevice) []lsblkDevice {
	result := make([]lsblkDevice, 0, len(devices))
	for _, dev := range devices {
		devType := strings.ToLower(dev.Type)
		nameLower := strings.ToLower(dev.Name)
		// 跳过 loop 设备（如 /dev/loop*）
		if devType == "loop" || strings.HasPrefix(nameLower, "loop") {
			continue
		}
		// 跳过 rom 设备（光驱等）
		if devType == "rom" {
			continue
		}
		// 跳过内存盘设备
		if strings.HasPrefix(nameLower, "ram") || strings.HasPrefix(nameLower, "zram") {
			continue
		}
		// 递归过滤子设备
		dev.Children = filterNonPhysical(dev.Children)
		result = append(result, dev)
	}
	return result
}

func readFindmntMap() map[string]findmntInfo {
	result := utils.ExecCommand("findmnt", "-J", "-b", "-o", "TARGET,SOURCE,FSTYPE,OPTIONS,SIZE,USED,AVAIL")
	if result.Error != nil {
		return map[string]findmntInfo{}
	}
	var out findmntOutput
	if err := json.Unmarshal([]byte(result.Stdout), &out); err != nil {
		return map[string]findmntInfo{}
	}
	mounts := make(map[string]findmntInfo)
	var walk func([]findmntInfo)
	walk = func(items []findmntInfo) {
		for _, item := range items {
			mounts[item.Target] = item
			walk(item.Children)
		}
	}
	walk(out.Filesystems)
	return mounts
}

func readDFUsage() map[string]mountUsage {
	result := utils.ExecCommand("df", "-B1", "--output=source,size,used,avail,pcent,target")
	if result.Error != nil {
		return map[string]mountUsage{}
	}
	usage := make(map[string]mountUsage)
	for i, line := range strings.Split(result.Stdout, "\n") {
		line = strings.TrimSpace(line)
		if i == 0 || line == "" {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) < 6 {
			continue
		}
		size, _ := strconv.ParseInt(fields[1], 10, 64)
		used, _ := strconv.ParseInt(fields[2], 10, 64)
		avail, _ := strconv.ParseInt(fields[3], 10, 64)
		target := fields[5]
		usage[target] = mountUsage{
			Source:    fields[0],
			Target:    target,
			Size:      size,
			Used:      used,
			Available: avail,
		}
	}
	return usage
}

func readDeviceAliasMap() map[string]string {
	aliases := make(map[string]string)
	fill := func(pattern string, allowUUID bool) {
		script := fmt.Sprintf("for p in %s; do [ -e \"$p\" ] || continue; t=$(readlink -f \"$p\" 2>/dev/null || true); [ -n \"$t\" ] && echo \"$t|$p\"; done", pattern)
		result := utils.ExecShell(script)
		if result.Error != nil {
			return
		}
		for _, line := range strings.Split(result.Stdout, "\n") {
			parts := strings.SplitN(strings.TrimSpace(line), "|", 2)
			if len(parts) != 2 {
				continue
			}
			if !allowUUID && strings.Contains(parts[1], "/by-uuid/") {
				continue
			}
			if aliases[parts[0]] == "" {
				aliases[parts[0]] = parts[1]
			}
		}
	}
	fill("/dev/disk/by-id/*", false)
	fill("/dev/disk/by-uuid/*", true)
	return aliases
}

func loadHostStoragePoolConfigs() map[string]model.HostStoragePool {
	configs := make(map[string]model.HostStoragePool)
	if model.DB == nil {
		return configs
	}
	var rows []model.HostStoragePool
	if err := model.DB.Find(&rows).Error; err != nil {
		return configs
	}
	for _, row := range rows {
		configs[row.DeviceID] = row
	}
	return configs
}

func getDefaultHostStoragePoolConfig() (model.HostStoragePool, bool) {
	var cfg model.HostStoragePool
	if model.DB == nil {
		return cfg, false
	}
	if err := model.DB.Where("is_default = ?", true).First(&cfg).Error; err != nil {
		return cfg, false
	}
	return cfg, true
}

func findStoragePoolByID(pools []HostStoragePoolInfo, id string) *HostStoragePoolInfo {
	for _, pool := range pools {
		if pool.ID == id {
			cp := pool
			return &cp
		}
		if found := findStoragePoolByID(pool.Children, id); found != nil {
			return found
		}
	}
	return nil
}

func walkStoragePools(pools []HostStoragePoolInfo, fn func(HostStoragePoolInfo)) {
	for _, pool := range pools {
		fn(pool)
		walkStoragePools(pool.Children, fn)
	}
}
