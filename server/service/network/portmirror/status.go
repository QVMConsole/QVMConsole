package portmirror

import (
	"fmt"
	"os/exec"
	"regexp"
	"strconv"
	"strings"

	"kvm_console/utils"
)

var (
	tcStatsPattern  = regexp.MustCompile(`Sent\s+(\d+)\s+bytes\s+(\d+)\s+pkt\s+\(dropped\s+(\d+)`)
	ovsStatsPattern = regexp.MustCompile(`n_packets=(\d+),\s*n_bytes=(\d+)`)
)

// GetStatus 以持久配置为期望，以每个源的 tc 和每个目标注入口的 OVS 流表为实际状态。
func GetStatus() (*Status, error) {
	cfg, err := loadConfig()
	if err != nil {
		return nil, err
	}
	status := &Status{Healthy: true, Issues: []string{}, Sources: []SourceStatus{}, TargetStats: []TargetStatus{}}
	if cfg == nil || !cfg.Enabled {
		return status, nil
	}
	status.Enabled = true
	status.SourceInterfaces = append([]string{}, cfg.SourceInterfaces...)
	status.Targets = append([]TargetConfig{}, cfg.Targets...)
	status.Direction = cfg.Direction
	status.UpdatedAt = cfg.UpdatedAt
	runtime, runtimeErr := loadRuntime()
	if runtimeErr != nil || runtime == nil {
		status.Healthy = false
		if runtimeErr != nil {
			status.Issues = append(status.Issues, runtimeErr.Error())
		} else {
			status.Issues = append(status.Issues, "端口镜像运行态文件不存在")
		}
		return status, nil
	}

	for _, source := range cfg.SourceInterfaces {
		outputs := connectionOutputs(*runtime, source)
		item := SourceStatus{SourceInterface: source}
		if utils.ExecCommandQuiet("ip", "link", "show", "dev", source).Error != nil {
			status.Healthy = false
			status.Issues = append(status.Issues, fmt.Sprintf("源接口 %s 不存在", source))
		}
		if cfg.Direction != DirectionEgress {
			item.Ingress = readDirectionStats(source, DirectionIngress, IngressPreference, outputs)
			if !item.Ingress.Enabled {
				status.Healthy = false
				status.Issues = append(status.Issues, fmt.Sprintf("源接口 %s 的入方向镜像规则缺失", source))
			}
		}
		if cfg.Direction != DirectionIngress {
			item.Egress = readDirectionStats(source, DirectionEgress, EgressPreference, outputs)
			if !item.Egress.Enabled {
				status.Healthy = false
				status.Issues = append(status.Issues, fmt.Sprintf("源接口 %s 的出方向镜像规则缺失", source))
			}
		}
		status.Ingress = addDirectionStats(status.Ingress, item.Ingress)
		status.Egress = addDirectionStats(status.Egress, item.Egress)
		status.Sources = append(status.Sources, item)
	}

	for _, target := range cfg.Targets {
		item := TargetStatus{SwitchID: target.SwitchID, SwitchName: target.SwitchName, Bridge: target.Bridge}
		for _, connection := range runtime.Connections {
			if connection.TargetSwitchID != target.SwitchID {
				continue
			}
			item.Connections++
			bridgeResult := utils.ExecCommandQuiet("ovs-vsctl", "port-to-br", connection.OVSPort)
			if bridgeResult.Error != nil || strings.TrimSpace(bridgeResult.Stdout) != target.Bridge {
				status.Healthy = false
				status.Issues = append(status.Issues, fmt.Sprintf("%s 到 %s 的 OVS 注入口缺失", connection.SourceInterface, target.SwitchName))
				continue
			}
			flowResult := utils.ExecCommandQuiet("ovs-ofctl", "-O", "OpenFlow13", "dump-flows", target.Bridge, "cookie="+connection.Cookie+"/-1")
			match := ovsStatsPattern.FindStringSubmatch(flowResult.Stdout)
			if flowResult.Error != nil || len(match) != 3 {
				status.Healthy = false
				status.Issues = append(status.Issues, fmt.Sprintf("%s 到 %s 的 OVS 镜像流缺失", connection.SourceInterface, target.SwitchName))
				continue
			}
			packets, _ := strconv.ParseUint(match[1], 10, 64)
			bytes, _ := strconv.ParseUint(match[2], 10, 64)
			item.OVSPackets += packets
			item.OVSBytes += bytes
		}
		if item.Connections != len(cfg.SourceInterfaces) {
			status.Healthy = false
			status.Issues = append(status.Issues, fmt.Sprintf("目标 %s 仅有 %d/%d 条源连接", target.SwitchName, item.Connections, len(cfg.SourceInterfaces)))
		}
		status.OVSPackets += item.OVSPackets
		status.OVSBytes += item.OVSBytes
		status.TargetStats = append(status.TargetStats, item)
	}
	return status, nil
}

func readDirectionStats(source, direction string, preference int, outputs []string) DirectionStats {
	result := utils.ExecCommandQuiet("tc", "-s", "filter", "show", "dev", source, direction, "pref", strconv.Itoa(preference))
	stats := DirectionStats{Enabled: result.Error == nil && filterOutputSetMatches(result.Stdout, outputs)}
	match := tcStatsPattern.FindStringSubmatch(result.Stdout)
	if len(match) == 4 {
		stats.Bytes, _ = strconv.ParseUint(match[1], 10, 64)
		stats.Packets, _ = strconv.ParseUint(match[2], 10, 64)
		stats.Dropped, _ = strconv.ParseUint(match[3], 10, 64)
	}
	return stats
}

func addDirectionStats(total, item DirectionStats) DirectionStats {
	total.Enabled = total.Enabled || item.Enabled
	total.Packets += item.Packets
	total.Bytes += item.Bytes
	total.Dropped += item.Dropped
	return total
}

// Preflight 在提交异步任务前进行全量只读校验。
func Preflight(req EnableRequest) (*Config, error) {
	cfg, err := resolveRequest(req)
	if err != nil {
		return nil, err
	}
	if err := ensureFilterSlots(cfg); err != nil {
		return nil, err
	}
	for _, command := range []string{"tc", "ip", "ovs-vsctl", "ovs-ofctl", "systemd-run", "systemctl"} {
		if _, err := exec.LookPath(command); err != nil {
			return nil, fmt.Errorf("缺少端口镜像依赖命令 %s", command)
		}
	}
	for _, target := range cfg.Targets {
		if result := utils.ExecCommandQuiet("ovs-ofctl", "-O", "OpenFlow13", "dump-flows", target.Bridge); result.Error != nil {
			return nil, fmt.Errorf("目标交换机 %s 不支持 OpenFlow13", target.SwitchName)
		}
	}
	return &cfg, nil
}
