package vm

import (
	"os"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"time"

	"kvm_console/logger"
	"kvm_console/service/arch"
	"kvm_console/service/libvirt_rpc"
	"kvm_console/utils"
)

// ==================== 虚拟机和宿主机统计信息 ====================

// GetVMStats 获取虚拟机实时资源使用（用于监控图表）
func GetVMStats(name string) (*VmStats, error) {
	stats := &VmStats{}

	// CPU 使用率（两次采样计算差值）
	var cpuTime1 float64
	if libvirt_rpc.IsLibvirtRPCAvailable() {
		if cput, err := libvirt_rpc.GetDomainCPUStatsRPC(name); err == nil {
			cpuTime1 = float64(cput) / 1e9
		} else {
			logger.Libvirt.Warn("GetDomainCPUStatsRPC 失败，降级为 virsh", "domain", name, "error", err)
		}
	}
	if cpuTime1 == 0 {
		re := regexp.MustCompile(`cpu_time\s+([\d.]+)\s+seconds`)
		cpuResult1 := utils.ExecCommand("virsh", "cpu-stats", name, "--total")
		if cpuResult1.Error == nil {
			if matches := re.FindStringSubmatch(cpuResult1.Stdout); len(matches) > 1 {
				cpuTime1, _ = strconv.ParseFloat(matches[1], 64)
			}
		}
	}

	// 获取 vCPU 数量
	vcpuCount := 1
	if libvirt_rpc.IsLibvirtRPCAvailable() {
		if vcpu, _, _, _, err := libvirt_rpc.GetDomainInfoRPC(name); err == nil {
			vcpuCount = vcpu
			if vcpuCount <= 0 {
				vcpuCount = 1
			}
		} else {
			logger.Libvirt.Warn("GetDomainInfoRPC 失败，降级为 virsh", "domain", name, "error", err)
		}
	}
	if vcpuCount == 1 {
		infoResult := utils.ExecCommand("virsh", "dominfo", name)
		if infoResult.Error == nil {
			vcpuCount = parseInfoInt(infoResult.Stdout, "CPU(s):")
			if vcpuCount <= 0 {
				vcpuCount = 1
			}
		}
	}

	// 等待 1 秒再采样
	time.Sleep(time.Second)

	var cpuTime2 float64
	if libvirt_rpc.IsLibvirtRPCAvailable() {
		if cput, err := libvirt_rpc.GetDomainCPUStatsRPC(name); err == nil {
			cpuTime2 = float64(cput) / 1e9
		} else {
			logger.Libvirt.Warn("GetDomainCPUStatsRPC 失败，降级为 virsh", "domain", name, "error", err)
		}
	}
	if cpuTime2 == 0 {
		re := regexp.MustCompile(`cpu_time\s+([\d.]+)\s+seconds`)
		cpuResult2 := utils.ExecCommand("virsh", "cpu-stats", name, "--total")
		if cpuResult2.Error == nil {
			if matches := re.FindStringSubmatch(cpuResult2.Stdout); len(matches) > 1 {
				cpuTime2, _ = strconv.ParseFloat(matches[1], 64)
			}
		}
	}
	if cpuTime2 > cpuTime1 {
		// CPU 使用率 = (差值 / 采样间隔 / vCPU数) * 100
		delta := cpuTime2 - cpuTime1
		if delta >= 0 {
			stats.CPUPercent = (delta / 1.0 / float64(vcpuCount)) * 100
			if stats.CPUPercent > 100 {
				stats.CPUPercent = 100
			}
		}
	}

	// 内存统计（通过 dommemstat）
	if libvirt_rpc.IsLibvirtRPCAvailable() {
		if memStats, err := libvirt_rpc.GetDomainMemoryStatsRPC(name); err == nil {
			stats.MemTotal = int64(memStats["actual"])
			stats.MemUsed = stats.MemTotal - int64(memStats["unused"])
			if memStats["available"] > 0 {
				stats.MemUsed = stats.MemTotal - int64(memStats["usable"])
			}
		} else {
			logger.Libvirt.Warn("GetDomainMemoryStatsRPC 失败，降级为 virsh", "domain", name, "error", err)
		}
	}
	if stats.MemTotal == 0 {
		memResult := utils.ExecCommand("virsh", "dommemstat", name)
		if memResult.Error == nil {
			stats.MemTotal = parseMemStat(memResult.Stdout, "actual")
			stats.MemUsed = stats.MemTotal - parseMemStat(memResult.Stdout, "unused")
			if parseMemStat(memResult.Stdout, "available") > 0 {
				stats.MemUsed = stats.MemTotal - parseMemStat(memResult.Stdout, "usable")
			}
		}
	}

	// 网络统计 - 动态获取网卡接口名
	ifListResult := utils.ExecCommand("virsh", "domiflist", name)
	if ifListResult.Error == nil {
		ifLines := strings.Split(ifListResult.Stdout, "\n")
		for _, ifLine := range ifLines {
			fields := strings.Fields(ifLine)
			if len(fields) >= 2 && fields[0] != "Interface" && !strings.HasPrefix(ifLine, "-") {
				ifName := fields[0]
				if ifName == "-" || ifName == "" {
					continue
				}
				var netRxBytes, netTxBytes int64
				if libvirt_rpc.IsLibvirtRPCAvailable() {
					if rx, tx, err := libvirt_rpc.GetDomainInterfaceStatsRPC(name, ifName); err == nil {
						netRxBytes = rx
						netTxBytes = tx
					} else {
						logger.Libvirt.Warn("GetDomainInterfaceStatsRPC 失败，降级为 virsh", "domain", name, "interface", ifName, "error", err)
					}
				}
				if netRxBytes == 0 && netTxBytes == 0 {
					netResult := utils.ExecCommand("virsh", "domifstat", name, ifName)
					if netResult.Error == nil {
						netRxBytes = parseIfStat(netResult.Stdout, "rx_bytes")
						netTxBytes = parseIfStat(netResult.Stdout, "tx_bytes")
					}
				}
				stats.NetRxBytes += netRxBytes
				stats.NetTxBytes += netTxBytes
			}
		}
	}

	// 磁盘 I/O - 动态获取第一个磁盘设备
	blkListResult := utils.ExecCommand("virsh", "domblklist", name)
	if blkListResult.Error == nil {
		blkLines := strings.Split(blkListResult.Stdout, "\n")
		for _, blkLine := range blkLines {
			fields := strings.Fields(blkLine)
			if len(fields) >= 2 && fields[0] != "Target" && !strings.HasPrefix(blkLine, "-") {
				dev := fields[0]
				// 跳过 cdrom 设备（通常是 sda/hdc 且路径为 iso）
				if strings.HasSuffix(fields[1], ".iso") {
					continue
				}
				var diskRdBytes, diskWrBytes, diskRdOps, diskWrOps int64
				if libvirt_rpc.IsLibvirtRPCAvailable() {
					if rdReq, rd, wrReq, wr, err := libvirt_rpc.GetDomainBlockStatsRPC(name, dev); err == nil {
						diskRdBytes = rd
						diskWrBytes = wr
						diskRdOps = rdReq
						diskWrOps = wrReq
					} else {
						logger.Libvirt.Warn("GetDomainBlockStatsRPC 失败，降级为 virsh", "domain", name, "device", dev, "error", err)
					}
				}
				if diskRdBytes == 0 && diskWrBytes == 0 {
					blkResult := utils.ExecCommand("virsh", "domblkstat", name, dev)
					if blkResult.Error == nil {
						diskRdBytes = parseBlkStat(blkResult.Stdout, "rd_bytes")
						diskWrBytes = parseBlkStat(blkResult.Stdout, "wr_bytes")
						diskRdOps = parseBlkStat(blkResult.Stdout, "rd_req")
						diskWrOps = parseBlkStat(blkResult.Stdout, "wr_req")
					}
				}
				stats.DiskRdBytes += diskRdBytes
				stats.DiskWrBytes += diskWrBytes
				stats.DiskRdOps += diskRdOps
				stats.DiskWrOps += diskWrOps
				break // 只取第一个磁盘
			}
		}
	}

	return stats, nil
}

