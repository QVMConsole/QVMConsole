package libvirt_rpc

import (
	"fmt"
	"strings"

	"github.com/digitalocean/go-libvirt"

	"kvm_console/logger"
)

// ==================== vCPU 标志常量 ====================
// go-libvirt 未定义 vcpu flags 常量，此处按 libvirt C 头文件补充

const (
	DomainVcpuLive         uint32 = 1  // VIR_DOMAIN_VCPU_LIVE
	DomainVcpuConfig       uint32 = 2  // VIR_DOMAIN_VCPU_CONFIG
	DomainVcpuMaximum      uint32 = 4  // VIR_DOMAIN_VCPU_MAXIMUM
	DomainVcpuCurrent      uint32 = 8  // VIR_DOMAIN_VCPU_CURRENT
	DomainVcpuHotpluggable uint32 = 16 // VIR_DOMAIN_VCPU_HOTPLUGGABLE
	DomainVcpuGuest        uint32 = 32 // VIR_DOMAIN_VCPU_GUEST
)

// ==================== 内存标志常量 ====================
// go-libvirt 未定义内存 flags 常量，此处按 libvirt C 头文件补充

const (
	DomainMemLive    libvirt.DomainMemoryModFlags = 1 // VIR_DOMAIN_AFFECT_LIVE
	DomainMemConfig  libvirt.DomainMemoryModFlags = 2 // VIR_DOMAIN_AFFECT_CONFIG
	DomainMemMaximum libvirt.DomainMemoryModFlags = 4 // VIR_DOMAIN_MEM_MAXIMUM
)

// ==================== QEMU Monitor 命令标志常量 ====================
// go-libvirt 未定义 qemu monitor command flags 常量，此处按 libvirt C 头文件补充

const (
	DomainQemuMonitorCommandHmp   uint32 = 1  // VIR_DOMAIN_QEMU_MONITOR_COMMAND_HMP
	DomainQemuAgentCommandBlock   int32  = -2 // 使用 libvirt 的阻塞等待语义
	DomainQemuAgentCommandDefault int32  = -1 // 使用 libvirt 默认超时
)

// ==================== 状态映射 ====================

// domainStateToString 将 libvirt 状态码映射为与 virsh 输出一致的字符串
// 0=nostate, 1=running, 2=blocked, 3=paused, 4=shutdown, 5=shutoff, 6=crashed, 7=pmsuspended
func domainStateToString(state libvirt.DomainState) string {
	switch state {
	case libvirt.DomainNostate:
		return "no state"
	case libvirt.DomainRunning:
		return "running"
	case libvirt.DomainBlocked:
		return "blocked"
	case libvirt.DomainPaused:
		return "paused"
	case libvirt.DomainShutdown:
		return "shutdown"
	case libvirt.DomainShutoff:
		return "shut off"
	case libvirt.DomainCrashed:
		return "crashed"
	case libvirt.DomainPmsuspended:
		return "pmsuspended"
	default:
		return fmt.Sprintf("unknown(%d)", state)
	}
}

// ==================== 辅助函数 ====================

// DomainExistsRPC 通过 RPC 检查指定名称的域是否已存在
// 返回 true 表示域存在，false 表示域不存在
// 如果 libvirt 连接失败，返回错误
func DomainExistsRPC(name string) (bool, error) {
	l, err := GetLibvirt()
	if err != nil {
		return false, fmt.Errorf("检查域 %s 存在性失败: %w", name, err)
	}
	_, err = l.DomainLookupByName(name)
	if err != nil {
		return false, nil
	}
	return true, nil
}

// lookupDomainByName 通过名称查找 Domain 对象
func lookupDomainByName(name string) (libvirt.Domain, error) {
	l, err := GetLibvirt()
	if err != nil {
		return libvirt.Domain{}, fmt.Errorf("查找域 %s 失败: %w", name, err)
	}
	dom, err := l.DomainLookupByName(name)
	if err != nil {
		return libvirt.Domain{}, fmt.Errorf("域 %s 不存在: %w", name, err)
	}
	return dom, nil
}

// ==================== 高频只读操作 ====================

// ListAllDomainsRPC 列出所有 VM（替代 virsh list --all --name）
func ListAllDomainsRPC() ([]libvirt.Domain, error) {
	l, err := GetLibvirt()
	if err != nil {
		return nil, fmt.Errorf("获取域列表失败: %w", err)
	}
	// NeedResults=1 表示要返回结果, flags=0 表示所有状态（等价于 Active|Inactive）
	domains, _, err := l.ConnectListAllDomains(1, 0)
	if err != nil {
		return nil, fmt.Errorf("列出所有域失败: %w", err)
	}
	logger.Libvirt.Info("RPC: 列出所有域成功", "count", len(domains))
	return domains, nil
}

// GetDomainStateRPC 获取 VM 状态字符串（替代 virsh domstate）
func GetDomainStateRPC(name string) (string, error) {
	dom, err := lookupDomainByName(name)
	if err != nil {
		return "", err
	}
	l, err := GetLibvirt()
	if err != nil {
		return "", fmt.Errorf("获取域 %s 状态失败: %w", name, err)
	}
	state, _, _, _, _, err := l.DomainGetInfo(dom)
	if err != nil {
		return "", fmt.Errorf("获取域 %s 信息失败: %w", name, err)
	}
	stateStr := domainStateToString(libvirt.DomainState(state))
	logger.Libvirt.Info("RPC: 获取域状态成功", "domain", name, "state", stateStr)
	return stateStr, nil
}

