package ip_resolver

import (
	"fmt"
	"strings"

	"kvm_console/service/libvirt_rpc"
	"kvm_console/utils"
)

// GetFirstVMMAC 获取 VM 的第一个网卡 MAC 地址（优先 RPC，降级 shell）
func GetFirstVMMAC(vmName string) string {
	// 优先通过 RPC 获取（更准确、不依赖 shell）
	if libvirt_rpc.IsLibvirtRPCAvailable() {
		xmlStr, err := libvirt_rpc.GetDomainXMLRPC(vmName, 0)
		if err == nil {
			ifaces := libvirt_rpc.ParseInterfacesFromDomainXML(xmlStr)
			if len(ifaces) > 0 {
				return strings.ToLower(ifaces[0].MAC)
			}
		}
	}
	// 降级为 shell 命令
	macResult := utils.ExecShell(fmt.Sprintf(
		"virsh domiflist %s 2>/dev/null | grep -oP '([0-9a-f]{2}:){5}[0-9a-f]{2}' | head -1",
		utils.ShellSingleQuote(vmName)))
	if macResult.Error != nil {
		return ""
	}
	return strings.TrimSpace(macResult.Stdout)
}

// GetVMVnetIF 获取运行中 VM 的第一个 vnet 接口名（优先 RPC，降级 shell）
func GetVMVnetIF(vmName string) string {
	// 优先通过 RPC 获取
	if libvirt_rpc.IsLibvirtRPCAvailable() {
		xmlStr, err := libvirt_rpc.GetDomainXMLRPC(vmName, 0)
		if err == nil {
			ifaces := libvirt_rpc.ParseInterfacesFromDomainXML(xmlStr)
			for _, iface := range ifaces {
				if strings.HasPrefix(iface.Target, "vnet") {
					return iface.Target
				}
			}
		}
	}
	// 降级为 shell 命令
	result := utils.ExecShell(fmt.Sprintf(
		"virsh domiflist %s 2>/dev/null | awk 'NR>2 && $1 ~ /^vnet/ {print $1; exit}'",
		utils.ShellSingleQuote(vmName)))
	if result.Error != nil || strings.TrimSpace(result.Stdout) == "" || strings.TrimSpace(result.Stdout) == "-" {
		return ""
	}
	return strings.TrimSpace(result.Stdout)
}

// GetVMBridgeInterface 获取虚拟机第一个网桥类型接口的桥名称
func GetVMBridgeInterface(vmName string) string {
	result := utils.ExecCommand("virsh", "domiflist", vmName)
	if result.Error != nil {
		return ""
	}
	for _, line := range strings.Split(result.Stdout, "\n") {
		fields := strings.Fields(line)
		if len(fields) >= 3 && fields[1] == "bridge" && fields[2] != "Source" {
			return fields[2]
		}
	}
	return ""
}

// HasVMBridgeVLANTag 检测虚拟机桥接接口是否带有 OVS VLAN tag
func HasVMBridgeVLANTag(vmName string) bool {
	vnet := GetVMVnetIF(vmName)
	if vnet == "" {
		return false
	}
	result := utils.ExecShell(fmt.Sprintf(
		"ovs-vsctl list port %s 2>/dev/null | awk '$1==\"tag\" {print $3}'",
		utils.ShellSingleQuote(vnet)))
	tag := strings.TrimSpace(result.Stdout)
	if tag == "" || tag == "[]" {
		return false
	}
	return true
}
