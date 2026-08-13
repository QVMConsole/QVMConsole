package portmirror

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"hash/crc32"
	"os"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"kvm_console/model"
	bridgepkg "kvm_console/service/network/bridge"
	"kvm_console/utils"
)

var (
	operationMu       sync.Mutex
	interfaceName     = regexp.MustCompile(`^[a-zA-Z0-9_.:-]{1,15}$`)
	mirrorVethName    = regexp.MustCompile(`\bqpm[0-9a-f]{8}\b`)
	mirrorOVSPortName = regexp.MustCompile(`^qpo[0-9a-f]{8}$`)
	mirrorOutputName  = regexp.MustCompile(`Mirror to device ([a-zA-Z0-9_.:-]{1,15})`)
)

type ipAddressInfo struct {
	IfName    string `json:"ifname"`
	OperState string `json:"operstate"`
	AddrInfo  []struct {
		Family    string `json:"family"`
		Local     string `json:"local"`
		PrefixLen int    `json:"prefixlen"`
	} `json:"addr_info"`
}

// ListOptions 从宿主机运行态和交换机配置生成可选项。
func ListOptions() (*Options, error) {
	addressResult := utils.ExecCommand("ip", "-j", "address", "show")
	if addressResult.Error != nil {
		return nil, fmt.Errorf("读取宿主机接口失败: %s", commandMessage(addressResult))
	}
	var addresses []ipAddressInfo
	if err := json.Unmarshal([]byte(addressResult.Stdout), &addresses); err != nil {
		return nil, fmt.Errorf("解析宿主机接口失败: %w", err)
	}
	bridgeSet := map[string]bool{}
	if result := utils.ExecCommandQuiet("ovs-vsctl", "list-br"); result.Error == nil {
		for _, name := range strings.Fields(result.Stdout) {
			bridgeSet[name] = true
		}
	}
	defaultSet := map[string]bool{}
	if result := utils.ExecCommandQuiet("ip", "-j", "route", "show", "default"); result.Error == nil {
		var routes []struct {
			Dev string `json:"dev"`
		}
		_ = json.Unmarshal([]byte(result.Stdout), &routes)
		for _, route := range routes {
			defaultSet[route.Dev] = true
		}
	}
	sources := make([]SourceOption, 0, len(addresses))
	for _, item := range addresses {
		if item.IfName == "lo" || !interfaceName.MatchString(item.IfName) {
			continue
		}
		option := SourceOption{Name: item.IfName, State: strings.ToLower(item.OperState), DefaultRoute: defaultSet[item.IfName], Kind: "interface", CaptureStage: "interface"}
		if bridgeSet[item.IfName] {
			option.Kind = "ovs_bridge"
			option.CaptureStage = "pre_nat"
		} else if physicalInterface(item.IfName) {
			option.Kind = "physical"
			option.CaptureStage = "post_nat"
		}
		for _, addr := range item.AddrInfo {
			if addr.Family == "inet" || addr.Family == "inet6" {
				option.Addresses = append(option.Addresses, fmt.Sprintf("%s/%d", addr.Local, addr.PrefixLen))
			}
		}
		if option.DefaultRoute {
			option.Risk = "此接口承载默认路由，启用前会建立自动回滚看门狗"
		}
		sources = append(sources, option)
	}
	sort.Slice(sources, func(i, j int) bool {
		if sources[i].Kind != sources[j].Kind {
			return sources[i].Kind < sources[j].Kind
		}
		return sources[i].Name < sources[j].Name
	})

	var switches []model.VPCSwitch
	if model.DB == nil {
		return nil, fmt.Errorf("交换机配置尚未初始化")
	}
	if err := model.DB.Order("name asc").Find(&switches).Error; err != nil {
		return nil, fmt.Errorf("读取交换机列表失败: %w", err)
	}
	targets := make([]TargetOption, 0)
	for _, sw := range switches {
		if sw.IsSystem || sw.DHCPEnabled || strings.TrimSpace(sw.UplinkIF) != "" {
			continue
		}
		bridge := bridgepkg.BridgeNameForSwitch(sw)
		if bridge == "" || utils.ExecCommandQuiet("ovs-vsctl", "br-exists", bridge).Error != nil {
			continue
		}
		var count int64
		model.DB.Model(&model.VPCVMBinding{}).Where("switch_id = ?", sw.ID).Count(&count)
		targets = append(targets, TargetOption{SwitchID: sw.ID, SwitchName: sw.Name, Bridge: bridge, VMCount: count})
	}
	return &Options{Sources: sources, Targets: targets}, nil
}

