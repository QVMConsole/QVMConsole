package bridge

import (
	"fmt"
	"net"
	"os"
	"regexp"
	"strings"

	"kvm_console/model"
	ovspkg "kvm_console/service/ovs"
	"kvm_console/utils"
)

func CreateNetworkBridge(req NetworkBridgeRequest) (*model.NetworkBridge, error) {
	req.Name = strings.TrimSpace(req.Name)
	req.Mode = NormalizeBridgeMode(req.Mode)
	req.UplinkIF = strings.TrimSpace(req.UplinkIF)
	if req.Mode != BridgeModeDirect {
		return nil, fmt.Errorf("当前仅允许创建桥接直通网桥")
	}
	if err := validateBridgeName(req.Name); err != nil {
		return nil, err
	}
	if req.Name == ovspkg.OvsBridgeName() {
		return nil, fmt.Errorf("默认 OVS 内网网桥已存在，不能重复创建")
	}
	if req.UplinkIF == "" {
		return nil, fmt.Errorf("请选择物理网卡")
	}
	if err := validateBridgeUplink(req.UplinkIF, req.Name); err != nil {
		return nil, err
	}
	if model.DB != nil {
		var count int64
		model.DB.Model(&model.NetworkBridge{}).Where("name = ?", req.Name).Count(&count)
		if count > 0 {
			return nil, fmt.Errorf("网桥名称已存在")
		}
	}
	// 创建前捕获物理网卡当前 IP 配置（必须在加入 OVS 之前）
	var ipCfg HostIPConfig
	if req.MigrateHostIP {
		ipCfg = CaptureInterfaceIP(req.UplinkIF)
	}
	if err := EnsureOVSBridgeDirect(req.Name, req.UplinkIF, req.MigrateHostIP, ipCfg); err != nil {
		return nil, err
	}
	row := &model.NetworkBridge{
		Name: req.Name, Mode: BridgeModeDirect, UplinkIF: req.UplinkIF,
		MigrateHostIP: req.MigrateHostIP,
		HostAddrs:     ipCfg.Addrs, HostGateway: ipCfg.Gateway, HostMetric: ipCfg.Metric, HostDNS: ipCfg.DNS,
		HostAddrs6:    ipCfg.Addrs6, HostGateway6: ipCfg.Gateway6, HostMetric6: ipCfg.Metric6,
	}
	if model.DB != nil {
		if err := model.DB.Create(row).Error; err != nil {
			return nil, fmt.Errorf("保存网桥配置失败: %w", err)
		}
		if HookEnsureOVSNetworkReady != nil {
			if err := HookEnsureOVSNetworkReady(); err != nil {
				return nil, fmt.Errorf("网桥已创建，但恢复默认 OVS 网络失败: %w", err)
			}
		}
		if HookEnsureAllVPCSwitchRuntime != nil {
			if err := HookEnsureAllVPCSwitchRuntime(); err != nil {
				return nil, fmt.Errorf("网桥已创建，但恢复 VPC 交换机网络失败: %w", err)
			}
		}
	}
	return row, nil
}