// GetDomainInfoRPC 获取 VM 基本信息（替代 virsh dominfo）
// 返回 vcpu数, 最大内存KB, 已用内存KB, 是否自动启动
func GetDomainInfoRPC(name string) (vcpu int, maxMemKB uint64, usedMemKB uint64, autostart bool, err error) {
	dom, err := lookupDomainByName(name)
	if err != nil {
		return 0, 0, 0, false, err
	}
	l, err := GetLibvirt()
	if err != nil {
		return 0, 0, 0, false, fmt.Errorf("获取域 %s 信息失败: %w", name, err)
	}

	// 获取 vcpu / 内存信息
	state, maxMem, memory, nrVirtCPU, _, infoErr := l.DomainGetInfo(dom)
	if infoErr != nil {
		return 0, 0, 0, false, fmt.Errorf("获取域 %s 基础信息失败: %w", name, infoErr)
	}
	_ = state // 状态由 GetDomainStateRPC 单独获取

	// 获取自动启动状态
	autoVal, autoErr := l.DomainGetAutostart(dom)
	if autoErr != nil {
		// 某些域可能不支持 autostart，不阻断整体信息获取
		autoVal = 0
	}

	logger.Libvirt.Info("RPC: 获取域基本信息成功", "domain", name,
		"vcpu", nrVirtCPU, "maxMemKB", maxMem, "usedMemKB", memory, "autostart", autoVal == 1)
	return int(nrVirtCPU), maxMem, memory, autoVal == 1, nil
}

// GetDomainXMLRPC 获取 VM XML 配置（替代 virsh dumpxml）
// flags: 0 = 当前状态, libvirt.DomainXMLInactive = inactive 配置
func GetDomainXMLRPC(name string, flags libvirt.DomainXMLFlags) (string, error) {
	dom, err := lookupDomainByName(name)
	if err != nil {
		return "", err
	}
	l, err := GetLibvirt()
	if err != nil {
		return "", fmt.Errorf("获取域 %s XML 失败: %w", name, err)
	}
	xml, err := l.DomainGetXMLDesc(dom, flags)
	if err != nil {
		return "", fmt.Errorf("获取域 %s XML 描述失败: %w", name, err)
	}
	logger.Libvirt.Info("RPC: 获取域 XML 成功", "domain", name, "size", len(xml))
	return xml, nil
}

// GetDomainCPUStatsRPC 获取 CPU 时间统计（替代 virsh cpu-stats --total）
// 返回 CPU 时间（纳秒）
func GetDomainCPUStatsRPC(name string) (cpuTime uint64, err error) {
	dom, err := lookupDomainByName(name)
	if err != nil {
		return 0, err
	}
	l, err := GetLibvirt()
	if err != nil {
		return 0, fmt.Errorf("获取域 %s CPU 统计失败: %w", name, err)
	}
	_, _, _, _, cput, infoErr := l.DomainGetInfo(dom)
	if infoErr != nil {
		return 0, fmt.Errorf("获取域 %s CPU 时间失败: %w", name, infoErr)
	}
	logger.Libvirt.Debug("RPC: 获取 CPU 统计成功", "domain", name, "cpuTime", cput)
	return cput, nil
}

// memoryStatTagToString 将 DomainMemoryStatTag 映射为字符串 key
var memoryStatTagToString = map[int32]string{
	0:  "swap_in",
	1:  "swap_out",
	2:  "major_fault",
	3:  "minor_fault",
	4:  "unused",
	5:  "available",
	6:  "actual",
	7:  "rss",
	8:  "usable",
	9:  "last_update",
	10: "disk_caches",
	11: "hugetlb_pgalloc",
	12: "hugetlb_pgfail",
}

// GetDomainMemoryStatsRPC 获取内存统计（替代 virsh dommemstat）
// 返回 map: "actual"→实际分配KB, "rss"→RSS, "available"→总可用, "unused"→未使用 等
func GetDomainMemoryStatsRPC(name string) (map[string]uint64, error) {
	dom, err := lookupDomainByName(name)
	if err != nil {
		return nil, err
	}
	l, err := GetLibvirt()
	if err != nil {
		return nil, fmt.Errorf("获取域 %s 内存统计失败: %w", name, err)
	}
	// MaxStats=16 覆盖所有已知统计项
	stats, err := l.DomainMemoryStats(dom, 16, 0)
	if err != nil {
		return nil, fmt.Errorf("获取域 %s 内存统计失败: %w", name, err)
	}

	result := make(map[string]uint64, len(stats))
	for _, s := range stats {
		key, ok := memoryStatTagToString[s.Tag]
		if !ok {
			key = fmt.Sprintf("tag_%d", s.Tag)
		}
		result[key] = s.Val
	}
	logger.Libvirt.Debug("RPC: 获取内存统计成功", "domain", name, "items", len(result))
	return result, nil
}

