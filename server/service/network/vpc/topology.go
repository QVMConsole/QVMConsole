package vpc

import (
	"fmt"
	"strings"

	"kvm_console/model"
)

// SwitchUsesManagedDHCP 表示交换机由面板提供网关、DHCP 与 NAT。
func SwitchUsesManagedDHCP(sw model.VPCSwitch) bool {
	return sw.IsSystem || sw.DHCPEnabled
}

// SwitchIsTrustedIsolated 表示交换机没有外部上行，作为软路由 LAN 使用的纯二层信任网络。
func SwitchIsTrustedIsolated(sw model.VPCSwitch) bool {
	return !SwitchUsesManagedDHCP(sw) && strings.TrimSpace(sw.UplinkIF) == "" && sw.OwnsBridge
}

func managedVPCBridgeName(vlanID int) string {
	return fmt.Sprintf("qvsw%d", vlanID)
}

func normalizeUplinkMode(value, uplink string, managed bool) string {
	value = strings.ToLower(strings.TrimSpace(value))
	uplink = strings.TrimSpace(uplink)
	if value == UplinkModeSystem {
		return UplinkModeSystem
	}
	if uplink != "" {
		return UplinkModePhysical
	}
	if managed {
		return UplinkModeSystem
	}
	return UplinkModeNone
}

func normalizeCreateTopology(role string, req *VPCSwitchRequest) {
	if req == nil {
		return
	}
	req.UplinkIF = strings.TrimSpace(req.UplinkIF)
	req.UplinkGateway = strings.TrimSpace(req.UplinkGateway)
	if role != "admin" {
		req.DHCPEnabled = false
		req.UplinkIF = ""
		req.UplinkGateway = ""
		req.UplinkMode = UplinkModeNone
		req.MigrateHostIP = false
		return
	}
	req.UplinkMode = normalizeUplinkMode(req.UplinkMode, req.UplinkIF, req.DHCPEnabled)
}

func topologyRequestChanged(sw model.VPCSwitch, req VPCSwitchRequest) bool {
	if strings.TrimSpace(req.UplinkMode) == "" && strings.TrimSpace(req.UplinkIF) == "" {
		return false
	}
	return sw.DHCPEnabled != req.DHCPEnabled ||
		strings.TrimSpace(sw.UplinkIF) != strings.TrimSpace(req.UplinkIF) ||
		strings.TrimSpace(sw.UplinkGateway) != strings.TrimSpace(req.UplinkGateway) ||
		normalizeUplinkMode(sw.UplinkMode, sw.UplinkIF, sw.DHCPEnabled) != normalizeUplinkMode(req.UplinkMode, req.UplinkIF, req.DHCPEnabled) ||
		sw.MigrateHostIP != req.MigrateHostIP ||
		sw.BridgeVLANID != req.BridgeVLANID
}

// validateHostIPMigrationSelection 防止把仍承载宿主机地址或默认路由的物理口直接加入二层网桥。
func validateHostIPMigrationSelection(req VPCSwitchRequest) error {
	if req.DHCPEnabled || strings.TrimSpace(req.UplinkIF) == "" || req.MigrateHostIP || HookCaptureHostIPConfig == nil {
		return nil
	}
	effective := strings.TrimSpace(req.UplinkIF)
	if HookEffectiveL3Interface != nil {
		if value := strings.TrimSpace(HookEffectiveL3Interface(req.UplinkIF)); value != "" {
			effective = value
		}
	}
	addrs, gateway, _, _ := HookCaptureHostIPConfig(effective)
	if strings.TrimSpace(addrs) != "" || strings.TrimSpace(gateway) != "" {
		return fmt.Errorf("物理网卡 %s 当前承载宿主机 IPv4 地址或默认路由，请开启宿主机 IP 迁移", req.UplinkIF)
	}
	return nil
}
