package diagnostics

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"kvm_console/config"
	"kvm_console/logger"
	"kvm_console/service/libvirt_rpc"
	poolpkg "kvm_console/service/storage/pool"
	"kvm_console/utils"
)

// DiagnosticCategory 诊断类别定义
type DiagnosticCategory struct {
	ID          string `json:"id"`
	Label       string `json:"label"`
	Description string `json:"description"`
}

// GetAvailableCategories 返回可用诊断类别
func GetAvailableCategories() []DiagnosticCategory {
	return []DiagnosticCategory{
		{ID: "system", Label: "系统信息", Description: "OS版本、内核、CPU、内存、磁盘占用、uptime、systemd关键服务状态、KVM模块、dmesg尾部"},
		{ID: "vm", Label: "虚拟机", Description: "所有VM列表、每台VM的dumpxml、dominfo"},
		{ID: "ovs", Label: "OVS网络", Description: "ovs-vsctl show、网桥/端口/接口列表、DHCP租约、静态绑定、VPC交换机"},
		{ID: "network", Label: "网络配置", Description: "IP地址、路由表、iptables(filter+nat)、网络桥接列表"},
		{ID: "storage", Label: "存储池", Description: "lsblk、vgs/lvs/pvs、存储池列表"},
		{ID: "logs", Label: "应用日志", Description: "当前 app.log、request.log、cmd.log、libvirt.log 文件"},
		{ID: "panel", Label: "面板配置", Description: "系统设置(脱敏)、面板版本、Go运行时信息"},
	}
}

// collectResult 单个类别收集结果
type collectResult struct {
	Category string `json:"category"`
	Success  bool   `json:"success"`
	Error    string `json:"error,omitempty"`
}

// manifest 诊断清单
type manifest struct {
	Timestamp  string          `json:"timestamp"`
	Hostname   string          `json:"hostname"`
	Version    string          `json:"version"`
	GoVersion  string          `json:"go_version"`
	Categories []string        `json:"categories"`
	Results    []collectResult `json:"results"`
}

// CollectDiagnostics 收集诊断信息并返回 ZIP 字节缓冲
func CollectDiagnostics(categories []string) (*bytes.Buffer, error) {
	if len(categories) == 0 {
		return nil, fmt.Errorf("至少需要选择一个诊断类别")
	}

	buf := new(bytes.Buffer)
	zipWriter := zip.NewWriter(buf)
	defer zipWriter.Close()

	timestamp := time.Now().Format("20060102-150405")
	hostname, _ := os.Hostname()
	if hostname == "" {
		hostname = "unknown"
	}

	rootDir := fmt.Sprintf("qvmconsole-diagnostics-%s/", timestamp)

	m := manifest{
		Timestamp:  timestamp,
		Hostname:   hostname,
		Version:    getBuildVersion(),
		GoVersion:  runtime.Version(),
		Categories: categories,
	}

	catSet := make(map[string]bool)
	for _, c := range categories {
		catSet[strings.TrimSpace(c)] = true
	}

	// system
	if catSet["system"] {
		r := collectSystem(rootDir, zipWriter)
		m.Results = append(m.Results, r)
	}
	// vm
	if catSet["vm"] {
		r := collectVM(rootDir, zipWriter)
		m.Results = append(m.Results, r)
	}
	// ovs
	if catSet["ovs"] {
		r := collectOVS(rootDir, zipWriter)
		m.Results = append(m.Results, r)
	}
	// network
	if catSet["network"] {
		r := collectNetwork(rootDir, zipWriter)
		m.Results = append(m.Results, r)
	}
	// storage
	if catSet["storage"] {
		r := collectStorage(rootDir, zipWriter)
		m.Results = append(m.Results, r)
	}
	// logs
	if catSet["logs"] {
		r := collectLogs(rootDir, zipWriter)
		m.Results = append(m.Results, r)
	}
	// panel
	if catSet["panel"] {
		r := collectPanel(rootDir, zipWriter)
		m.Results = append(m.Results, r)
	}

	// 写入 manifest.json
	manifestData, _ := json.MarshalIndent(m, "", "  ")
	writeZipEntry(zipWriter, rootDir+"manifest.json", manifestData)

	return buf, nil
}