func physicalInterface(name string) bool {
	data, err := os.ReadFile("/sys/class/net/" + name + "/type")
	if err != nil || strings.TrimSpace(string(data)) != "1" {
		return false
	}
	_, err = os.Stat("/sys/class/net/" + name + "/device")
	return err == nil
}

func resolveRequest(req EnableRequest) (Config, error) {
	direction := strings.ToLower(strings.TrimSpace(req.Direction))
	if direction != DirectionIngress && direction != DirectionEgress && direction != DirectionBoth {
		return Config{}, fmt.Errorf("镜像方向必须为 ingress、egress 或 both")
	}
	sourceSet := map[string]bool{}
	sources := make([]string, 0, len(req.SourceInterfaces))
	for _, raw := range req.SourceInterfaces {
		source := strings.TrimSpace(raw)
		if !interfaceName.MatchString(source) {
			return Config{}, fmt.Errorf("源接口名称无效: %s", source)
		}
		if sourceSet[source] {
			continue
		}
		if utils.ExecCommandQuiet("ip", "link", "show", "dev", source).Error != nil {
			return Config{}, fmt.Errorf("源接口 %s 不存在", source)
		}
		sourceSet[source] = true
		sources = append(sources, source)
	}
	if len(sources) == 0 {
		return Config{}, fmt.Errorf("请至少选择一个镜像来源")
	}
	if model.DB == nil {
		return Config{}, fmt.Errorf("交换机配置尚未初始化")
	}
	idSet := map[uint]bool{}
	bridgeSet := map[string]bool{}
	targets := make([]TargetConfig, 0, len(req.TargetSwitchIDs))
	for _, id := range req.TargetSwitchIDs {
		if id == 0 || idSet[id] {
			continue
		}
		var sw model.VPCSwitch
		if err := model.DB.First(&sw, id).Error; err != nil {
			return Config{}, fmt.Errorf("目标交换机 %d 不存在", id)
		}
		if sw.IsSystem || sw.DHCPEnabled || strings.TrimSpace(sw.UplinkIF) != "" {
			return Config{}, fmt.Errorf("目标 %s 不是没有上行和内置 DHCP 的空交换机", sw.Name)
		}
		bridge := bridgepkg.BridgeNameForSwitch(sw)
		if !interfaceName.MatchString(bridge) || utils.ExecCommandQuiet("ovs-vsctl", "br-exists", bridge).Error != nil {
			return Config{}, fmt.Errorf("目标交换机 %s 的网桥 %s 不存在", sw.Name, bridge)
		}
		if sourceSet[bridge] {
			return Config{}, fmt.Errorf("源接口不能包含目标交换机网桥 %s，避免镜像环路", bridge)
		}
		if bridgeSet[bridge] {
			return Config{}, fmt.Errorf("多个目标交换机映射到同一 OVS 网桥 %s", bridge)
		}
		idSet[id] = true
		bridgeSet[bridge] = true
		targets = append(targets, TargetConfig{SwitchID: sw.ID, SwitchName: sw.Name, Bridge: bridge})
	}
	if len(targets) == 0 {
		return Config{}, fmt.Errorf("请至少选择一个目标空交换机")
	}
	sort.Strings(sources)
	sort.Slice(targets, func(i, j int) bool { return targets[i].SwitchID < targets[j].SwitchID })
	return Config{Enabled: true, SourceInterfaces: sources, Targets: targets, Direction: direction, UpdatedAt: time.Now()}, nil
}

func runtimeNames(source, targetBridge string) (string, string, string) {
	sum := crc32.ChecksumIEEE([]byte(source + "\x00" + targetBridge))
	return fmt.Sprintf("qpm%08x", sum), fmt.Sprintf("qpo%08x", sum), fmt.Sprintf("0x%016x", CookiePrefix|uint64(sum))
}