// GetDomainBlockStatsRPC 获取磁盘 I/O 统计（替代 virsh domblkstat）
// 返回 读请求数, 读字节数, 写请求数, 写字节数
func GetDomainBlockStatsRPC(name, dev string) (rdReq, rdBytes, wrReq, wrBytes int64, err error) {
	dom, err := lookupDomainByName(name)
	if err != nil {
		return 0, 0, 0, 0, err
	}
	l, err := GetLibvirt()
	if err != nil {
		return 0, 0, 0, 0, fmt.Errorf("获取域 %s 磁盘统计失败: %w", name, err)
	}
	rdReq, rdByt, wrReq, wrByt, errs, statErr := l.DomainBlockStats(dom, dev)
	if statErr != nil {
		return 0, 0, 0, 0, fmt.Errorf("获取域 %s 磁盘 %s 统计失败: %w", name, dev, statErr)
	}
	_ = errs
	logger.Libvirt.Debug("RPC: 获取磁盘 I/O 统计成功", "domain", name, "device", dev,
		"rdBytes", rdByt, "wrBytes", wrByt, "rdReq", rdReq, "wrReq", wrReq)
	return rdReq, rdByt, wrReq, wrByt, nil
}

// GetDomainInterfaceStatsRPC 获取网络 I/O 统计（替代 virsh domifstat）
// 返回 接收字节数, 发送字节数
func GetDomainInterfaceStatsRPC(name, iface string) (rxBytes, txBytes int64, err error) {
	dom, err := lookupDomainByName(name)
	if err != nil {
		return 0, 0, err
	}
	l, err := GetLibvirt()
	if err != nil {
		return 0, 0, fmt.Errorf("获取域 %s 网络统计失败: %w", name, err)
	}
	rxByt, rxPkt, rxErr, rxDrop, txByt, txPkt, txErr, txDrop, statErr := l.DomainInterfaceStats(dom, iface)
	if statErr != nil {
		return 0, 0, fmt.Errorf("获取域 %s 网卡 %s 统计失败: %w", name, iface, statErr)
	}
	_ = rxPkt
	_ = rxErr
	_ = rxDrop
	_ = txPkt
	_ = txErr
	_ = txDrop
	logger.Libvirt.Debug("RPC: 获取网卡 I/O 统计成功", "domain", name, "interface", iface,
		"rxBytes", rxByt, "txBytes", txByt)
	return rxByt, txByt, nil
}

// ==================== 中频控制操作 ====================

// StartDomainRPC 启动 VM（替代 virsh start）
func StartDomainRPC(name string) error {
	dom, err := lookupDomainByName(name)
	if err != nil {
		return err
	}
	l, err := GetLibvirt()
	if err != nil {
		return fmt.Errorf("启动域 %s 失败: %w", name, err)
	}
	if err := l.DomainCreate(dom); err != nil {
		return fmt.Errorf("启动域 %s 失败: %w", name, err)
	}
	logger.Libvirt.Info("RPC: 启动域成功", "domain", name)
	return nil
}

// StartDomainPausedRPC 以暂停模式启动 VM（替代 virsh start --paused）
func StartDomainPausedRPC(name string) error {
	dom, err := lookupDomainByName(name)
	if err != nil {
		return err
	}
	l, err := GetLibvirt()
	if err != nil {
		return fmt.Errorf("暂停启动域 %s 失败: %w", name, err)
	}
	// DomainCreateWithFlags 的 Flags 参数类型为 uint32
	if _, err := l.DomainCreateWithFlags(dom, uint32(libvirt.DomainStartPaused)); err != nil {
		return fmt.Errorf("暂停启动域 %s 失败: %w", name, err)
	}
	logger.Libvirt.Info("RPC: 暂停启动域成功", "domain", name)
	return nil
}

// ShutdownDomainRPC 正常关机（替代 virsh shutdown）
func ShutdownDomainRPC(name string) error {
	dom, err := lookupDomainByName(name)
	if err != nil {
		return err
	}
	l, err := GetLibvirt()
	if err != nil {
		return fmt.Errorf("关机域 %s 失败: %w", name, err)
	}
	if err := l.DomainShutdown(dom); err != nil {
		return fmt.Errorf("关机域 %s 失败: %w", name, err)
	}
	logger.Libvirt.Info("RPC: 关机域成功", "domain", name)
	return nil
}

// DestroyDomainRPC 强制断电（替代 virsh destroy）
func DestroyDomainRPC(name string) error {
	dom, err := lookupDomainByName(name)
	if err != nil {
		return err
	}
	l, err := GetLibvirt()
	if err != nil {
		return fmt.Errorf("强制断电域 %s 失败: %w", name, err)
	}
	if err := l.DomainDestroy(dom); err != nil {
		return fmt.Errorf("强制断电域 %s 失败: %w", name, err)
	}
	logger.Libvirt.Info("RPC: 强制断电成功", "domain", name)
	return nil
}

// RebootDomainRPC 重启（替代 virsh reboot）
func RebootDomainRPC(name string) error {
	dom, err := lookupDomainByName(name)
	if err != nil {
		return err
	}
	l, err := GetLibvirt()
	if err != nil {
		return fmt.Errorf("重启域 %s 失败: %w", name, err)
	}
	if err := l.DomainReboot(dom, 0); err != nil {
		return fmt.Errorf("重启域 %s 失败: %w", name, err)
	}
	logger.Libvirt.Info("RPC: 重启域成功", "domain", name)
	return nil
}

