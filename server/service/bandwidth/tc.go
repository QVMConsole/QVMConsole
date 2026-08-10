package bandwidth

import (
	"fmt"

	"github.com/digitalocean/go-libvirt"

	"kvm_console/logger"
	"kvm_console/service/libvirt_rpc"
	"kvm_console/utils"
)

// ==================== TC 命令相关 ====================

func clearTCBandwidthLimit(vnetIF string) {
	if vnetIF == "" {
		return
	}
	utils.ExecShellQuiet(fmt.Sprintf("tc qdisc del dev %s root 2>/dev/null", utils.ShellSingleQuote(vnetIF)))
	clearTCUploadLimit(vnetIF)
}

// ApplyTCVPCSwitchDownlinkLimit 在 VPC 网关口上设置下行限速（tc HTB）
func ApplyTCVPCSwitchDownlinkLimit(gwPort string, downMbps int) {
	if gwPort == "" {
		return
	}
	utils.ExecShellQuiet(fmt.Sprintf("tc qdisc del dev %s root 2>/dev/null", utils.ShellSingleQuote(gwPort)))
	if downMbps <= 0 {
		return
	}
	rateKbit := downMbps * 1000
	burstBytes := downMbps * 1000 * 1024 / 10
	if burstBytes < 15360 {
		burstBytes = 15360
	}
	result := utils.ExecShell(fmt.Sprintf(
		"tc qdisc add dev %s root handle 1: htb default 1", utils.ShellSingleQuote(gwPort)))
	if result.Error != nil {
		logger.App.Warn("添加VPC网关tc qdisc失败", "port", gwPort, "stderr", result.Stderr)
		return
	}
	result = utils.ExecShell(fmt.Sprintf(
		"tc class add dev %s parent 1: classid 1:1 htb rate %dkbit ceil %dkbit burst %d",
		utils.ShellSingleQuote(gwPort), rateKbit, rateKbit, burstBytes))
	if result.Error != nil {
		logger.App.Warn("添加VPC网关tc class失败", "port", gwPort, "stderr", result.Stderr)
	}
}

// ClearTCVPCSwitchDownlinkLimit 清除 VPC 网关口上的下行 tc 限速
func ClearTCVPCSwitchDownlinkLimit(gwPort string) {
	if gwPort == "" {
		return
	}
	utils.ExecShellQuiet(fmt.Sprintf("tc qdisc del dev %s root 2>/dev/null", utils.ShellSingleQuote(gwPort)))
}

// applyTCDownloadLimit 使用 tc 命令在 vnet 接口的 egress 方向设置下行限速
// 解决 virsh domiftune inbound 限速不生效的 libvirt 已知问题
// 从宿主机 vnet 接口角度：egress(发出) = 数据发往VM = VM的下行
// 注意：tc HTB 的 burst 是 token bucket 大小，不等于 virsh domiftune 的 burst（突发时长）
// 这里用 rate=ceil 做硬限制，突发行为由 TC 自己控制
func applyTCDownloadLimit(vnetIF string, avgKBps, peakKBps, burstKB int) {
	if vnetIF == "" {
		return
	}

	// 先清除已有的 tc qdisc
	utils.ExecShellQuiet(fmt.Sprintf("tc qdisc del dev %s root 2>/dev/null", utils.ShellSingleQuote(vnetIF)))

	if avgKBps <= 0 {
		return // 不限制，清除即可
	}

	// rate = average（硬限制速率）
	rateKbit := TcRateKbit(avgKBps)
	burstBytes := TcBurstBytes(avgKBps)

	// 创建根 qdisc
	result := utils.ExecShell(fmt.Sprintf(
		"tc qdisc add dev %s root handle 1: htb default 1", utils.ShellSingleQuote(vnetIF)))
	if result.Error != nil {
		logger.App.Warn("添加tc qdisc失败", "iface", vnetIF, "stderr", result.Stderr)
		return
	}

	// 创建限速 class，rate=ceil 做硬限制
	result = utils.ExecShell(fmt.Sprintf(
		"tc class add dev %s parent 1: classid 1:1 htb rate %dkbit ceil %dkbit burst %d",
		utils.ShellSingleQuote(vnetIF), rateKbit, rateKbit, burstBytes))
	if result.Error != nil {
		logger.App.Warn("添加tc class失败", "iface", vnetIF, "stderr", result.Stderr)
	}
}