func buildRuntimeTemplate(cfg Config) RuntimeState {
	state := RuntimeState{Config: cfg, Sources: make([]RuntimeSource, 0, len(cfg.SourceInterfaces)), Connections: []RuntimeConnection{}}
	for _, source := range cfg.SourceInterfaces {
		qdisc := utils.ExecCommandQuiet("tc", "qdisc", "show", "dev", source)
		state.Sources = append(state.Sources, RuntimeSource{Name: source, ClsactCreated: !strings.Contains(qdisc.Stdout, "clsact")})
		for _, target := range cfg.Targets {
			vethSource, ovsPort, cookie := runtimeNames(source, target.Bridge)
			state.Connections = append(state.Connections, RuntimeConnection{
				SourceInterface: source, TargetSwitchID: target.SwitchID, TargetBridge: target.Bridge,
				VethSource: vethSource, OVSPort: ovsPort, Cookie: cookie,
			})
		}
	}
	return state
}

func connectionOutputs(state RuntimeState, source string) []string {
	outputs := make([]string, 0)
	for _, connection := range state.Connections {
		if connection.SourceInterface == source {
			outputs = append(outputs, connection.VethSource)
		}
	}
	sort.Strings(outputs)
	return outputs
}

func ensureFilterSlots(cfg Config) error {
	runtime, _ := loadRuntime()
	for _, source := range cfg.SourceInterfaces {
		checks := []struct {
			direction string
			pref      int
			enabled   bool
		}{{DirectionIngress, IngressPreference, cfg.Direction != DirectionEgress}, {DirectionEgress, EgressPreference, cfg.Direction != DirectionIngress}}
		for _, check := range checks {
			if !check.enabled {
				continue
			}
			result := utils.ExecCommandQuiet("tc", "filter", "show", "dev", source, check.direction, "pref", strconv.Itoa(check.pref))
			owned := runtime != nil && filterOutputSetMatches(result.Stdout, connectionOutputs(*runtime, source))
			if strings.Contains(result.Stdout, fmt.Sprintf("pref %d", check.pref)) && !owned {
				return fmt.Errorf("源接口 %s 的 %s 优先级 %d 已被其他 tc 规则占用", source, check.direction, check.pref)
			}
		}
	}
	return nil
}

func applyRuntime(cfg Config) (*RuntimeState, error) {
	if err := ensureFilterSlots(cfg); err != nil {
		return nil, err
	}
	state := buildRuntimeTemplate(cfg)
	if err := ensureRuntimeNamesAvailable(state); err != nil {
		return nil, err
	}
	failed := true
	defer func() {
		if failed {
			_ = cleanupRuntime(state)
		}
	}()
	for index := range state.Connections {
		connection := &state.Connections[index]
		if result := utils.ExecCommand("ip", "link", "add", connection.VethSource, "type", "veth", "peer", "name", connection.OVSPort); result.Error != nil {
			return nil, fmt.Errorf("创建镜像 veth 失败: %s", commandMessage(result))
		}
		for _, iface := range []string{connection.VethSource, connection.OVSPort} {
			if result := utils.ExecCommand("ip", "link", "set", "dev", iface, "mtu", "1500", "up"); result.Error != nil {
				return nil, fmt.Errorf("启用镜像接口 %s 失败: %s", iface, commandMessage(result))
			}
		}
		if result := utils.ExecCommand("ovs-vsctl", "--may-exist", "add-port", connection.TargetBridge, connection.OVSPort, "--", "set", "Interface", connection.OVSPort, "external_ids:qvm-purpose=port-mirror", "external_ids:qvm-source="+connection.SourceInterface, fmt.Sprintf("external_ids:qvm-target-switch=%d", connection.TargetSwitchID)); result.Error != nil {
			return nil, fmt.Errorf("接入目标交换机失败: %s", commandMessage(result))
		}
		ofportResult := utils.ExecCommand("ovs-vsctl", "get", "Interface", connection.OVSPort, "ofport")
		ofport, err := strconv.Atoi(strings.Trim(strings.TrimSpace(ofportResult.Stdout), `"`))
		if ofportResult.Error != nil || err != nil || ofport <= 0 {
			return nil, fmt.Errorf("镜像注入口 %s 的 ofport 无效", connection.OVSPort)
		}
		connection.OFPort = ofport
		flow := fmt.Sprintf("cookie=%s,priority=200,in_port=%d,actions=FLOOD", connection.Cookie, ofport)
		if result := utils.ExecCommand("ovs-ofctl", "-O", "OpenFlow13", "add-flow", connection.TargetBridge, flow); result.Error != nil {
			return nil, fmt.Errorf("创建镜像转发流失败: %s", commandMessage(result))
		}
	}
	for sourceIndex := range state.Sources {
		source := &state.Sources[sourceIndex]
		if source.ClsactCreated {
			if result := utils.ExecCommand("tc", "qdisc", "add", "dev", source.Name, "clsact"); result.Error != nil {
				return nil, fmt.Errorf("创建源接口 %s 的 clsact 失败: %s", source.Name, commandMessage(result))
			}
		}
		outputs := connectionOutputs(state, source.Name)
		if cfg.Direction != DirectionEgress {
			if err := addMirrorFilter(source.Name, DirectionIngress, IngressPreference, outputs); err != nil {
				return nil, err
			}
		}
		if cfg.Direction != DirectionIngress {
			if err := addMirrorFilter(source.Name, DirectionEgress, EgressPreference, outputs); err != nil {
				return nil, err
			}
		}
	}
	if err := writeJSONAtomic(RuntimePath, state); err != nil {
		return nil, err
	}
	if err := validateRuntime(state); err != nil {
		return nil, err
	}
	failed = false
	return &state, nil
}

