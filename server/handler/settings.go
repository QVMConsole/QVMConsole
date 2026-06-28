package handler

import (
	"archive/zip"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"kvm_console/config"
	"kvm_console/logger"
	"kvm_console/model"
	"kvm_console/service"
	"kvm_console/service/storage/quota"
	"kvm_console/taskqueue"
)

// SettingsResponse 设置响应
type SettingsResponse struct {
	Port                                  int    `json:"port"`
	TemplateDir                           string `json:"template_dir"`
	TemplateImportDir                     string `json:"template_import_dir"`
	TemplateExportDir                     string `json:"template_export_dir"`
	CloneDir                              string `json:"clone_dir"`
	ISODir                                string `json:"iso_dir"`
	DefaultNetwork                        string `json:"default_network"`
	NetworkBackend                        string `json:"network_backend"`
	OVSBridge                             string `json:"ovs_bridge"`
	OVSUplink                             string `json:"ovs_uplink"`
	OVSDHCPStart                          string `json:"ovs_dhcp_start"`
	OVSDHCPEnd                            string `json:"ovs_dhcp_end"`
	SubnetPrefix                          string `json:"subnet_prefix"`
	AutoPortStart                         int    `json:"auto_port_start"`
	AutoPortEnd                           int    `json:"auto_port_end"`
	PortForwardDir                        string `json:"port_forward_dir"`
	HostIP                                string `json:"host_ip"`
	ExternalNIC                           string `json:"external_nic"`
	MaxBurstInbound                       int    `json:"max_burst_inbound"`
	MaxBurstOutbound                      int    `json:"max_burst_outbound"`
	RescueISO                             string `json:"rescue_iso"`
	SpiceEnabledByDefault                 bool   `json:"spice_enabled_by_default"`
	PublicBaseURL                         string `json:"public_base_url"`
	SiteTitle                             string `json:"site_title"`
	DevelopmentMode                       bool   `json:"development_mode"`
	MaintenanceMode                       bool   `json:"maintenance_mode"`
	MaintenanceServiceUnits               string `json:"maintenance_service_units"`
	MaintenanceVMShutdownTimeoutSeconds   int    `json:"maintenance_vm_shutdown_timeout_seconds"`
	SMTPHost                              string `json:"smtp_host"`
	SMTPPort                              int    `json:"smtp_port"`
	SMTPUsername                          string `json:"smtp_username"`
	SMTPFromName                          string `json:"smtp_from_name"`
	SMTPFromAddress                       string `json:"smtp_from_address"`
	SMTPSecurity                          string `json:"smtp_security"`
	SMTPTimeoutSeconds                    int    `json:"smtp_timeout_seconds"`
	SMTPPasswordConfigured                bool   `json:"smtp_password_configured"`
	SMTPConfigured                        bool   `json:"smtp_configured"`
	DynamicMemorySchedulerEnabled         bool   `json:"dynamic_memory_scheduler_enabled"`
	DynamicMemoryIntervalSeconds          int    `json:"dynamic_memory_interval_seconds"`
	DynamicMemoryHostReserveMB            int    `json:"dynamic_memory_host_reserve_mb"`
	DynamicMemoryHostReservePercent       int    `json:"dynamic_memory_host_reserve_percent"`
	DynamicMemoryIncreaseThresholdPercent int    `json:"dynamic_memory_increase_threshold_percent"`
	DynamicMemoryReclaimThresholdPercent  int    `json:"dynamic_memory_reclaim_threshold_percent"`
	DynamicMemoryCooldownSeconds          int    `json:"dynamic_memory_cooldown_seconds"`
	DynamicMemoryObservationHours         int    `json:"dynamic_memory_observation_hours"`
	SchedulerEventRetentionHours          int    `json:"scheduler_event_retention_hours"`
	PortForwardHTTPProbeEnabled           bool   `json:"port_forward_http_probe_enabled"`
	PortForwardHTTPProbeIntervalMinutes   int    `json:"port_forward_http_probe_interval_minutes"`
	PortForwardHTTPProbeTimeoutSeconds    int    `json:"port_forward_http_probe_timeout_seconds"`
	// 虚拟机磁盘 IOPS 默认限制
	DefaultDiskIOPSTotal int `json:"default_disk_iops_total"` // 默认总 IOPS 限制（0 表示不限制）
	DefaultDiskIOPSRead  int `json:"default_disk_iops_read"`  // 默认读 IOPS 限制（0 表示不限制）
	DefaultDiskIOPSWrite int `json:"default_disk_iops_write"` // 默认写 IOPS 限制（0 表示不限制）
	// 批量克隆最大同时克隆数量
	BatchCloneMaxConcurrency int `json:"batch_clone_max_concurrency"`
	// JWT 密钥自动轮换间隔（小时，0=禁用）
	JWTSecretRotateHours int    `json:"jwt_secret_rotate_hours"`
	JWTSecretLastRotated string `json:"jwt_secret_last_rotated"`
	// 日志管理
	LogMaxBackups int `json:"log_max_backups"`
	// 网络等待就绪检测
	NetworkWaitOnlineDisabled bool   `json:"network_wait_online_disabled"`
	NetworkWaitOnlineSummary  string `json:"network_wait_online_summary"`
	// 安全防护
	SessionFingerprintEnabled bool `json:"session_fingerprint_enabled"`
	RequestFilterEnabled      bool `json:"request_filter_enabled"`
	PasswordBreachCheckEnabled bool `json:"password_breach_check_enabled"`
}