// ── Command helpers ──

func runCmd(entryPath string, zw *zip.Writer, name string, args ...string) {
	result := utils.ExecCommandQuietWithTimeout(name, 15*time.Second, args...)
	content := fmt.Sprintf("# Command: %s %s\n# ExitCode: %d\n\n%s\n",
		name, strings.Join(args, " "), result.ExitCode, result.Stdout)
	if result.Stderr != "" {
		content += fmt.Sprintf("\n# Stderr:\n%s\n", result.Stderr)
	}
	writeZipEntry(zw, entryPath, []byte(content))
}

func runShell(entryPath string, zw *zip.Writer, command string) {
	result := utils.ExecShellQuiet(command)
	content := fmt.Sprintf("# Command: %s\n# ExitCode: %d\n\n%s\n",
		command, result.ExitCode, result.Stdout)
	if result.Stderr != "" {
		content += fmt.Sprintf("\n# Stderr:\n%s\n", result.Stderr)
	}
	writeZipEntry(zw, entryPath, []byte(content))
}

func writeZipEntry(zw *zip.Writer, path string, data []byte) {
	w, err := zw.Create(path)
	if err != nil {
		return
	}
	w.Write(data)
}

func writeZipJSON(zw *zip.Writer, path string, v interface{}) {
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return
	}
	writeZipEntry(zw, path, data)
}

// ── System collection ──

func collectSystem(rootDir string, zw *zip.Writer) collectResult {
	prefix := rootDir + "system/"
	r := collectResult{Category: "system", Success: true}

	runShell(prefix+"os-release.txt", zw, "cat /etc/os-release 2>/dev/null; cat /etc/lsb-release 2>/dev/null; cat /etc/debian_version 2>/dev/null")
	runCmd(prefix+"uname.txt", zw, "uname", "-a")
	runCmd(prefix+"cpu-info.txt", zw, "lscpu")
	runShell(prefix+"memory.txt", zw, "free -h; echo '---'; cat /proc/meminfo | head -30")
	runShell(prefix+"disk-usage.txt", zw, "df -h")
	runShell(prefix+"uptime.txt", zw, "uptime")
	runShell(prefix+"systemd-services.txt", zw, "systemctl is-active libvirtd 2>/dev/null; systemctl is-active openvswitch-switch 2>/dev/null; systemctl is-active openvswitch 2>/dev/null; echo '---'; systemctl list-units --type=service --state=running | head -60")
	runShell(prefix+"kernel-modules.txt", zw, "lsmod | grep -E 'kvm|tun|vhost' 2>/dev/null; echo '---'; cat /sys/module/kvm*/parameters/* 2>/dev/null")
	runShell(prefix+"dmesg.txt", zw, "dmesg | tail -200")

	return r
}

// ── VM collection ──

func collectVM(rootDir string, zw *zip.Writer) collectResult {
	prefix := rootDir + "vm/"
	r := collectResult{Category: "vm", Success: true}

	// VM 列表
	domains, err := libvirt_rpc.ListAllDomainsRPC()
	if err != nil {
		r.Success = false
		r.Error = fmt.Sprintf("获取VM列表失败: %v", err)
		writeZipEntry(zw, prefix+"error.txt", []byte(r.Error))
		return r
	}

	// vm-list.txt
	var vmList strings.Builder
	vmList.WriteString(fmt.Sprintf("Total VMs: %d\n\n", len(domains)))
	for _, d := range domains {
		stateStr, _ := libvirt_rpc.GetDomainStateRPC(d.Name)
		vcpu, maxMemKB, usedMemKB, autostart, _ := libvirt_rpc.GetDomainInfoRPC(d.Name)
		vmList.WriteString(fmt.Sprintf("Name: %s  State: %s  vCPU: %d  MaxMem(MB): %d  UsedMem(MB): %d  Autostart: %v\n",
			d.Name, stateStr, vcpu, maxMemKB/1024, usedMemKB/1024, autostart))
	}
	writeZipEntry(zw, prefix+"vm-list.txt", []byte(vmList.String()))

	// 每台 VM 的 XML 和 info
	for _, d := range domains {
		// dominfo
		vcpu, maxMemKB, usedMemKB, autostart, _ := libvirt_rpc.GetDomainInfoRPC(d.Name)
		stateStr, _ := libvirt_rpc.GetDomainStateRPC(d.Name)
		info := fmt.Sprintf("Name: %s\nState: %s\nvCPU: %d\nMaxMem(KB): %d\nUsedMem(KB): %d\nAutostart: %v\n",
			d.Name, stateStr, vcpu, maxMemKB, usedMemKB, autostart)
		writeZipEntry(zw, prefix+d.Name+"-info.txt", []byte(info))

		// dumpxml (runtime XML)
		xml, err := libvirt_rpc.GetDomainXMLRPC(d.Name, 0)
		if err != nil {
			writeZipEntry(zw, prefix+d.Name+".xml", []byte(fmt.Sprintf("# Error: %v\n", err)))
		} else {
			writeZipEntry(zw, prefix+d.Name+".xml", []byte(xml))
		}
	}

	return r
}

