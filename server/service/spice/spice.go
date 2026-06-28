package spice

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/digitalocean/go-libvirt"

	"kvm_console/logger"
	"kvm_console/service/libvirt_rpc"
	"kvm_console/service/vm_xml"
)

// spice.go — SPICE 显示协议运行时管理（与 service/vnc/vnc.go 同构，差异：
//   - 状态/连接用 QEMU monitor "info spice"
//   - 改密用 "set_password spice"
//   - 改密无 VNC 的 8 位截断
//   - 暴露需要额外开宿主防火墙端口（见 expose.go））

// GetSpiceStatus 获取 SPICE 状态
func GetSpiceStatus(vmName string) (*SpiceInfo, error) {
	info := &SpiceInfo{}

	vmState, err := libvirt_rpc.GetDomainStateRPC(vmName)
	if err != nil {
		return nil, fmt.Errorf("虚拟机不存在: %s", vmName)
	}

	if vmState != "running" && vmState != "paused" {
		// 从 inactive XML 检查是否配置了 SPICE 及对外暴露状态（关机态无运行端口，按配置判断）
		xmlStr, xmlErr := libvirt_rpc.GetDomainXMLRPC(vmName, libvirt.DomainXMLInactive)
		if xmlErr == nil {
			info.Enabled = HasSPICEGraphics(xmlStr)
			if strings.Contains(xmlStr, "listen='0.0.0.0'") {
				info.Exposed = true
			}
		}
		return info, nil
	}

	// 运行中：通过 QEMU Monitor "info spice" 获取详情
	spiceOutput, spiceErr := libvirt_rpc.QemuMonitorCommandRPC(vmName, "info spice", libvirt_rpc.DomainQemuMonitorCommandHmp)
	if spiceErr == nil {
		info.Enabled = strings.Contains(strings.ToLower(spiceOutput), "address:") || strings.Contains(strings.ToLower(spiceOutput), "channels:")
		for _, line := range strings.Split(spiceOutput, "\n") {
			line = strings.TrimSpace(line)
			// spice 输出形如: "Address: 127.0.0.1:5910 (ipv4)"
			if strings.HasPrefix(strings.ToLower(line), "address:") {
				fields := strings.Fields(line)
				if len(fields) >= 2 {
					addr := fields[1]
					if idx := strings.LastIndex(addr, ":"); idx >= 0 {
						info.Port = addr[idx+1:]
					}
				}
				if strings.Contains(line, "0.0.0.0") {
					info.Exposed = true
				}
			}
		}
	} else {
		// 运行中但 monitor 不可用，回退查 XML
		xmlStr, xmlErr := libvirt_rpc.GetDomainXMLRPC(vmName, libvirt.DomainXMLInactive)
		if xmlErr == nil {
			info.Enabled = HasSPICEGraphics(xmlStr)
			if strings.Contains(xmlStr, "listen='0.0.0.0'") {
				info.Exposed = true
			}
		}
	}

	// 是否设置密码：从 XML graphics 块判断
	xmlStr, xmlErr := libvirt_rpc.GetDomainXMLRPC(vmName, libvirt.DomainXMLInactive)
	if xmlErr == nil {
		if block := spiceGraphicsBlockRe.FindString(xmlStr); block != "" {
			info.Password = extractSPICEPasswd(block) != ""
		}
	}

	return info, nil
}

// getVMXMLForSpice 获取 VM XML（运行中取活跃配置，否则取 inactive 配置）
func getVMXMLForSpice(vmName, vmState string) (string, error) {
	if vmState == "running" {
		return libvirt_rpc.GetDomainXMLRPC(vmName, 0)
	}
	return libvirt_rpc.GetDomainXMLRPC(vmName, libvirt.DomainXMLInactive)
}

// applySpiceXMLAndRestart 应用修改后的 XML 配置，若 VM 原本运行则先 destroy 再 define 再启动。
// 与 vnc.applyVncXMLAndRestart 同构。
func applySpiceXMLAndRestart(vmName, vmState, newXML string) error {
	if vmState == "running" {
		if err := libvirt_rpc.DestroyDomainRPC(vmName); err != nil {
			logger.Libvirt.Warn("SPICE配置: 关闭虚拟机失败(可能已关闭)", "error", err)
		}
		if _, err := libvirt_rpc.DefineDomainXMLRPC(newXML); err != nil {
			return fmt.Errorf("SPICE配置: 应用XML定义失败: %w", err)
		}
		if err := HookStartVM(vmName); err != nil {
			return err
		}
	} else {
		if _, err := libvirt_rpc.DefineDomainXMLRPC(newXML); err != nil {
			return fmt.Errorf("SPICE配置: 应用XML定义失败: %w", err)
		}
	}
	return nil
}

