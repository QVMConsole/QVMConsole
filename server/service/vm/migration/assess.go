package migration

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"kvm_console/model"
	"kvm_console/service"
	"kvm_console/utils"
)

func AssessLiveMigration(ctx context.Context, node model.HostNode, vmName string, enableCPUThrottle bool, cpuThrottlePercent int) (*MigrationLiveAssessment, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	percent := normalizeMigrationCPUThrottlePercent(cpuThrottlePercent)
	assessment := &MigrationLiveAssessment{
		Allowed:            true,
		CPUThrottlePercent: percent,
		DirtyRateStats:     map[string]string{},
		Dommemstat:         map[string]int64{},
	}
	speed, seconds, err := runMigrationSpeedTest(ctx, node)
	if err != nil {
		return nil, fmt.Errorf("热迁移线路测速失败: %w", err)
	}
	assessment.AverageBandwidthMiB = speed
	assessment.SpeedTestSeconds = seconds
	dirtyRate, stats, err := readDomainDirtyRateMiB(vmName)
	if err != nil {
		return nil, fmt.Errorf("读取热迁移脏页速率失败: %w", err)
	}
	assessment.DirtyRateMiB = dirtyRate
	assessment.DirtyRateStats = stats
	assessment.Dommemstat = readDomainMemstat(vmName)
	kvmAvailable, pageFaultRate, message := readKVMPageFaultRate()
	assessment.KVMStatAvailable = kvmAvailable
	assessment.KVMPageFaultRate = pageFaultRate
	assessment.KVMStatMessage = message
	if !kvmAvailable {
		assessment.Warnings = append(assessment.Warnings, "kvm_stat 不可用，已仅使用 libvirt dirty-rate 作为热迁移判断依据")
	}
	if speed <= 0 {
		assessment.Allowed = false
		assessment.BlockReason = "热迁移线路测速结果无效，无法判断迁移风险"
		return assessment, nil
	}
	assessment.DirtyRateRatio = dirtyRate / speed
	assessment.DirtyRateRatioPercent = assessment.DirtyRateRatio * 100
	if assessment.DirtyRateRatio >= liveMigrationDirtyRateBlockRatio {
		assessment.Allowed = false
		assessment.BlockReason = fmt.Sprintf("热迁移风险过高：脏页速率 %.2f MiB/s，占平均带宽 %.2f MiB/s 的 %.1f%%，已达到或超过 50%% 阈值",
			dirtyRate, speed, assessment.DirtyRateRatioPercent)
		return assessment, nil
	}
	if assessment.DirtyRateRatio >= liveMigrationDirtyRateThrottleRatio {
		assessment.RequiresCPUThrottle = true
		assessment.CPUThrottleEnabled = true
		assessment.CPUThrottlePercent = percent
		assessment.Warnings = append(assessment.Warnings, fmt.Sprintf("脏页速率占平均带宽 %.1f%%，迁移时将强制限制 VM CPU 使用率为 %d%%", assessment.DirtyRateRatioPercent, percent))
		return assessment, nil
	}
	if enableCPUThrottle {
		assessment.CPUThrottleEnabled = true
		assessment.CPUThrottlePercent = percent
		assessment.Warnings = append(assessment.Warnings, fmt.Sprintf("已启用迁移 CPU 限制，迁移时 VM CPU 使用率将限制为 %d%%", percent))
	}
	return assessment, nil
}

func normalizeMigrationCPUThrottlePercent(value int) int {
	if value <= 0 {
		return defaultMigrationCPUThrottlePercent
	}
	if value < 10 {
		return 10
	}
	if value > 100 {
		return 100
	}
	return value
}