func ensureRuntimeNamesAvailable(state RuntimeState) error {
	for _, connection := range state.Connections {
		if utils.ExecCommandQuiet("ip", "link", "show", "dev", connection.VethSource).Error == nil {
			return fmt.Errorf("端口镜像临时接口名称冲突: %s", connection.VethSource)
		}
		if utils.ExecCommandQuiet("ip", "link", "show", "dev", connection.OVSPort).Error == nil {
			return fmt.Errorf("端口镜像临时接口名称冲突: %s", connection.OVSPort)
		}
	}
	return nil
}

func addMirrorFilter(source, direction string, preference int, outputs []string) error {
	if len(outputs) == 0 {
		return fmt.Errorf("源接口 %s 没有镜像输出目标", source)
	}
	args := []string{"filter", "add", "dev", source, direction, "pref", strconv.Itoa(preference), "protocol", "all", "matchall"}
	for _, output := range outputs {
		args = append(args, "action", "mirred", "egress", "mirror", "dev", output)
	}
	result := utils.ExecCommand("tc", args...)
	if result.Error != nil {
		return fmt.Errorf("创建 %s 的 %s 镜像规则失败: %s", source, direction, commandMessage(result))
	}
	return nil
}

func filterOutputSetMatches(output string, expected []string) bool {
	if len(expected) == 0 || strings.Count(output, "Mirror to device") != len(expected) {
		return false
	}
	for _, iface := range expected {
		if !strings.Contains(output, "Mirror to device "+iface) {
			return false
		}
	}
	return true
}

func validateRuntime(state RuntimeState) error {
	expectedConnections := len(state.SourceInterfaces) * len(state.Targets)
	if len(state.Sources) != len(state.SourceInterfaces) || len(state.Connections) != expectedConnections {
		return fmt.Errorf("端口镜像运行态矩阵不完整: 来源 %d/%d，连接 %d/%d", len(state.Sources), len(state.SourceInterfaces), len(state.Connections), expectedConnections)
	}
	expectedSources := make(map[string]bool, len(state.SourceInterfaces))
	for _, source := range state.SourceInterfaces {
		expectedSources[source] = true
	}
	seenSources := make(map[string]bool, len(state.Sources))
	for _, source := range state.Sources {
		if !expectedSources[source.Name] || seenSources[source.Name] {
			return fmt.Errorf("端口镜像运行态包含无效或重复来源: %s", source.Name)
		}
		seenSources[source.Name] = true
	}
	expected := make(map[string]bool, expectedConnections)
	for _, source := range state.SourceInterfaces {
		for _, target := range state.Targets {
			expected[fmt.Sprintf("%s\x00%d\x00%s", source, target.SwitchID, target.Bridge)] = true
		}
	}
	seen := make(map[string]bool, len(state.Connections))
	for _, connection := range state.Connections {
		key := fmt.Sprintf("%s\x00%d\x00%s", connection.SourceInterface, connection.TargetSwitchID, connection.TargetBridge)
		if !expected[key] || seen[key] {
			return fmt.Errorf("端口镜像运行态包含无效或重复连接: %s → %s", connection.SourceInterface, connection.TargetBridge)
		}
		seen[key] = true
	}
	for _, source := range state.Sources {
		outputs := connectionOutputs(state, source.Name)
		if state.Direction != DirectionEgress && !filterOwned(source.Name, DirectionIngress, IngressPreference, outputs) {
			return fmt.Errorf("源接口 %s 的入方向镜像规则回读失败", source.Name)
		}
		if state.Direction != DirectionIngress && !filterOwned(source.Name, DirectionEgress, EgressPreference, outputs) {
			return fmt.Errorf("源接口 %s 的出方向镜像规则回读失败", source.Name)
		}
	}
	for _, connection := range state.Connections {
		if result := utils.ExecCommandQuiet("ip", "link", "show", "dev", connection.VethSource); result.Error != nil {
			return fmt.Errorf("镜像 veth %s 未创建", connection.VethSource)
		}
		if result := utils.ExecCommandQuiet("ovs-vsctl", "port-to-br", connection.OVSPort); result.Error != nil || strings.TrimSpace(result.Stdout) != connection.TargetBridge {
			return fmt.Errorf("镜像注入口 %s 未接入目标交换机", connection.OVSPort)
		}
		flows := utils.ExecCommandQuiet("ovs-ofctl", "-O", "OpenFlow13", "dump-flows", connection.TargetBridge, "cookie="+connection.Cookie+"/-1")
		if flows.Error != nil || !strings.Contains(strings.ToLower(flows.Stdout), strings.ToLower(strings.TrimPrefix(connection.Cookie, "0x"))) {
			return fmt.Errorf("镜像注入口 %s 的转发流回读失败", connection.OVSPort)
		}
	}
	return nil
}