// UpdateSettingsRequest 更新设置请求
type UpdateSettingsRequest struct {
	TemplateDir                           *string `json:"template_dir"`
	TemplateImportDir                     *string `json:"template_import_dir"`
	TemplateExportDir                     *string `json:"template_export_dir"`
	CloneDir                              *string `json:"clone_dir"`
	ISODir                                *string `json:"iso_dir"`
	DefaultNetwork                        *string `json:"default_network"`
	NetworkBackend                        *string `json:"network_backend"`
	OVSBridge                             *string `json:"ovs_bridge"`
	OVSUplink                             *string `json:"ovs_uplink"`
	OVSDHCPStart                          *string `json:"ovs_dhcp_start"`
	OVSDHCPEnd                            *string `json:"ovs_dhcp_end"`
	SubnetPrefix                          *string `json:"subnet_prefix"`
	AutoPortStart                         *int    `json:"auto_port_start"`
	AutoPortEnd                           *int    `json:"auto_port_end"`
	HostIP                                *string `json:"host_ip"`
	ExternalNIC                           *string `json:"external_nic"`
	MaxBurstInbound                       *int    `json:"max_burst_inbound"`
	MaxBurstOutbound                      *int    `json:"max_burst_outbound"`
	RescueISO                             *string `json:"rescue_iso"`
	SpiceEnabledByDefault                 *bool   `json:"spice_enabled_by_default"`
	PublicBaseURL                         *string `json:"public_base_url"`
	SiteTitle                             *string `json:"site_title"`
	DevelopmentMode                       *bool   `json:"development_mode"`
	MaintenanceMode                       *bool   `json:"maintenance_mode"`
	MaintenanceServiceUnits               *string `json:"maintenance_service_units"`
	MaintenanceVMShutdownTimeoutSeconds   *int    `json:"maintenance_vm_shutdown_timeout_seconds"`
	SMTPHost                              *string `json:"smtp_host"`
	SMTPPort                              *int    `json:"smtp_port"`
	SMTPUsername                          *string `json:"smtp_username"`
	SMTPPassword                          *string `json:"smtp_password"`
	SMTPFromName                          *string `json:"smtp_from_name"`
	SMTPFromAddress                       *string `json:"smtp_from_address"`
	SMTPSecurity                          *string `json:"smtp_security"`
	SMTPTimeoutSeconds                    *int    `json:"smtp_timeout_seconds"`
	DynamicMemorySchedulerEnabled         *bool   `json:"dynamic_memory_scheduler_enabled"`
	DynamicMemoryIntervalSeconds          *int    `json:"dynamic_memory_interval_seconds"`
	DynamicMemoryHostReserveMB            *int    `json:"dynamic_memory_host_reserve_mb"`
	DynamicMemoryHostReservePercent       *int    `json:"dynamic_memory_host_reserve_percent"`
	DynamicMemoryIncreaseThresholdPercent *int    `json:"dynamic_memory_increase_threshold_percent"`
	DynamicMemoryReclaimThresholdPercent  *int    `json:"dynamic_memory_reclaim_threshold_percent"`
	DynamicMemoryCooldownSeconds          *int    `json:"dynamic_memory_cooldown_seconds"`
	DynamicMemoryObservationHours         *int    `json:"dynamic_memory_observation_hours"`
	SchedulerEventRetentionHours          *int    `json:"scheduler_event_retention_hours"`
	PortForwardHTTPProbeEnabled           *bool   `json:"port_forward_http_probe_enabled"`
	PortForwardHTTPProbeIntervalMinutes   *int    `json:"port_forward_http_probe_interval_minutes"`
	PortForwardHTTPProbeTimeoutSeconds    *int    `json:"port_forward_http_probe_timeout_seconds"`
	// 虚拟机磁盘 IOPS 默认限制
	DefaultDiskIOPSTotal *int `json:"default_disk_iops_total"` // 默认总 IOPS 限制（0 表示不限制）
	DefaultDiskIOPSRead  *int `json:"default_disk_iops_read"`  // 默认读 IOPS 限制（0 表示不限制）
	DefaultDiskIOPSWrite *int `json:"default_disk_iops_write"` // 默认写 IOPS 限制（0 表示不限制）
	// 批量克隆最大同时克隆数量
	BatchCloneMaxConcurrency *int `json:"batch_clone_max_concurrency"`
	// JWT 密钥轮换间隔
	JWTSecretRotateHours *int `json:"jwt_secret_rotate_hours"`
	// 日志最大备份数
	LogMaxBackups *int `json:"log_max_backups"`
	// 网络等待就绪检测
	NetworkWaitOnlineDisabled *bool `json:"network_wait_online_disabled"`
	// 安全防护
	SessionFingerprintEnabled *bool `json:"session_fingerprint_enabled"`
	RequestFilterEnabled      *bool `json:"request_filter_enabled"`
	PasswordBreachCheckEnabled *bool `json:"password_breach_check_enabled"`
}

type TestSMTPRequest struct {
	Email              string `json:"email" binding:"required"`
	SMTPHost           string `json:"smtp_host"`
	SMTPPort           int    `json:"smtp_port"`
	SMTPUsername       string `json:"smtp_username"`
	SMTPPassword       string `json:"smtp_password"`
	SMTPFromName       string `json:"smtp_from_name"`
	SMTPFromAddress    string `json:"smtp_from_address"`
	SMTPSecurity       string `json:"smtp_security"`
	SMTPTimeoutSeconds int    `json:"smtp_timeout_seconds"`
}

type PublicSettingsResponse struct {
	SiteTitle                  string `json:"site_title"`
	PasswordBreachCheckEnabled bool   `json:"password_breach_check_enabled"`
	SpiceEnabledByDefault      bool   `json:"spice_enabled_by_default"` // 创建虚拟机 SPICE 开关的默认初始值
}