// ResetDomainRPC 硬重置（替代 virsh reset）
func ResetDomainRPC(name string) error {
	dom, err := lookupDomainByName(name)
	if err != nil {
		return err
	}
	l, err := GetLibvirt()
	if err != nil {
		return fmt.Errorf("硬重置域 %s 失败: %w", name, err)
	}
	if err := l.DomainReset(dom, 0); err != nil {
		return fmt.Errorf("硬重置域 %s 失败: %w", name, err)
	}
	logger.Libvirt.Info("RPC: 硬重置成功", "domain", name)
	return nil
}

// SuspendDomainRPC 暂停（替代 virsh suspend）
func SuspendDomainRPC(name string) error {
	dom, err := lookupDomainByName(name)
	if err != nil {
		return err
	}
	l, err := GetLibvirt()
	if err != nil {
		return fmt.Errorf("暂停域 %s 失败: %w", name, err)
	}
	if err := l.DomainSuspend(dom); err != nil {
		return fmt.Errorf("暂停域 %s 失败: %w", name, err)
	}
	logger.Libvirt.Info("RPC: 暂停域成功", "domain", name)
	return nil
}

// ResumeDomainRPC 恢复（替代 virsh resume）
func ResumeDomainRPC(name string) error {
	dom, err := lookupDomainByName(name)
	if err != nil {
		return err
	}
	l, err := GetLibvirt()
	if err != nil {
		return fmt.Errorf("恢复域 %s 失败: %w", name, err)
	}
	if err := l.DomainResume(dom); err != nil {
		return fmt.Errorf("恢复域 %s 失败: %w", name, err)
	}
	logger.Libvirt.Info("RPC: 恢复域成功", "domain", name)
	return nil
}

// SetDomainAutostartRPC 设置自动启动（替代 virsh autostart）
func SetDomainAutostartRPC(name string, autostart bool) error {
	dom, err := lookupDomainByName(name)
	if err != nil {
		return err
	}
	l, err := GetLibvirt()
	if err != nil {
		return fmt.Errorf("设置域 %s 自动启动失败: %w", name, err)
	}
	autostartInt := int32(0)
	if autostart {
		autostartInt = 1
	}
	if err := l.DomainSetAutostart(dom, autostartInt); err != nil {
		return fmt.Errorf("设置域 %s 自动启动失败: %w", name, err)
	}
	logger.Libvirt.Info("RPC: 设置自动启动成功", "domain", name, "autostart", autostart)
	return nil
}

// DefineDomainXMLRPC 定义/更新 VM 配置（替代 virsh define）
func DefineDomainXMLRPC(xmlContent string) (libvirt.Domain, error) {
	l, err := GetLibvirt()
	if err != nil {
		return libvirt.Domain{}, fmt.Errorf("定义域失败: %w", err)
	}
	dom, err := l.DomainDefineXML(xmlContent)
	if err != nil {
		return libvirt.Domain{}, fmt.Errorf("定义域失败: %w", err)
	}
	logger.Libvirt.Info("RPC: 定义域配置成功", "domain", dom.Name)
	return dom, nil
}

// SetDomainVcpusFlagsRPC 设置 vCPU 数量（替代 virsh setvcpus）
// flags 组合使用 DomainVcpu* 常量（如 DomainVcpuConfig | DomainVcpuMaximum）
func SetDomainVcpusFlagsRPC(name string, count uint32, flags uint32) error {
	dom, err := lookupDomainByName(name)
	if err != nil {
		return err
	}
	l, err := GetLibvirt()
	if err != nil {
		return fmt.Errorf("设置域 %s vCPU 失败: %w", name, err)
	}
	if err := l.DomainSetVcpusFlags(dom, count, flags); err != nil {
		return fmt.Errorf("设置域 %s vCPU 为 %d 失败: %w", name, count, err)
	}
	logger.Libvirt.Info("RPC: 设置 vCPU 成功", "domain", name, "count", count, "flags", flags)
	return nil
}

// SetDomainMemoryFlagsRPC 设置内存大小（替代 virsh setmem/setmaxmem）
// flags 组合使用 libvirt.DomainMem* 常量
func SetDomainMemoryFlagsRPC(name string, memKB uint64, flags libvirt.DomainMemoryModFlags) error {
	dom, err := lookupDomainByName(name)
	if err != nil {
		return err
	}
	l, err := GetLibvirt()
	if err != nil {
		return fmt.Errorf("设置域 %s 内存失败: %w", name, err)
	}
	if err := l.DomainSetMemoryFlags(dom, memKB, uint32(flags)); err != nil {
		return fmt.Errorf("设置域 %s 内存为 %d KB 失败: %w", name, memKB, err)
	}
	logger.Libvirt.Info("RPC: 设置内存成功", "domain", name, "memKB", memKB, "flags", flags)
	return nil
}