func restorePreviousConfig(previous *Config) error {
	if previous == nil || !previous.Enabled {
		return nil
	}
	restored, err := applyRuntime(*previous)
	if err != nil {
		return err
	}
	if err := writeJSONAtomic(ConfigPath, previous); err != nil {
		_ = cleanupRuntime(*restored)
		return err
	}
	return nil
}

func withRestoreError(cause error, previous *Config) error {
	if restoreErr := restorePreviousConfig(previous); restoreErr != nil {
		return fmt.Errorf("%w；恢复旧镜像失败: %v", cause, restoreErr)
	}
	return cause
}

func filterOwned(source, direction string, preference int, outputs []string) bool {
	result := utils.ExecCommandQuiet("tc", "filter", "show", "dev", source, direction, "pref", strconv.Itoa(preference))
	return result.Error == nil && filterOutputSetMatches(result.Stdout, outputs)
}

func cleanupRuntime(state RuntimeState) error {
	var errs []error
	for _, source := range state.Sources {
		if !interfaceName.MatchString(source.Name) || utils.ExecCommandQuiet("ip", "link", "show", "dev", source.Name).Error != nil {
			continue
		}
		outputs := connectionOutputs(state, source.Name)
		if state.Direction != DirectionEgress && filterOwned(source.Name, DirectionIngress, IngressPreference, outputs) {
			if result := utils.ExecCommandQuiet("tc", "filter", "del", "dev", source.Name, DirectionIngress, "pref", strconv.Itoa(IngressPreference)); result.Error != nil {
				errs = append(errs, fmt.Errorf("清理 %s 入方向镜像规则失败", source.Name))
			}
		}
		if state.Direction != DirectionIngress && filterOwned(source.Name, DirectionEgress, EgressPreference, outputs) {
			if result := utils.ExecCommandQuiet("tc", "filter", "del", "dev", source.Name, DirectionEgress, "pref", strconv.Itoa(EgressPreference)); result.Error != nil {
				errs = append(errs, fmt.Errorf("清理 %s 出方向镜像规则失败", source.Name))
			}
		}
	}
	for _, connection := range state.Connections {
		bridgeExists := interfaceName.MatchString(connection.TargetBridge) && utils.ExecCommandQuiet("ovs-vsctl", "br-exists", connection.TargetBridge).Error == nil
		if bridgeExists && connection.Cookie != "" {
			if result := utils.ExecCommandQuiet("ovs-ofctl", "-O", "OpenFlow13", "del-flows", connection.TargetBridge, "cookie="+connection.Cookie+"/-1"); result.Error != nil {
				errs = append(errs, fmt.Errorf("清理 %s 的镜像流失败", connection.OVSPort))
			}
		}
		if bridgeExists && interfaceName.MatchString(connection.OVSPort) {
			if result := utils.ExecCommandQuiet("ovs-vsctl", "--if-exists", "del-port", connection.TargetBridge, connection.OVSPort); result.Error != nil {
				errs = append(errs, fmt.Errorf("清理 OVS 镜像端口 %s 失败", connection.OVSPort))
			}
		}
		if interfaceName.MatchString(connection.VethSource) {
			utils.ExecCommandQuiet("ip", "link", "del", connection.VethSource)
		}
	}
	for _, source := range state.Sources {
		if !source.ClsactCreated || !interfaceName.MatchString(source.Name) {
			continue
		}
		ingress := utils.ExecCommandQuiet("tc", "filter", "show", "dev", source.Name, DirectionIngress)
		egress := utils.ExecCommandQuiet("tc", "filter", "show", "dev", source.Name, DirectionEgress)
		if strings.TrimSpace(ingress.Stdout) == "" && strings.TrimSpace(egress.Stdout) == "" {
			utils.ExecCommandQuiet("tc", "qdisc", "del", "dev", source.Name, "clsact")
		}
	}
	os.Remove(RuntimePath)
	return errors.Join(errs...)
}