func runMigrationSpeedTest(ctx context.Context, node model.HostNode) (float64, float64, error) {
	tmp, err := os.CreateTemp("", "kvm-console-migration-speed-*.bin")
	if err != nil {
		return 0, 0, err
	}
	tmpPath := tmp.Name()
	defer os.Remove(tmpPath)
	if err := tmp.Truncate(100 * 1024 * 1024); err != nil {
		_ = tmp.Close()
		return 0, 0, err
	}
	if err := tmp.Close(); err != nil {
		return 0, 0, err
	}
	sourceIP, err := localAddressForRemote(node.SSHHost)
	if err != nil {
		return 0, 0, err
	}
	listener, err := net.Listen("tcp", net.JoinHostPort(sourceIP, "0"))
	if err != nil {
		return 0, 0, err
	}
	defer listener.Close()
	mux := http.NewServeMux()
	token := "speed-" + strconv.FormatInt(time.Now().UnixNano(), 10)
	mux.HandleFunc("/"+token, func(w http.ResponseWriter, r *http.Request) {
		http.ServeFile(w, r, tmpPath)
	})
	server := &http.Server{Handler: mux}
	defer server.Close()
	go func() {
		defer utils.RecoverAndLog("migration-speed-test")
		_ = server.Serve(listener)
	}()
	url := "http://" + net.JoinHostPort(sourceIP, strconv.Itoa(listener.Addr().(*net.TCPAddr).Port)) + "/" + token
	cmd := "curl -fsS --max-time 180 -w '%{speed_download} %{time_total}' -o /dev/null " + utils.ShellSingleQuote(url)
	out, err := service.RemoteSSHCommand(ctx, node, cmd, 4*time.Minute)
	if err != nil {
		return 0, 0, err
	}
	fields := strings.Fields(strings.TrimSpace(out))
	if len(fields) < 2 {
		return 0, 0, fmt.Errorf("测速输出格式无效: %s", strings.TrimSpace(out))
	}
	bytesPerSecond, err := strconv.ParseFloat(fields[0], 64)
	if err != nil {
		return 0, 0, fmt.Errorf("解析测速速度失败: %w", err)
	}
	seconds, _ := strconv.ParseFloat(fields[1], 64)
	return bytesPerSecond / 1024 / 1024, seconds, nil
}

func localAddressForRemote(remoteHost string) (string, error) {
	remoteHost = strings.Trim(remoteHost, "[]")
	if remoteHost == "" {
		return "", fmt.Errorf("目标 SSH 地址为空")
	}
	conn, err := net.DialTimeout("udp", net.JoinHostPort(remoteHost, "22"), 5*time.Second)
	if err != nil {
		return "", err
	}
	defer conn.Close()
	addr, ok := conn.LocalAddr().(*net.UDPAddr)
	if !ok || addr.IP == nil {
		return "", fmt.Errorf("无法识别源节点出口地址")
	}
	return addr.IP.String(), nil
}

func readDomainDirtyRateMiB(vmName string) (float64, map[string]string, error) {
	calc := utils.ExecCommandWithTimeout("virsh", 90*time.Second, "domdirtyrate-calc", vmName, "--seconds", "5")
	if calc.Error != nil {
		message := firstNonEmpty(calc.Stderr, calc.Error.Error())
		if !strings.Contains(strings.ToLower(message), "already being measured") {
			return 0, nil, fmt.Errorf("%s", message)
		}
	}
	deadline := time.Now().Add(70 * time.Second)
	var lastStats map[string]string
	for {
		stats, dirtyRate, hasRate, status, _, err := readDomainDirtyRateStats(vmName)
		if err != nil {
			return 0, stats, err
		}
		lastStats = stats
		if hasRate && status != 1 {
			return dirtyRate, stats, nil
		}
		if time.Now().After(deadline) {
			return 0, lastStats, fmt.Errorf("dirty-rate 测量未完成，请稍后重试")
		}
		time.Sleep(2 * time.Second)
	}
}