// ── OVS collection ──

func collectOVS(rootDir string, zw *zip.Writer) collectResult {
	prefix := rootDir + "ovs/"
	r := collectResult{Category: "ovs", Success: true}

	runCmd(prefix+"ovs-show.txt", zw, "ovs-vsctl", "show")

	// OVS status (from service)
	status, err := getOVSStatusSafe()
	if err != nil {
		writeZipEntry(zw, prefix+"ovs-status.json", []byte(fmt.Sprintf(`{"error": "%v"}`, err)))
	} else {
		writeZipJSON(zw, prefix+"ovs-status.json", status)
	}

	// OVS ports
	ports, err := getOVSPortsSafe()
	if err != nil {
		writeZipEntry(zw, prefix+"ovs-ports.json", []byte(fmt.Sprintf(`{"error": "%v"}`, err)))
	} else {
		writeZipJSON(zw, prefix+"ovs-ports.json", ports)
	}

	// OVS leases
	leases, err := getOVSLeasesSafe()
	if err != nil {
		writeZipEntry(zw, prefix+"ovs-leases.json", []byte(fmt.Sprintf(`{"error": "%v"}`, err)))
	} else {
		writeZipJSON(zw, prefix+"ovs-leases.json", leases)
	}

	// VPC switches
	vpcSwitches, err := getVPCSwitchesSafe()
	if err != nil {
		writeZipEntry(zw, prefix+"vpc-switches.json", []byte(fmt.Sprintf(`{"error": "%v"}`, err)))
	} else {
		writeZipJSON(zw, prefix+"vpc-switches.json", vpcSwitches)
	}

	// OVS DHCP leases file
	runShell(prefix+"ovs-leases-file.txt", zw, "cat /var/lib/misc/dnsmasq.ovs-br-ovs.leases 2>/dev/null || echo 'file not found'")

	return r
}

// ── Network collection ──

func collectNetwork(rootDir string, zw *zip.Writer) collectResult {
	prefix := rootDir + "network/"
	r := collectResult{Category: "network", Success: true}

	runCmd(prefix+"ip-addr.txt", zw, "ip", "addr", "show")
	runCmd(prefix+"ip-route.txt", zw, "ip", "route", "show", "table", "all")
	runCmd(prefix+"iptables-filter.txt", zw, "iptables", "-L", "-n", "-v")
	runCmd(prefix+"iptables-nat.txt", zw, "iptables", "-t", "nat", "-L", "-n", "-v")
	runShell(prefix+"network-bridges.json", zw, "ip link show type bridge 2>/dev/null; echo '---'; bridge link show 2>/dev/null")

	return r
}

// ── Storage collection ──