func cleanupCurrentRuntime() error {
	state, err := loadRuntime()
	if err != nil {
		return err
	}
	if state != nil {
		return cleanupRuntime(*state)
	}
	cfg, configErr := loadConfig()
	if configErr != nil {
		return configErr
	}
	if cfg != nil && len(cfg.SourceInterfaces) > 0 && len(cfg.Targets) > 0 {
		return cleanupRuntime(buildRuntimeTemplate(*cfg))
	}
	return cleanupOwnedResidue()
}

func cleanupOwnedResidue() error {
	type ownedPort struct {
		Name   string
		Bridge string
	}
	var errs []error
	ports := make([]ownedPort, 0)
	veths := map[string]bool{}
	result := utils.ExecCommandQuiet("ovs-vsctl", "--data=bare", "--no-heading", "--columns=name", "find", "Interface", "external_ids:qvm-purpose=port-mirror")
	for _, port := range strings.Fields(result.Stdout) {
		if !mirrorOVSPortName.MatchString(port) {
			continue
		}
		bridge := utils.ExecCommandQuiet("ovs-vsctl", "port-to-br", port)
		if bridge.Error == nil && interfaceName.MatchString(strings.TrimSpace(bridge.Stdout)) {
			ports = append(ports, ownedPort{Name: port, Bridge: strings.TrimSpace(bridge.Stdout)})
		}
		veths["qpm"+strings.TrimPrefix(port, "qpo")] = true
	}
	links := utils.ExecCommandQuiet("ip", "-o", "link", "show")
	for _, line := range strings.Split(links.Stdout, "\n") {
		fields := strings.Fields(line)
		if len(fields) < 2 {
			continue
		}
		iface := strings.TrimSuffix(strings.Split(fields[1], "@")[0], ":")
		if !interfaceName.MatchString(iface) {
			continue
		}
		for _, check := range []struct {
			direction string
			pref      int
		}{{DirectionIngress, IngressPreference}, {DirectionEgress, EgressPreference}} {
			filter := utils.ExecCommandQuiet("tc", "filter", "show", "dev", iface, check.direction, "pref", strconv.Itoa(check.pref))
			matches := mirrorOutputName.FindAllStringSubmatch(filter.Stdout, -1)
			if len(matches) == 0 {
				continue
			}
			owned := true
			for _, match := range matches {
				if len(match) != 2 || (!veths[match[1]] && !mirrorVethName.MatchString(match[1])) {
					owned = false
					break
				}
				veths[match[1]] = true
			}
			if owned {
				if deleted := utils.ExecCommandQuiet("tc", "filter", "del", "dev", iface, check.direction, "pref", strconv.Itoa(check.pref)); deleted.Error != nil {
					errs = append(errs, fmt.Errorf("清理 %s 的残留 %s 镜像规则失败", iface, check.direction))
				}
			}
		}
	}
	for _, bridge := range strings.Fields(utils.ExecCommandQuiet("ovs-vsctl", "list-br").Stdout) {
		if interfaceName.MatchString(bridge) {
			if deleted := utils.ExecCommandQuiet("ovs-ofctl", "-O", "OpenFlow13", "del-flows", bridge, "cookie="+CookiePrefixText+"/"+CookiePrefixMask); deleted.Error != nil {
				errs = append(errs, fmt.Errorf("清理网桥 %s 的残留镜像流失败", bridge))
			}
		}
	}
	for _, port := range ports {
		if deleted := utils.ExecCommandQuiet("ovs-vsctl", "--if-exists", "del-port", port.Bridge, port.Name); deleted.Error != nil {
			errs = append(errs, fmt.Errorf("清理残留 OVS 镜像端口 %s 失败", port.Name))
		}
	}
	for veth := range veths {
		if interfaceName.MatchString(veth) && utils.ExecCommandQuiet("ip", "link", "show", "dev", veth).Error == nil {
			if deleted := utils.ExecCommandQuiet("ip", "link", "del", veth); deleted.Error != nil {
				errs = append(errs, fmt.Errorf("清理残留镜像接口 %s 失败", veth))
			}
		}
	}
	return errors.Join(errs...)
}