func readDomainDirtyRateStats(vmName string) (map[string]string, float64, bool, int, bool, error) {
	statsResult := utils.ExecCommandWithTimeout("virsh", 30*time.Second, "domstats", "--dirtyrate", vmName)
	if statsResult.Error != nil {
		return nil, 0, false, 0, false, fmt.Errorf("%s", firstNonEmpty(statsResult.Stderr, statsResult.Error.Error()))
	}
	stats := map[string]string{}
	var dirtyRate float64
	foundDirtyRate := false
	var status int
	foundStatus := false
	for _, line := range strings.Split(statsResult.Stdout, "\n") {
		line = strings.TrimSpace(line)
		if !strings.Contains(line, "=") {
			continue
		}
		parts := strings.SplitN(line, "=", 2)
		key := strings.TrimSpace(parts[0])
		value := strings.Trim(strings.TrimSpace(parts[1]), `"'`)
		if !strings.Contains(strings.ToLower(key), "dirtyrate") {
			continue
		}
		stats[key] = value
		if strings.EqualFold(key, "dirtyrate.calc_status") {
			if parsed, err := strconv.Atoi(value); err == nil {
				status = parsed
				foundStatus = true
			}
			continue
		}
		if isDirtyRateValueKey(key) {
			if parsed, err := strconv.ParseFloat(value, 64); err == nil {
				foundDirtyRate = true
				if parsed > dirtyRate {
					dirtyRate = parsed
				}
			}
		}
	}
	if !foundDirtyRate {
		return stats, 0, false, status, foundStatus, nil
	}
	return stats, dirtyRate, true, status, foundStatus, nil
}

func isDirtyRateValueKey(key string) bool {
	key = strings.ToLower(key)
	return strings.HasSuffix(key, "megabytes_per_second") || strings.HasSuffix(key, "mib_per_second")
}

func readDomainMemstat(vmName string) map[string]int64 {
	result := utils.ExecCommandWithTimeout("virsh", 15*time.Second, "dommemstat", vmName)
	stats := map[string]int64{}
	if result.Error != nil {
		return stats
	}
	for _, line := range strings.Split(result.Stdout, "\n") {
		fields := strings.Fields(strings.TrimSpace(line))
		if len(fields) != 2 {
			continue
		}
		if value, err := strconv.ParseInt(fields[1], 10, 64); err == nil {
			stats[fields[0]] = value
		}
	}
	return stats
}

func readKVMPageFaultRate() (bool, float64, string) {
	cmd := "KVM_STAT=$(command -v kvm_stat || find /usr/lib/linux-tools -name kvm_stat -type f 2>/dev/null | sort | tail -1); " +
		"test -n \"$KVM_STAT\" || exit 7; timeout 5 \"$KVM_STAT\" -1 2>/dev/null || timeout 5 \"$KVM_STAT\" 2>/dev/null"
	result := utils.ExecShell(cmd)
	if result.Error != nil {
		return false, 0, firstNonEmpty(strings.TrimSpace(result.Stderr), "未找到可用 kvm_stat")
	}
	for _, line := range strings.Split(result.Stdout, "\n") {
		if !strings.Contains(strings.ToLower(line), "kvm_page_fault") {
			continue
		}
		if value, ok := lastFloatInText(line); ok {
			return true, value, strings.TrimSpace(line)
		}
		return true, 0, strings.TrimSpace(line)
	}
	return true, 0, "kvm_stat 未返回 kvm_page_fault 行"
}

func lastFloatInText(text string) (float64, bool) {
	replacer := strings.NewReplacer(",", " ", ":", " ", "=", " ", "/s", " ")
	fields := strings.Fields(replacer.Replace(text))
	for i := len(fields) - 1; i >= 0; i-- {
		if value, err := strconv.ParseFloat(fields[i], 64); err == nil {
			return value, true
		}
	}
	return 0, false
}

// ---------- CPU throttle ----------

type migrationCPUThrottleRestore struct {
	Previous  map[string]string
	PeriodKey string
	QuotaKey  string
}

