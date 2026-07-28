package pool

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"kvm_console/logger"
	"kvm_console/service/libvirt_rpc"
	"kvm_console/utils"
)

// VMDiskUsageInfo 虚拟机磁盘使用信息
type VMDiskUsageInfo struct {
	Name        string `json:"name"`         // 虚拟机名称
	DiskPath    string `json:"disk_path"`    // 磁盘完整路径（聚合多块盘时为首块盘路径）
	VirtualSize int64  `json:"virtual_size"` // 虚拟配置大小（字节）
	ActualSize  int64  `json:"actual_size"`  // 实际占用大小（字节）
	MountPath   string `json:"mount_path"`   // 所在挂载点路径
}

// getAllVMDiskUsage 获取所有虚拟机的磁盘使用详情（每台虚拟机的每块有效磁盘一条记录）。
// 注意：vm 包依赖本包，无法复用 vm.GetVMDiskInfo（会 import cycle），
// 这里复用底层 libvirt_rpc.ParseDisksFromDomainXML 统一 XML 解析。
func getAllVMDiskUsage() []VMDiskUsageInfo {
	var usageList []VMDiskUsageInfo

	// 1. 获取所有虚拟机列表
	virshResult := utils.ExecCommand("virsh", "list", "--all", "--name")
	if virshResult.Error != nil {
		logger.App.Warn("获取虚拟机列表失败", "error", virshResult.Stderr)
		return usageList
	}

	for _, name := range strings.Split(virshResult.Stdout, "\n") {
		name = strings.TrimSpace(name)
		if name == "" {
			continue
		}

		// 2. 通过 dumpxml 获取域名 XML
		xmlResult := utils.ExecCommand("virsh", "dumpxml", name)
		if xmlResult.Error != nil {
			continue
		}

		// 3. 复用统一解析器提取所有磁盘（自动跳过 backingStore 内部 source）
		for _, disk := range libvirt_rpc.ParseDisksFromDomainXML(xmlResult.Stdout) {
			// 跳过光驱/软盘等非磁盘设备（ISO 不计入虚拟机磁盘占用）
			if disk.Device != "" && disk.Device != "disk" {
				continue
			}
			if disk.Source == "" || disk.Source == "-" {
				continue
			}

			// 4. qemu-img info 获取虚拟配置大小与实际占用大小
			virtualSize, actualSize := qemuImageSizes(disk.Source)

			usageList = append(usageList, VMDiskUsageInfo{
				Name:        name,
				DiskPath:    disk.Source,
				VirtualSize: virtualSize,
				ActualSize:  actualSize,
			})
		}
	}

	return usageList
}

// qemuImageSizes 通过 qemu-img info 获取磁盘虚拟配置大小与实际占用大小（字节）
func qemuImageSizes(diskPath string) (virtualSize, actualSize int64) {
	result := utils.ExecShell(fmt.Sprintf("qemu-img info --output=json -U %s 2>/dev/null", utils.ShellSingleQuote(diskPath)))
	if result.Error != nil {
		return 0, 0
	}
	var qemuData map[string]interface{}
	if err := json.Unmarshal([]byte(result.Stdout), &qemuData); err != nil {
		return 0, 0
	}
	if vs, ok := qemuData["virtual-size"].(float64); ok {
		virtualSize = int64(vs)
	}
	if as, ok := qemuData["actual-size"].(float64); ok {
		actualSize = int64(as)
	}
	return virtualSize, actualSize
}

// injectVMUsageIntoPools 注入虚拟机磁盘使用统计到存储池树。
// 匹配策略：磁盘路径按最长前缀匹配到可承载虚拟机的挂载点；
// 同一虚拟机在同一挂载点下有多块盘时按名称聚合为一条记录。
func injectVMUsageIntoPools(pools []HostStoragePoolInfo) []HostStoragePoolInfo {
	// 1. 获取所有虚拟机的磁盘使用详情
	vmUsageList := getAllVMDiskUsage()
	if len(vmUsageList) == 0 {
		return pools
	}

	// 2. 收集可承载虚拟机的挂载点候选
	var mountCandidates []string
	walkStoragePools(pools, func(p HostStoragePoolInfo) {
		if p.MountPath != "" && p.CanUseForVM {
			mountCandidates = append(mountCandidates, p.MountPath)
		}
	})
	if len(mountCandidates) == 0 {
		return pools
	}

	// 3. 每块 VM 磁盘按最长前缀匹配归属挂载点，按挂载点分组
	usageByMount := make(map[string][]VMDiskUsageInfo)
	for _, vm := range vmUsageList {
		mountPath := longestMountMatch(vm.DiskPath, mountCandidates)
		if mountPath == "" {
			continue
		}
		vm.MountPath = mountPath
		usageByMount[mountPath] = append(usageByMount[mountPath], vm)
	}

	// 4. 递归注入（通过索引写回树中实际节点，walkStoragePools 按值传递无法写回）
	var inject func(nodes []HostStoragePoolInfo)
	inject = func(nodes []HostStoragePoolInfo) {
		for i := range nodes {
			if nodes[i].MountPath != "" && nodes[i].CanUseForVM {
				if vms, ok := usageByMount[nodes[i].MountPath]; ok {
					nodes[i].VmUsageList = aggregateVMUsageByName(vms)
					for _, vm := range nodes[i].VmUsageList {
						nodes[i].VmTotalVirtual += vm.VirtualSize
						nodes[i].VmTotalActual += vm.ActualSize
					}
				}
			}
			inject(nodes[i].Children)
		}
	}
	inject(pools)

	return pools
}

// longestMountMatch 返回包含磁盘路径的最长挂载点（无匹配时返回空串）
func longestMountMatch(diskPath string, mountCandidates []string) string {
	best := ""
	for _, mountPath := range mountCandidates {
		if isPathUnderMount(diskPath, mountPath) && len(mountPath) > len(best) {
			best = mountPath
		}
	}
	return best
}

// aggregateVMUsageByName 按虚拟机名称聚合磁盘占用（同一 VM 多块盘合并为一条，大小累加）
func aggregateVMUsageByName(vms []VMDiskUsageInfo) []VMDiskUsageInfo {
	agg := make(map[string]*VMDiskUsageInfo)
	var names []string
	for _, vm := range vms {
		if existing, ok := agg[vm.Name]; ok {
			existing.VirtualSize += vm.VirtualSize
			existing.ActualSize += vm.ActualSize
		} else {
			cp := vm
			agg[vm.Name] = &cp
			names = append(names, vm.Name)
		}
	}
	// 按名称排序，保证前端展示顺序稳定
	sort.Strings(names)
	result := make([]VMDiskUsageInfo, 0, len(names))
	for _, name := range names {
		result = append(result, *agg[name])
	}
	return result
}