// Enable 建立多源到多目标镜像。新配置失败时会尝试恢复先前配置。
func Enable(ctx context.Context, req EnableRequest, progress func(int, string)) (string, error) {
	operationMu.Lock()
	defer operationMu.Unlock()
	if progress != nil {
		progress(5, "检查全部源接口与目标空交换机...")
	}
	cfg, err := resolveRequest(req)
	if err != nil {
		return "", err
	}
	previous, err := loadConfig()
	if err != nil {
		return "", err
	}
	if err := ctx.Err(); err != nil {
		return "", fmt.Errorf("端口镜像任务已取消")
	}
	if err := cleanupCurrentRuntime(); err != nil {
		return "", fmt.Errorf("清理旧镜像运行态失败: %w", err)
	}
	stateTemplate := buildRuntimeTemplate(cfg)
	if err := ensureRuntimeNamesAvailable(stateTemplate); err != nil {
		return "", withRestoreError(err, previous)
	}
	if progress != nil {
		progress(15, "建立多连接自动回滚看门狗...")
	}
	token, err := startWatchdog(stateTemplate)
	if err != nil {
		return "", withRestoreError(err, previous)
	}
	defer stopWatchdog(token)
	if progress != nil {
		progress(35, "创建多源到多目标镜像连接矩阵...")
	}
	state, applyErr := applyRuntime(cfg)
	if applyErr != nil {
		_ = cleanupCurrentRuntime()
		return "", withRestoreError(applyErr, previous)
	}
	watchdog, watchdogErr := loadWatchdog()
	if watchdogErr != nil || watchdog == nil || watchdog.Token != token {
		_ = cleanupRuntime(*state)
		return "", withRestoreError(fmt.Errorf("读取自动回滚状态失败"), previous)
	}
	watchdog.Runtime = *state
	if err := writeJSONAtomic(WatchdogPath, watchdog); err != nil {
		_ = cleanupRuntime(*state)
		return "", withRestoreError(fmt.Errorf("更新自动回滚状态失败: %w", err), previous)
	}
	if progress != nil {
		progress(75, "回读全部 tc 规则、OVS 端口和流表...")
	}
	if err := validateRuntime(*state); err != nil {
		_ = cleanupRuntime(*state)
		return "", withRestoreError(err, previous)
	}
	if err := writeJSONAtomic(ConfigPath, cfg); err != nil {
		_ = cleanupRuntime(*state)
		return "", withRestoreError(err, previous)
	}
	if progress != nil {
		progress(100, "多源多目标端口镜像已启用")
	}
	data, _ := json.Marshal(state)
	return string(data), nil
}

// Disable 清理本模块的全部运行态和持久配置。
func Disable(progress func(int, string)) (string, error) {
	operationMu.Lock()
	defer operationMu.Unlock()
	if progress != nil {
		progress(20, "清理全部端口镜像过滤器、连接和 OVS 流表...")
	}
	if err := cleanupCurrentRuntime(); err != nil {
		return "", err
	}
	if err := os.Remove(ConfigPath); err != nil && !errors.Is(err, os.ErrNotExist) {
		return "", fmt.Errorf("删除端口镜像配置失败: %w", err)
	}
	stopWatchdog("")
	if progress != nil {
		progress(100, "端口镜像已停用")
	}
	return `{"enabled":false}`, nil
}