// GetDomainVcpuCountRPC 获取 vCPU 计数（替代 virsh vcpucount）
func GetDomainVcpuCountRPC(name string, flags uint32) (int, error) {
	dom, err := lookupDomainByName(name)
	if err != nil {
		return 0, err
	}
	l, err := GetLibvirt()
	if err != nil {
		return 0, fmt.Errorf("获取域 %s vCPU 计数失败: %w", name, err)
	}
	count, err := l.DomainGetVcpusFlags(dom, flags)
	if err != nil {
		return 0, fmt.Errorf("获取域 %s vCPU 计数失败: %w", name, err)
	}
	logger.Libvirt.Debug("RPC: 获取 vCPU 计数成功", "domain", name, "count", count)
	return int(count), nil
}

// ==================== 域管理扩展操作 ====================

// UndefineDomainRPC 取消定义 VM（替代 virsh undefine [--nvram] [--snapshots-metadata]）
// flags: libvirt.DomainUndefineNvram(4) / libvirt.DomainUndefineSnapshotsMetadata(2) 等
func UndefineDomainRPC(name string, flags libvirt.DomainUndefineFlagsValues) error {
	dom, err := lookupDomainByName(name)
	if err != nil {
		return err
	}
	l, err := GetLibvirt()
	if err != nil {
		return fmt.Errorf("取消定义域 %s 失败: %w", name, err)
	}
	if err := l.DomainUndefineFlags(dom, flags); err != nil {
		return fmt.Errorf("取消定义域 %s 失败: %w", name, err)
	}
	logger.Libvirt.Info("RPC: 取消定义域成功", "domain", name, "flags", flags)
	return nil
}

// QemuMonitorCommandRPC 执行 QEMU Monitor 命令（替代 virsh qemu-monitor-command --hmp）
// flags: 0 = QMP 模式, DomainQemuMonitorCommandHmp(1) = HMP 模式
func QemuMonitorCommandRPC(name string, cmd string, flags uint32) (string, error) {
	dom, err := lookupDomainByName(name)
	if err != nil {
		return "", err
	}
	l, err := GetLibvirt()
	if err != nil {
		return "", fmt.Errorf("执行域 %s QEMU Monitor 命令失败: %w", name, err)
	}
	result, err := l.QEMUDomainMonitorCommand(dom, cmd, flags)
	if err != nil {
		return "", fmt.Errorf("执行域 %s QEMU Monitor 命令失败: %w", name, err)
	}
	logger.Libvirt.Info("RPC: 执行 QEMU Monitor 命令成功", "domain", name, "cmd", cmd, "flags", flags)
	return result, nil
}

// QemuAgentCommandRPC 通过 libvirt QEMU Agent RPC 执行来宾命令。
// 调用方只传递已经序列化的 QGA JSON；这里刻意不记录命令正文，避免密码和脚本进入日志。
func QemuAgentCommandRPC(name, cmd string, timeoutSeconds int32) (string, error) {
	dom, err := lookupDomainByName(name)
	if err != nil {
		return "", err
	}
	l, err := GetLibvirt()
	if err != nil {
		return "", fmt.Errorf("连接虚拟机 %s 的 QEMU Guest Agent 失败: %w", name, err)
	}
	result, err := l.QEMUDomainAgentCommand(dom, cmd, timeoutSeconds, 0)
	if err != nil {
		return "", fmt.Errorf("虚拟机 %s 的 QEMU Guest Agent 命令执行失败: %w", name, err)
	}
	if len(result) == 0 {
		return "", fmt.Errorf("虚拟机 %s 的 QEMU Guest Agent 未返回结果", name)
	}
	return result[0], nil
}

// GetDomainMetadataRPC 获取域元数据（替代 virsh metadata）
// metadataType: libvirt.DomainMetadataDescription(0) / DomainMetadataTitle(1) / DomainMetadataElement(2)
func GetDomainMetadataRPC(name string, metadataType int32, uri string, flags uint32) (string, error) {
	dom, err := lookupDomainByName(name)
	if err != nil {
		return "", err
	}
	l, err := GetLibvirt()
	if err != nil {
		return "", fmt.Errorf("获取域 %s 元数据失败: %w", name, err)
	}
	// 将 string 转换为 OptString：空字符串对应 nil（null），非空对应单元素切片
	var uriOpt libvirt.OptString
	if uri != "" {
		uriOpt = libvirt.OptString{uri}
	}
	metadata, err := l.DomainGetMetadata(dom, metadataType, uriOpt, libvirt.DomainModificationImpact(flags))
	if err != nil {
		return "", fmt.Errorf("获取域 %s 元数据失败: %w", name, err)
	}
	logger.Libvirt.Info("RPC: 获取域元数据成功", "domain", name, "type", metadataType)
	return metadata, nil
}