// GetPublicSettings 获取公开系统设置
func GetPublicSettings(c *gin.Context) {
	siteTitle := strings.TrimSpace(config.GlobalConfig.SiteTitle)
	if siteTitle == "" {
		siteTitle = config.DefaultSiteTitle
	}
	c.JSON(http.StatusOK, gin.H{
		"code":    200,
		"message": "ok",
		"data": PublicSettingsResponse{
			SiteTitle:                 siteTitle,
			PasswordBreachCheckEnabled: config.GlobalConfig.PasswordBreachCheckEnabled,
			SpiceEnabledByDefault:     config.GlobalConfig.SpiceEnabledByDefault,
		},
	})
}

// GetSettings 获取系统设置
func GetSettings(c *gin.Context) {
	cfg := config.GlobalConfig
	smtpView := service.GetSMTPConfigView()
	siteTitle := strings.TrimSpace(cfg.SiteTitle)
	if siteTitle == "" {
		siteTitle = config.DefaultSiteTitle
	}
	maintenanceServiceUnits := strings.TrimSpace(cfg.MaintenanceServiceUnits)
	if maintenanceServiceUnits == "" {
		maintenanceServiceUnits = config.DefaultMaintenanceServiceUnits()
	}
	jwtLastRotated, _ := model.GetSetting("jwt_secret_last_rotated")
	c.JSON(http.StatusOK, gin.H{
		"code":    200,
		"message": "ok",
		"data": SettingsResponse{
			Port:                                  cfg.Port,
			TemplateDir:                           cfg.TemplateDir,
			TemplateImportDir:                     cfg.TemplateImportDir,
			TemplateExportDir:                     cfg.TemplateExportDir,
			CloneDir:                              cfg.CloneDir,
			ISODir:                                cfg.ISODir,
			DefaultNetwork:                        cfg.DefaultNetwork,
			NetworkBackend:                        cfg.NetworkBackend,
			OVSBridge:                             cfg.OVSBridge,
			OVSUplink:                             cfg.OVSUplink,
			OVSDHCPStart:                          cfg.OVSDHCPStart,
			OVSDHCPEnd:                            cfg.OVSDHCPEnd,
			SubnetPrefix:                          cfg.SubnetPrefix,
			AutoPortStart:                         cfg.AutoPortStart,
			AutoPortEnd:                           cfg.AutoPortEnd,
			PortForwardDir:                        cfg.PortForwardDir,
			HostIP:                                cfg.HostIP,
			ExternalNIC:                           cfg.ExternalNIC,
			MaxBurstInbound:                       cfg.MaxBurstInbound,
			MaxBurstOutbound:                      cfg.MaxBurstOutbound,
			RescueISO:                             cfg.RescueISO,
			SpiceEnabledByDefault:                 cfg.SpiceEnabledByDefault,
			PublicBaseURL:                         cfg.PublicBaseURL,
			SiteTitle:                             siteTitle,
			DevelopmentMode:                       cfg.DevelopmentMode,
			MaintenanceMode:                       cfg.MaintenanceMode,
			MaintenanceServiceUnits:               maintenanceServiceUnits,
			MaintenanceVMShutdownTimeoutSeconds:   cfg.MaintenanceVMShutdownTimeoutSeconds,
			SMTPHost:                              smtpView.Host,
			SMTPPort:                              smtpView.Port,
			SMTPUsername:                          smtpView.Username,
			SMTPFromName:                          smtpView.FromName,
			SMTPFromAddress:                       smtpView.FromAddress,
			SMTPSecurity:                          smtpView.Security,
			SMTPTimeoutSeconds:                    smtpView.TimeoutSeconds,
			SMTPPasswordConfigured:                smtpView.PasswordConfigured,
			SMTPConfigured:                        smtpView.Configured,
			DynamicMemorySchedulerEnabled:         cfg.DynamicMemorySchedulerEnabled,
			DynamicMemoryIntervalSeconds:          cfg.DynamicMemoryIntervalSeconds,
			DynamicMemoryHostReserveMB:            cfg.DynamicMemoryHostReserveMB,
			DynamicMemoryHostReservePercent:       cfg.DynamicMemoryHostReservePercent,
			DynamicMemoryIncreaseThresholdPercent: cfg.DynamicMemoryIncreaseThresholdPercent,
			DynamicMemoryReclaimThresholdPercent:  cfg.DynamicMemoryReclaimThresholdPercent,
			DynamicMemoryCooldownSeconds:          cfg.DynamicMemoryCooldownSeconds,
			DynamicMemoryObservationHours:         cfg.DynamicMemoryObservationHours,
			SchedulerEventRetentionHours:          cfg.SchedulerEventRetentionHours,
			PortForwardHTTPProbeEnabled:           cfg.PortForwardHTTPProbeEnabled,
			PortForwardHTTPProbeIntervalMinutes:   cfg.PortForwardHTTPProbeIntervalMinutes,
			PortForwardHTTPProbeTimeoutSeconds:    cfg.PortForwardHTTPProbeTimeoutSeconds,
			DefaultDiskIOPSTotal:                  cfg.DefaultDiskIOPSTotal,
			DefaultDiskIOPSRead:                   cfg.DefaultDiskIOPSRead,
			DefaultDiskIOPSWrite:                  cfg.DefaultDiskIOPSWrite,
			BatchCloneMaxConcurrency:              cfg.BatchCloneMaxConcurrency,
			JWTSecretRotateHours:                  cfg.JWTSecretRotateHours,
			JWTSecretLastRotated:                  jwtLastRotated,
			LogMaxBackups:                         cfg.LogMaxBackups,
			NetworkWaitOnlineDisabled:             cfg.NetworkWaitOnlineDisabled,
			NetworkWaitOnlineSummary:              networkWaitOnlineSummary(cfg.NetworkWaitOnlineDisabled),
			SessionFingerprintEnabled:             cfg.SessionFingerprintEnabled,
			RequestFilterEnabled:                  cfg.RequestFilterEnabled,
			PasswordBreachCheckEnabled:            cfg.PasswordBreachCheckEnabled,
		},
	})
}