// Restore 在服务启动时按持久配置恢复全部镜像连接。
func Restore() error {
	operationMu.Lock()
	defer operationMu.Unlock()
	cfg, err := loadConfig()
	if err != nil || cfg == nil || !cfg.Enabled {
		return err
	}
	ids := make([]uint, 0, len(cfg.Targets))
	for _, target := range cfg.Targets {
		ids = append(ids, target.SwitchID)
	}
	resolved, err := resolveRequest(EnableRequest{SourceInterfaces: cfg.SourceInterfaces, TargetSwitchIDs: ids, Direction: cfg.Direction})
	if err != nil {
		return fmt.Errorf("端口镜像启动恢复预检失败: %w", err)
	}
	_ = cleanupCurrentRuntime()
	if _, err = applyRuntime(resolved); err != nil {
		return err
	}
	return writeJSONAtomic(ConfigPath, resolved)
}

func ExecuteTask(ctx context.Context, params TaskParams, progress func(int, string)) (string, error) {
	switch strings.ToLower(strings.TrimSpace(params.Action)) {
	case "enable":
		return Enable(ctx, params.Request, progress)
	case "disable":
		return Disable(progress)
	default:
		return "", fmt.Errorf("未知端口镜像动作: %s", params.Action)
	}
}

func startWatchdog(state RuntimeState) (string, error) {
	stopWatchdog("")
	random := make([]byte, 16)
	if _, err := rand.Read(random); err != nil {
		return "", fmt.Errorf("生成自动回滚令牌失败: %w", err)
	}
	token := hex.EncodeToString(random)
	unit := WatchdogUnit + "-" + token[:12]
	if err := writeJSONAtomic(WatchdogPath, watchdogState{Token: token, Unit: unit, Runtime: state}); err != nil {
		return "", err
	}
	executable, err := os.Executable()
	if err != nil {
		return "", fmt.Errorf("定位服务程序失败: %w", err)
	}
	result := utils.ExecCommand("systemd-run", "--quiet", "--unit="+unit, "--on-active=120s", executable, "port-mirror-watchdog", token)
	if result.Error != nil {
		os.Remove(WatchdogPath)
		return "", fmt.Errorf("启动端口镜像自动回滚失败: %s", commandMessage(result))
	}
	active := utils.ExecCommandQuiet("systemctl", "is-active", unit+".timer")
	if strings.TrimSpace(active.Stdout) != "active" {
		os.Remove(WatchdogPath)
		return "", fmt.Errorf("端口镜像自动回滚定时器未进入运行状态")
	}
	return token, nil
}

func stopWatchdog(token string) {
	state, _ := loadWatchdog()
	if state == nil || (token != "" && state.Token != token) {
		return
	}
	if state.Unit != "" {
		utils.ExecCommandQuiet("systemctl", "stop", state.Unit+".timer")
		utils.ExecCommandQuiet("systemctl", "stop", state.Unit+".service")
	}
	os.Remove(WatchdogPath)
}

func loadWatchdog() (*watchdogState, error) {
	var state watchdogState
	if err := readJSON(WatchdogPath, &state); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, err
	}
	if len(state.Runtime.Sources) > 0 && len(state.Runtime.Connections) > 0 {
		return &state, nil
	}
	var legacy struct {
		Token   string             `json:"token"`
		Unit    string             `json:"unit"`
		Runtime legacyRuntimeState `json:"runtime"`
	}
	if err := readJSON(WatchdogPath, &legacy); err != nil {
		return nil, err
	}
	if migrated := migrateLegacyRuntime(legacy.Runtime); migrated != nil {
		state.Token = legacy.Token
		state.Unit = legacy.Unit
		state.Runtime = *migrated
	}
	return &state, nil
}

// RunWatchdog 由 systemd 瞬态单元调用，仅按令牌清理当前尝试创建的全部对象。
func RunWatchdog(token string) error {
	var state watchdogState
	if err := readJSON(WatchdogPath, &state); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return err
	}
	if token == "" || token != state.Token {
		return fmt.Errorf("端口镜像自动回滚令牌不匹配")
	}
	err := cleanupRuntime(state.Runtime)
	os.Remove(WatchdogPath)
	return err
}

func commandMessage(result *utils.CmdResult) string {
	if result == nil {
		return "未知命令错误"
	}
	if strings.TrimSpace(result.Stderr) != "" {
		return strings.TrimSpace(result.Stderr)
	}
	if result.Error != nil {
		return result.Error.Error()
	}
	return strings.TrimSpace(result.Stdout)
}