func clearTCUploadLimit(vnetIF string) {
	if vnetIF == "" {
		return
	}
	ifbIF := TcUploadIFBName(vnetIF)
	utils.ExecShellQuiet(fmt.Sprintf("tc qdisc del dev %s ingress 2>/dev/null", utils.ShellSingleQuote(vnetIF)))
	if ifbIF != "" {
		utils.ExecShellQuiet(fmt.Sprintf("tc qdisc del dev %s root 2>/dev/null", utils.ShellSingleQuote(ifbIF)))
		utils.ExecShell(fmt.Sprintf("ip link set %s down 2>/dev/null || true", utils.ShellSingleQuote(ifbIF)))
		utils.ExecShell(fmt.Sprintf("ip link del %s 2>/dev/null || true", utils.ShellSingleQuote(ifbIF)))
	}
}

// applyTCUploadLimit 使用 IFB + HTB 在 vnet 入口方向设置上行整形。
// 从宿主机 vnet 接口角度：ingress(进入) = VM 发出的包 = VM 的上行。
// 相比 ingress police 直接丢包，IFB 队列整形能显著降低单 TCP 流的重传。
func applyTCUploadLimit(vnetIF string, avgKBps int) {
	if vnetIF == "" {
		return
	}

	clearTCUploadLimit(vnetIF)

	if avgKBps <= 0 {
		return
	}

	rateKbit := TcRateKbit(avgKBps)
	burstBytes := TcBurstBytes(avgKBps)
	ifbIF := TcUploadIFBName(vnetIF)
	if ifbIF == "" {
		return
	}

	utils.ExecShell("modprobe ifb 2>/dev/null || true")
	result := utils.ExecShell(fmt.Sprintf("ip link show %s >/dev/null 2>&1 || ip link add %s type ifb",
		utils.ShellSingleQuote(ifbIF), utils.ShellSingleQuote(ifbIF)))
	if result.Error != nil {
		logger.App.Warn("创建IFB上行整形接口失败", "iface", ifbIF, "stderr", result.Stderr)
		return
	}
	result = utils.ExecShell(fmt.Sprintf("ip link set %s up", utils.ShellSingleQuote(ifbIF)))
	if result.Error != nil {
		logger.App.Warn("启用IFB上行整形接口失败", "iface", ifbIF, "stderr", result.Stderr)
		return
	}
	result = utils.ExecShell(fmt.Sprintf("ip link set dev %s txqueuelen %d", utils.ShellSingleQuote(ifbIF), TcIFBTxQueueLen()))
	if result.Error != nil {
		logger.App.Warn("调整IFB上行队列长度失败", "iface", ifbIF, "stderr", result.Stderr)
	}
	result = utils.ExecShell(fmt.Sprintf(
		"tc qdisc add dev %s root handle 1: htb default 1", utils.ShellSingleQuote(ifbIF)))
	if result.Error != nil {
		logger.App.Warn("添加IFB上行qdisc失败", "iface", ifbIF, "stderr", result.Stderr)
		return
	}
	result = utils.ExecShell(fmt.Sprintf(
		"tc class add dev %s parent 1: classid 1:1 htb rate %dkbit ceil %dkbit burst %d",
		utils.ShellSingleQuote(ifbIF), rateKbit, rateKbit, burstBytes))
	if result.Error != nil {
		logger.App.Warn("添加IFB上行class失败", "iface", ifbIF, "stderr", result.Stderr)
		return
	}
	result = utils.ExecShell(fmt.Sprintf(
		"tc qdisc add dev %s parent 1:1 handle 10: fq_codel limit 100 target 20ms interval 100ms",
		utils.ShellSingleQuote(ifbIF)))
	if result.Error != nil {
		logger.App.Warn("添加IFB上行fq_codel队列失败", "iface", ifbIF, "stderr", result.Stderr)
	}

	result = utils.ExecShell(fmt.Sprintf(
		"tc qdisc add dev %s ingress", utils.ShellSingleQuote(vnetIF)))
	if result.Error != nil {
		logger.App.Warn("添加tc ingress qdisc失败", "iface", vnetIF, "stderr", result.Stderr)
		return
	}

	result = utils.ExecShell(fmt.Sprintf(
		"tc filter add dev %s parent ffff: protocol all prio 1 matchall action mirred egress redirect dev %s",
		utils.ShellSingleQuote(vnetIF), utils.ShellSingleQuote(ifbIF)))
	if result.Error != nil {
		result = utils.ExecShell(fmt.Sprintf(
			"tc filter add dev %s parent ffff: protocol all prio 1 u32 match u32 0 0 action mirred egress redirect dev %s",
			utils.ShellSingleQuote(vnetIF), utils.ShellSingleQuote(ifbIF)))
		if result.Error != nil {
			logger.App.Warn("添加tc上行IFB重定向规则失败", "vnetIF", vnetIF, "ifbIF", ifbIF, "stderr", result.Stderr)
		}
	}
}