// GetHostStats 获取宿主机资源信息
func GetHostStats() (*HostStats, error) {
	stats := &HostStats{}

	// CPU 核心数
	stats.CPUCount = runtime.NumCPU()

	// CPU 使用率
	cpuUsageResult := utils.ExecShellQuiet(`top -bn1 | grep "Cpu(s)" | awk '{print $2}' | cut -d'%' -f1`)
	if cpuUsageResult.Error == nil {
		stats.CPUPercent, _ = strconv.ParseFloat(strings.TrimSpace(cpuUsageResult.Stdout), 64)
	}

	// 内存和 Swap 信息
	if memInfo, err := utils.ReadMemInfo(); err == nil {
		if v, ok := memInfo["MemTotal"]; ok {
			stats.MemTotal = v
		}
		if v, ok := memInfo["MemFree"]; ok {
			stats.MemFree = v
		}
		// 优先使用 MemAvailable 计算实际已用内存（排除可回收的 buffer/cache）
		if v, ok := memInfo["MemAvailable"]; ok && v > 0 {
			stats.MemAvailable = v
			stats.MemUsed = stats.MemTotal - v
		} else {
			// 兼容旧内核：退回到 MemFree 计算
			stats.MemUsed = stats.MemTotal - stats.MemFree
		}
		if v, ok := memInfo["SwapTotal"]; ok {
			stats.SwapTotal = v
		}
		if v, ok := memInfo["SwapFree"]; ok {
			stats.SwapFree = v
		}
		stats.SwapUsed = stats.SwapTotal - stats.SwapFree
	}

	// KSM 内存合并页数
	if data, err := os.ReadFile("/sys/kernel/mm/ksm/pages_shared"); err == nil {
		stats.KSMPagesShared, _ = strconv.ParseInt(strings.TrimSpace(string(data)), 10, 64)
	}
	if data, err := os.ReadFile("/sys/kernel/mm/ksm/pages_sharing"); err == nil {
		stats.KSMPagesSharing, _ = strconv.ParseInt(strings.TrimSpace(string(data)), 10, 64)
	}

	// 磁盘 IO 延迟（毫秒），通过 iostat 1 秒间隔采样获取实时 r_await
	ioLatencyResult := utils.ExecShell(`iostat -d -x 1 2 2>/dev/null | awk 'BEGIN{sec=0} /^Device/{sec++; next} sec==2 && $1 ~ /^(sd|vd|nvme|dm-)/ && $6 != "" {s+=$6; c++} END{if(c>0) printf "%.1f", s/c; else print "0"}'`)
	if ioLatencyResult.Error == nil {
		stats.DiskIOLatencyMs, _ = strconv.ParseFloat(strings.TrimSpace(ioLatencyResult.Stdout), 64)
	}

	// 磁盘信息（全部已挂载的本地文件系统，按源设备去重累加，涵盖存储池盘）
	if total, used, available, err := utils.GetAllMountedDiskSpace(); err == nil {
		stats.DiskTotal = total
		stats.DiskUsed = used
		stats.DiskFree = available
	} else if total, used, available, err := utils.GetDiskSpace("/"); err == nil {
		// 兜底：全量统计失败时退回根分区口径
		stats.DiskTotal = total
		stats.DiskUsed = used
		stats.DiskFree = available
	}

	// 虚拟机实际磁盘占用（带缓存，异步刷新，避免阻塞 SSE 推送）
	// 用于计算存储理论最大值时扣除系统占用部分
	stats.VMDiskActual = GetTotalVMActualDiskUsage() / 1024 // 字节转 KB

	// 宿主机网络 IO (累加常见物理网卡，排除 virbr, vnet, docker, lo)
	netIOResult := utils.ExecShell(`awk 'NR>2 {if ($1 !~ /lo|virbr|vnet|docker/) {rx+=$2; tx+=$10}} END {print rx, tx}' /proc/net/dev`)
	if netIOResult.Error == nil {
		parts := strings.Fields(netIOResult.Stdout)
		if len(parts) >= 2 {
			stats.NetRxBytes, _ = strconv.ParseInt(parts[0], 10, 64)
			stats.NetTxBytes, _ = strconv.ParseInt(parts[1], 10, 64)
		}
	}

	// 宿主机磁盘 IO
	// 优先通过 lsblk 获取 type=disk 的顶层设备，再从 /proc/diskstats 汇总累计扇区数。
	if diskRdBytes, diskWrBytes, err := D.CollectHostDiskIOBytes(); err == nil {
		stats.DiskRdBytes = diskRdBytes
		stats.DiskWrBytes = diskWrBytes
	}

	// 主机名
	hostnameResult := utils.ExecCommand("hostname")
	if hostnameResult.Error == nil {
		stats.Hostname = strings.TrimSpace(hostnameResult.Stdout)
	}

	// 宿主机架构
	stats.Arch = arch.GetHostArchDisplayName()

	// 运行时间
	uptimeResult := utils.ExecShell(`uptime -p`)
	if uptimeResult.Error == nil {
		stats.Uptime = strings.TrimSpace(uptimeResult.Stdout)
	}

	// 虚拟机数量
	runningResult := utils.ExecShellQuiet(`virsh list --name 2>/dev/null | grep -v '^$' | wc -l`)
	if runningResult.Error == nil {
		stats.VMRunning, _ = strconv.Atoi(strings.TrimSpace(runningResult.Stdout))
	}
	totalResult := utils.ExecShellQuiet(`virsh list --all --name 2>/dev/null | grep -v '^$' | wc -l`)
	if totalResult.Error == nil {
		stats.VMTotal, _ = strconv.Atoi(strings.TrimSpace(totalResult.Stdout))
	}

	return stats, nil
}