func collectStorage(rootDir string, zw *zip.Writer) collectResult {
	prefix := rootDir + "storage/"
	r := collectResult{Category: "storage", Success: true}

	runCmd(prefix+"lsblk.txt", zw, "lsblk", "-o", "NAME,KNAME,PATH,TYPE,SIZE,FSTYPE,MOUNTPOINTS,MODEL,SERIAL")
	runCmd(prefix+"lvm-vgs.txt", zw, "vgs", "--units", "g")
	runCmd(prefix+"lvm-lvs.txt", zw, "lvs", "--units", "g")
	runCmd(prefix+"lvm-pvs.txt", zw, "pvs", "--units", "g")

	// 存储池列表
	pools, err := poolpkg.ListStoragePools()
	if err != nil {
		writeZipEntry(zw, prefix+"storage-pools.json", []byte(fmt.Sprintf(`{"error": "%v"}`, err)))
	} else {
		writeZipJSON(zw, prefix+"storage-pools.json", pools)
	}

	runShell(prefix+"df.txt", zw, "df -h")

	return r
}

// ── Logs collection ──

func collectLogs(rootDir string, zw *zip.Writer) collectResult {
	prefix := rootDir + "logs/"
	r := collectResult{Category: "logs", Success: true}

	logDir := logger.GetLogDir()
	if logDir == "" {
		logDir = config.GlobalConfig.LogDir
	}

	logFiles := []string{"app.log", "request.log", "cmd.log", "libvirt.log"}
	for _, f := range logFiles {
		srcPath := filepath.Join(logDir, f)
		data, err := os.ReadFile(srcPath)
		if err != nil {
			writeZipEntry(zw, prefix+f, []byte(fmt.Sprintf("# Error reading %s: %v\n", f, err)))
			continue
		}
		// Truncate to last 1MB to avoid huge ZIP files
		maxSize := 1024 * 1024
		if len(data) > maxSize {
			truncated := fmt.Sprintf("# File truncated from %d bytes to last %d bytes\n\n", len(data), maxSize)
			truncated += string(data[len(data)-maxSize:])
			writeZipEntry(zw, prefix+f, []byte(truncated))
		} else {
			writeZipEntry(zw, prefix+f, data)
		}
	}

	return r
}

// ── Panel collection ──

func collectPanel(rootDir string, zw *zip.Writer) collectResult {
	prefix := rootDir + "panel/"
	r := collectResult{Category: "panel", Success: true}

	// 面板配置（脱敏处理）
	cfg := config.GlobalConfig
	settingsMap := sanitizedSettings(cfg)
	writeZipJSON(zw, prefix+"settings.json", settingsMap)

	// 版本
	versionInfo := map[string]string{
		"version":    getBuildVersion(),
		"go_version": runtime.Version(),
		"go_os":      runtime.GOOS,
		"go_arch":    runtime.GOARCH,
	}
	writeZipJSON(zw, prefix+"version.txt", versionInfo)

	return r
}

// sanitizedSettings 返回脱敏后的设置
func sanitizedSettings(cfg *config.Config) map[string]interface{} {
	return map[string]interface{}{
		"port":                             cfg.Port,
		"template_dir":                     cfg.TemplateDir,
		"clone_dir":                        cfg.CloneDir,
		"iso_dir":                          cfg.ISODir,
		"default_network":                  cfg.DefaultNetwork,
		"network_backend":                  cfg.NetworkBackend,
		"ovs_bridge":                       cfg.OVSBridge,
		"ovs_uplink":                       cfg.OVSUplink,
		"ovs_dhcp_start":                   cfg.OVSDHCPStart,
		"ovs_dhcp_end":                     cfg.OVSDHCPEnd,
		"subnet_prefix":                    cfg.SubnetPrefix,
		"auto_port_start":                  cfg.AutoPortStart,
		"auto_port_end":                    cfg.AutoPortEnd,
		"host_ip":                          cfg.HostIP,
		"external_nic":                     cfg.ExternalNIC,
		"max_burst_inbound":                cfg.MaxBurstInbound,
		"max_burst_outbound":               cfg.MaxBurstOutbound,
		"rescue_iso":                       cfg.RescueISO,
		"public_base_url":                  cfg.PublicBaseURL,
		"site_title":                       cfg.SiteTitle,
		"development_mode":                 cfg.DevelopmentMode,
		"maintenance_mode":                 cfg.MaintenanceMode,
		"batch_clone_max_concurrency":      cfg.BatchCloneMaxConcurrency,
		"jwt_secret_rotate_hours":          cfg.JWTSecretRotateHours,
		"log_max_backups":                  cfg.LogMaxBackups,
		"log_dir":                          cfg.LogDir,
		"log_level":                        cfg.LogLevel,
		"network_wait_online_disabled":     cfg.NetworkWaitOnlineDisabled,
		"session_fingerprint_enabled":      cfg.SessionFingerprintEnabled,
		"request_filter_enabled":           cfg.RequestFilterEnabled,
		"password_breach_check_enabled":    cfg.PasswordBreachCheckEnabled,
		"smtp_configured":                  cfg.SMTPHost != "",
		"smtp_host":                        maskValue(cfg.SMTPHost),
		"smtp_port":                        cfg.SMTPPort,
		"smtp_username":                    maskValue(cfg.SMTPUsername),
		"smtp_from_name":                   cfg.SMTPFromName,
		"smtp_from_address":                maskValue(cfg.SMTPFromAddress),
		"smtp_security":                    cfg.SMTPSecurity,
		// 以下字段已脱敏移除：JWTSecret, VMCredentialSecret, SecuritySecret, SMTPPasswordEnc
	}
}

