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

// dnatSRuleSignature 解析 iptables -S 的 DNAT 行：
// 返回完整签名（协议|宿主机端口|来源|目标IP|目标端口）与目标四元组签名（协议|来源|目标IP|目标端口）。
func dnatSRuleSignature(line string) (string, string, bool) {
	args := strings.Fields(strings.TrimSpace(line))
	joined := strings.Join(args, " ")
	if !strings.Contains(joined, " -j DNAT") {
		return "", "", false
	}
	proto := strings.ToLower(strings.TrimSpace(iptablesArgValue(args, "-p")))
	hostPort := strings.TrimSpace(iptablesArgValue(args, "--dport"))
	source := strings.TrimSpace(iptablesArgValue(args, "-s"))
	if source == "" {
		source = "0.0.0.0/0"
	}
	dest := strings.TrimSpace(iptablesArgValue(args, "--to-destination"))
	parts := strings.SplitN(dest, ":", 2)
	if len(parts) < 2 || hostPort == "" || parts[0] == "" || parts[1] == "" {
		return "", "", false
	}
	sub := proto + "|" + source + "|" + parts[0] + "|" + parts[1]
	return proto + "|" + hostPort + "|" + source + "|" + parts[0] + "|" + parts[1], sub, true
}

// forwardAcceptSRuleSignature 解析 iptables -S 的 FORWARD 放行行（端口转发目标），返回目标四元组签名。
func forwardAcceptSRuleSignature(line string) (string, []string, bool) {
	args := strings.Fields(strings.TrimSpace(line))
	joined := strings.Join(args, " ")
	if !strings.Contains(joined, " -j ACCEPT") || !strings.Contains(joined, " --dport ") {
		return "", nil, false
	}
	proto := strings.ToLower(strings.TrimSpace(iptablesArgValue(args, "-p")))
	dport := strings.TrimSpace(iptablesArgValue(args, "--dport"))
	destIP := stripIPTablesCIDR(iptablesArgValue(args, "-d"))
	source := strings.TrimSpace(iptablesArgValue(args, "-s"))
	if source == "" {
		source = "0.0.0.0/0"
	}
	if dport == "" || destIP == "" {
		return "", nil, false
	}
	return proto + "|" + source + "|" + destIP + "|" + dport, args, true
}

// cleanupOrphanPortForwardRules 清理端口转发侧链孤儿规则（随持久化调用）：
// - OUTPUT 链：无 PREROUTING 配对（协议|宿主机端口|来源|目标均一致）的 DNAT 残留，
//   这类残留会让宿主机本地访问旧端口仍被转发到旧目标；
// - FORWARD 链：无任一转发规则指向（协议|来源|目标IP|端口）的 ACCEPT 放行残留，
//   残留放行会让内网主机绕过白名单直连 VM 端口。
func cleanupOrphanPortForwardRules() {
	fullValid := map[string]bool{}
	subValid := map[string]bool{}
	if res := utils.ExecShellQuiet("iptables -t nat -S PREROUTING 2>/dev/null | grep DNAT"); res.Error == nil {
		for _, line := range strings.Split(res.Stdout, "\n") {
			if full, sub, ok := dnatSRuleSignature(line); ok {
				fullValid[full] = true
				subValid[sub] = true
			}
		}
	}
	// OUTPUT 孤儿 DNAT
	if res := utils.ExecShellQuiet("iptables -t nat -S OUTPUT 2>/dev/null | grep DNAT"); res.Error == nil {
		removed := 0
		for _, line := range strings.Split(res.Stdout, "\n") {
			line = strings.TrimSpace(line)
			if line == "" {
				continue
			}
			full, _, ok := dnatSRuleSignature(line)
			if !ok || fullValid[full] {
				continue
			}
			args := strings.Fields(line)
			if len(args) < 3 || args[0] != "-A" || args[1] != "OUTPUT" {
				continue
			}
			deleteArgs := append([]string{"-t", "nat", "-D", "OUTPUT"}, args[2:]...)
			if res := utils.ExecCommandQuiet("iptables", deleteArgs...); res.Error == nil {
				removed++
			}
		}
		if removed > 0 {
			logger.App.Info("已清理 OUTPUT 链孤儿端口转发规则", "count", removed)
		}
	}
	// FORWARD 孤儿放行（VPC 托管目标由 RemoveVPCPortForwardAcceptRules 统一处理，这里跳过）
	if res := utils.ExecShellQuiet("iptables -S FORWARD 2>/dev/null | grep -- '-j ACCEPT' | grep -- '-d ' | grep -- '--dport '"); res.Error == nil {
		removed := 0
		for _, line := range strings.Split(res.Stdout, "\n") {
			line = strings.TrimSpace(line)
			if line == "" {
				continue
			}
			sig, args, ok := forwardAcceptSRuleSignature(line)
			if !ok {
				continue
			}
			destIP := stripIPTablesCIDR(iptablesArgValue(args, "-d"))
			if isVPCManagedIP(destIP) || subValid[sig] {
				continue
			}
			if len(args) < 3 || args[0] != "-A" || args[1] != "FORWARD" {
				continue
			}
			deleteArgs := append([]string{"-D", "FORWARD"}, args[2:]...)
			if res := utils.ExecCommandQuiet("iptables", deleteArgs...); res.Error == nil {
				removed++
			}
		}
		if removed > 0 {
			logger.App.Info("已清理 FORWARD 链孤儿端口转发放行规则", "count", removed)
		}
	}
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

	// 清理 OUTPUT/FORWARD 链孤儿规则（防止历史残留影响宿主机本地访问与内网直连）
	cleanupOrphanPortForwardRules()

	// 对账 VPC 安全组自动放行规则（同步入站 IP 白名单，清理残留）
	if err := syncSecurityGroupPortForwardRules(); err != nil {
		logger.App.Warn("对账安全组端口转发放行规则失败", "error", err)
	}

	return nil
}