// ApplyVMBandwidth 设置单台 VM 的网卡速率限制
// virsh domiftune 方向（从域/VM 视角）：
//
//	inbound  = 流量进入 VM = VM 的下行
//	outbound = 流量离开 VM = VM 的上行
//
// 参数说明：
//
//	downAvg/downPeak/downBurst → 对应 inbound（VM 下行）
//	upAvg/upPeak/upBurst     → 对应 outbound（VM 上行）
//
// 单位：average/peak 为 KB/s，burst 为 KB
// 所有值为 0 时清除限制
func ApplyVMBandwidth(vmName string, downAvg, downPeak, downBurst, upAvg, upPeak, upBurst int) error {
	interfaces := listVMBandwidthInterfaces(vmName)
	if len(interfaces) == 0 {
		return fmt.Errorf("无法获取虚拟机 %s 的网卡 MAC 地址", vmName)
	}

	// 构建 inbound/outbound TypedParam 列表
	configParams := BuildBandwidthParams(downAvg, downPeak, downBurst, upAvg, upPeak, upBurst)

	// 应用到 config（持久化）
	for _, iface := range interfaces {
		if err := libvirt_rpc.SetInterfaceParametersRPC(vmName, iface.MAC, configParams, uint32(libvirt.DomainAffectConfig)); err != nil {
			return fmt.Errorf("设置第 %d 张网卡速率限制失败(config): %w", iface.Order+1, err)
		}
	}

	// 如果 VM 正在运行，同时应用到 live
	state, _ := libvirt_rpc.GetDomainStateRPC(vmName)
	if state == "running" {
		if HookUseOVSNetwork() {
			// OVS 运行态由 queue/meter 接管；保留 live domiftune 会把上传变成端口 policing，导致低速率时抖动明显。
			zeroParams := BuildBandwidthParams(0, 0, 0, 0, 0, 0)
			clearOVSBandwidthLimit(vmName, "")
			for _, iface := range interfaces {
				if err := libvirt_rpc.SetInterfaceParametersRPC(vmName, iface.MAC, zeroParams, uint32(libvirt.DomainAffectLive)); err != nil {
					logger.App.Warn("清理实时domiftune速率限制失败", "vm", vmName, "order", iface.Order, "error", err)
				}
				if iface.Name != "" {
					if err := applyOVSBandwidthLimit(vmName, iface, downAvg, upAvg); err != nil {
						return err
					}
				}
			}
		} else {
			for _, iface := range interfaces {
				if err := libvirt_rpc.SetInterfaceParametersRPC(vmName, iface.MAC, configParams, uint32(libvirt.DomainAffectLive)); err != nil {
					logger.App.Warn("实时应用速率限制失败", "vm", vmName, "order", iface.Order, "error", err)
				}
				if iface.Name != "" {
					// 非 OVS 环境保留旧的下行 tc 兜底。
					applyTCDownloadLimit(iface.Name, downAvg, downPeak, downBurst)
				}
			}
		}
	}

	return nil
}

// ClearVMBandwidth 清除 VM 的速率限制
func ClearVMBandwidth(vmName string) error {
	clearOVSBandwidthLimit(vmName, "")
	for _, iface := range listVMBandwidthInterfaces(vmName) {
		if iface.Name != "" {
			clearTCBandwidthLimit(iface.Name)
		}
	}
	return ApplyVMBandwidth(vmName, 0, 0, 0, 0, 0, 0)
}
