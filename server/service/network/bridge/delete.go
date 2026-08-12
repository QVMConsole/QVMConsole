package bridge

import (
	"fmt"
	"os"
	"strings"

	"kvm_console/logger"
	"kvm_console/model"
	ovspkg "kvm_console/service/ovs"
	"kvm_console/utils"
)

func DeleteNetworkBridge(id uint) error {
	if id == 0 {
		return fmt.Errorf("默认网桥不能删除")
	}
	var row model.NetworkBridge
	if err := model.DB.First(&row, id).Error; err != nil {
		return fmt.Errorf("网桥不存在")
	}
	return deleteBridgeRecord(&row)
}

// DeleteNetworkBridgeByName 通过网桥名称删除（用于处理 OVS 中残留但数据库无记录的网桥）。
func DeleteNetworkBridgeByName(name string) error {
	name = strings.TrimSpace(name)
	if name == "" {
		return fmt.Errorf("网桥名称不能为空")
	}
	// 默认 NAT 网桥不允许删除
	if name == ovspkg.OvsBridgeName() {
		return fmt.Errorf("默认网桥不能删除")
	}
	// 检查是否有关联的交换机
	var count int64
	if model.DB != nil {
		model.DB.Model(&model.VPCSwitch{}).Where("bridge_name = ?", name).Count(&count)
	}
	if count > 0 {
		return fmt.Errorf("该网桥仍有交换机使用，不能删除")
	}
	// 查找数据库记录
	var row model.NetworkBridge
	dbErr := model.DB.Where("name = ?", name).First(&row).Error
	if dbErr == nil {
		return deleteBridgeRecord(&row)
	}
	// 数据库无记录，但 OVS 中存在该网桥 — 直接执行物理清理
	if !ovsBridgeExists(name) {
		return fmt.Errorf("网桥不存在")
	}
	_ = os.Remove(bridgeRestoreScriptPath(name))
	// 获取 uplink 用于清理
	uplink := DetectOVSBridgePhysicalUplink(name)
	if uplink != "" {
		// 尝试迁移 IP 回物理口
		migrateBridgeIPv4ToInterface(name, uplink)
		utils.ExecCommand("ovs-vsctl", "--if-exists", "del-port", name, uplink)
		removeNetworkdDHCPOverrideForPort(uplink)
	}
	utils.ExecCommand("ovs-vsctl", "--if-exists", "del-br", name)
	disableBridgeRestoreUnitIfEmpty()
	// 恢复默认网络
	if HookEnsureOVSNetworkReady != nil {
		if err := HookEnsureOVSNetworkReady(); err != nil {
			return fmt.Errorf("网桥已删除，但恢复默认 OVS 网络失败: %w", err)
		}
	}
	if HookEnsureAllVPCSwitchRuntime != nil {
		if err := HookEnsureAllVPCSwitchRuntime(); err != nil {
			return fmt.Errorf("网桥已删除，但恢复 VPC 交换机网络失败: %w", err)
		}
	}
	return nil
}

func deleteBridgeRecord(row *model.NetworkBridge) error {
	var count int64
	model.DB.Model(&model.VPCSwitch{}).Where("bridge_name = ?", row.Name).Count(&count)
	if count > 0 {
		return fmt.Errorf("该网桥仍有交换机使用，不能删除")
	}
	_ = os.Remove(bridgeRestoreScriptPath(row.Name))
	// 仅当 OVS 桥实际存在时执行物理清理；桥已不存在则跳过直接清理记录
	if ovsBridgeExists(row.Name) {
		if row.MigrateHostIP {
			migrateBridgeIPv4ToInterface(row.Name, row.UplinkIF)
		}
		utils.ExecCommand("ovs-vsctl", "--if-exists", "del-port", row.Name, row.UplinkIF)
		utils.ExecCommand("ovs-vsctl", "--if-exists", "del-br", row.Name)
	}
	disableBridgeRestoreUnitIfEmpty()
	// 先恢复 OVS 默认网络（此时 IP/路由已迁回物理口，可正常检测 uplink）
	if HookEnsureOVSNetworkReady != nil {
		if err := HookEnsureOVSNetworkReady(); err != nil {
			return fmt.Errorf("网桥已删除，但恢复默认 OVS 网络失败: %w", err)
		}
	}
	if HookEnsureAllVPCSwitchRuntime != nil {
		if err := HookEnsureAllVPCSwitchRuntime(); err != nil {
			return fmt.Errorf("网桥已删除，但恢复 VPC 交换机网络失败: %w", err)
		}
	}
	// 最后恢复 networkd DHCP（networkctl reload 可能短暂影响路由，必须在 EnsureOVSNetworkReady 之后）
	removeNetworkdDHCPOverrideForPort(row.UplinkIF)
	if err := model.DB.Delete(&row).Error; err != nil {
		return err
	}
	return nil
}

func EnsureAllNetworkBridgesRuntime() error {
	if model.DB == nil {
		return nil
	}
	var rows []model.NetworkBridge
	if err := model.DB.Where("mode = ?", BridgeModeDirect).Find(&rows).Error; err != nil {
		return err
	}
	var lastErr error
	for _, row := range rows {
		cfg := HostIPConfig{
			Addrs:   row.HostAddrs,
			Gateway: row.HostGateway,
			Metric:  row.HostMetric,
			DNS:     row.HostDNS,
		}
		if err := EnsureOVSBridgeDirect(row.Name, row.UplinkIF, row.MigrateHostIP, cfg); err != nil {
			lastErr = err
			logger.App.Warn("恢复桥接网桥失败", "bridge", row.Name, "error", err)
		}
		// 兼容旧记录：DNS 未持久化时，从当前网桥状态捕获并更新数据库
		if row.MigrateHostIP && strings.TrimSpace(row.HostDNS) == "" {
			dns := captureInterfaceDNSServers(row.Name)
			if dns != "" && model.DB != nil {
				if err := model.DB.Model(&row).Update("host_dns", dns).Error; err != nil {
					logger.App.Warn("更新网桥 DNS 记录失败", "bridge", row.Name, "error", err)
				}
			}
		}
	}
	return lastErr
}

// DeleteOwnedVPCSwitchBridge 清理由 VPC 交换机独占管理的 OVS 网桥。
func DeleteOwnedVPCSwitchBridge(name, uplink string, migrateHostIP bool, hostDNS string) error {
	name = strings.TrimSpace(name)
	uplink = strings.TrimSpace(uplink)
	if name == "" || name == ovspkg.OvsBridgeName() {
		return nil
	}
	_ = os.Remove(bridgeRestoreScriptPath(name))
	if ovsBridgeExists(name) {
		if migrateHostIP && uplink != "" {
			migrateBridgeIPv4ToInterface(name, uplink)
			restoreInterfaceResolvedDNS(uplink, name, hostDNS)
		}
		if uplink != "" {
			utils.ExecCommand("ovs-vsctl", "--if-exists", "del-port", name, uplink)
		}
		utils.ExecCommand("ovs-vsctl", "--if-exists", "del-br", name)
	}
	if uplink != "" {
		removeNetworkdDHCPOverrideForPort(uplink)
	}
	disableBridgeRestoreUnitIfEmpty()
	return nil
}

func defaultNetworkBridgeRecords() []NetworkBridgeInfo {
	return []NetworkBridgeInfo{{
		Name:      ovspkg.OvsBridgeName(),
		Mode:      BridgeModeNAT,
		IsDefault: true,
	}}
}