// UpdateSettings 更新系统设置（运行时生效，同时持久化到数据库）
func UpdateSettings(c *gin.Context) {
	cfg := config.GlobalConfig
	previousMaintenanceMode := cfg.MaintenanceMode

	var req UpdateSettingsRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": "参数错误"})
		return
	}

	maintenanceChanged := req.MaintenanceMode != nil && *req.MaintenanceMode != previousMaintenanceMode
	if maintenanceChanged {
		operation := "disable_maintenance_mode"
		if *req.MaintenanceMode {
			operation = "enable_maintenance_mode"
		}
		if !requireHighRiskVerification(c, operation) {
			return
		}
	}

	if req.TemplateDir != nil {
		cfg.TemplateDir = *req.TemplateDir
	}
	if req.TemplateImportDir != nil {
		cfg.TemplateImportDir = *req.TemplateImportDir
	}
	if req.TemplateExportDir != nil {
		cfg.TemplateExportDir = *req.TemplateExportDir
	}
	if req.CloneDir != nil {
		cfg.CloneDir = *req.CloneDir
	}
	if req.ISODir != nil {
		cfg.ISODir = strings.TrimSpace(*req.ISODir)
		if cfg.ISODir == "" {
			cfg.ISODir = config.DefaultISODir
		}
	}
	if req.DefaultNetwork != nil {
		cfg.DefaultNetwork = *req.DefaultNetwork
	}
	if req.NetworkBackend != nil {
		backend := strings.TrimSpace(*req.NetworkBackend)
		if backend == "" {
			backend = "ovs"
		}
		if backend != "ovs" {
			c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": "网络后端当前仅支持 OVS"})
			return
		}
		cfg.NetworkBackend = backend
	}
	if req.OVSBridge != nil {
		cfg.OVSBridge = strings.TrimSpace(*req.OVSBridge)
	}
	if req.OVSUplink != nil {
		cfg.OVSUplink = strings.TrimSpace(*req.OVSUplink)
	}
	if req.OVSDHCPStart != nil {
		cfg.OVSDHCPStart = strings.TrimSpace(*req.OVSDHCPStart)
	}
	if req.OVSDHCPEnd != nil {
		cfg.OVSDHCPEnd = strings.TrimSpace(*req.OVSDHCPEnd)
	}
	if req.SubnetPrefix != nil {
		cfg.SubnetPrefix = *req.SubnetPrefix
	}
	if req.AutoPortStart != nil {
		if *req.AutoPortStart < 1024 || *req.AutoPortStart > 65535 {
			c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": "端口起始范围无效（1024-65535）"})
			return
		}
		cfg.AutoPortStart = *req.AutoPortStart
	}
	if req.AutoPortEnd != nil {
		if *req.AutoPortEnd < 1024 || *req.AutoPortEnd > 65535 {
			c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": "端口结束范围无效（1024-65535）"})
			return
		}
		cfg.AutoPortEnd = *req.AutoPortEnd
	}
	if req.HostIP != nil {
		cfg.HostIP = strings.TrimSpace(*req.HostIP)
	}
	if req.ExternalNIC != nil {
		cfg.ExternalNIC = *req.ExternalNIC
	}
	if req.MaxBurstInbound != nil {
		if *req.MaxBurstInbound < 0 {
			c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": "最大下行速率不能为负数"})
			return
		}
		cfg.MaxBurstInbound = *req.MaxBurstInbound
	}
	if req.MaxBurstOutbound != nil {
		if *req.MaxBurstOutbound < 0 {
			c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": "最大上行速率不能为负数"})
			return
		}
		cfg.MaxBurstOutbound = *req.MaxBurstOutbound
	}
	if req.RescueISO != nil {
		cfg.RescueISO = *req.RescueISO
	}
	if req.SpiceEnabledByDefault != nil {
		cfg.SpiceEnabledByDefault = *req.SpiceEnabledByDefault
	}
	if req.PublicBaseURL != nil {
		cfg.PublicBaseURL = strings.TrimSpace(*req.PublicBaseURL)
	}
	if req.SiteTitle != nil {
		cfg.SiteTitle = strings.TrimSpace(*req.SiteTitle)
		if cfg.SiteTitle == "" {
			cfg.SiteTitle = config.DefaultSiteTitle
		}
	}
	if req.DevelopmentMode != nil {
		cfg.DevelopmentMode = *req.DevelopmentMode
	}
	if req.MaintenanceMode != nil {
		cfg.MaintenanceMode = *req.MaintenanceMode
	}
	if req.MaintenanceServiceUnits != nil {
		cfg.MaintenanceServiceUnits = strings.TrimSpace(*req.MaintenanceServiceUnits)
	}
	if strings.TrimSpace(cfg.MaintenanceServiceUnits) == "" {
		cfg.MaintenanceServiceUnits = config.DefaultMaintenanceServiceUnits()
	}
	if req.MaintenanceVMShutdownTimeoutSeconds != nil {
		if *req.MaintenanceVMShutdownTimeoutSeconds < 5 || *req.MaintenanceVMShutdownTimeoutSeconds > 3600 {
			c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": "维护模式虚拟机关机等待时间需在 5 - 3600 秒之间"})
			return
		}
		cfg.MaintenanceVMShutdownTimeoutSeconds = *req.MaintenanceVMShutdownTimeoutSeconds
	}
	if req.SMTPHost != nil {
		cfg.SMTPHost = strings.TrimSpace(*req.SMTPHost)
	}
	if req.SMTPPort != nil {
		if *req.SMTPPort <= 0 || *req.SMTPPort > 65535 {
			c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": "SMTP 端口无效"})
			return
		}
		cfg.SMTPPort = *req.SMTPPort
	}
	if req.SMTPUsername != nil {
		cfg.SMTPUsername = strings.TrimSpace(*req.SMTPUsername)
	}
	if req.SMTPFromName != nil {
		cfg.SMTPFromName = strings.TrimSpace(*req.SMTPFromName)
	}
	if req.SMTPFromAddress != nil {
		cfg.SMTPFromAddress = strings.TrimSpace(*req.SMTPFromAddress)
	}
	if req.SMTPSecurity != nil {
		cfg.SMTPSecurity = strings.TrimSpace(*req.SMTPSecurity)
	}
	if req.SMTPTimeoutSeconds != nil {
		if *req.SMTPTimeoutSeconds <= 0 {
			c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": "SMTP 超时时间必须大于 0"})
			return
		}
		cfg.SMTPTimeoutSeconds = *req.SMTPTimeoutSeconds
	}
	if req.SMTPPassword != nil && strings.TrimSpace(*req.SMTPPassword) != "" {
		if err := service.SetSMTPPassword(*req.SMTPPassword); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "message": "SMTP 密码加密失败"})
			return
		}
	}
	if req.DynamicMemorySchedulerEnabled != nil {
		cfg.DynamicMemorySchedulerEnabled = *req.DynamicMemorySchedulerEnabled
	}
	if req.DynamicMemoryIntervalSeconds != nil {
		if *req.DynamicMemoryIntervalSeconds < 10 {
			c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": "动态内存调度间隔不能小于 10 秒"})
			return
		}
		cfg.DynamicMemoryIntervalSeconds = *req.DynamicMemoryIntervalSeconds
	}
	if req.DynamicMemoryHostReserveMB != nil {
		if *req.DynamicMemoryHostReserveMB < 512 {
			c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": "宿主机保留内存不能小于 512MB"})
			return
		}
		cfg.DynamicMemoryHostReserveMB = *req.DynamicMemoryHostReserveMB
	}
	if req.DynamicMemoryHostReservePercent != nil {
		if *req.DynamicMemoryHostReservePercent < 5 || *req.DynamicMemoryHostReservePercent > 80 {
			c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": "宿主机保留比例需在 5% - 80% 之间"})
			return
		}
		cfg.DynamicMemoryHostReservePercent = *req.DynamicMemoryHostReservePercent
	}
	if req.DynamicMemoryIncreaseThresholdPercent != nil {
		if *req.DynamicMemoryIncreaseThresholdPercent < 5 || *req.DynamicMemoryIncreaseThresholdPercent > 50 {
			c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": "增长触发阈值需在 5% - 50% 之间"})
			return
		}
		cfg.DynamicMemoryIncreaseThresholdPercent = *req.DynamicMemoryIncreaseThresholdPercent
	}
	if req.DynamicMemoryReclaimThresholdPercent != nil {
		if *req.DynamicMemoryReclaimThresholdPercent < 10 || *req.DynamicMemoryReclaimThresholdPercent > 90 {
			c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": "回收触发阈值需在 10% - 90% 之间"})
			return
		}
		cfg.DynamicMemoryReclaimThresholdPercent = *req.DynamicMemoryReclaimThresholdPercent
	}
	if req.DynamicMemoryCooldownSeconds != nil {
		if *req.DynamicMemoryCooldownSeconds < 30 {
			c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": "动态内存冷却时间不能小于 30 秒"})
			return
		}
		cfg.DynamicMemoryCooldownSeconds = *req.DynamicMemoryCooldownSeconds
	}
	if req.DynamicMemoryObservationHours != nil {
		if *req.DynamicMemoryObservationHours < 0 || *req.DynamicMemoryObservationHours > 168 {
			c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": "观察期需在 0 - 168 小时之间"})
			return
		}
		cfg.DynamicMemoryObservationHours = *req.DynamicMemoryObservationHours
	}
	if req.SchedulerEventRetentionHours != nil {
		if *req.SchedulerEventRetentionHours < 1 || *req.SchedulerEventRetentionHours > 2160 {
			c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": "调度事件保留时长需在 1 - 2160 小时之间"})
			return
		}
		cfg.SchedulerEventRetentionHours = *req.SchedulerEventRetentionHours
	}
	if req.PortForwardHTTPProbeEnabled != nil {
		cfg.PortForwardHTTPProbeEnabled = *req.PortForwardHTTPProbeEnabled
	}
	if req.PortForwardHTTPProbeIntervalMinutes != nil {
		if *req.PortForwardHTTPProbeIntervalMinutes < 5 || *req.PortForwardHTTPProbeIntervalMinutes > 1440 {
			c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": "端口转发 HTTP 探测间隔需在 5 - 1440 分钟之间"})
			return
		}
		cfg.PortForwardHTTPProbeIntervalMinutes = *req.PortForwardHTTPProbeIntervalMinutes
	}
	if req.PortForwardHTTPProbeTimeoutSeconds != nil {
		if *req.PortForwardHTTPProbeTimeoutSeconds < 1 || *req.PortForwardHTTPProbeTimeoutSeconds > 30 {
			c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": "端口转发 HTTP 探测超时需在 1 - 30 秒之间"})
			return
		}
		cfg.PortForwardHTTPProbeTimeoutSeconds = *req.PortForwardHTTPProbeTimeoutSeconds
	}
	if req.DefaultDiskIOPSTotal != nil {
		if *req.DefaultDiskIOPSTotal < 0 {
			c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": "默认总 IOPS 限制不能为负数"})
			return
		}
		cfg.DefaultDiskIOPSTotal = *req.DefaultDiskIOPSTotal
	}
	if req.DefaultDiskIOPSRead != nil {
		if *req.DefaultDiskIOPSRead < 0 {
			c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": "默认读 IOPS 限制不能为负数"})
			return
		}
		cfg.DefaultDiskIOPSRead = *req.DefaultDiskIOPSRead
	}
	if req.DefaultDiskIOPSWrite != nil {
		if *req.DefaultDiskIOPSWrite < 0 {
			c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": "默认写 IOPS 限制不能为负数"})
			return
		}
		cfg.DefaultDiskIOPSWrite = *req.DefaultDiskIOPSWrite
	}
	if req.BatchCloneMaxConcurrency != nil {
		if *req.BatchCloneMaxConcurrency < 1 || *req.BatchCloneMaxConcurrency > 100 {
			c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": "批量克隆最大并发数需在 1 - 100 之间"})
			return
		}
		cfg.BatchCloneMaxConcurrency = *req.BatchCloneMaxConcurrency
	}
	if req.JWTSecretRotateHours != nil {
		if *req.JWTSecretRotateHours < 0 || *req.JWTSecretRotateHours > 720 {
			c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": "JWT 密钥轮换间隔需在 0 - 720 小时之间"})
			return
		}
		cfg.JWTSecretRotateHours = *req.JWTSecretRotateHours
	}
	if req.LogMaxBackups != nil {
		if *req.LogMaxBackups < 0 || *req.LogMaxBackups > 10000 {
			c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": "日志最大备份数需在 0 - 10000 之间"})
			return
		}
		cfg.LogMaxBackups = *req.LogMaxBackups
	}
	networkWaitOnlineChanged := false
	if req.NetworkWaitOnlineDisabled != nil {
		networkWaitOnlineChanged = *req.NetworkWaitOnlineDisabled != cfg.NetworkWaitOnlineDisabled
		cfg.NetworkWaitOnlineDisabled = *req.NetworkWaitOnlineDisabled
	}
	if req.SessionFingerprintEnabled != nil {
		cfg.SessionFingerprintEnabled = *req.SessionFingerprintEnabled
	}
	if req.RequestFilterEnabled != nil {
		cfg.RequestFilterEnabled = *req.RequestFilterEnabled
	}
	if req.PasswordBreachCheckEnabled != nil {
		cfg.PasswordBreachCheckEnabled = *req.PasswordBreachCheckEnabled
	}

	if cfg.AutoPortStart >= cfg.AutoPortEnd {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": "端口起始值必须小于结束值"})
		return
	}
	persistErrors := persistSettings(cfg)
	if len(persistErrors) > 0 {
		c.JSON(http.StatusOK, gin.H{"code": 200, "message": fmt.Sprintf("设置已更新，但部分持久化失败: %v", persistErrors)})
		return
	}

	// 带宽设置变更后异步触发全局带宽重新分配
	if req.MaxBurstInbound != nil || req.MaxBurstOutbound != nil {
		go func() {
			if cfg.MaxBurstInbound <= 0 && cfg.MaxBurstOutbound <= 0 {
				if err := service.ClearGlobalBandwidthLimit(); err != nil {
					logger.App.Warn("清除全局带宽限制失败", "component", "全局带宽", "error", err)
				}
				return
			}
			if err := service.ApplyGlobalBandwidthLimit(); err != nil {
				logger.App.Warn("应用全局带宽限制失败", "component", "全局带宽", "error", err)
			}
		}()
	}

	// 网络等待就绪检测设置变更后执行 systemctl 操作
	if networkWaitOnlineChanged {
		if err := service.SetNetworkWaitOnlineDisabled(cfg.NetworkWaitOnlineDisabled); err != nil {
			logger.App.Warn("设置网络等待就绪检测失败", "disabled", cfg.NetworkWaitOnlineDisabled, "error", err)
			// 不阻断设置保存，仅记录警告
		}
	}

	if maintenanceChanged {
		taskType := model.TaskTypeEnterMaintenanceMode
		taskMessage := "设置已保存，维护模式启用任务已提交"
		if !cfg.MaintenanceMode {
			taskType = model.TaskTypeExitMaintenanceMode
			taskMessage = "设置已保存，维护模式恢复任务已提交"
		}
		username := c.GetString("username")
		task, err := taskqueue.SubmitWithStruct(taskType, service.MaintenanceModeTaskParams{
			ServiceUnits: service.ParseMaintenanceServiceUnits(cfg.MaintenanceServiceUnits),
		}, username)
		if err != nil {
			cfg.MaintenanceMode = previousMaintenanceMode
			revertErrors := persistSettings(cfg)
			message := "设置回滚失败，请检查系统设置"
			if len(revertErrors) == 0 {
				message = "提交维护模式任务失败，设置已回滚"
			}
			c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "message": message + ": " + err.Error()})
			return
		}

		c.JSON(http.StatusOK, gin.H{
			"code":    200,
			"message": taskMessage,
			"data": gin.H{
				"task_id": task.ID,
			},
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{"code": 200, "message": "设置已保存"})
}