// SetDomainMetadataRPC 设置域元数据（替代 virsh metadata --edit）
// metadataType: libvirt.DomainMetadataDescription(0) / DomainMetadataTitle(1) / DomainMetadataElement(2)
// 传入空字符串的 metadata 表示删除对应元数据
func SetDomainMetadataRPC(name string, metadataType int32, metadata string, key string, uri string, flags uint32) error {
	dom, err := lookupDomainByName(name)
	if err != nil {
		return err
	}
	l, err := GetLibvirt()
	if err != nil {
		return fmt.Errorf("设置域 %s 元数据失败: %w", name, err)
	}
	// 将 string 转换为 OptString：空字符串对应 nil（null），非空对应单元素切片
	var metadataOpt libvirt.OptString
	if metadata != "" {
		metadataOpt = libvirt.OptString{metadata}
	}
	var keyOpt libvirt.OptString
	if key != "" {
		keyOpt = libvirt.OptString{key}
	}
	var uriOpt libvirt.OptString
	if uri != "" {
		uriOpt = libvirt.OptString{uri}
	}
	if err := l.DomainSetMetadata(dom, metadataType, metadataOpt, keyOpt, uriOpt, libvirt.DomainModificationImpact(flags)); err != nil {
		return fmt.Errorf("设置域 %s 元数据失败: %w", name, err)
	}
	logger.Libvirt.Info("RPC: 设置域元数据成功", "domain", name, "type", metadataType)
	return nil
}

// AttachDeviceFlagsRPC 热插拔设备（替代 virsh attach-device）
// flags: libvirt.DomainDeviceModifyLive(1) / DomainDeviceModifyConfig(2) / DomainDeviceModifyForce(4)
func AttachDeviceFlagsRPC(name string, xmlDesc string, flags uint32) error {
	dom, err := lookupDomainByName(name)
	if err != nil {
		return err
	}
	l, err := GetLibvirt()
	if err != nil {
		return fmt.Errorf("挂载设备到域 %s 失败: %w", name, err)
	}
	if err := l.DomainAttachDeviceFlags(dom, xmlDesc, flags); err != nil {
		return fmt.Errorf("挂载设备到域 %s 失败: %w", name, err)
	}
	logger.Libvirt.Info("RPC: 挂载设备成功", "domain", name, "flags", flags)
	return nil
}

// DetachDeviceFlagsRPC 热拔插设备（替代 virsh detach-device）
// flags: libvirt.DomainDeviceModifyLive(1) / DomainDeviceModifyConfig(2) / DomainDeviceModifyForce(4)
func DetachDeviceFlagsRPC(name string, xmlDesc string, flags uint32) error {
	dom, err := lookupDomainByName(name)
	if err != nil {
		return err
	}
	l, err := GetLibvirt()
	if err != nil {
		return fmt.Errorf("卸载域 %s 设备失败: %w", name, err)
	}
	if err := l.DomainDetachDeviceFlags(dom, xmlDesc, flags); err != nil {
		return fmt.Errorf("卸载域 %s 设备失败: %w", name, err)
	}
	logger.Libvirt.Info("RPC: 卸载设备成功", "domain", name, "flags", flags)
	return nil
}

// GetBlockInfoRPC 获取磁盘块信息（替代 virsh domblkinfo）
// 返回磁盘容量/分配/物理大小（字节）
func GetBlockInfoRPC(name string, dev string) (capacity uint64, allocation uint64, physical uint64, err error) {
	dom, err := lookupDomainByName(name)
	if err != nil {
		return 0, 0, 0, err
	}
	l, err := GetLibvirt()
	if err != nil {
		return 0, 0, 0, fmt.Errorf("获取域 %s 磁盘 %s 信息失败: %w", name, dev, err)
	}
	// API 返回顺序: allocation, capacity, physical
	allocVal, capVal, physVal, infoErr := l.DomainGetBlockInfo(dom, dev, 0)
	if infoErr != nil {
		return 0, 0, 0, fmt.Errorf("获取域 %s 磁盘 %s 信息失败: %w", name, dev, infoErr)
	}
	logger.Libvirt.Debug("RPC: 获取磁盘信息成功", "domain", name, "device", dev,
		"capacity", capVal, "allocation", allocVal, "physical", physVal)
	return capVal, allocVal, physVal, nil
}

// GetInterfaceParametersRPC 获取网卡参数（替代 virsh domiftune）
// 采用两次调用模式：先获取参数数量，再获取实际参数
func GetInterfaceParametersRPC(name string, device string, flags uint32) ([]libvirt.TypedParam, error) {
	dom, err := lookupDomainByName(name)
	if err != nil {
		return nil, err
	}
	l, err := GetLibvirt()
	if err != nil {
		return nil, fmt.Errorf("获取域 %s 网卡 %s 参数失败: %w", name, device, err)
	}
	// 第一次调用：获取参数数量
	_, nparams, err := l.DomainGetInterfaceParameters(dom, device, 0, libvirt.DomainModificationImpact(flags))
	if err != nil {
		return nil, fmt.Errorf("获取域 %s 网卡 %s 参数数量失败: %w", name, device, err)
	}
	if nparams == 0 {
		logger.Libvirt.Debug("RPC: 网卡参数为空", "domain", name, "device", device)
		return []libvirt.TypedParam{}, nil
	}
	// 第二次调用：获取实际参数
	params, _, err := l.DomainGetInterfaceParameters(dom, device, nparams, libvirt.DomainModificationImpact(flags))
	if err != nil {
		return nil, fmt.Errorf("获取域 %s 网卡 %s 参数失败: %w", name, device, err)
	}
	logger.Libvirt.Debug("RPC: 获取网卡参数成功", "domain", name, "device", device, "count", len(params))
	return params, nil
}

// ==================== XML 解析辅助函数 ====================