func EnsureOVSBridgeDirect(bridge, uplink string, migrateHostIP bool, cfg HostIPConfig) error {
	if result := utils.ExecCommand("bash", "-c", "command -v ovs-vsctl"); result.Error != nil {
		return fmt.Errorf("OVS 未安装，请先安装 openvswitch-switch 或 openvswitch")
	}
	bridge = strings.TrimSpace(bridge)
	uplink = strings.TrimSpace(uplink)
	if err := os.MkdirAll(bridgeConfigDir, 0755); err != nil {
		return fmt.Errorf("创建网桥配置目录失败: %w", err)
	}
	if result := utils.ExecCommand("ovs-vsctl", "--may-exist", "add-br", bridge); result.Error != nil {
		return fmt.Errorf("创建桥接网桥失败: %s", result.Stderr)
	}
	utils.ExecCommand("ip", "link", "set", bridge, "up")
	if uplink != "" {
		if result := utils.ExecCommand("ovs-vsctl", "--may-exist", "add-port", bridge, uplink); result.Error != nil {
			return fmt.Errorf("添加物理网卡到桥接网桥失败: %s", result.Stderr)
		}
		utils.ExecCommand("ip", "link", "set", uplink, "up")
	}
	// IP 迁移逻辑（同时处理 IPv4 和 IPv6）
	if migrateHostIP && uplink != "" {
		// 检查网桥是否已有 IP（重启恢复场景：systemd 服务已应用了静态 IP）
		bridgeCfg := CaptureInterfaceIP(bridge)
		bridgeHasIP := strings.TrimSpace(bridgeCfg.Addrs) != "" || strings.TrimSpace(bridgeCfg.Addrs6) != ""
		if !bridgeHasIP {
			// 网桥没有 IP，尝试从物理口迁移或使用存储值
			uplinkCfg := CaptureInterfaceIP(uplink)
			if strings.TrimSpace(uplinkCfg.Addrs) != "" || strings.TrimSpace(uplinkCfg.Addrs6) != "" {
				// 物理口有 IP，执行动态迁移（IPv4 + IPv6）
				migrateInterfaceIPToBridge(uplink, bridge)
			} else if strings.TrimSpace(cfg.Addrs) != "" || strings.TrimSpace(cfg.Addrs6) != "" {
				// 物理口也没 IP，使用存储的静态配置恢复（IPv4 + IPv6）
				applyStaticIPToBridge(bridge, cfg)
			}
		}
		// DNS 总是需要确保配置正确（重启恢复场景下即使网桥已有 IP，DNS 也可能丢失）
		ensureBridgeResolvedDNSWithStatic(uplink, bridge, cfg.DNS)
		// 如果 cfg 为空但网桥已有 IP，更新 cfg 用于写入脚本
		if strings.TrimSpace(cfg.Addrs) == "" && strings.TrimSpace(cfg.Addrs6) == "" {
			cfg = CaptureInterfaceIP(bridge)
			// 同时保留已有的 DNS 信息
			if cfg.DNS == "" {
				cfg.DNS = captureInterfaceDNSServers(bridge)
			}
		}
		// 兼容旧记录：IP 已存储但 DNS 未存储，从网桥当前状态捕获 DNS
		if strings.TrimSpace(cfg.DNS) == "" {
			cfg.DNS = captureInterfaceDNSServers(bridge)
			// 网桥也没有则回退到 uplink
			if cfg.DNS == "" {
				cfg.DNS = captureInterfaceDNSServers(uplink)
			}
		}
	}
	// IP 已迁移完成后再禁用 networkd DHCP，避免周期性 DHCP Discover 干扰 OVS 数据通道
	if uplink != "" {
		disableNetworkdDHCPForPort(uplink)
	}
	if err := writeBridgeRestoreScript(bridge, uplink, migrateHostIP, cfg); err != nil {
		return err
	}
	if err := writeBridgeRestoreUnit(); err != nil {
		return err
	}
	return nil
}

func validateBridgeName(name string) error {
	if name == "" {
		return fmt.Errorf("网桥名称不能为空")
	}
	if len(name) > 15 {
		return fmt.Errorf("网桥名称不能超过 15 个字符")
	}
	if ok, _ := regexp.MatchString(`^[A-Za-z0-9_.-]+$`, name); !ok {
		return fmt.Errorf("网桥名称只能包含字母、数字、点、下划线和短横线")
	}
	return nil
}

func validateBridgeUplink(uplink, targetBridge string) error {
	if !isPhysicalInterface(uplink) {
		return fmt.Errorf("请选择真实物理网卡")
	}
	ports := readOVSPortBridgeMap()
	if bridge := ports[uplink]; bridge != "" && bridge != targetBridge {
		return fmt.Errorf("物理网卡 %s 已接入 OVS 网桥 %s", uplink, bridge)
	}
	if model.DB != nil {
		var count int64
		model.DB.Model(&model.NetworkBridge{}).Where("uplink_if = ?", uplink).Count(&count)
		if count > 0 {
			return fmt.Errorf("物理网卡 %s 已被其它桥接网桥使用", uplink)
		}
	}
	return nil
}

