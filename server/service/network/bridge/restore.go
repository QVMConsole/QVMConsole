package bridge

import (
	"fmt"
	"path/filepath"
	"strings"

	"kvm_console/utils"
)

func writeBridgeRestoreScript(bridge, uplink string, migrateHostIP bool, cfg HostIPConfig) error {
	content := buildBridgeRestoreScriptContent(bridge, uplink, migrateHostIP, cfg)
	_, err := HookWriteFileIfChanged(bridgeRestoreScriptPath(bridge), []byte(content), 0755)
	return err
}

func buildBridgeRestoreScriptContent(bridge, uplink string, migrateHostIP bool, cfg HostIPConfig) string {
	content := fmt.Sprintf(`#!/bin/bash
set -e
BRIDGE=%s
UPLINK=%s
`, utils.ShellSingleQuote(bridge), utils.ShellSingleQuote(uplink))
	content += `ovs-vsctl --may-exist add-br "$BRIDGE"
ip link set "$BRIDGE" up
`
	if strings.TrimSpace(uplink) != "" {
		content += `ovs-vsctl --may-exist add-port "$BRIDGE" "$UPLINK"
ip link set "$UPLINK" up
`
	}
	// 有 IPv4 或 IPv6 静态配置时，使用硬编变量恢复（避免重启后动态捕获失败）
	hasV4 := strings.TrimSpace(cfg.Addrs) != ""
	hasV6 := strings.TrimSpace(cfg.Addrs6) != ""
	if migrateHostIP && (hasV4 || hasV6) {
		content += fmt.Sprintf(`# 静态 IPv4 配置
HOST_ADDRS=%s
HOST_GW=%s
HOST_METRIC=%s
# 静态 IPv6 配置
HOST_ADDRS6=%s
HOST_GW6=%s
HOST_METRIC6=%s
`, utils.ShellSingleQuote(cfg.Addrs),
			utils.ShellSingleQuote(cfg.Gateway),
			utils.ShellSingleQuote(cfg.Metric),
			utils.ShellSingleQuote(cfg.Addrs6),
			utils.ShellSingleQuote(cfg.Gateway6),
			utils.ShellSingleQuote(cfg.Metric6))
		content += bridgeHostIPApplyStaticShell()
		// DNS：优先使用静态持久化 DNS，避免重启后因 uplink 被 networkd 禁用导致动态捕获失败
		if strings.TrimSpace(cfg.DNS) != "" {
			content += fmt.Sprintf(`# 静态 DNS 配置（创建时捕获）
HOST_DNS=%s
`, utils.ShellSingleQuote(cfg.DNS))
			content += bridgeResolvedDNSStaticShell()
		} else {
			content += `# DNS 迁移到 bridge，避免默认路由切换后解析仍绑定在物理口。
`
			content += bridgeResolvedDNSShell()
		}
	} else if migrateHostIP {
		// 没有存储配置时回退到动态捕获（兼容旧记录）
		content += bridgeHostIPCaptureShell()
		content += bridgeHostIPApplyShell()
		content += `# DNS 迁移到 bridge。
`
		content += bridgeResolvedDNSShell()
	}
	return content
}

func bridgeRestoreScriptPath(bridge string) string {
	return filepath.Join(bridgeConfigDir, strings.TrimSpace(bridge)+".sh")
}