// getBuildVersion 返回构建版本号
// 版本号由 ldflags 在构建时注入，跨包不可访问时返回 "dev"
func getBuildVersion() string {
	return "dev"
}

func maskValue(s string) string {
	if s == "" {
		return ""
	}
	if len(s) <= 4 {
		return "***"
	}
	return s[:2] + "***" + s[len(s)-2:]
}

// ── Safe wrappers for service calls (don't fail on errors) ──

type ovsStatusWrapper struct {
	Bridge           string   `json:"bridge"`
	BridgeExists     bool     `json:"bridge_exists"`
	BridgeHasGateway bool     `json:"bridge_has_gateway"`
	GatewayIP        string   `json:"gateway_ip"`
	SubnetCIDR       string   `json:"subnet_cidr"`
	Healthy          bool     `json:"healthy"`
	Issues           []string `json:"issues"`
}

type ovsPortWrapper struct {
	Bridge string            `json:"bridge"`
	Ports  []ovsPortItemWrap `json:"ports"`
}

type ovsPortItemWrap struct {
	Name   string `json:"name"`
	Type   string `json:"type"`
	MAC    string `json:"mac"`
	IP     string `json:"ip"`
	VMName string `json:"vm_name"`
}

type ovsLeaseWrapper struct {
	StaticHosts []ovsHostWrap  `json:"static_hosts"`
	DHCPLeases  []ovsLeaseWrap `json:"dhcp_leases"`
}

type ovsHostWrap struct {
	VMName string `json:"vm_name"`
	MAC    string `json:"mac"`
	IP     string `json:"ip"`
}

type ovsLeaseWrap struct {
	MAC      string `json:"mac"`
	IP       string `json:"ip"`
	Hostname string `json:"hostname"`
}

func getOVSStatusSafe() (*ovsStatusWrapper, error) {
	// Use local minimal implementation to avoid circular imports
	result := utils.ExecCommandQuiet("ovs-vsctl", "show")
	bridge := config.GlobalConfig.OVSBridge
	w := &ovsStatusWrapper{
		Bridge:       bridge,
		BridgeExists: result.Error == nil && strings.Contains(result.Stdout, bridge),
		GatewayIP:    config.GlobalConfig.SubnetPrefix + ".1",
		SubnetCIDR:   config.GlobalConfig.SubnetPrefix + ".0/24",
	}

	addrResult := utils.ExecCommandQuiet("ip", "-4", "addr", "show", "dev", bridge)
	w.BridgeHasGateway = addrResult.Error == nil && strings.Contains(addrResult.Stdout, w.GatewayIP+"/24")

	sysctlResult := utils.ExecCommandQuiet("sysctl", "-n", "net.ipv4.ip_forward")
	ipForward := sysctlResult.Error == nil && strings.TrimSpace(sysctlResult.Stdout) == "1"

	if !w.BridgeExists {
		w.Issues = append(w.Issues, "OVS网桥不存在")
	}
	if !w.BridgeHasGateway {
		w.Issues = append(w.Issues, "OVS网桥未配置网关IP")
	}
	if !ipForward {
		w.Issues = append(w.Issues, "IPv4转发未开启")
	}
	w.Healthy = len(w.Issues) == 0

	return w, nil
}