// TestSMTP 测试 SMTP 发信
func TestSMTP(c *gin.Context) {
	var req TestSMTPRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": "请输入测试邮箱"})
		return
	}

	// 如果请求中携带了 SMTP 配置参数，则使用传入的配置直接测试（不保存）
	if strings.TrimSpace(req.SMTPHost) != "" {
		err := service.SendEmailWithConfig(service.SMTPTestConfig{
			Host:           req.SMTPHost,
			Port:           req.SMTPPort,
			Username:       req.SMTPUsername,
			Password:       req.SMTPPassword,
			FromName:       req.SMTPFromName,
			FromAddress:    req.SMTPFromAddress,
			Security:       req.SMTPSecurity,
			TimeoutSeconds: req.SMTPTimeoutSeconds,
		}, strings.TrimSpace(req.Email), "SMTP 测试邮件", "这是一封来自QVMConsole的 SMTP 测试邮件。")
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "message": "测试邮件发送失败: " + err.Error()})
			return
		}
		c.JSON(http.StatusOK, gin.H{"code": 200, "message": "测试邮件已发送"})
		return
	}

	// 兼容旧调用：未传 SMTP 配置时使用已保存的全局配置
	if err := service.SendEmail(strings.TrimSpace(req.Email), "SMTP 测试邮件", "这是一封来自QVMConsole的 SMTP 测试邮件。"); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "message": "测试邮件发送失败: " + err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"code": 200, "message": "测试邮件已发送"})
}

