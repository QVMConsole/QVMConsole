package config

import (
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

const (
	defaultMaintenanceServiceUnits = "kvm-console.service,libvirtd.service,libvirtd.socket,libvirtd-ro.socket,libvirtd-admin.socket"
	DefaultISODir                  = "/var/lib/libvirt/images/ISO"
	DefaultSiteTitle               = "QVMConsole"
	defaultJWTSecret               = "kvm-console-secret-key-change-me"
)

// Config 全局配置
type Config struct {
	// 服务端口
	Port int `json:"port"`
	// 数据库路径
	DBPath string `json:"db_path"`
	// JWT 密钥
	JWTSecret string `json:"jwt_secret"`
	// 虚拟机凭据加密密钥
	VMCredentialSecret string `json:"vm_credential_secret"`
	// 账户安全加密密钥
	SecuritySecret string `json:"security_secret"`
	// 旧的 SecuritySecret（过渡期兼容）
	LegacySecuritySecret string `json:"-"`
	// JWT 过期时间（小时）
	JWTExpireHours int `json:"jwt_expire_hours"`
	// JWT 密钥自动轮换间隔（小时，0=禁用）
	JWTSecretRotateHours int `json:"jwt_secret_rotate_hours"`
	// API 限频：公开接口每 IP 每分钟最大请求数（0=不禁用）
	RateLimitPublicPerMin int `json:"rate_limit_public_per_min"`
	// API 限频：认证接口每 IP 每分钟最大请求数（0=不禁用）
	RateLimitAuthPerMin int `json:"rate_limit_auth_per_min"`
	// CORS 允许的来源（逗号分隔，留空=允许所有 *）
	CORSAllowedOrigins string `json:"cors_allowed_origins"`
	// 可信代理 IP 列表（逗号分隔，用于解析 X-Forwarded-For）
	TrustedProxies []string `json:"trusted_proxies"`
	// 模板目录
	TemplateDir string `json:"template_dir"`
	// 模板导入临时目录
	TemplateImportDir string `json:"template_import_dir"`
	// 模板导出目录
	TemplateExportDir string `json:"template_export_dir"`
	// 克隆磁盘目录
	CloneDir string `json:"clone_dir"`
	// 全局 ISO 目录
	ISODir string `json:"iso_dir"`
	// 默认网络
	DefaultNetwork string `json:"default_network"`
	// 网络后端：ovs
	NetworkBackend string `json:"network_backend"`
	// OVS 网桥名称
	OVSBridge string `json:"ovs_bridge"`
	// OVS NAT 出口网卡，留空自动检测默认路由
	OVSUplink string `json:"ovs_uplink"`
	// OVS DHCP 起始地址
	OVSDHCPStart string `json:"ovs_dhcp_start"`
	// OVS DHCP 结束地址
	OVSDHCPEnd string `json:"ovs_dhcp_end"`
	// 网段前缀
	SubnetPrefix string `json:"subnet_prefix"`
	// 自动端口分配范围
	AutoPortStart int `json:"auto_port_start"`
	AutoPortEnd   int `json:"auto_port_end"`
	// 端口转发持久化目录
	PortForwardDir string `json:"port_forward_dir"`
	// VM 权限配置目录
	VMAccessDir string `json:"vm_access_dir"`
	// 默认管理员账号
	DefaultAdminUser string `json:"default_admin_user"`
	DefaultAdminPass string `json:"default_admin_pass"`
	// 宿主机外网 IP（端口转发用，留空自动检测）
	HostIP string `json:"host_ip"`
	// 外网网卡名称（如 eth0、ens33，留空自动检测）
	ExternalNIC string `json:"external_nic"`
	// 外网最大下行速率（Mbps），作为全局限速总带宽，所有VM/VPC交换机均分，有效值=配置-5Mbps，0=不限制
	MaxBurstInbound int `json:"max_burst_inbound"`
	// 外网最大上行速率（Mbps），作为全局限速总带宽，所有VM/VPC交换机均分，有效值=配置-5Mbps，0=不限制
	MaxBurstOutbound int `json:"max_burst_outbound"`
	// 救援系统 ISO 路径
	RescueISO string `json:"rescue_iso"`
	// 创建虚拟机时"SPICE 开关"的默认初始值（与 VNC 共存，默认本地监听）；每台 VM 可在高级选项覆盖
	SpiceEnabledByDefault bool `json:"spice_enabled_by_default"`
	// 面板对外访问地址，用于邮件中的跳转链接
	PublicBaseURL string `json:"public_base_url"`
	// 网站标题，用于登录页、浏览器标签页等展示
	SiteTitle string `json:"site_title"`
	// 开发环境模式，启用后绕过安全验证
	DevelopmentMode bool `json:"development_mode"`
	// systemd 中当前面板服务的 unit 名称
	ServiceUnitName string `json:"service_unit_name"`
	// 维护模式，启用后阻止 VM 启动类操作
	MaintenanceMode bool `json:"maintenance_mode"`
	// 维护模式需要停用的 systemd units，逗号或换行分隔
	MaintenanceServiceUnits string `json:"maintenance_service_units"`
	// 维护模式关闭 VM 时的优雅关机等待时间（秒）
	MaintenanceVMShutdownTimeoutSeconds int `json:"maintenance_vm_shutdown_timeout_seconds"`
	// SMTP 配置
	SMTPHost           string `json:"smtp_host"`
	SMTPPort           int    `json:"smtp_port"`
	SMTPUsername       string `json:"smtp_username"`
	SMTPPasswordEnc    string `json:"smtp_password_enc"`
	SMTPFromName       string `json:"smtp_from_name"`
	SMTPFromAddress    string `json:"smtp_from_address"`
	SMTPSecurity       string `json:"smtp_security"`
	SMTPTimeoutSeconds int    `json:"smtp_timeout_seconds"`
	// 动态内存调度配置
	DynamicMemorySchedulerEnabled         bool `json:"dynamic_memory_scheduler_enabled"`
	DynamicMemoryIntervalSeconds          int  `json:"dynamic_memory_interval_seconds"`
	DynamicMemoryHostReserveMB            int  `json:"dynamic_memory_host_reserve_mb"`
	DynamicMemoryHostReservePercent       int  `json:"dynamic_memory_host_reserve_percent"`
	DynamicMemoryIncreaseThresholdPercent int  `json:"dynamic_memory_increase_threshold_percent"`
	DynamicMemoryReclaimThresholdPercent  int  `json:"dynamic_memory_reclaim_threshold_percent"`
	DynamicMemoryCooldownSeconds          int  `json:"dynamic_memory_cooldown_seconds"`
	DynamicMemoryObservationHours         int  `json:"dynamic_memory_observation_hours"`
	SchedulerEventRetentionHours          int  `json:"scheduler_event_retention_hours"`
	// VPC 逻辑交换机配置
	VPCSubnetPrefix string `json:"vpc_subnet_prefix"`
	VPCVLANStart    int    `json:"vpc_vlan_start"`
	VPCVLANEnd      int    `json:"vpc_vlan_end"`
	VPCDNS          string `json:"vpc_dns"`
	VPCACLTable     string `json:"vpc_acl_table"`
	// 网络抓包配置
	NetworkCaptureDir            string `json:"network_capture_dir"`
	NetworkCaptureDefaultSeconds int    `json:"network_capture_default_seconds"`
	NetworkCaptureMaxSeconds     int    `json:"network_capture_max_seconds"`
	NetworkCaptureMaxMB          int    `json:"network_capture_max_mb"`
	NetworkCaptureMaxPackets     int    `json:"network_capture_max_packets"`
	// 端口转发 HTTP 探测
	PortForwardHTTPProbeEnabled         bool `json:"port_forward_http_probe_enabled"`
	PortForwardHTTPProbeIntervalMinutes int  `json:"port_forward_http_probe_interval_minutes"`
	PortForwardHTTPProbeTimeoutSeconds  int  `json:"port_forward_http_probe_timeout_seconds"`
	// 虚拟机磁盘 IOPS 默认限制（0 表示不限制）
	DefaultDiskIOPSTotal int `json:"default_disk_iops_total"` // 默认总 IOPS 限制
	DefaultDiskIOPSRead  int `json:"default_disk_iops_read"`  // 默认读 IOPS 限制
	DefaultDiskIOPSWrite int `json:"default_disk_iops_write"` // 默认写 IOPS 限制
	// 批量克隆最大同时克隆数量
	BatchCloneMaxConcurrency int `json:"batch_clone_max_concurrency"`
	// 是否使用 go-libvirt RPC（默认 true，关闭后降级为 virsh 命令行）
	UseGoLibvirt bool `json:"use_go_libvirt"`
	// 日志配置
	LogDir          string `json:"log_dir"`
	LogLevel        string `json:"log_level"`
	LogMaxDays      int    `json:"log_max_days"`
	LogCompress     bool   `json:"log_compress"`
	LogConsole      bool   `json:"log_console"`
	LogConsoleTypes string `json:"log_console_types"` // 终端输出的日志类型，逗号分隔：app,request,cmd,libvirt
	LogConsoleLevel string `json:"log_console_level"` // 终端输出的日志级别（可独立于文件级别）
	LogMaxSizeMB    int    `json:"log_max_size_mb"`
	LogMaxBackups   int    `json:"log_max_backups"` // 日志最大归档备份数（0=不限制）
	// 禁用网络等待就绪检测（解决 OVS 桥接后开机卡 systemd-networkd-wait-online.service）
	NetworkWaitOnlineDisabled bool `json:"network_wait_online_disabled"`
	// 请求过滤开关
	RequestFilterEnabled bool `json:"request_filter_enabled"`
	// API 请求体最大大小（MB）
	APIMaxBodySizeMB int `json:"api_max_body_size_mb"`
	// 错误响应中是否包含详细错误信息
	ErrorDetailInResponse bool `json:"error_detail_in_response"`
	// 会话指纹绑定开关（默认开启）
	SessionFingerprintEnabled bool `json:"session_fingerprint_enabled"`
	// 泄露密码检测开关（默认开启，关闭后跳过所有密码校验）
	PasswordBreachCheckEnabled bool `json:"password_breach_check_enabled"`
}

// GlobalConfig 全局配置实例
var GlobalConfig *Config

// loadEnvFile 启动时加载 .env 文件内容到环境变量
// 仅在对应环境变量未设置时加载（环境变量优先级高于 .env 文件）
func loadEnvFile() {
	envPath := EnvFilePath()
	content, err := os.ReadFile(envPath)
	if err != nil {
		return
	}
	for _, line := range strings.Split(string(content), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if idx := strings.IndexByte(line, '='); idx > 0 {
			key := strings.TrimSpace(line[:idx])
			val := strings.TrimSpace(line[idx+1:])
			// 去掉可选引号
			if len(val) >= 2 && (val[0] == '"' || val[0] == '\'') && val[len(val)-1] == val[0] {
				val = val[1 : len(val)-1]
			}
			// 仅在环境变量未设置时从 .env 加载（环境变量优先）
			if os.Getenv(key) == "" {
				os.Setenv(key, val)
			}
		}
	}
}

// Init 初始化配置
func Init() {
	// 启动时从 .env 文件加载持久化的配置（环境变量优先）
	loadEnvFile()

	templateDir := getEnv("KVM_TEMPLATE_DIR", "/var/lib/libvirt/images/templates")
	GlobalConfig = &Config{
		Port:                                  getEnvInt("KVM_PORT", 8080),
		DBPath:                                getEnv("KVM_DB_PATH", "./data/kvm_console.db"),
		JWTSecret:                             getEnv("KVM_JWT_SECRET", defaultJWTSecret),
		VMCredentialSecret:                    getEnv("KVM_VM_CREDENTIAL_SECRET", ""),
		SecuritySecret:                        getEnv("KVM_SECURITY_SECRET", ""),
		JWTExpireHours:                        getEnvInt("KVM_JWT_EXPIRE_HOURS", 24),
		JWTSecretRotateHours:                  getEnvInt("KVM_JWT_SECRET_ROTATE_HOURS", 24),
		TemplateDir:                           templateDir,
		TemplateImportDir:                     getEnv("KVM_TEMPLATE_IMPORT_DIR", filepath.Join(templateDir, "_imports")),
		TemplateExportDir:                     getEnv("KVM_TEMPLATE_EXPORT_DIR", filepath.Join(templateDir, "_exports")),
		CloneDir:                              getEnv("KVM_CLONE_DIR", "/var/lib/libvirt/images"),
		ISODir:                                getEnv("KVM_ISO_DIR", DefaultISODir),
		DefaultNetwork:                        getEnv("KVM_DEFAULT_NETWORK", "default"),
		NetworkBackend:                        getEnv("KVM_NETWORK_BACKEND", "ovs"),
		OVSBridge:                             getEnv("KVM_OVS_BRIDGE", "br-ovs"),
		OVSUplink:                             getEnv("KVM_OVS_UPLINK", ""),
		OVSDHCPStart:                          getEnv("KVM_OVS_DHCP_START", ""),
		OVSDHCPEnd:                            getEnv("KVM_OVS_DHCP_END", ""),
		SubnetPrefix:                          getEnv("KVM_SUBNET_PREFIX", "192.168.122"),
		AutoPortStart:                         getEnvInt("KVM_AUTO_PORT_START", 10000),
		AutoPortEnd:                           getEnvInt("KVM_AUTO_PORT_END", 20000),
		PortForwardDir:                        getEnv("KVM_PORTFORWARD_DIR", "/etc/kvm-portforward"),
		VMAccessDir:                           getEnv("KVM_VM_ACCESS_DIR", "/etc/libvirt/vm-access"),
		DefaultAdminUser:                      getEnv("KVM_ADMIN_USER", "admin"),
		DefaultAdminPass:                      getEnv("KVM_ADMIN_PASS", "admin123"),
		HostIP:                                getEnv("KVM_HOST_IP", ""),
		ExternalNIC:                           getEnv("KVM_EXTERNAL_NIC", ""),
		MaxBurstInbound:                       getEnvInt("KVM_MAX_BURST_INBOUND", 0),
		MaxBurstOutbound:                      getEnvInt("KVM_MAX_BURST_OUTBOUND", 0),
		RescueISO:                             getEnv("KVM_RESCUE_ISO", ""),
		SpiceEnabledByDefault:                 getEnvBool("KVM_SPICE_ENABLED_BY_DEFAULT", false),
		PublicBaseURL:                         getEnv("KVM_PUBLIC_BASE_URL", ""),
		SiteTitle:                             getEnv("KVM_SITE_TITLE", DefaultSiteTitle),
		DevelopmentMode:                       getEnvBool("KVM_DEVELOPMENT_MODE", false),
		ServiceUnitName:                       getEnv("KVM_SERVICE_UNIT_NAME", "kvm-console.service"),
		MaintenanceMode:                       getEnvBool("KVM_MAINTENANCE_MODE", false),
		MaintenanceServiceUnits:               getEnv("KVM_MAINTENANCE_SERVICE_UNITS", defaultMaintenanceServiceUnits),
		MaintenanceVMShutdownTimeoutSeconds:   getEnvInt("KVM_MAINTENANCE_VM_SHUTDOWN_TIMEOUT_SECONDS", 40),
		SMTPHost:                              getEnv("KVM_SMTP_HOST", ""),
		SMTPPort:                              getEnvInt("KVM_SMTP_PORT", 587),
		SMTPUsername:                          getEnv("KVM_SMTP_USERNAME", ""),
		SMTPPasswordEnc:                       getEnv("KVM_SMTP_PASSWORD_ENC", ""),
		SMTPFromName:                          getEnv("KVM_SMTP_FROM_NAME", "QVMConsole"),
		SMTPFromAddress:                       getEnv("KVM_SMTP_FROM_ADDRESS", ""),
		SMTPSecurity:                          getEnv("KVM_SMTP_SECURITY", "starttls"),
		SMTPTimeoutSeconds:                    getEnvInt("KVM_SMTP_TIMEOUT_SECONDS", 15),
		DynamicMemorySchedulerEnabled:         getEnvBool("KVM_DYNAMIC_MEMORY_SCHEDULER_ENABLED", true),
		DynamicMemoryIntervalSeconds:          getEnvInt("KVM_DYNAMIC_MEMORY_INTERVAL_SECONDS", 30),
		DynamicMemoryHostReserveMB:            getEnvInt("KVM_DYNAMIC_MEMORY_HOST_RESERVE_MB", 2048),
		DynamicMemoryHostReservePercent:       getEnvInt("KVM_DYNAMIC_MEMORY_HOST_RESERVE_PERCENT", 20),
		DynamicMemoryIncreaseThresholdPercent: getEnvInt("KVM_DYNAMIC_MEMORY_INCREASE_THRESHOLD_PERCENT", 15),
		DynamicMemoryReclaimThresholdPercent:  getEnvInt("KVM_DYNAMIC_MEMORY_RECLAIM_THRESHOLD_PERCENT", 35),
		DynamicMemoryCooldownSeconds:          getEnvInt("KVM_DYNAMIC_MEMORY_COOLDOWN_SECONDS", 120),
		DynamicMemoryObservationHours:         getEnvInt("KVM_DYNAMIC_MEMORY_OBSERVATION_HOURS", 24),
		SchedulerEventRetentionHours:          getEnvInt("KVM_SCHEDULER_EVENT_RETENTION_HOURS", 168),
		VPCSubnetPrefix:                       getEnv("KVM_VPC_SUBNET_PREFIX", "10.200"),
		VPCVLANStart:                          getEnvInt("KVM_VPC_VLAN_START", 100),
		VPCVLANEnd:                            getEnvInt("KVM_VPC_VLAN_END", 4094),
		VPCDNS:                                getEnv("KVM_VPC_DNS", "223.5.5.5,223.6.6.6"),
		VPCACLTable:                           getEnv("KVM_VPC_ACL_TABLE", "kvm_console_vpc_acl"),
		NetworkCaptureDir:                     getEnv("KVM_NETWORK_CAPTURE_DIR", "/var/lib/kvm-console/captures"),
		NetworkCaptureDefaultSeconds:          getEnvInt("KVM_NETWORK_CAPTURE_DEFAULT_SECONDS", 30),
		NetworkCaptureMaxSeconds:              getEnvInt("KVM_NETWORK_CAPTURE_MAX_SECONDS", 120),
		NetworkCaptureMaxMB:                   getEnvInt("KVM_NETWORK_CAPTURE_MAX_MB", 64),
		NetworkCaptureMaxPackets:              getEnvInt("KVM_NETWORK_CAPTURE_MAX_PACKETS", 5000),
		PortForwardHTTPProbeEnabled:           getEnvBool("KVM_PORT_FORWARD_HTTP_PROBE_ENABLED", true),
		PortForwardHTTPProbeIntervalMinutes:   getEnvInt("KVM_PORT_FORWARD_HTTP_PROBE_INTERVAL_MINUTES", 60),
		PortForwardHTTPProbeTimeoutSeconds:    getEnvInt("KVM_PORT_FORWARD_HTTP_PROBE_TIMEOUT_SECONDS", 3),
		BatchCloneMaxConcurrency:              getEnvInt("KVM_BATCH_CLONE_MAX_CONCURRENCY", 10),
		RateLimitPublicPerMin:                 getEnvInt("KVM_RATE_LIMIT_PUBLIC", 20),
		RateLimitAuthPerMin:                   getEnvInt("KVM_RATE_LIMIT_AUTH", 0),
		UseGoLibvirt:                          getEnvBool("KVM_USE_GO_LIBVIRT", true),
		LogDir:                                getEnv("KVM_LOG_DIR", "./log"),
		LogLevel:                              getEnv("KVM_LOG_LEVEL", "info"),
		LogMaxDays:                            getEnvInt("KVM_LOG_MAX_DAYS", 7),
		LogCompress:                           getEnvBool("KVM_LOG_COMPRESS", true),
		LogConsole:                            getEnvBool("KVM_LOG_CONSOLE", true),
		LogConsoleTypes:                       getEnv("KVM_LOG_CONSOLE_TYPES", "app,cmd,libvirt"),
		LogConsoleLevel:                       getEnv("KVM_LOG_CONSOLE_LEVEL", ""),
		LogMaxSizeMB:                          getEnvInt("KVM_LOG_MAX_SIZE_MB", 100),
		LogMaxBackups:                         getEnvInt("KVM_LOG_MAX_BACKUPS", 0),
		NetworkWaitOnlineDisabled:             getEnvBool("KVM_NETWORK_WAIT_ONLINE_DISABLED", false),
		RequestFilterEnabled:                  getEnvBool("KVM_REQUEST_FILTER_ENABLED", true),
		APIMaxBodySizeMB:                      getEnvInt("KVM_API_MAX_BODY_SIZE_MB", 2),
		ErrorDetailInResponse:                 getEnvBool("KVM_ERROR_DETAIL_IN_RESPONSE", false),
		SessionFingerprintEnabled:             getEnvBool("KVM_SESSION_FINGERPRINT_ENABLED", true),
		PasswordBreachCheckEnabled:            getEnvBool("KVM_PASSWORD_BREACH_CHECK_ENABLED", true),
		CORSAllowedOrigins:                    getEnv("KVM_CORS_ALLOWED_ORIGINS", ""),
	}
	// 解析可信代理列表
	if proxies := getEnv("KVM_TRUSTED_PROXIES", ""); proxies != "" {
		for _, p := range strings.Split(proxies, ",") {
			if trimmed := strings.TrimSpace(p); trimmed != "" {
				GlobalConfig.TrustedProxies = append(GlobalConfig.TrustedProxies, trimmed)
			}
		}
	}
	// 密钥隔离：自动生成独立密钥而非 fallback 到 JWTSecret
	if GlobalConfig.VMCredentialSecret == "" {
		generated := generateRandomKey(32)
		GlobalConfig.VMCredentialSecret = generated
		appendEnvKey("KVM_VM_CREDENTIAL_SECRET", generated)
		fmt.Fprintf(os.Stderr, "[安全] 已自动生成并持久化 KVM_VM_CREDENTIAL_SECRET\n")
	}
	if GlobalConfig.SecuritySecret == "" {
		// 检查是否有旧的 JWTSecret 作为 legacy 密钥（用于过渡兼容）
		GlobalConfig.LegacySecuritySecret = GlobalConfig.JWTSecret
		generated := generateRandomKey(32)
		GlobalConfig.SecuritySecret = generated
		appendEnvKey("KVM_SECURITY_SECRET", generated)
		appendEnvKey("KVM_LEGACY_SECURITY_SECRET", GlobalConfig.LegacySecuritySecret)
		fmt.Fprintf(os.Stderr, "[安全] 已自动生成并持久化 KVM_SECURITY_SECRET\n")
	} else {
		// 加载持久化的 legacy 密钥（用于解密旧数据）
		GlobalConfig.LegacySecuritySecret = getEnv("KVM_LEGACY_SECURITY_SECRET", "")
	}
}

// ValidateSecurity 启动后安全检查（需在数据库设置加载完成后调用）
func ValidateSecurity() {
	if GlobalConfig.VMCredentialSecret == GlobalConfig.JWTSecret &&
		GlobalConfig.JWTSecret != defaultJWTSecret && !GlobalConfig.DevelopmentMode {
		fmt.Fprintf(os.Stderr, "[安全警告] KVM_VM_CREDENTIAL_SECRET 与 KVM_JWT_SECRET 相同，建议使用独立密钥。\n")
	}
	if GlobalConfig.SecuritySecret == GlobalConfig.JWTSecret &&
		GlobalConfig.JWTSecret != defaultJWTSecret && !GlobalConfig.DevelopmentMode {
		fmt.Fprintf(os.Stderr, "[安全警告] KVM_SECURITY_SECRET 与 KVM_JWT_SECRET 相同，建议使用独立密钥。\n")
	}

	// 拒绝默认 JWT 密钥启动（开发模式仅警告）
	if GlobalConfig.JWTSecret == defaultJWTSecret {
		if GlobalConfig.DevelopmentMode {
			fmt.Fprintf(os.Stderr, "\n[安全警告] ================================================\n")
			fmt.Fprintf(os.Stderr, "[安全警告] 当前正在使用默认 JWT 密钥运行！\n")
			fmt.Fprintf(os.Stderr, "[安全警告] 任何人可以通过已知密钥伪造身份令牌。\n")
			fmt.Fprintf(os.Stderr, "[安全警告] 由于当前处于开发模式，服务仍将继续启动。\n")
			fmt.Fprintf(os.Stderr, "[安全警告] 生产环境请务必设置 KVM_JWT_SECRET 环境变量！\n")
			fmt.Fprintf(os.Stderr, "[安全警告] 建议运行: KVM_JWT_SECRET=$(openssl rand -base64 48)\n")
			fmt.Fprintf(os.Stderr, "[安全警告] ================================================\n\n")
		} else {
			fmt.Fprintf(os.Stderr, "\n[安全错误] ================================================\n")
			fmt.Fprintf(os.Stderr, "[安全错误] 检测到默认 JWT 密钥，出于安全考虑拒绝启动！\n")
			fmt.Fprintf(os.Stderr, "[安全错误] 请设置 KVM_JWT_SECRET 环境变量为随机强密钥。\n")
			fmt.Fprintf(os.Stderr, "[安全错误] 生成密钥: openssl rand -base64 48\n")
			fmt.Fprintf(os.Stderr, "[安全错误] 或在 .env 文件中写入: KVM_JWT_SECRET=<随机密钥>\n")
			fmt.Fprintf(os.Stderr, "[安全错误] 如果你确实需要测试环境，可设置 KVM_DEVELOPMENT_MODE=true\n")
			fmt.Fprintf(os.Stderr, "[安全错误] ================================================\n\n")
			os.Exit(1)
		}
	}
}

// generateRandomKey 生成指定长度的随机 base64 密钥
func generateRandomKey(byteLen int) string {
	b := make([]byte, byteLen)
	if _, err := rand.Read(b); err != nil {
		// crypto/rand.Read 失败意味着系统熵池严重异常，不应带着弱密钥继续运行
		fmt.Fprintf(os.Stderr, "[安全致命] 无法生成安全随机数: %v\n", err)
		os.Exit(1)
	}
	return base64.RawURLEncoding.EncodeToString(b)
}

// appendEnvKey 将密钥写入 .env 文件（已有 key 则更新，避免重复追加）
func appendEnvKey(key, value string) {
	envPath := EnvFilePath()

	// 确保父目录存在
	if dir := filepath.Dir(envPath); dir != "" && dir != "." {
		if err := os.MkdirAll(dir, 0755); err != nil {
			fmt.Fprintf(os.Stderr, "[安全警告] 无法创建 .env 目录 %s: %v\n", dir, err)
			return
		}
	}

	// 读取已有内容
	envData := make(map[string]string)
	if content, err := os.ReadFile(envPath); err == nil {
		for _, line := range strings.Split(string(content), "\n") {
			line = strings.TrimSpace(line)
			if line == "" {
				continue
			}
			if idx := strings.IndexByte(line, '='); idx > 0 {
				k := strings.TrimSpace(line[:idx])
				v := line[idx+1:]
				envData[k] = v
			}
		}
	}

	// 更新或新增
	envData[key] = value

	// 排序后写入
	var lines []string
	for k, v := range envData {
		lines = append(lines, k+"="+v)
	}
	sort.Strings(lines)
	newContent := strings.Join(lines, "\n") + "\n"
	if err := os.WriteFile(envPath, []byte(newContent), 0600); err != nil {
		fmt.Fprintf(os.Stderr, "[安全警告] 无法写入 .env 文件: %v\n", err)
	}
}

// getEnv 获取环境变量，提供默认值
func getEnv(key, defaultVal string) string {
	if val := os.Getenv(key); val != "" {
		return val
	}
	return defaultVal
}

// getEnvInt 获取整型环境变量
func getEnvInt(key string, defaultVal int) int {
	if val := os.Getenv(key); val != "" {
		if intVal, err := strconv.Atoi(val); err == nil {
			return intVal
		}
	}
	return defaultVal
}

// getEnvBool 获取布尔环境变量
func getEnvBool(key string, defaultVal bool) bool {
	if val := os.Getenv(key); val != "" {
		if boolVal, err := strconv.ParseBool(val); err == nil {
			return boolVal
		}
	}
	return defaultVal
}

// DefaultMaintenanceServiceUnits 返回维护模式的默认服务列表。
func DefaultMaintenanceServiceUnits() string {
	return defaultMaintenanceServiceUnits
}

// PersistableKeys 可以通过界面持久化的配置项列表
var PersistableKeys = []string{
	"template_dir",
	"template_import_dir",
	"template_export_dir",
	"clone_dir",
	"iso_dir",
	"default_network",
	"network_backend",
	"ovs_bridge",
	"ovs_uplink",
	"ovs_dhcp_start",
	"ovs_dhcp_end",
	"subnet_prefix",
	"auto_port_start",
	"auto_port_end",
	"host_ip",
	"external_nic",
	"max_burst_inbound",
	"max_burst_outbound",
	"rescue_iso",
	"public_base_url",
	"site_title",
	"development_mode",
	"maintenance_mode",
	"maintenance_service_units",
	"maintenance_vm_shutdown_timeout_seconds",
	"smtp_host",
	"smtp_port",
	"smtp_username",
	"smtp_password_enc",
	"smtp_from_name",
	"smtp_from_address",
	"smtp_security",
	"smtp_timeout_seconds",
	"dynamic_memory_scheduler_enabled",
	"dynamic_memory_interval_seconds",
	"dynamic_memory_host_reserve_mb",
	"dynamic_memory_host_reserve_percent",
	"dynamic_memory_increase_threshold_percent",
	"dynamic_memory_reclaim_threshold_percent",
	"dynamic_memory_cooldown_seconds",
	"dynamic_memory_observation_hours",
	"scheduler_event_retention_hours",
	"port_forward_http_probe_enabled",
	"port_forward_http_probe_interval_minutes",
	"port_forward_http_probe_timeout_seconds",
	"vpc_subnet_prefix",
	"vpc_vlan_start",
	"vpc_vlan_end",
	"vpc_dns",
	"vpc_acl_table",
	"default_disk_iops_total",
	"default_disk_iops_read",
	"default_disk_iops_write",
	"batch_clone_max_concurrency",
	"jwt_secret_rotate_hours",
	"use_go_libvirt",
	"log_dir",
	"log_level",
	"log_max_days",
	"log_compress",
	"log_console",
	"log_console_types",
	"log_console_level",
	"log_max_size_mb",
	"log_max_backups",
	"network_wait_online_disabled",
}

// keyToEnvVar 配置项到环境变量的映射
var keyToEnvVar = map[string]string{
	"template_dir":              "KVM_TEMPLATE_DIR",
	"template_import_dir":       "KVM_TEMPLATE_IMPORT_DIR",
	"template_export_dir":       "KVM_TEMPLATE_EXPORT_DIR",
	"clone_dir":                 "KVM_CLONE_DIR",
	"iso_dir":                   "KVM_ISO_DIR",
	"default_network":           "KVM_DEFAULT_NETWORK",
	"network_backend":           "KVM_NETWORK_BACKEND",
	"ovs_bridge":                "KVM_OVS_BRIDGE",
	"ovs_uplink":                "KVM_OVS_UPLINK",
	"ovs_dhcp_start":            "KVM_OVS_DHCP_START",
	"ovs_dhcp_end":              "KVM_OVS_DHCP_END",
	"subnet_prefix":             "KVM_SUBNET_PREFIX",
	"auto_port_start":           "KVM_AUTO_PORT_START",
	"auto_port_end":             "KVM_AUTO_PORT_END",
	"host_ip":                   "KVM_HOST_IP",
	"external_nic":              "KVM_EXTERNAL_NIC",
	"max_burst_inbound":         "KVM_MAX_BURST_INBOUND",
	"max_burst_outbound":        "KVM_MAX_BURST_OUTBOUND",
	"rescue_iso":                "KVM_RESCUE_ISO",
	"public_base_url":           "KVM_PUBLIC_BASE_URL",
	"site_title":                "KVM_SITE_TITLE",
	"development_mode":          "KVM_DEVELOPMENT_MODE",
	"maintenance_mode":          "KVM_MAINTENANCE_MODE",
	"maintenance_service_units": "KVM_MAINTENANCE_SERVICE_UNITS",
	"maintenance_vm_shutdown_timeout_seconds": "KVM_MAINTENANCE_VM_SHUTDOWN_TIMEOUT_SECONDS",
	"smtp_host":                                 "KVM_SMTP_HOST",
	"smtp_port":                                 "KVM_SMTP_PORT",
	"smtp_username":                             "KVM_SMTP_USERNAME",
	"smtp_password_enc":                         "KVM_SMTP_PASSWORD_ENC",
	"smtp_from_name":                            "KVM_SMTP_FROM_NAME",
	"smtp_from_address":                         "KVM_SMTP_FROM_ADDRESS",
	"smtp_security":                             "KVM_SMTP_SECURITY",
	"smtp_timeout_seconds":                      "KVM_SMTP_TIMEOUT_SECONDS",
	"dynamic_memory_scheduler_enabled":          "KVM_DYNAMIC_MEMORY_SCHEDULER_ENABLED",
	"dynamic_memory_interval_seconds":           "KVM_DYNAMIC_MEMORY_INTERVAL_SECONDS",
	"dynamic_memory_host_reserve_mb":            "KVM_DYNAMIC_MEMORY_HOST_RESERVE_MB",
	"dynamic_memory_host_reserve_percent":       "KVM_DYNAMIC_MEMORY_HOST_RESERVE_PERCENT",
	"dynamic_memory_increase_threshold_percent": "KVM_DYNAMIC_MEMORY_INCREASE_THRESHOLD_PERCENT",
	"dynamic_memory_reclaim_threshold_percent":  "KVM_DYNAMIC_MEMORY_RECLAIM_THRESHOLD_PERCENT",
	"dynamic_memory_cooldown_seconds":           "KVM_DYNAMIC_MEMORY_COOLDOWN_SECONDS",
	"dynamic_memory_observation_hours":          "KVM_DYNAMIC_MEMORY_OBSERVATION_HOURS",
	"scheduler_event_retention_hours":           "KVM_SCHEDULER_EVENT_RETENTION_HOURS",
	"port_forward_http_probe_enabled":           "KVM_PORT_FORWARD_HTTP_PROBE_ENABLED",
	"port_forward_http_probe_interval_minutes":  "KVM_PORT_FORWARD_HTTP_PROBE_INTERVAL_MINUTES",
	"port_forward_http_probe_timeout_seconds":   "KVM_PORT_FORWARD_HTTP_PROBE_TIMEOUT_SECONDS",
	"vpc_subnet_prefix":                         "KVM_VPC_SUBNET_PREFIX",
	"vpc_vlan_start":                            "KVM_VPC_VLAN_START",
	"vpc_vlan_end":                              "KVM_VPC_VLAN_END",
	"vpc_dns":                                   "KVM_VPC_DNS",
	"vpc_acl_table":                             "KVM_VPC_ACL_TABLE",
	"default_disk_iops_total":                   "KVM_DEFAULT_DISK_IOPS_TOTAL",
	"default_disk_iops_read":                    "KVM_DEFAULT_DISK_IOPS_READ",
	"default_disk_iops_write":                   "KVM_DEFAULT_DISK_IOPS_WRITE",
	"batch_clone_max_concurrency":               "KVM_BATCH_CLONE_MAX_CONCURRENCY",
	"jwt_secret_rotate_hours":                   "KVM_JWT_SECRET_ROTATE_HOURS",
	"use_go_libvirt":                            "KVM_USE_GO_LIBVIRT",
	"log_dir":                                   "KVM_LOG_DIR",
	"log_level":                                 "KVM_LOG_LEVEL",
	"log_max_days":                              "KVM_LOG_MAX_DAYS",
	"log_compress":                              "KVM_LOG_COMPRESS",
	"log_console":                               "KVM_LOG_CONSOLE",
	"log_console_types":                         "KVM_LOG_CONSOLE_TYPES",
	"log_console_level":                         "KVM_LOG_CONSOLE_LEVEL",
	"log_max_size_mb":                           "KVM_LOG_MAX_SIZE_MB",
	"log_max_backups":                           "KVM_LOG_MAX_BACKUPS",
	"network_wait_online_disabled":              "KVM_NETWORK_WAIT_ONLINE_DISABLED",
}

// LoadFromDB 从数据库加载持久化的设置覆盖当前配置
// 优先级: 环境变量 > 数据库 > 默认值
// 只有对应环境变量未设置时，才使用数据库中的持久化值
func (c *Config) LoadFromDB(settings map[string]string) {
	for key, value := range settings {
		if value == "" {
			continue
		}
		// 如果对应环境变量已设置，则跳过（环境变量优先）
		if envKey, ok := keyToEnvVar[key]; ok {
			if os.Getenv(envKey) != "" {
				continue
			}
		}
		switch key {
		case "template_dir":
			c.TemplateDir = value
		case "template_import_dir":
			c.TemplateImportDir = value
		case "template_export_dir":
			c.TemplateExportDir = value
		case "clone_dir":
			c.CloneDir = value
		case "iso_dir":
			c.ISODir = value
		case "default_network":
			c.DefaultNetwork = value
		case "network_backend":
			c.NetworkBackend = value
		case "ovs_bridge":
			c.OVSBridge = value
		case "ovs_uplink":
			c.OVSUplink = value
		case "ovs_dhcp_start":
			c.OVSDHCPStart = value
		case "ovs_dhcp_end":
			c.OVSDHCPEnd = value
		case "subnet_prefix":
			c.SubnetPrefix = value
		case "auto_port_start":
			if v, err := strconv.Atoi(value); err == nil {
				c.AutoPortStart = v
			}
		case "auto_port_end":
			if v, err := strconv.Atoi(value); err == nil {
				c.AutoPortEnd = v
			}
		case "host_ip":
			c.HostIP = value
		case "external_nic":
			c.ExternalNIC = value
		case "max_burst_inbound":
			if v, err := strconv.Atoi(value); err == nil {
				c.MaxBurstInbound = v
			}
		case "max_burst_outbound":
			if v, err := strconv.Atoi(value); err == nil {
				c.MaxBurstOutbound = v
			}
		case "rescue_iso":
			c.RescueISO = value
		case "spice_enabled_by_default":
			if v, err := strconv.ParseBool(value); err == nil {
				c.SpiceEnabledByDefault = v
			}
		case "public_base_url":
			c.PublicBaseURL = value
		case "site_title":
			c.SiteTitle = value
		case "development_mode":
			if v, err := strconv.ParseBool(value); err == nil {
				c.DevelopmentMode = v
			}
		case "maintenance_mode":
			if v, err := strconv.ParseBool(value); err == nil {
				c.MaintenanceMode = v
			}
		case "maintenance_service_units":
			c.MaintenanceServiceUnits = value
		case "maintenance_vm_shutdown_timeout_seconds":
			if v, err := strconv.Atoi(value); err == nil {
				c.MaintenanceVMShutdownTimeoutSeconds = v
			}
		case "smtp_host":
			c.SMTPHost = value
		case "smtp_port":
			if v, err := strconv.Atoi(value); err == nil {
				c.SMTPPort = v
			}
		case "smtp_username":
			c.SMTPUsername = value
		case "smtp_password_enc":
			c.SMTPPasswordEnc = value
		case "smtp_from_name":
			c.SMTPFromName = value
		case "smtp_from_address":
			c.SMTPFromAddress = value
		case "smtp_security":
			c.SMTPSecurity = value
		case "smtp_timeout_seconds":
			if v, err := strconv.Atoi(value); err == nil {
				c.SMTPTimeoutSeconds = v
			}
		case "dynamic_memory_scheduler_enabled":
			if v, err := strconv.ParseBool(value); err == nil {
				c.DynamicMemorySchedulerEnabled = v
			}
		case "dynamic_memory_interval_seconds":
			if v, err := strconv.Atoi(value); err == nil {
				c.DynamicMemoryIntervalSeconds = v
			}
		case "dynamic_memory_host_reserve_mb":
			if v, err := strconv.Atoi(value); err == nil {
				c.DynamicMemoryHostReserveMB = v
			}
		case "dynamic_memory_host_reserve_percent":
			if v, err := strconv.Atoi(value); err == nil {
				c.DynamicMemoryHostReservePercent = v
			}
		case "dynamic_memory_increase_threshold_percent":
			if v, err := strconv.Atoi(value); err == nil {
				c.DynamicMemoryIncreaseThresholdPercent = v
			}
		case "dynamic_memory_reclaim_threshold_percent":
			if v, err := strconv.Atoi(value); err == nil {
				c.DynamicMemoryReclaimThresholdPercent = v
			}
		case "dynamic_memory_cooldown_seconds":
			if v, err := strconv.Atoi(value); err == nil {
				c.DynamicMemoryCooldownSeconds = v
			}
		case "dynamic_memory_observation_hours":
			if v, err := strconv.Atoi(value); err == nil {
				c.DynamicMemoryObservationHours = v
			}
		case "scheduler_event_retention_hours":
			if v, err := strconv.Atoi(value); err == nil {
				c.SchedulerEventRetentionHours = v
			}
		case "port_forward_http_probe_enabled":
			if v, err := strconv.ParseBool(value); err == nil {
				c.PortForwardHTTPProbeEnabled = v
			}
		case "port_forward_http_probe_interval_minutes":
			if v, err := strconv.Atoi(value); err == nil {
				c.PortForwardHTTPProbeIntervalMinutes = v
			}
		case "port_forward_http_probe_timeout_seconds":
			if v, err := strconv.Atoi(value); err == nil {
				c.PortForwardHTTPProbeTimeoutSeconds = v
			}
		case "vpc_subnet_prefix":
			c.VPCSubnetPrefix = value
		case "vpc_vlan_start":
			if v, err := strconv.Atoi(value); err == nil {
				c.VPCVLANStart = v
			}
		case "vpc_vlan_end":
			if v, err := strconv.Atoi(value); err == nil {
				c.VPCVLANEnd = v
			}
		case "vpc_dns":
			c.VPCDNS = value
		case "vpc_acl_table":
			c.VPCACLTable = value
		case "default_disk_iops_total":
			if v, err := strconv.Atoi(value); err == nil {
				c.DefaultDiskIOPSTotal = v
			}
		case "default_disk_iops_read":
			if v, err := strconv.Atoi(value); err == nil {
				c.DefaultDiskIOPSRead = v
			}
		case "default_disk_iops_write":
			if v, err := strconv.Atoi(value); err == nil {
				c.DefaultDiskIOPSWrite = v
			}
		case "batch_clone_max_concurrency":
			if v, err := strconv.Atoi(value); err == nil && v > 0 {
				c.BatchCloneMaxConcurrency = v
			}
		case "jwt_secret_rotate_hours":
			if v, err := strconv.Atoi(value); err == nil && v >= 0 {
				c.JWTSecretRotateHours = v
			}
		case "use_go_libvirt":
			if v, err := strconv.ParseBool(value); err == nil {
				c.UseGoLibvirt = v
			}
		case "log_dir":
			c.LogDir = value
		case "log_level":
			c.LogLevel = value
		case "log_max_days":
			if v, err := strconv.Atoi(value); err == nil {
				c.LogMaxDays = v
			}
		case "log_compress":
			if v, err := strconv.ParseBool(value); err == nil {
				c.LogCompress = v
			}
		case "log_console":
			if v, err := strconv.ParseBool(value); err == nil {
				c.LogConsole = v
			}
		case "log_console_types":
			c.LogConsoleTypes = value
		case "log_console_level":
			c.LogConsoleLevel = value
		case "log_max_size_mb":
			if v, err := strconv.Atoi(value); err == nil {
				c.LogMaxSizeMB = v
			}
		case "log_max_backups":
			if v, err := strconv.Atoi(value); err == nil {
				c.LogMaxBackups = v
			}
		case "network_wait_online_disabled":
			if v, err := strconv.ParseBool(value); err == nil {
				c.NetworkWaitOnlineDisabled = v
			}
		case "session_fingerprint_enabled":
			c.SessionFingerprintEnabled = value != "false"
		case "request_filter_enabled":
			c.RequestFilterEnabled = value != "false"
		case "password_breach_check_enabled":
			c.PasswordBreachCheckEnabled = value != "false"
		case "api_max_body_size_mb":
			if n, err := strconv.Atoi(value); err == nil && n > 0 {
				c.APIMaxBodySizeMB = n
			}
		case "error_detail_in_response":
			c.ErrorDetailInResponse = value == "true"
		}
	}
}

// ToSettingsMap 将当前可持久化的配置导出为 map
func (c *Config) ToSettingsMap() map[string]string {
	return map[string]string{
		"template_dir":              c.TemplateDir,
		"template_import_dir":       c.TemplateImportDir,
		"template_export_dir":       c.TemplateExportDir,
		"clone_dir":                 c.CloneDir,
		"iso_dir":                   c.ISODir,
		"default_network":           c.DefaultNetwork,
		"network_backend":           c.NetworkBackend,
		"ovs_bridge":                c.OVSBridge,
		"ovs_uplink":                c.OVSUplink,
		"ovs_dhcp_start":            c.OVSDHCPStart,
		"ovs_dhcp_end":              c.OVSDHCPEnd,
		"subnet_prefix":             c.SubnetPrefix,
		"auto_port_start":           strconv.Itoa(c.AutoPortStart),
		"auto_port_end":             strconv.Itoa(c.AutoPortEnd),
		"host_ip":                   c.HostIP,
		"external_nic":              c.ExternalNIC,
		"max_burst_inbound":         strconv.Itoa(c.MaxBurstInbound),
		"max_burst_outbound":        strconv.Itoa(c.MaxBurstOutbound),
		"rescue_iso":                c.RescueISO,
		"spice_enabled_by_default":  strconv.FormatBool(c.SpiceEnabledByDefault),
		"public_base_url":           c.PublicBaseURL,
		"site_title":                c.SiteTitle,
		"development_mode":          strconv.FormatBool(c.DevelopmentMode),
		"maintenance_mode":          strconv.FormatBool(c.MaintenanceMode),
		"maintenance_service_units": c.MaintenanceServiceUnits,
		"maintenance_vm_shutdown_timeout_seconds": strconv.Itoa(c.MaintenanceVMShutdownTimeoutSeconds),
		"smtp_host":                                 c.SMTPHost,
		"smtp_port":                                 strconv.Itoa(c.SMTPPort),
		"smtp_username":                             c.SMTPUsername,
		"smtp_password_enc":                         c.SMTPPasswordEnc,
		"smtp_from_name":                            c.SMTPFromName,
		"smtp_from_address":                         c.SMTPFromAddress,
		"smtp_security":                             c.SMTPSecurity,
		"smtp_timeout_seconds":                      strconv.Itoa(c.SMTPTimeoutSeconds),
		"dynamic_memory_scheduler_enabled":          strconv.FormatBool(c.DynamicMemorySchedulerEnabled),
		"dynamic_memory_interval_seconds":           strconv.Itoa(c.DynamicMemoryIntervalSeconds),
		"dynamic_memory_host_reserve_mb":            strconv.Itoa(c.DynamicMemoryHostReserveMB),
		"dynamic_memory_host_reserve_percent":       strconv.Itoa(c.DynamicMemoryHostReservePercent),
		"dynamic_memory_increase_threshold_percent": strconv.Itoa(c.DynamicMemoryIncreaseThresholdPercent),
		"dynamic_memory_reclaim_threshold_percent":  strconv.Itoa(c.DynamicMemoryReclaimThresholdPercent),
		"dynamic_memory_cooldown_seconds":           strconv.Itoa(c.DynamicMemoryCooldownSeconds),
		"dynamic_memory_observation_hours":          strconv.Itoa(c.DynamicMemoryObservationHours),
		"scheduler_event_retention_hours":           strconv.Itoa(c.SchedulerEventRetentionHours),
		"port_forward_http_probe_enabled":           strconv.FormatBool(c.PortForwardHTTPProbeEnabled),
		"port_forward_http_probe_interval_minutes":  strconv.Itoa(c.PortForwardHTTPProbeIntervalMinutes),
		"port_forward_http_probe_timeout_seconds":   strconv.Itoa(c.PortForwardHTTPProbeTimeoutSeconds),
		"vpc_subnet_prefix":                         c.VPCSubnetPrefix,
		"vpc_vlan_start":                            strconv.Itoa(c.VPCVLANStart),
		"vpc_vlan_end":                              strconv.Itoa(c.VPCVLANEnd),
		"vpc_dns":                                   c.VPCDNS,
		"vpc_acl_table":                             c.VPCACLTable,
		"default_disk_iops_total":                   strconv.Itoa(c.DefaultDiskIOPSTotal),
		"default_disk_iops_read":                    strconv.Itoa(c.DefaultDiskIOPSRead),
		"default_disk_iops_write":                   strconv.Itoa(c.DefaultDiskIOPSWrite),
		"batch_clone_max_concurrency":               strconv.Itoa(c.BatchCloneMaxConcurrency),
		"jwt_secret_rotate_hours":                   strconv.Itoa(c.JWTSecretRotateHours),
		"use_go_libvirt":                            strconv.FormatBool(c.UseGoLibvirt),
		"log_dir":                                   c.LogDir,
		"log_level":                                 c.LogLevel,
		"log_max_days":                              strconv.Itoa(c.LogMaxDays),
		"log_compress":                              strconv.FormatBool(c.LogCompress),
		"log_console":                               strconv.FormatBool(c.LogConsole),
		"log_console_types":                         c.LogConsoleTypes,
		"log_console_level":                         c.LogConsoleLevel,
		"log_max_size_mb":                           strconv.Itoa(c.LogMaxSizeMB),
		"log_max_backups":                           strconv.Itoa(c.LogMaxBackups),
		"network_wait_online_disabled":              strconv.FormatBool(c.NetworkWaitOnlineDisabled),
		"session_fingerprint_enabled":               strconv.FormatBool(c.SessionFingerprintEnabled),
		"request_filter_enabled":                    strconv.FormatBool(c.RequestFilterEnabled),
		"password_breach_check_enabled":             strconv.FormatBool(c.PasswordBreachCheckEnabled),
	}
}

// EnvFilePath 返回 .env 文件路径
func EnvFilePath() string {
	return getEnv("KVM_ENV_FILE", "./.env")
}

// SyncEnvFile 将当前配置同步写入 .env 文件（仅更新数据库中已持久化的 key）
func SyncEnvFile() {
	envPath := EnvFilePath()
	if envPath == "" {
		return
	}

	envData := make(map[string]string)
	if content, err := os.ReadFile(envPath); err == nil {
		for _, line := range stringOrEmptyLines(string(content)) {
			line = stripBOM(line)
			if idx := indexOf(line, '='); idx > 0 {
				key := line[:idx]
				val := line[idx+1:]
				envData[key] = val
			}
		}
	}

	settingsMap := GlobalConfig.ToSettingsMap()
	changed := false
	for key, value := range settingsMap {
		envKey, ok := keyToEnvVar[key]
		if !ok {
			continue
		}
		envData[envKey] = value
		changed = true
	}

	if !changed {
		return
	}

	var lines []string
	for k, v := range envData {
		lines = append(lines, k+"="+v)
	}
	sort.Strings(lines)

	newContent := strings.Join(lines, "\n") + "\n"
	if err := os.WriteFile(envPath, []byte(newContent), 0600); err != nil {
		fmt.Printf("[config] 同步 .env 文件失败: %v\n", err)
	}
}

func stringOrEmptyLines(s string) []string {
	if s == "" {
		return nil
	}
	return strings.Split(s, "\n")
}

func stripBOM(s string) string {
	if len(s) >= 3 && s[0] == 0xEF && s[1] == 0xBB && s[2] == 0xBF {
		return s[3:]
	}
	return s
}

func indexOf(s string, c byte) int {
	for i := 0; i < len(s); i++ {
		if s[i] == c {
			return i
		}
	}
	return -1
}
