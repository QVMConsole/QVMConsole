package bridge

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"kvm_console/model"
	"kvm_console/utils"
)

func ListHostPhysicalInterfaces() ([]HostInterfaceInfo, error) {
	items, err := readIPAddrJSON()
	if err != nil {
		return nil, err
	}
	defaults := readDefaultRouteIfaces()
	ovsPorts := readOVSPortBridgeMap()
	managed := readManagedBridgeByUplink()
	var result []HostInterfaceInfo
	for _, item := range items {
		if item.IfName == "" {
			continue
		}
		info := HostInterfaceInfo{
			Name:         item.IfName,
			MAC:          item.Address,
			State:        item.OperState,
			MTU:          item.MTU,
			DefaultRoute: defaults[item.IfName],
			Physical:     isPhysicalInterface(item.IfName),
		}
		for _, addr := range item.AddrInfo {
			if addr.Local != "" {
				info.Addresses = append(info.Addresses, fmt.Sprintf("%s/%d", addr.Local, addr.PrefixLen))
			}
		}
		if bridge := ovsPorts[item.IfName]; bridge != "" {
			info.OVSPort = true
			info.OVSBridge = bridge
		}
		info.ManagedBridge = managed[item.IfName]
		if info.DefaultRoute {
			info.Risk = "承载默认路由，桥接时可能短暂中断宿主机网络"
		}
		if info.Physical {
			result = append(result, info)
		}
	}
	sort.Slice(result, func(i, j int) bool { return result[i].Name < result[j].Name })
	return result, nil
}

func ListNetworkBridges() ([]NetworkBridgeInfo, error) {
	bridges := defaultNetworkBridgeRecords()
	// 从数据库加载已管理的网桥记录
	if model.DB != nil {
		var rows []model.NetworkBridge
		if err := model.DB.Order("is_default DESC, name ASC").Find(&rows).Error; err != nil {
			return nil, err
		}
		seen := map[string]bool{}
		for _, item := range bridges {
			seen[item.Name] = true
		}
		for _, row := range rows {
			if seen[row.Name] {
				continue
			}
			bridges = append(bridges, NetworkBridgeInfo{
				ID:            row.ID,
				Name:          row.Name,
				Mode:          NormalizeBridgeMode(row.Mode),
				UplinkIF:      row.UplinkIF,
				MigrateHostIP: row.MigrateHostIP,
				IsDefault:     row.IsDefault,
				HostAddrs:     row.HostAddrs,
				HostGateway:   row.HostGateway,
				HostDNS:       row.HostDNS,
			})
		}
	}
	// 从 OVS 系统层发现数据库中未记录的网桥（如数据库被删除后残留的 OVS 网桥）
	// 仅纳入有物理网卡上联的网桥，跳过设备已不存在的残留网桥
	ovsBridges := readOVSBridgeList()
	seen := map[string]bool{}
	for _, b := range bridges {
		seen[b.Name] = true
	}
	for _, ovsBr := range ovsBridges {
		if seen[ovsBr] {
			continue
		}
		uplink := DetectOVSBridgePhysicalUplink(ovsBr)
		if uplink == "" {
			// 跳过没有物理网卡上联的残留网桥（端口设备已不存在）
			continue
		}
		bridges = append(bridges, NetworkBridgeInfo{
			Name:     ovsBr,
			Mode:     BridgeModeDirect,
			UplinkIF: uplink,
		})
	}
	for i := range bridges {
		bridges[i].Exists = ovsBridgeExists(bridges[i].Name)
		bridges[i].Active = linkIsUp(bridges[i].Name)
		if model.DB != nil {
			if bridges[i].IsDefault {
				model.DB.Model(&model.VPCSwitch{}).Where("bridge_name = ? OR bridge_name = '' OR bridge_name IS NULL", bridges[i].Name).Count(&bridges[i].SwitchCount)
			} else {
				model.DB.Model(&model.VPCSwitch{}).Where("bridge_name = ?", bridges[i].Name).Count(&bridges[i].SwitchCount)
			}
		}
	}
	return bridges, nil
}

// readOVSBridgeList returns all OVS bridge names on the system.
func readOVSBridgeList() []string {
	result := utils.ExecCommand("ovs-vsctl", "list-br")
	if result.Error != nil {
		return nil
	}
	return strings.Fields(strings.TrimSpace(result.Stdout))
}

// DetectOVSBridgePhysicalUplink finds the physical NIC attached to the given OVS bridge.
// It reverses readOVSPortBridgeMap to find the first physical port on the bridge.
func DetectOVSBridgePhysicalUplink(bridge string) string {
	ports := readOVSPortBridgeMap()
	for port, br := range ports {
		if br == bridge && isPhysicalInterface(port) {
			return port
		}
	}
	return ""
}

func readIPAddrJSON() ([]ipAddrJSON, error) {
	result := utils.ExecCommand("ip", "-j", "addr")
	if result.Error != nil {
		return nil, fmt.Errorf("读取宿主机网卡失败: %s", result.Stderr)
	}
	var items []ipAddrJSON
	if err := json.Unmarshal([]byte(result.Stdout), &items); err != nil {
		return nil, fmt.Errorf("解析宿主机网卡失败: %w", err)
	}
	return items, nil
}

func readDefaultRouteIfaces() map[string]bool {
	result := utils.ExecCommand("ip", "-j", "route", "show", "default")
	var routes []ipRouteJSON
	_ = json.Unmarshal([]byte(result.Stdout), &routes)
	out := map[string]bool{}
	for _, route := range routes {
		if route.Dev != "" {
			out[route.Dev] = true
		}
	}
	return out
}

func readOVSPortBridgeMap() map[string]string {
	result := utils.ExecCommand("ovs-vsctl", "--format=json", "--columns=name,ports", "list", "Bridge")
	if result.Error != nil {
		return map[string]string{}
	}
	bridges := strings.Fields(strings.TrimSpace(utils.ExecCommand("ovs-vsctl", "list-br").Stdout))
	out := map[string]string{}
	for _, bridge := range bridges {
		ports := strings.Fields(strings.TrimSpace(utils.ExecCommand("ovs-vsctl", "list-ports", bridge).Stdout))
		for _, port := range ports {
			out[port] = bridge
		}
	}
	return out
}

func readManagedBridgeByUplink() map[string]string {
	out := map[string]string{}
	if model.DB == nil {
		return out
	}
	var rows []model.NetworkBridge
	model.DB.Find(&rows)
	for _, row := range rows {
		if row.UplinkIF != "" {
			out[row.UplinkIF] = row.Name
		}
	}
	return out
}

func isPhysicalInterface(name string) bool {
	if name == "lo" || strings.HasPrefix(name, "vnet") || strings.HasPrefix(name, "tap") || strings.HasPrefix(name, "docker") || strings.HasPrefix(name, "br-") || strings.HasPrefix(name, "ovs") {
		return false
	}
	if _, err := os.Stat(filepath.Join("/sys/class/net", name, "device")); err == nil {
		return true
	}
	return false
}