// RotateJWTSecret 手动轮换 JWT 密钥
func RotateJWTSecret(c *gin.Context) {
	if !requireHighRiskVerification(c, "rotate_jwt_secret") {
		return
	}
	if config.GlobalConfig.DevelopmentMode {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": "开发模式下不允许手动轮换 JWT 密钥"})
		return
	}

	newSecret, err := service.RotateJWTSecret()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "message": "轮换 JWT 密钥失败: " + err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"code":    200,
		"message": "JWT 密钥轮换成功，所有 Token 已失效，请重新登录",
		"data": gin.H{
			"new_secret_prefix": newSecret[:8] + "...",
		},
	})
}

func persistSettings(cfg *config.Config) []string {
	settingsMap := cfg.ToSettingsMap()
	var persistErrors []string
	for key, value := range settingsMap {
		if value == "" || (value == "0" && key != "dynamic_memory_observation_hours" && key != "spice_enabled_by_default") {
			_ = model.DeleteSetting(key)
			continue
		}
		if err := model.SetSetting(key, value); err != nil {
			persistErrors = append(persistErrors, fmt.Sprintf("%s: %v", key, err))
		}
	}
	// 同步 .env 文件，确保面板重启后环境变量与数据库一致
	config.SyncEnvFile()
	return persistErrors
}