// EnableSpice 开启 SPICE（默认 TCP 本地模式，绑定 127.0.0.1）。
// 与 VNC 共存：仅注入/替换 spice graphics 块，不动 vnc。
func EnableSpice(vmName, password string) error {
	vmState, err := libvirt_rpc.GetDomainStateRPC(vmName)
	if err != nil {
		return fmt.Errorf("虚拟机不存在: %s", vmName)
	}

	xmlStr, err := getVMXMLForSpice(vmName, vmState)
	if err != nil {
		return fmt.Errorf("获取虚拟机 XML 失败: %w", err)
	}

	// 注入 SPICE graphics（默认本地监听），并确保 video 为 qxl
	xmlStr = InjectSPICEGraphicsToDomainXML(xmlStr, password, "127.0.0.1")
	xmlStr = EnsureQXLVideo(xmlStr)

	// 保留已有视频模型探测，避免覆盖用户显式配置
	detectedOSType := HookDetectVMOSType("", xmlStr)
	xmlStr = vm_xml.ApplyVMVideoModelToDomainXML(xmlStr, vm_xml.ParseVMVideoModelFromDomainXML(xmlStr), detectedOSType)

	return applySpiceXMLAndRestart(vmName, vmState, xmlStr)
}

// DisableSpice 关闭 SPICE（移除 spice graphics 块）。不删除 video，避免影响 VNC。
func DisableSpice(vmName string) error {
	vmState, err := libvirt_rpc.GetDomainStateRPC(vmName)
	if err != nil {
		return fmt.Errorf("虚拟机不存在: %s", vmName)
	}

	xmlStr, err := getVMXMLForSpice(vmName, vmState)
	if err != nil {
		return fmt.Errorf("获取虚拟机 XML 失败: %w", err)
	}

	if !HasSPICEGraphics(xmlStr) {
		return nil // 本来就没有
	}

	// 若当前对外暴露，先收回防火墙规则（按运行态端口）
	if strings.Contains(xmlStr, "listen='0.0.0.0'") {
		if port, pErr := detectSpicePort(vmName); pErr == nil && port != "" {
			_ = revokeSpiceFirewall(port)
		}
	}
	xmlStr = RemoveSPICEGraphicsFromDomainXML(xmlStr)

	return applySpiceXMLAndRestart(vmName, vmState, xmlStr)
}

// ChangeSpicePassword 修改 SPICE 密码（热修改，无需重启）。无 VNC 的 8 位截断。
func ChangeSpicePassword(vmName, newPassword string) error {
	vmState, err := libvirt_rpc.GetDomainStateRPC(vmName)
	if err != nil {
		return fmt.Errorf("虚拟机不存在: %s", vmName)
	}
	if vmState != "running" {
		return fmt.Errorf("虚拟机未运行，无法热修改密码")
	}

	// 热修改密码
	if _, err := libvirt_rpc.QemuMonitorCommandRPC(vmName, fmt.Sprintf("set_password spice %s", newPassword), libvirt_rpc.DomainQemuMonitorCommandHmp); err != nil {
		return fmt.Errorf("修改密码失败: %w", err)
	}

	// 同步持久化到 XML 定义
	xmlStr, xmlErr := libvirt_rpc.GetDomainXMLRPC(vmName, 0)
	if xmlErr == nil {
		block := spiceGraphicsBlockRe.FindString(xmlStr)
		if block != "" {
			newBlock := buildSpiceGraphicsXML(extractListenAddr(block), newPassword)
			updatedXML := spiceGraphicsBlockRe.ReplaceAllString(xmlStr, newBlock)
			if _, defineErr := libvirt_rpc.DefineDomainXMLRPC(updatedXML); defineErr != nil {
				logger.Libvirt.Warn("SPICE配置: 持久化密码到XML定义失败", "error", defineErr)
			}
		}
	}
	return nil
}

// spiceListenExtractRe 从 spice graphics 块提取 listen='...' 地址
var spiceListenExtractRe = regexp.MustCompile(`listen='([^']*)'`)

// extractListenAddr 从 spice graphics 块提取 listen 地址（回退 127.0.0.1）。
func extractListenAddr(block string) string {
	m := spiceListenExtractRe.FindStringSubmatch(block)
	if len(m) >= 2 && m[1] != "" {
		return m[1]
	}
	return "127.0.0.1"
}