// ==================== 统计解析辅助函数 ====================

// parseMemStat 从 dommemstat 输出解析内存值（KB）
func parseMemStat(output, key string) int64 {
	lines := strings.Split(output, "\n")
	for _, line := range lines {
		fields := strings.Fields(line)
		if len(fields) >= 2 && fields[0] == key {
			val, _ := strconv.ParseInt(fields[1], 10, 64)
			return val
		}
	}
	return 0
}

// parseIfStat 从 domifstat 输出解析网络值
func parseIfStat(output, key string) int64 {
	lines := strings.Split(output, "\n")
	for _, line := range lines {
		if strings.Contains(line, key) {
			fields := strings.Fields(line)
			if len(fields) >= 2 {
				val, _ := strconv.ParseInt(fields[len(fields)-1], 10, 64)
				return val
			}
		}
	}
	return 0
}

// parseBlkStat 从 domblkstat 输出解析磁盘值
func parseBlkStat(output, key string) int64 {
	lines := strings.Split(output, "\n")
	for _, line := range lines {
		if strings.Contains(line, key) {
			fields := strings.Fields(line)
			if len(fields) >= 2 {
				val, _ := strconv.ParseInt(fields[len(fields)-1], 10, 64)
				return val
			}
		}
	}
	return 0
}