// DiskBlockInfo 从域 XML 解析的磁盘块设备简要信息
type DiskBlockInfo struct {
	Target string // 设备名（如 vda, vdb）
	Source string // 磁盘文件路径
	Device string // 设备类型（disk/cdrom/floppy/lun），空表示未声明
}

// ParseDisksFromDomainXML 从域 XML 中解析所有磁盘块设备（替代 virsh domblklist）
func ParseDisksFromDomainXML(xmlStr string) []DiskBlockInfo {
	var disks []DiskBlockInfo
	lines := strings.Split(xmlStr, "\n")
	inDisk := false
	inBackingStore := 0 // 跟踪 <backingStore> 嵌套层级，忽略其内部的 <source>
	var current DiskBlockInfo

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "<disk ") {
			inDisk = true
			inBackingStore = 0
			current = DiskBlockInfo{}
			// 提取 device 属性（如 device='cdrom'），便于调用方过滤非磁盘设备
			if strings.Contains(trimmed, "device='") {
				parts := strings.Split(trimmed, "device='")
				if len(parts) > 1 {
					current.Device = strings.Split(parts[1], "'")[0]
				}
			}
		}
		if inDisk {
			// 跟踪 backingStore 嵌套，防止 backing file 路径覆盖主磁盘路径
			if strings.HasPrefix(trimmed, "<backingStore") && !strings.Contains(trimmed, "/>") {
				inBackingStore++
			}
			if strings.HasPrefix(trimmed, "</backingStore>") {
				if inBackingStore > 0 {
					inBackingStore--
				}
			}
			// 只在非 backingStore 嵌套内时提取 <source> 路径
			if inBackingStore == 0 && strings.Contains(trimmed, "<source ") {
				if strings.Contains(trimmed, "file='") {
					parts := strings.Split(trimmed, "file='")
					if len(parts) > 1 {
						current.Source = strings.Split(parts[1], "'")[0]
					}
				}
			}
			if strings.Contains(trimmed, "<target") && strings.Contains(trimmed, "dev='") {
				parts := strings.Split(trimmed, "dev='")
				if len(parts) > 1 {
					current.Target = strings.Split(parts[1], "'")[0]
				}
			}
			if strings.Contains(trimmed, "</disk>") {
				if current.Target != "" {
					disks = append(disks, current)
				}
				inDisk = false
			}
		}
	}
	return disks
}

// InterfaceInfo 从域 XML 解析的网卡信息
type InterfaceInfo struct {
	MAC    string // MAC 地址
	Type   string // 接口类型（network/bridge 等）
	Source string // 网络源（网络名或桥接名）
	Model  string // 网卡模型（virtio/e1000 等）
	Target string // vnet 设备名（运行时）
}

// ParseInterfacesFromDomainXML 从域 XML 中解析所有网卡（替代 virsh domiflist）
func ParseInterfacesFromDomainXML(xmlStr string) []InterfaceInfo {
	var ifaces []InterfaceInfo
	lines := strings.Split(xmlStr, "\n")
	inIface := false
	var current InterfaceInfo

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "<interface ") {
			inIface = true
			current = InterfaceInfo{}
			if strings.Contains(trimmed, "type='") {
				parts := strings.Split(trimmed, "type='")
				if len(parts) > 1 {
					current.Type = strings.Split(parts[1], "'")[0]
				}
			}
		}
		if inIface {
			if strings.Contains(trimmed, "<source ") {
				if strings.Contains(trimmed, "network='") {
					parts := strings.Split(trimmed, "network='")
					if len(parts) > 1 {
						current.Source = strings.Split(parts[1], "'")[0]
					}
				} else if strings.Contains(trimmed, "bridge='") {
					parts := strings.Split(trimmed, "bridge='")
					if len(parts) > 1 {
						current.Source = strings.Split(parts[1], "'")[0]
					}
				}
			}
			if strings.Contains(trimmed, "<mac") && strings.Contains(trimmed, "address='") {
				parts := strings.Split(trimmed, "address='")
				if len(parts) > 1 {
					current.MAC = strings.Split(parts[1], "'")[0]
				}
			}
			if strings.Contains(trimmed, "<model") && strings.Contains(trimmed, "type='") {
				parts := strings.Split(trimmed, "type='")
				if len(parts) > 1 {
					current.Model = strings.Split(parts[1], "'")[0]
				}
			}
			if strings.Contains(trimmed, "<target") && strings.Contains(trimmed, "dev='") {
				parts := strings.Split(trimmed, "dev='")
				if len(parts) > 1 {
					current.Target = strings.Split(parts[1], "'")[0]
				}
			}
			if strings.Contains(trimmed, "</interface>") {
				ifaces = append(ifaces, current)
				inIface = false
			}
		}
	}
	return ifaces
}

// GetFirstVMMACFromXML 通过 RPC 获取 VM 的第一个网卡 MAC 地址
func GetFirstVMMACFromXML(vmName string) string {
	xmlStr, err := GetDomainXMLRPC(vmName, 0)
	if err != nil {
		return ""
	}
	ifaces := ParseInterfacesFromDomainXML(xmlStr)
	if len(ifaces) > 0 {
		return strings.ToLower(ifaces[0].MAC)
	}
	return ""
}