func applyMigrationCPUThrottle(vmName string, percent int) (*migrationCPUThrottleRestore, error) {
	percent = normalizeMigrationCPUThrottlePercent(percent)
	current := parseVirshSchedInfo(utils.ExecCommand("virsh", "schedinfo", vmName).Stdout)
	vcpus := 1
	if info := utils.ExecCommand("virsh", "dominfo", vmName); info.Error == nil {
		if parsed := parseInfoInt(info.Stdout, "CPU(s):"); parsed > 0 {
			vcpus = parsed
		}
	}
	period := 100000
	quota := period * vcpus * percent / 100
	if err := setVirshSchedInfo(vmName, "global_period", strconv.Itoa(period), "global_quota", strconv.Itoa(quota)); err != nil {
		if fallbackErr := setVirshSchedInfo(vmName, "vcpu_period", strconv.Itoa(period), "vcpu_quota", strconv.Itoa(quota)); fallbackErr != nil {
			return nil, err
		}
		return &migrationCPUThrottleRestore{Previous: current, PeriodKey: "vcpu_period", QuotaKey: "vcpu_quota"}, nil
	}
	return &migrationCPUThrottleRestore{Previous: current, PeriodKey: "global_period", QuotaKey: "global_quota"}, nil
}

func parseVirshSchedInfo(output string) map[string]string {
	values := map[string]string{}
	for _, line := range strings.Split(output, "\n") {
		parts := strings.SplitN(line, ":", 2)
		if len(parts) != 2 {
			continue
		}
		values[strings.TrimSpace(parts[0])] = strings.TrimSpace(parts[1])
	}
	return values
}

func setVirshSchedInfo(vmName, periodKey, periodValue, quotaKey, quotaValue string) error {
	result := utils.ExecCommand("virsh", "schedinfo", vmName, "--live", "--set", periodKey+"="+periodValue, "--set", quotaKey+"="+quotaValue)
	if result.Error != nil {
		return fmt.Errorf("%s", firstNonEmpty(result.Stderr, result.Error.Error()))
	}
	return nil
}

func restoreVirshSchedInfo(vmName string, previous map[string]string, periodKey, quotaKey string) error {
	periodValue := firstNonEmpty(previous[periodKey], "0")
	quotaValue := firstNonEmpty(previous[quotaKey], "0")
	return setVirshSchedInfo(vmName, periodKey, periodValue, quotaKey, quotaValue)
}

func (restore *migrationCPUThrottleRestore) Restore(ctx context.Context, node model.HostNode, vmName string) error {
	if restore == nil {
		return nil
	}
	periodValue := firstNonEmpty(restore.Previous[restore.PeriodKey], "0")
	quotaValue := firstNonEmpty(restore.Previous[restore.QuotaKey], "0")
	if err := setVirshSchedInfo(vmName, restore.PeriodKey, periodValue, restore.QuotaKey, quotaValue); err == nil {
		return nil
	}
	cmd := "virsh schedinfo " + utils.ShellSingleQuote(vmName) + " --live --set " +
		utils.ShellSingleQuote(restore.PeriodKey+"="+periodValue) + " --set " + utils.ShellSingleQuote(restore.QuotaKey+"="+quotaValue)
	if _, err := service.RemoteSSHCommand(ctx, node, cmd, 30*time.Second); err != nil {
		return err
	}
	return nil
}

// parseInfoInt 从 virsh dominfo 输出中解析整数值
func parseInfoInt(output, key string) int {
	lines := strings.Split(output, "\n")
	for _, line := range lines {
		if strings.Contains(line, key) {
			parts := strings.Fields(line)
			if len(parts) >= 2 {
				valStr := parts[len(parts)-2]
				if val, err := strconv.Atoi(valStr); err == nil {
					return val
				}
				valStr = parts[len(parts)-1]
				if val, err := strconv.Atoi(valStr); err == nil {
					return val
				}
			}
		}
	}
	return 0
}