// GetCPUAffinityPresets 获取 CPU 亲和性预设列表（所有登录用户可访问）
func GetCPUAffinityPresets(c *gin.Context) {
	presets := service.GetCPUAffinityPresets()
	c.JSON(http.StatusOK, gin.H{
		"code":    200,
		"message": "ok",
		"data":    presets,
	})
}

// SaveCPUAffinityPresetsRequest 保存 CPU 亲和性预设请求
type SaveCPUAffinityPresetsRequest struct {
	Presets []service.CPUAffinityPreset `json:"presets" binding:"required"`
}

// SaveCPUAffinityPresets 保存 CPU 亲和性预设列表（管理员专用）
func SaveCPUAffinityPresets(c *gin.Context) {
	var req SaveCPUAffinityPresetsRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": "参数错误"})
		return
	}
	if err := service.SaveCPUAffinityPresets(req.Presets); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "message": "保存预设失败: " + err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"code": 200, "message": "预设已保存"})
}

// GetUserStorageISOPath 获取当前用户的存储 ISO 目录路径（用于一键修改系统 ISO 存放位置）
func GetUserStorageISOPath(c *gin.Context) {
	username := c.GetString("username")
	if username == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"code": 401, "message": "未获取到用户信息"})
		return
	}
	isoPath := service.GetUserISODir(username)
	c.JSON(http.StatusOK, gin.H{
		"code":    200,
		"message": "ok",
		"data": gin.H{
			"iso_path": isoPath,
		},
	})
}

// ==================== 日志管理 ====================

// LogFileInfo 日志文件信息
type LogFileInfo struct {
	Name     string `json:"name"`     // 文件名
	Size     int64  `json:"size"`     // 文件大小（字节）
	ModTime  string `json:"mod_time"` // 修改时间
	IsToday  bool   `json:"is_today"` // 是否为今日日志（未归档）
	Category string `json:"category"` // 日志类型：app, request, cmd, libvirt
}

// LogStatusResponse 日志状态响应
type LogStatusResponse struct {
	TotalSize      int64         `json:"total_size"`       // 总占用磁盘大小（字节）
	TotalSizeHuman string        `json:"total_size_human"` // 人类可读的总大小
	Files          []LogFileInfo `json:"files"`            // 日志文件列表
	Categories     []string      `json:"categories"`       // 日志类别列表
}