// GetFirstVMVnetIFFromXML 通过 RPC 获取运行中 VM 的第一个 vnet 接口名
func GetFirstVMVnetIFFromXML(vmName string) string {
	xmlStr, err := GetDomainXMLRPC(vmName, 0)
	if err != nil {
		return ""
	}
	ifaces := ParseInterfacesFromDomainXML(xmlStr)
	for _, iface := range ifaces {
		if strings.HasPrefix(iface.Target, "vnet") {
			return iface.Target
		}
	}
	return ""
}

// GetFirstVMInterfaceModelFromXML 通过 RPC 获取 VM 的第一个网卡模型
func GetFirstVMInterfaceModelFromXML(vmName string) string {
	xmlStr, err := GetDomainXMLRPC(vmName, 0)
	if err != nil {
		return "virtio"
	}
	ifaces := ParseInterfacesFromDomainXML(xmlStr)
	if len(ifaces) > 0 && ifaces[0].Model != "" {
		return ifaces[0].Model
	}
	return "virtio"
}

// SetInterfaceParametersRPC 设置网卡参数（替代 virsh domiftune --set）
func SetInterfaceParametersRPC(name string, device string, params []libvirt.TypedParam, flags uint32) error {
	dom, err := lookupDomainByName(name)
	if err != nil {
		return err
	}
	l, err := GetLibvirt()
	if err != nil {
		return fmt.Errorf("设置域 %s 网卡 %s 参数失败: %w", name, device, err)
	}
	if err := l.DomainSetInterfaceParameters(dom, device, params, flags); err != nil {
		return fmt.Errorf("设置域 %s 网卡 %s 参数失败: %w", name, device, err)
	}
	logger.Libvirt.Info("RPC: 设置网卡参数成功", "domain", name, "device", device, "flags", flags)
	return nil
}

// ==================== 磁盘 I/O Tuning ====================

// BlockResizeRPC 调整磁盘块大小（替代 virsh blockresize）
// size 单位为字节；flags: DomainBlockResizeBytes(1)=size以字节为单位, DomainBlockResizeCapacity(2)=size以扇区为单位
func BlockResizeRPC(name string, disk string, size uint64, flags libvirt.DomainBlockResizeFlags) error {
	dom, err := lookupDomainByName(name)
	if err != nil {
		return err
	}
	l, err := GetLibvirt()
	if err != nil {
		return fmt.Errorf("调整域 %s 磁盘 %s 大小失败: %w", name, disk, err)
	}
	if err := l.DomainBlockResize(dom, disk, size, flags); err != nil {
		return fmt.Errorf("调整域 %s 磁盘 %s 大小为 %d 失败: %w", name, disk, size, err)
	}
	logger.Libvirt.Info("RPC: 调整磁盘大小成功", "domain", name, "disk", disk, "size", size, "flags", flags)
	return nil
}

// GetBlkIOParametersRPC 获取磁盘 I/O Tuning 参数（替代 virsh blkdeviotune）
// 采用两次调用模式：先获取参数数量，再获取实际参数
func GetBlkIOParametersRPC(name string, disk string, flags uint32) ([]libvirt.TypedParam, error) {
	dom, err := lookupDomainByName(name)
	if err != nil {
		return nil, err
	}
	l, err := GetLibvirt()
	if err != nil {
		return nil, fmt.Errorf("获取域 %s 磁盘 %s IOTune 失败: %w", name, disk, err)
	}
	// 第一次调用：获取参数数量
	var diskOpt libvirt.OptString
	if disk != "" {
		diskOpt = libvirt.OptString{disk}
	}
	_, nparams, err := l.DomainGetBlockIOTune(dom, diskOpt, 0, flags)
	if err != nil {
		return nil, fmt.Errorf("获取域 %s 磁盘 %s IOTune 参数数量失败: %w", name, disk, err)
	}
	if nparams == 0 {
		logger.Libvirt.Debug("RPC: 磁盘 IOTune 参数为空", "domain", name, "disk", disk)
		return []libvirt.TypedParam{}, nil
	}
	// 第二次调用：获取实际参数
	params, _, err := l.DomainGetBlockIOTune(dom, diskOpt, nparams, flags)
	if err != nil {
		return nil, fmt.Errorf("获取域 %s 磁盘 %s IOTune 参数失败: %w", name, disk, err)
	}
	logger.Libvirt.Debug("RPC: 获取磁盘 IOTune 成功", "domain", name, "disk", disk, "count", len(params))
	return params, nil
}

// SetBlkIOParametersRPC 设置磁盘 I/O Tuning 参数（替代 virsh blkdeviotune --set）
func SetBlkIOParametersRPC(name string, disk string, params []libvirt.TypedParam, flags uint32) error {
	dom, err := lookupDomainByName(name)
	if err != nil {
		return err
	}
	l, err := GetLibvirt()
	if err != nil {
		return fmt.Errorf("设置域 %s 磁盘 %s IOTune 失败: %w", name, disk, err)
	}
	if err := l.DomainSetBlockIOTune(dom, disk, params, flags); err != nil {
		return fmt.Errorf("设置域 %s 磁盘 %s IOTune 失败: %w", name, disk, err)
	}
	logger.Libvirt.Info("RPC: 设置磁盘 IOTune 成功", "domain", name, "disk", disk, "flags", flags)
	return nil
}