// ValidateVPCSwitchUplink 校验交换机上行链路的占用关系与三层出口条件。
// 托管 NAT 上行允许在多个托管交换机间共享；二层直通上行保持独占。
func ValidateVPCSwitchUplink(uplink, uplinkGateway string, managed bool, switchID uint, targetBridge string) error {
	uplink = strings.TrimSpace(uplink)
	uplinkGateway = strings.TrimSpace(uplinkGateway)
	if !isPhysicalInterface(uplink) {
		return fmt.Errorf("请选择真实物理网卡")
	}
	if model.DB != nil {
		query := model.DB.Model(&model.VPCSwitch{}).Where("uplink_if = ?", uplink)
		if switchID > 0 {
			query = query.Where("id <> ?", switchID)
		}
		if managed {
			// 托管 NAT 只借用宿主机三层出口，允许与已有物理直通交换机共享同一出口。
		} else {
			var count int64
			query.Count(&count)
			if count > 0 {
				return fmt.Errorf("物理网卡 %s 已由其它交换机使用", uplink)
			}
		}

		var legacyCount int64
		model.DB.Model(&model.NetworkBridge{}).Where("uplink_if = ? AND name <> ?", uplink, strings.TrimSpace(targetBridge)).Count(&legacyCount)
		if legacyCount > 0 && !managed {
			return fmt.Errorf("物理网卡 %s 已由历史宿主机网桥使用", uplink)
		}
	}
	ports := readOVSPortBridgeMap()
	if currentBridge := strings.TrimSpace(ports[uplink]); currentBridge != "" && currentBridge != strings.TrimSpace(targetBridge) {
		if !managed && currentBridge == ovspkg.OvsBridgeName() {
			return fmt.Errorf("物理网卡 %s 是系统基础网络上行，不能切换为二层直通", uplink)
		}
		ownedByCurrentSwitch := false
		if model.DB != nil && switchID > 0 {
			var current model.VPCSwitch
			if err := model.DB.First(&current, switchID).Error; err == nil {
				ownedByCurrentSwitch = strings.EqualFold(strings.TrimSpace(current.UplinkIF), uplink) &&
					strings.EqualFold(strings.TrimSpace(current.BridgeName), currentBridge)
			}
		}
		// 托管 NAT 可以从已有 OVS 直通网桥的三层接口出站，不会重复接管物理端口。
		if !ownedByCurrentSwitch && !managed {
			return fmt.Errorf("物理网卡 %s 已接入 OVS 网桥 %s", uplink, currentBridge)
		}
	}
	if managed {
		effective := EffectiveL3Interface(uplink)
		cfg := CaptureInterfaceIPv4(effective)
		if strings.TrimSpace(cfg.Addrs) == "" {
			return fmt.Errorf("物理网卡 %s 的有效三层接口 %s 缺少可用的 IPv4 地址", uplink, effective)
		}
		gateway := uplinkGateway
		if gateway == "" {
			gateway = strings.TrimSpace(cfg.Gateway)
		}
		if gateway == "" {
			return fmt.Errorf("物理网卡 %s 已检测到 IPv4 地址，但未检测到该出口的默认网关，请填写上行网关", uplink)
		}
		if ip := net.ParseIP(gateway); ip == nil || ip.To4() == nil {
			return fmt.Errorf("物理网卡 %s 的上行网关不是有效的 IPv4 地址", uplink)
		}
	}
	return nil
}

// EffectiveL3Interface 返回内核实际承载地址与默认路由的三层接口。
func EffectiveL3Interface(uplink string) string {
	uplink = strings.TrimSpace(uplink)
	if uplink == "" {
		return ""
	}
	bridge := strings.TrimSpace(readOVSPortBridgeMap()[uplink])
	if bridge != "" && len(CaptureInterfaceIPv4(bridge).Addrs) > 0 {
		return bridge
	}
	return uplink
}

func ovsBridgeExists(name string) bool {
	return utils.ExecCommand("ovs-vsctl", "br-exists", strings.TrimSpace(name)).Error == nil
}

func linkIsUp(name string) bool {
	result := utils.ExecCommand("ip", "-j", "link", "show", "dev", strings.TrimSpace(name))
	return result.Error == nil && strings.Contains(strings.ToUpper(result.Stdout), "UP")
}
