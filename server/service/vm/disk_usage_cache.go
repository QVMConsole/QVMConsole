package vm

import (
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"

	"kvm_console/logger"
	"kvm_console/service/libvirt_rpc"
	"kvm_console/utils"
)

// 虚拟机实际磁盘占用缓存
//
// 由于遍历所有虚拟机并执行 qemu-img info 较为耗时，
// 采用内存缓存 + 异步刷新策略，避免阻塞宿主机状态采集（SSE 每 5 秒推送一次）。
var (
	vmDiskActualCache      int64     // 所有虚拟机全部磁盘的实际占用总和（字节）
	vmDiskActualCacheTime  time.Time // 上次刷新时间
	vmDiskActualCacheMu    sync.RWMutex
	vmDiskActualRefreshing bool
	vmDiskActualCacheTTL   = 60 * time.Second // 缓存有效期
)

// GetTotalVMActualDiskUsage 获取所有虚拟机实际磁盘占用总和（字节，带缓存）。
// 首次调用或缓存过期时触发异步刷新，返回上次缓存值（首次返回 0）。
func GetTotalVMActualDiskUsage() int64 {
	vmDiskActualCacheMu.RLock()
	if !vmDiskActualCacheTime.IsZero() && time.Since(vmDiskActualCacheTime) < vmDiskActualCacheTTL {
		val := vmDiskActualCache
		vmDiskActualCacheMu.RUnlock()
		return val
	}
	vmDiskActualCacheMu.RUnlock()

	// 缓存已过期（或未初始化），异步刷新避免阻塞调用方
	refreshVMActualDiskUsageAsync()

	// 返回旧缓存值（首次调用时为 0，待异步刷新完成后下次即可取到）
	vmDiskActualCacheMu.RLock()
	val := vmDiskActualCache
	vmDiskActualCacheMu.RUnlock()
	return val
}

// refreshVMActualDiskUsageAsync 异步刷新虚拟机实际磁盘占用缓存。
// 通过 vmDiskActualRefreshing 标志防止并发刷新。
func refreshVMActualDiskUsageAsync() {
	vmDiskActualCacheMu.Lock()
	if vmDiskActualRefreshing {
		vmDiskActualCacheMu.Unlock()
		return
	}
	vmDiskActualRefreshing = true
	vmDiskActualCacheMu.Unlock()

	go func() {
		defer utils.RecoverAndLog("vm-disk-usage-refresh")
		defer func() {
			vmDiskActualCacheMu.Lock()
			vmDiskActualRefreshing = false
			vmDiskActualCacheMu.Unlock()
		}()

		total := calculateTotalVMActualDiskUsage()
		vmDiskActualCacheMu.Lock()
		vmDiskActualCache = total
		vmDiskActualCacheTime = time.Now()
		vmDiskActualCacheMu.Unlock()
		logger.App.Debug("虚拟机实际磁盘占用缓存已刷新", "total_bytes", total)
	}()
}

// calculateTotalVMActualDiskUsage 遍历所有虚拟机，累加每台虚拟机全部磁盘的实际占用（字节）。
// 复用 libvirt_rpc.ParseDisksFromDomainXML 解析所有磁盘（跳过光驱/软盘），
// 每块盘通过 qemu-img info 解析 actual-size，涵盖存放在其他存储池盘上的额外磁盘。
func calculateTotalVMActualDiskUsage() int64 {
	domains, err := libvirt_rpc.ListAllDomainsRPC()
	if err != nil {
		logger.App.Warn("获取虚拟机列表失败，无法统计实际磁盘占用", "error", err)
		return 0
	}

	var total int64
	for _, dom := range domains {
		name := strings.TrimSpace(dom.Name)
		if name == "" {
			continue
		}
		xmlStr, err := libvirt_rpc.GetDomainXMLRPC(name, 0)
		if err != nil {
			continue
		}
		for _, disk := range libvirt_rpc.ParseDisksFromDomainXML(xmlStr) {
			// 跳过光驱/软盘等非磁盘设备（ISO 不计入占用）
			if disk.Device != "" && disk.Device != "disk" {
				continue
			}
			if disk.Source == "" || disk.Source == "-" {
				continue
			}
			qemuInfoResult := utils.ExecShell(fmt.Sprintf("qemu-img info --output=json -U %s 2>/dev/null", utils.ShellSingleQuote(disk.Source)))
			if qemuInfoResult.Error != nil {
				continue
			}
			total += parseQemuActualSizeBytes(qemuInfoResult.Stdout)
		}
	}
	return total
}

// parseQemuSizeBytes 从 qemu-img info --output=json 的输出中解析指定大小字段（字节）。
func parseQemuSizeBytes(output, key string) int64 {
	var data map[string]json.RawMessage
	if err := json.Unmarshal([]byte(output), &data); err != nil {
		return 0
	}
	raw, ok := data[key]
	if !ok {
		return 0
	}
	var val int64
	if err := json.Unmarshal(raw, &val); err != nil {
		return 0
	}
	return val
}

// parseQemuActualSizeBytes 从 qemu-img info --output=json 的输出中解析 actual-size 字段（字节）。
// actual-size 表示 qcow2 文件在宿主机上实际分配的数据量（稀疏文件场景下通常小于 virtual-size）。
func parseQemuActualSizeBytes(output string) int64 {
	return parseQemuSizeBytes(output, "actual-size")
}

// parseQemuVirtualSizeBytes 从 qemu-img info --output=json 的输出中解析 virtual-size 字段（字节）。
// virtual-size 表示磁盘的虚拟配置容量（虚拟机内看到的盘大小）。
func parseQemuVirtualSizeBytes(output string) int64 {
	return parseQemuSizeBytes(output, "virtual-size")
}