// GetLogStatus 获取日志状态（文件列表、磁盘占用）
func GetLogStatus(c *gin.Context) {
	logDir := logger.GetLogDir()
	if logDir == "" {
		logDir = config.GlobalConfig.LogDir
	}

	entries, err := os.ReadDir(logDir)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{
			"code":    200,
			"message": "ok",
			"data": LogStatusResponse{
				TotalSize:      0,
				TotalSizeHuman: "0 B",
				Files:          []LogFileInfo{},
				Categories:     []string{"app", "request", "cmd", "libvirt"},
			},
		})
		return
	}

	today := time.Now().Format("2006-01-02")
	var files []LogFileInfo
	var totalSize int64

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		// 仅处理日志相关文件
		if !strings.HasSuffix(name, ".log") && !strings.HasSuffix(name, ".log.gz") {
			continue
		}

		info, err := entry.Info()
		if err != nil {
			continue
		}

		size := info.Size()
		totalSize += size
		modTime := info.ModTime().Format("2006-01-02 15:04:05")

		// 判断是否为今日日志（未归档的当前文件）
		// 当前日志文件格式如: app.log, request.log, cmd.log, libvirt.log
		isToday := false
		category := ""

		// 检查是否为当前日志文件（无时间戳后缀）
		for _, cat := range []string{"app", "request", "cmd", "libvirt"} {
			if name == cat+".log" {
				isToday = true
				category = cat
				break
			}
		}

		// 如果不是当前文件，从文件名中提取类别
		if category == "" {
			for _, cat := range []string{"app", "request", "cmd", "libvirt"} {
				if strings.HasPrefix(name, cat+"-") || strings.HasPrefix(name, cat+".") {
					category = cat
					break
				}
			}
			// 检查归档文件是否也是今天的（修改时间在今天）
			if strings.Contains(info.ModTime().Format("2006-01-02"), today) {
				isToday = true
			}
		}

		files = append(files, LogFileInfo{
			Name:     name,
			Size:     size,
			ModTime:  modTime,
			IsToday:  isToday,
			Category: category,
		})
	}

	// 按修改时间倒序排序
	sort.Slice(files, func(i, j int) bool {
		return files[i].ModTime > files[j].ModTime
	})

	c.JSON(http.StatusOK, gin.H{
		"code":    200,
		"message": "ok",
		"data": LogStatusResponse{
			TotalSize:      totalSize,
			TotalSizeHuman: formatBytes(totalSize),
			Files:          files,
			Categories:     []string{"app", "request", "cmd", "libvirt"},
		},
	})
}

// DeleteLogsRequest 删除日志请求
type DeleteLogsRequest struct {
	Files []string `json:"files" binding:"required"` // 要删除的文件名列表
}

// DeleteLogs 删除指定日志文件
func DeleteLogs(c *gin.Context) {
	var req DeleteLogsRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": "参数错误"})
		return
	}

	if len(req.Files) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": "请选择要删除的日志文件"})
		return
	}

	logDir := logger.GetLogDir()
	if logDir == "" {
		logDir = config.GlobalConfig.LogDir
	}

	var deleted []string
	var failed []string

	for _, filename := range req.Files {
		// 安全校验：确保文件名不包含路径穿越
		baseName := filepath.Base(filename)
		if baseName != filename {
			failed = append(failed, filename)
			continue
		}
		// 只允许删除日志文件
		if !strings.HasSuffix(baseName, ".log") && !strings.HasSuffix(baseName, ".log.gz") {
			failed = append(failed, filename)
			continue
		}

		fullPath := filepath.Join(logDir, baseName)
		if err := os.Remove(fullPath); err != nil {
			failed = append(failed, filename)
			logger.App.Warn("删除日志文件失败", "file", filename, "error", err)
		} else {
			deleted = append(deleted, filename)
		}
	}

	c.JSON(http.StatusOK, gin.H{
		"code":    200,
		"message": fmt.Sprintf("成功删除 %d 个文件", len(deleted)),
		"data": gin.H{
			"deleted": deleted,
			"failed":  failed,
		},
	})
}

// ExportLogsRequest 导出日志请求
type ExportLogsRequest struct {
	Files []string `json:"files" binding:"required"` // 要导出的文件名列表
}

// ExportLogs 导出选中的日志文件为 ZIP 压缩包
func ExportLogs(c *gin.Context) {
	var req ExportLogsRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": "参数错误"})
		return
	}

	if len(req.Files) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": "请选择要导出的日志文件"})
		return
	}

	logDir := logger.GetLogDir()
	if logDir == "" {
		logDir = config.GlobalConfig.LogDir
	}

	// 生成导出文件名
	exportName := fmt.Sprintf("qvmconsole-logs-%s.zip", time.Now().Format("20060102-150405"))

	c.Header("Content-Type", "application/zip")
	c.Header("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, exportName))

	zipWriter := zip.NewWriter(c.Writer)
	defer zipWriter.Close()

	for _, filename := range req.Files {
		// 安全校验
		baseName := filepath.Base(filename)
		if baseName != filename {
			continue
		}
		if !strings.HasSuffix(baseName, ".log") && !strings.HasSuffix(baseName, ".log.gz") {
			continue
		}

		fullPath := filepath.Join(logDir, baseName)
		file, err := os.Open(fullPath)
		if err != nil {
			logger.App.Warn("导出日志文件失败：无法打开", "file", filename, "error", err)
			continue
		}

		// 在 ZIP 中使用清理后的文件名（去掉路径）
		zipEntry, err := zipWriter.Create(baseName)
		if err != nil {
			file.Close()
			logger.App.Warn("导出日志文件失败：无法创建 ZIP 条目", "file", filename, "error", err)
			continue
		}

		_, err = io.Copy(zipEntry, file)
		file.Close()
		if err != nil {
			logger.App.Warn("导出日志文件失败：写入 ZIP 错误", "file", filename, "error", err)
		}
	}
}

// TrimUserStorage 执行用户存储回收（fstrim + fallocate --dig-holes）
func TrimUserStorage(c *gin.Context) {
	result, err := quota.TrimStorage()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "message": "存储回收失败: " + err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"code":    200,
		"message": "存储回收完成",
		"data":    result,
	})
}

// networkWaitOnlineSummary 生成网络等待就绪检测的状态摘要
func networkWaitOnlineSummary(disabled bool) string {
	if disabled {
		return "已禁用 — systemd-networkd-wait-online.service 已 disable + mask，系统开机不再等待网络就绪，适合 OVS 桥接环境"
	}
	return "已启用 — systemd-networkd-wait-online.service 正常运行，OVS 桥接后开机可能卡在此服务上"
}

// formatBytes 将字节数转换为人类可读格式
func formatBytes(bytes int64) string {
	const unit = 1024
	if bytes < unit {
		return fmt.Sprintf("%d B", bytes)
	}
	div, exp := int64(unit), 0
	for n := bytes / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(bytes)/float64(div), "KMGTPE"[exp])
}