func getOVSPortsSafe() (*ovsPortWrapper, error) {
	bridge := config.GlobalConfig.OVSBridge
	w := &ovsPortWrapper{Bridge: bridge}

	result := utils.ExecCommandQuiet("ovs-vsctl", "--format=csv", "--data=bare", "--no-heading", "--columns=name,type,mac_in_use", "list", "Interface")
	if result.Error != nil {
		return w, nil
	}

	lines := strings.Split(result.Stdout, "\n")
	for _, line := range lines {
		parts := strings.Split(line, ",")
		if len(parts) < 1 {
			continue
		}
		port := ovsPortItemWrap{Name: strings.TrimSpace(parts[0])}
		if len(parts) >= 2 {
			port.Type = strings.TrimSpace(parts[1])
		}
		if len(parts) >= 3 {
			port.MAC = strings.TrimSpace(parts[2])
		}
		w.Ports = append(w.Ports, port)
	}

	return w, nil
}

func getOVSLeasesSafe() (*ovsLeaseWrapper, error) {
	w := &ovsLeaseWrapper{}

	// static hosts from config
	staticResult := utils.ExecCommandQuiet("cat", "/etc/dnsmasq.d/ovs-static.conf")
	if staticResult.Error == nil {
		for _, line := range strings.Split(staticResult.Stdout, "\n") {
			line = strings.TrimSpace(line)
			if line == "" || strings.HasPrefix(line, "#") || !strings.Contains(line, ",") {
				continue
			}
			parts := strings.SplitN(line, ",", 3)
			if len(parts) >= 2 {
				host := ovsHostWrap{MAC: strings.TrimSpace(parts[0]), IP: strings.TrimSpace(parts[1])}
				if len(parts) >= 3 {
					host.VMName = strings.TrimSpace(parts[2])
				}
				w.StaticHosts = append(w.StaticHosts, host)
			}
		}
	}

	// DHCP leases
	leaseResult := utils.ExecCommandQuiet("cat", "/var/lib/misc/dnsmasq.ovs-"+config.GlobalConfig.OVSBridge+".leases")
	if leaseResult.Error == nil {
		for _, line := range strings.Split(leaseResult.Stdout, "\n") {
			line = strings.TrimSpace(line)
			if line == "" || strings.HasPrefix(line, "#") {
				continue
			}
			fields := strings.Fields(line)
			if len(fields) >= 4 {
				w.DHCPLeases = append(w.DHCPLeases, ovsLeaseWrap{
					MAC:      strings.TrimSpace(fields[1]),
					IP:       strings.TrimSpace(fields[2]),
					Hostname: strings.TrimSpace(fields[3]),
				})
			}
		}
	}

	return w, nil
}

type vpcSwitchWrap struct {
	ID      uint   `json:"id"`
	Name    string `json:"name"`
	CIDR    string `json:"cidr"`
	Gateway string `json:"gateway"`
	VLANID  int    `json:"vlan_id"`
	Bridge  string `json:"bridge"`
}

func getVPCSwitchesSafe() ([]vpcSwitchWrap, error) {
	// Query from DB model directly to avoid circular imports
	result := utils.ExecCommandQuiet("ovs-vsctl", "--format=json", "list", "Bridge")
	if result.Error != nil {
		return nil, result.Error
	}

	// Simple approach: list bridges from ovs-vsctl that match VPC patterns
	var switches []vpcSwitchWrap
	brResult := utils.ExecCommandQuiet("ovs-vsctl", "list-br")
	if brResult.Error != nil {
		return switches, nil
	}

	for _, br := range strings.Split(brResult.Stdout, "\n") {
		br = strings.TrimSpace(br)
		if br == "" {
			continue
		}
		// VPC bridges typically start with vpc- or vbr-
		if strings.HasPrefix(br, "vpc-") || strings.HasPrefix(br, "vbr-") {
			switches = append(switches, vpcSwitchWrap{
				Name:   br,
				Bridge: br,
			})
		}
	}

	return switches, nil
}
