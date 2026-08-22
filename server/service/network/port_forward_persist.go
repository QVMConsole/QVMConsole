package network

import (
	"fmt"
	"os"
	"strings"

	"kvm_console/config"
	"kvm_console/logger"
	"kvm_console/utils"
)

func iptablesCheckLineForAddLine(line string) (string, bool) {
	line = strings.TrimSpace(line)
	if !strings.HasPrefix(line, "iptables ") {
		return "", false
	}
	idx := strings.Index(line, " -A ")
	if idx < 0 {
		return "", false
	}
	return line[:idx] + " -C " + line[idx+4:], true
}

func idempotentIPTablesAddLine(line string) string {
	line = normalizePortForwardIPTablesLine(strings.TrimSpace(line))
	checkLine, ok := iptablesCheckLineForAddLine(line)
	if !ok {
		return line
	}
	return checkLine + " 2>/dev/null || " + line
}

func normalizePortForwardIPTablesLine(line string) string {
	line = strings.TrimSpace(line)
	if !strings.Contains(line, " DNAT") || strings.Contains(line, " -t nat ") {
		return line
	}
	// 为 PREROUTING 和 OUTPUT 链的 DNAT 规则补充 -t nat
	if strings.Contains(line, " PREROUTING") || strings.Contains(line, " OUTPUT") {
		replacer := strings.NewReplacer(
			"iptables -A PREROUTING", "iptables -t nat -A PREROUTING",
			"iptables -C PREROUTING", "iptables -t nat -C PREROUTING",
			"iptables -D PREROUTING", "iptables -t nat -D PREROUTING",
			"iptables -A OUTPUT", "iptables -t nat -A OUTPUT",
			"iptables -C OUTPUT", "iptables -t nat -C OUTPUT",
			"iptables -D OUTPUT", "iptables -t nat -D OUTPUT",
		)
		return replacer.Replace(line)
	}
	return line
}

func restorePortForwardCommand(line, hostIP string) error {
	line = normalizePortForwardIPTablesLine(strings.TrimSpace(line))
	if line == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, "HOST_IP=") || strings.HasPrefix(line, "#!") {
		return nil
	}
	if strings.Contains(line, "||") {
		result := utils.ExecShell("HOST_IP=" + utils.ShellSingleQuote(hostIP) + "; " + line)
		if result.Error != nil {
			return fmt.Errorf("%s: %s", line, result.Stderr)
		}
		return nil
	}
	if !strings.HasPrefix(line, "iptables ") {
		return nil
	}
	checkLine, ok := iptablesCheckLineForAddLine(line)
	if !ok {
		return nil
	}
	prefix := "HOST_IP=" + utils.ShellSingleQuote(hostIP) + "; "
	if result := utils.ExecShell(prefix + checkLine); result.Error == nil {
		return nil
	}
	result := utils.ExecShell(prefix + line)
	if result.Error != nil {
		return fmt.Errorf("%s: %s", line, result.Stderr)
	}
	return nil
}

// RestorePortForwardRules 从持久化脚本恢复端口转发规则。
func RestorePortForwardRules() error {
	if err := HookEnsureOVSNetworkReady(); err != nil {
		return err
	}
	portfwdDir := config.GlobalConfig.PortForwardDir
	rulesPath := portfwdDir + "/rules.sh"
	data, err := os.ReadFile(rulesPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("读取端口转发持久化规则失败: %w", err)
	}
	hostIP := getHostIP()
	var lastErr error
	restored := 0
	for _, line := range strings.Split(string(data), "\n") {
		if err := restorePortForwardCommand(line, hostIP); err != nil {
			lastErr = err
			logger.App.Warn("恢复端口转发规则失败", "error", err)
			continue
		}
		if strings.HasPrefix(strings.TrimSpace(line), "iptables ") {
			restored++
		}
	}
	if restored > 0 && lastErr == nil {
		if err := SavePortForwardRules(); err != nil {
			logger.App.Warn("重写端口转发持久化规则失败", "error", err)
		}
	}
	return lastErr
}

// SavePortForwardRules 持久化端口转发规则
func SavePortForwardRules() error {
	portfwdDir := config.GlobalConfig.PortForwardDir
	os.MkdirAll(portfwdDir+"/backups", 0755)

	hostIP := getHostIP()

	// 备份
	utils.ExecShell(fmt.Sprintf(
		"[ -f %s/rules.sh ] && cp %s/rules.sh %s/backups/rules.sh.$(date +%%Y%%m%%d_%%H%%M%%S)",
		utils.ShellSingleQuote(portfwdDir), utils.ShellSingleQuote(portfwdDir), utils.ShellSingleQuote(portfwdDir)))

	// 只保留最近 10 个备份
	utils.ExecShell(fmt.Sprintf(
		"ls -t %s/backups/rules.sh.* 2>/dev/null | tail -n +11 | xargs rm -f 2>/dev/null",
		utils.ShellSingleQuote(portfwdDir)))

	// 导出规则
	script := fmt.Sprintf("#!/bin/bash\n# KVM 端口转发规则 - 自动生成\nHOST_IP=\"%s\"\n\n", hostIP)

	// DNAT 规则 (PREROUTING - 外部流量)
	script += "# === DNAT 转发规则 (PREROUTING - 外部流量) ===\n"
	dnatResult := utils.ExecShellQuiet("iptables -t nat -S PREROUTING 2>/dev/null | grep DNAT")
	if dnatResult.Stdout != "" {
		for _, line := range strings.Split(dnatResult.Stdout, "\n") {
			line = strings.TrimSpace(line)
			if line != "" {
				script += idempotentIPTablesAddLine("iptables -t nat "+line) + "\n"
			}
		}
	}

	// DNAT 规则 (OUTPUT - 宿主机本地流量)
	script += "\n# === DNAT 转发规则 (OUTPUT - 宿主机本地流量) ===\n"
	outputDnatResult := utils.ExecShellQuiet("iptables -t nat -S OUTPUT 2>/dev/null | grep DNAT")
	if outputDnatResult.Stdout != "" {
		for _, line := range strings.Split(outputDnatResult.Stdout, "\n") {
			line = strings.TrimSpace(line)
			if line != "" {
				script += idempotentIPTablesAddLine("iptables -t nat "+line) + "\n"
			}
		}
	}

	// FORWARD 规则
	script += "\n# === FORWARD 放行规则 ===\n"
	fwdResult := utils.ExecShellQuiet("iptables -S FORWARD 2>/dev/null | grep -- '-j ACCEPT' | grep -- '-d ' | grep -- '--dport '")
	if fwdResult.Stdout != "" {
		for _, line := range strings.Split(fwdResult.Stdout, "\n") {
			line = strings.TrimSpace(line)
			if line != "" {
				args := strings.Fields(line)
				destIP := stripIPTablesCIDR(iptablesArgValue(args, "-d"))
				if isVPCManagedIP(destIP) {
					continue
				}
				script += idempotentIPTablesAddLine("iptables "+line) + "\n"
			}
		}
	}

	rulesPath := portfwdDir + "/rules.sh"
	if err := os.WriteFile(rulesPath, []byte(script), 0755); err != nil {
		return fmt.Errorf("保存规则失败: %v", err)
	}

	// 对账 VPC 安全组自动放行规则（同步入站 IP 白名单，清理残留）
	if err := syncSecurityGroupPortForwardRules(); err != nil {
		logger.App.Warn("对账安全组端口转发放行规则失败", "error", err)
	}

	return nil
}
