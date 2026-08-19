package router

import (
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"kvm_console/config"
	"kvm_console/handler"
	"kvm_console/logger"
	"kvm_console/middleware"
)

// Setup 初始化路由
func Setup() *gin.Engine {
	r := gin.New()

	// 配置可信代理
	if len(config.GlobalConfig.TrustedProxies) > 0 {
		r.SetTrustedProxies(config.GlobalConfig.TrustedProxies)
	} else {
		r.SetTrustedProxies(nil) // 不信任任何代理头
	}

	r.Use(middleware.RequestLoggerMiddleware(), middleware.SafeRecoveryMiddleware())

	// 全局中间件
	r.Use(middleware.CORSMiddleware())
	r.Use(middleware.SecurityHeadersMiddleware())
	r.Use(middleware.RequestFilterMiddleware())
	r.Use(middleware.RequestGuardMiddleware())

	// 全局 API 限频
	rlConfig := middleware.RateLimitConfig{
		PublicPerMinute: config.GlobalConfig.RateLimitPublicPerMin,
		AuthPerMinute:   config.GlobalConfig.RateLimitAuthPerMin,
		CleanupInterval: 5 * time.Minute,
	}
	rateLimiter := middleware.NewRateLimiter(rlConfig)
	r.Use(middleware.RateLimitMiddleware(rateLimiter))

	// API 路由组
	api := r.Group("/api")
	{
		api.GET("/public/settings", handler.GetPublicSettings)
		api.GET("/public/version", handler.GetVersion)

		// ==================== 认证（无需登录） ====================
		auth := api.Group("/auth")
		{
			auth.POST("/login", handler.Login)
			auth.GET("/invite", handler.GetInviteInfo)
			auth.POST("/invite/complete", handler.CompleteInvite)
			auth.POST("/password/forgot", handler.ForgotPassword)
			auth.POST("/password/forgot/send-code", handler.ForgotPasswordSendCode)
			auth.POST("/password/forgot/verify-code", handler.ForgotPasswordVerifyCode)
			auth.POST("/password/forgot/select-account", handler.ForgotPasswordSelectAccount)
			auth.POST("/password/reset", handler.ResetPasswordByEmail)
			auth.POST("/check-password", handler.CheckPasswordBreach)
		}

		// ==================== 登录中间态验证 ====================
		loginAuth := auth.Group("/login")
		loginAuth.Use(middleware.TokenTypeMiddleware("login"))
		{
			loginAuth.POST("/email/send", handler.SendLoginEmailCode)
			loginAuth.POST("/verify", handler.VerifyLoginStage)
		}

		// ==================== 安全初始化与安全设置 ====================
		secureAuth := auth.Group("")
		secureAuth.Use(middleware.JWTTokenTypeMiddleware("access", "bootstrap"))
		{
			secureAuth.POST("/email/code/send", handler.SendEmailCode)
			secureAuth.POST("/email/bind", handler.BindEmail)
			secureAuth.POST("/2fa/setup", handler.SetupTOTP)
			secureAuth.POST("/2fa/enable", handler.EnableTOTP)
			secureAuth.POST("/2fa/disable", handler.DisableTOTP)
			secureAuth.POST("/2fa/recovery/regen", handler.RegenRecoveryCodes)
			secureAuth.POST("/skip-bootstrap", handler.SkipBootstrap) // 管理员跳过安全初始化
		}

		// ==================== 高风险验证 ====================
		highRiskAuth := auth.Group("")
		highRiskAuth.Use(middleware.AuthMiddleware())
		highRiskAuth.Use(middleware.ForcePasswordChangeMiddleware())
		{
			highRiskAuth.GET("/info", handler.GetUserInfo)
			highRiskAuth.GET("/api-key", handler.GetAPIKeyInfo)
			highRiskAuth.POST("/api-key", handler.RotateAPIKey)
			highRiskAuth.DELETE("/api-key", handler.RevokeAPIKey)
			highRiskAuth.PUT("/password", handler.ChangePassword)
			highRiskAuth.PUT("/username", handler.ChangeUsername)
			highRiskAuth.POST("/high-risk/verify", handler.VerifyHighRisk)
		}

		// ==================== 系统设置（管理员 access/bootstrap 均可） ====================
		settings := api.Group("/settings")
		settings.Use(middleware.TokenTypeMiddleware("access", "bootstrap"), middleware.AdminMiddleware())
		{
			settings.GET("", handler.GetSettings)
			settings.PUT("", handler.UpdateSettings)
			settings.GET("/user-storage-iso-path", handler.GetUserStorageISOPath)
			settings.POST("/smtp/test", handler.TestSMTP)
			settings.PUT("/cpu-affinity-presets", handler.SaveCPUAffinityPresets)
			settings.POST("/jwt-secret/rotate", handler.RotateJWTSecret)
			settings.GET("/log/status", handler.GetLogStatus)
			settings.GET("/log/read", handler.ReadLogFile) // 读取日志文件内容（在线预览，向前分页）
			settings.POST("/log/delete", handler.DeleteLogs)
			settings.POST("/log/export", handler.ExportLogs)
			settings.GET("/diagnostics/categories", handler.GetDiagnosticCategories)
			settings.POST("/diagnostics/export", handler.ExportDiagnostics)
			settings.POST("/storage/trim", handler.TrimUserStorage)
		}

		// ==================== 需要认证的路由 ====================
		authorized := api.Group("")
		authorized.Use(middleware.AuthMiddleware())
		authorized.Use(middleware.ForcePasswordChangeMiddleware())
		{
			security := authorized.Group("/security")
			security.Use(middleware.AdminMiddleware())
			{
				security.GET("/password-breach/status", handler.GetPasswordBreachStatus) // 获取泄露密码状态
				security.POST("/password-breach/scan", handler.StartPasswordBreachScan)  // 立即执行泄露密码检测
			}

			// ==================== 虚拟机管理 ====================
			vm := authorized.Group("/vm")
			vm.Use(middleware.VMAccessMiddleware()) // 非admin用户操作VM时校验归属权限
			{
				vm.GET("/list", handler.GetVmList)
				vm.GET("/sse", handler.GetVmListSSE)
				vm.GET("/:name", handler.GetVmDetail)
				vm.GET("/:name/xml", middleware.ElasticCloudOnlyMiddleware(), middleware.AdminMiddleware(), handler.GetVmXML)
				vm.GET("/:name/ip", handler.GetVmIP)
				vm.GET("/:name/sse", handler.GetVmDetailSSE)
				vm.GET("/:name/pcie-info", handler.GetVmPCIEInfo)
				vm.POST("/:name/operate", handler.OperateVm)
				vm.PUT("/:name", middleware.ElasticCloudOnlyMiddleware(), handler.EditVm)
				vm.PUT("/:name/xml", middleware.ElasticCloudOnlyMiddleware(), middleware.AdminMiddleware(), handler.UpdateVmXML)
				vm.GET("/:name/stats", handler.GetVmStats)
				vm.GET("/:name/stats/history", handler.GetVmStatsHistory)
				vm.GET("/:name/schedules", middleware.ElasticCloudOnlyMiddleware(), handler.GetVMSchedules)
				vm.POST("/:name/schedules", middleware.ElasticCloudOnlyMiddleware(), handler.CreateVMSchedule)
				vm.PUT("/:name/schedules/:id", middleware.ElasticCloudOnlyMiddleware(), handler.UpdateVMSchedule)
				vm.DELETE("/:name/schedules/:id", middleware.ElasticCloudOnlyMiddleware(), handler.DeleteVMSchedule)
				vm.GET("/:name/network/status", handler.GetVMNetworkRuntimeStatus)
				vm.GET("/:name/network/diagnostics", middleware.AdminMiddleware(), handler.GetVMNetworkDiagnostics)
				vm.POST("/:name/network/capture", middleware.AdminMiddleware(), handler.StartVMNetworkCapture)
				vm.GET("/:name/vpc", handler.GetVMVPCBinding)
				vm.PUT("/:name/vpc", handler.BindVMVPC)
				vm.POST("/:name/migration/preview", middleware.AdminMiddleware(), handler.PreviewVMMigration)
				vm.POST("/:name/migrate", middleware.AdminMiddleware(), handler.MigrateVM)
				vm.PUT("/:name/security-group", handler.SwitchVMSecurityGroup)
			// 多网口管理（管理员全量；弹性云用户可自助管理本人虚拟机的附加网口）
			vm.GET("/:name/interfaces", handler.ListVMInterfaces)
				vm.POST("/:name/interfaces", handler.AddVMInterface)
				vm.PUT("/:name/interfaces/:order", handler.UpdateVMInterface)
				vm.DELETE("/:name/interfaces/:order", handler.RemoveVMInterface)
				vm.DELETE("/:name", middleware.ElasticCloudOnlyMiddleware(), handler.DeleteVm)
				vm.POST("/:name/force-delete", middleware.ElasticCloudOnlyMiddleware(), middleware.AdminMiddleware(), handler.ForceDeleteVm)
				vm.GET("/:name/qcow2-disks", handler.GetVmQcow2Disks)

				// 虚拟机锁定管理
				vm.POST("/:name/lock", middleware.ElasticCloudOnlyMiddleware(), handler.LockVM)
				vm.POST("/:name/unlock", middleware.ElasticCloudOnlyMiddleware(), handler.UnlockVM)
				vm.GET("/:name/lock", handler.GetVMLockStatus)

				// 硬件直通
				vm.GET("/:name/passthrough", handler.GetVMPassthroughDevices)
				vm.POST("/:name/passthrough", middleware.ElasticCloudOnlyMiddleware(), middleware.AdminMiddleware(), handler.AttachPCIDeviceToVM)
				vm.DELETE("/:name/passthrough", middleware.ElasticCloudOnlyMiddleware(), middleware.AdminMiddleware(), handler.DetachPCIDeviceFromVM)

				// 普通创建
				vm.POST("/create", middleware.ElasticCloudOnlyMiddleware(), handler.CreateVm)
				vm.POST("/import-disk", middleware.ElasticCloudOnlyMiddleware(), middleware.AdminMiddleware(), handler.AdminImportDisk)
				vm.POST("/import-appliance/inspect", middleware.ElasticCloudOnlyMiddleware(), middleware.AdminMiddleware(), handler.InspectAdminAppliance) // 检查 OVF/OVA 虚拟机包
				vm.POST("/import-appliance", middleware.ElasticCloudOnlyMiddleware(), middleware.AdminMiddleware(), handler.ImportAdminAppliance)          // 导入 OVF/OVA 虚拟机包
				vm.GET("/os-variants", handler.GetOSVariants)
				vm.GET("/iso-list", handler.GetISOList)

				// 克隆
				vm.POST("/clone", middleware.ElasticCloudOnlyMiddleware(), handler.CloneVm)
				vm.POST("/linked-clone", middleware.ElasticCloudOnlyMiddleware(), middleware.AdminMiddleware(), handler.LinkedCloneVm)
				vm.POST("/batch-clone", middleware.ElasticCloudOnlyMiddleware(), handler.BatchCloneVm)
				vm.POST("/:name/reinstall", middleware.ElasticCloudOnlyMiddleware(), handler.ReinstallVm)

				// 快照
				vm.GET("/:name/snapshots", handler.GetSnapshots)
				vm.DELETE("/:name/snapshots", handler.DeleteAllSnapshots)
				vm.POST("/:name/snapshot", handler.CreateSnapshot)
				vm.POST("/:name/snapshot/:snap/revert", handler.RevertSnapshot)
				vm.DELETE("/:name/snapshot/:snap", handler.DeleteSnapshot)

				// VNC
				vm.GET("/:name/vnc/status", handler.GetVncStatus)
				vm.POST("/:name/vnc/enable", handler.EnableVnc)
				vm.POST("/:name/vnc/disable", handler.DisableVnc)
				vm.POST("/:name/vnc/passwd", handler.ChangeVncPassword)
				vm.POST("/:name/vnc/expose", handler.ExposeVnc)
				vm.GET("/:name/vnc/ws", handler.VncWebSocket)

				// SPICE（外部客户端直连，不走 WS 代理；提供 .vv 下载）
				vm.GET("/:name/spice/status", handler.GetSpiceStatus)
				vm.GET("/:name/spice/info", handler.GetSpiceConnInfoHandler)
				vm.POST("/:name/spice/enable", handler.EnableSpice)
				vm.POST("/:name/spice/disable", handler.DisableSpice)
				vm.POST("/:name/spice/passwd", handler.ChangeSpicePassword)
				vm.POST("/:name/spice/expose", handler.ExposeSpice)
				vm.GET("/:name/spice/vv", handler.DownloadSpiceVV)

				// 磁盘管理
				vm.GET("/:name/disks", handler.GetDiskList)
				vm.GET("/:name/disk-migration/options", middleware.AdminMiddleware(), handler.GetDiskMigrationOptions)
				vm.POST("/:name/disk", middleware.ElasticCloudOnlyMiddleware(), handler.AddDisk)
				vm.POST("/:name/disk/:dev/resize", middleware.ElasticCloudOnlyMiddleware(), handler.ResizeDisk)
				vm.GET("/:name/disk/:dev/guest-status", handler.GetDiskGuestStatus)                                      // 获取磁盘来宾映射与文件系统状态
				vm.POST("/:name/disk/:dev/guest-mount", middleware.ElasticCloudOnlyMiddleware(), handler.GuestMountDisk) // 配置或重试来宾磁盘挂载
				vm.POST("/:name/disk/:dev/guest-grow", middleware.ElasticCloudOnlyMiddleware(), handler.GuestGrowDisk)   // 重试 Linux 系统分区扩容
				vm.PUT("/:name/disk/:dev/bus", middleware.ElasticCloudOnlyMiddleware(), handler.ChangeDiskBus)
				vm.POST("/:name/disk/attach", middleware.ElasticCloudOnlyMiddleware(), handler.AttachDisk)
				vm.POST("/:name/disk/import", middleware.ElasticCloudOnlyMiddleware(), middleware.AdminMiddleware(), handler.ImportDiskForVM)
				vm.POST("/:name/disk/:dev/migrate", middleware.AdminMiddleware(), handler.MigrateDisk)
				vm.DELETE("/:name/disk/:dev", middleware.ElasticCloudOnlyMiddleware(), handler.DeleteDisk)
				vm.GET("/:name/disk/:dev/iops", handler.GetDiskIOPS)
				vm.PUT("/:name/disk/:dev/iops", middleware.AdminMiddleware(), handler.SetDiskIOPS)

				// CD/DVD 管理
				vm.POST("/:name/cdrom", middleware.ElasticCloudOnlyMiddleware(), handler.ChangeCDROM)
				vm.PUT("/:name/cdrom/:dev/bus", middleware.ElasticCloudOnlyMiddleware(), handler.ChangeCDROMBus) // 修改光驱驱动类型
				vm.POST("/:name/cdrom/eject", middleware.ElasticCloudOnlyMiddleware(), handler.EjectCDROM)
				vm.DELETE("/:name/cdrom", middleware.ElasticCloudOnlyMiddleware(), handler.RemoveCDROMHandler)

				// 软盘管理
				vm.POST("/:name/floppy", middleware.ElasticCloudOnlyMiddleware(), handler.ChangeFloppy)
				vm.POST("/:name/floppy/eject", middleware.ElasticCloudOnlyMiddleware(), handler.EjectFloppy)
				vm.DELETE("/:name/floppy", middleware.ElasticCloudOnlyMiddleware(), handler.RemoveFloppyHandler)

				// 救援系统
				vm.POST("/:name/rescue", handler.RescueVm)
				vm.POST("/:name/password/reset", handler.ResetLinuxPassword)

				// 转为独立虚拟机（仅管理员）
				vm.POST("/:name/make-independent", middleware.ElasticCloudOnlyMiddleware(), middleware.AdminMiddleware(), handler.MakeVMIndependent)

				// 共享目录
				vm.GET("/:name/shares", middleware.ElasticCloudOnlyMiddleware(), handler.GetShareList)
				vm.POST("/:name/share", middleware.ElasticCloudOnlyMiddleware(), handler.AddShare)
				vm.DELETE("/:name/share/:tag", middleware.ElasticCloudOnlyMiddleware(), handler.DeleteShare)
			}

			// ==================== 模板管理 ====================
			tpl := authorized.Group("/template")
			tpl.Use(middleware.ElasticCloudOnlyMiddleware())
			{
				tpl.GET("/list", handler.GetTemplateList)
				tpl.POST("/prepare", handler.PrepareTemplate)
				tpl.GET("/:name/prepare-linux/check", middleware.AdminMiddleware(), handler.GetLinuxTemplatePrepareCheck) // 检查 Linux 模板预处理链式依赖
				tpl.POST("/:name/prepare-linux", middleware.AdminMiddleware(), handler.PrepareImportedLinuxTemplate)
				tpl.POST("/upload/init", handler.TemplateUploadInit)         // 模板包分片上传-初始化/秒传
				tpl.POST("/upload/chunk", handler.TemplateUploadChunk)       // 模板包分片上传-单片
				tpl.POST("/upload/complete", handler.TemplateUploadComplete) // 模板包分片上传-完成
				tpl.DELETE("/upload", handler.TemplateUploadCancel)          // 清理已上传的模板临时包
				tpl.POST("/import", handler.ImportTemplateHandler)
				tpl.POST("/import/preview", handler.PreviewImportTemplateHandler)
				tpl.POST("/import/confirm", handler.ConfirmImportTemplateHandler)
				tpl.GET("/download/:filename", handler.DownloadTemplateExportHandler)
				tpl.GET("/:name/delete-preview", handler.GetDeleteTemplatePreview)
				tpl.GET("/:name/vms", handler.GetTemplateVMs)
				tpl.POST("/:name/export", handler.ExportTemplateHandler)
				tpl.DELETE("/:name/export", handler.DeleteExportedTemplateHandler)
				tpl.PUT("/:name/publish", handler.UpdateTemplatePublish)
				tpl.PUT("/:name/meta", handler.UpdateTemplateMeta)
				tpl.DELETE("/:name", handler.DeleteTemplate)
			}

			// ==================== 网络管理 ====================
			network := authorized.Group("/network")
			{
				// 静态 IP
				network.GET("/static-ip/list", handler.GetStaticIPList)
				network.POST("/static-ip/bind", handler.BindStaticIP)
				network.POST("/static-ip/unbind", middleware.ElasticCloudOnlyMiddleware(), handler.UnbindStaticIP)

				// 端口转发
				network.GET("/port-forward/list", handler.GetPortForwardList)
				network.POST("/port-forward/add", handler.AddPortForward)
				network.PUT("/port-forward/:id", handler.UpdatePortForward)
				network.DELETE("/port-forward/:id", handler.DeletePortForward)
				network.POST("/port-forward/batch-delete", handler.BatchDeletePortForward)
				network.POST("/port-forward/save", handler.SavePortForwardRules)

				// 端口转发手动 IP 映射
				network.GET("/port-forward/ip-mapping", handler.GetPortForwardIPs)
				network.POST("/port-forward/ip-mapping", middleware.ElasticCloudOnlyMiddleware(), handler.AddPortForwardIP)
				network.DELETE("/port-forward/ip-mapping/:id", middleware.ElasticCloudOnlyMiddleware(), handler.DeletePortForwardIP)

				// UFW 防火墙
				network.GET("/ufw/status", middleware.AdminMiddleware(), handler.GetUFWStatus)
				network.POST("/ufw/rule", middleware.AdminMiddleware(), handler.ManageUFWRule)

				// 宿主机网桥管理
				network.GET("/host/interfaces", middleware.AdminMiddleware(), handler.ListHostInterfaces)
				network.GET("/bridges", middleware.AdminMiddleware(), handler.ListNetworkBridges)
				network.POST("/bridges", middleware.AdminMiddleware(), handler.CreateNetworkBridge)
				network.DELETE("/bridges/:id", middleware.AdminMiddleware(), handler.DeleteNetworkBridge)

				// 接口 IP/DNS 配置
				network.GET("/interfaces/:name/config", middleware.AdminMiddleware(), handler.GetInterfaceConfig)
				network.PUT("/interfaces/:name/config", middleware.AdminMiddleware(), handler.SetInterfaceConfig)

			// 公网 IP / 浮动 IP
			network.GET("/public-ips", middleware.AdminMiddleware(), handler.ListPublicIPs)
			network.POST("/public-ips", middleware.AdminMiddleware(), handler.CreatePublicIP)
			network.GET("/public-ips/ipv6-prefixes", middleware.AdminMiddleware(), handler.DiscoverPublicIPv6Prefixes)     // 检测上联网卡公网 IPv6 前缀
	network.POST("/public-ips/ipv6-prefixes/import", middleware.AdminMiddleware(), handler.ImportPublicIPv6Prefix) // 批量导入公网 IPv6 /128 地址资源
	network.POST("/public-ips/batch", middleware.AdminMiddleware(), handler.BatchCreatePublicIPs)                 // 批量新增公网 IP（共用除 IP 外的字段）
	network.DELETE("/public-ips/batch", middleware.AdminMiddleware(), handler.BatchDeletePublicIPs)              // 批量删除公网 IP（已绑定的自动跳过）
	network.POST("/public-ips/batch/bind", middleware.AdminMiddleware(), handler.BatchBindPublicIPs)              // 批量绑定公网 IP（高风险，任务队列）
	network.POST("/public-ips/batch/unbind", middleware.AdminMiddleware(), handler.BatchUnbindPublicIPs)          // 批量解绑公网 IP（高风险，任务队列）
	network.PUT("/public-ips/:id", middleware.AdminMiddleware(), handler.UpdatePublicIP)
			network.DELETE("/public-ips/:id", middleware.AdminMiddleware(), handler.DeletePublicIP)
			network.POST("/public-ips/:id/preview", middleware.AdminMiddleware(), handler.PreviewPublicIP)
			network.POST("/public-ips/:id/bind", middleware.AdminMiddleware(), handler.BindPublicIP)
			network.POST("/public-ips/:id/unbind", middleware.AdminMiddleware(), handler.UnbindPublicIP)
			network.POST("/public-ips/:id/migrate", middleware.AdminMiddleware(), handler.MigratePublicIP)
			network.POST("/public-ips/apply", middleware.AdminMiddleware(), handler.ApplyPublicIPRules)

				// 网络抓包诊断
				network.GET("/captures/:task_id", middleware.AdminMiddleware(), handler.GetNetworkCaptureSession)
				network.GET("/captures/:task_id/download", middleware.AdminMiddleware(), handler.DownloadNetworkCapture)
				network.DELETE("/captures/:task_id", middleware.AdminMiddleware(), handler.DeleteNetworkCapture)
			}

			// ==================== VPC 网络与安全组 ====================
			vpc := authorized.Group("/vpc")
			{
				vpc.GET("/quota", middleware.ElasticCloudOnlyMiddleware(), handler.GetVPCQuota)
				vpc.GET("/switches", handler.ListVPCSwitches)
				vpc.POST("/switches", middleware.ElasticCloudOnlyMiddleware(), handler.CreateVPCSwitch)
				vpc.PUT("/switches/:id", middleware.ElasticCloudOnlyMiddleware(), handler.UpdateVPCSwitch)
				vpc.POST("/switches/:id/reconfigure", middleware.ElasticCloudOnlyMiddleware(), handler.ReconfigureVPCSwitch) // 异步重配置交换机拓扑
				vpc.POST("/switches/:id/traffic/reset", middleware.ElasticCloudOnlyMiddleware(), handler.ResetVPCSwitchTraffic)
				vpc.DELETE("/switches/:id", middleware.ElasticCloudOnlyMiddleware(), handler.DeleteVPCSwitch)
				vpc.GET("/switches/:id/vms", handler.GetVPCSwitchVMs)
				vpc.GET("/security-groups", handler.ListVPCSecurityGroups)
				vpc.POST("/security-groups", middleware.ElasticCloudOnlyMiddleware(), handler.CreateVPCSecurityGroup)
				vpc.PUT("/security-groups/:id", middleware.ElasticCloudOnlyMiddleware(), handler.UpdateVPCSecurityGroup)
				vpc.DELETE("/security-groups/:id", middleware.ElasticCloudOnlyMiddleware(), handler.DeleteVPCSecurityGroup)
				vpc.POST("/security-groups/:id/rules", handler.AddVPCSecurityGroupRule)
				vpc.PUT("/security-groups/rules/:id", handler.UpdateVPCSecurityGroupRule) // 编辑安全组规则（保存后重建 VPC ACL）
				vpc.DELETE("/security-groups/rules/:id", handler.DeleteVPCSecurityGroupRule)
				vpc.GET("/acl/preview", handler.PreviewVPCACL)
				vpc.POST("/acl/apply", handler.ApplyVPCACL)
			}

			// ==================== KVM 全局网络防火墙（管理员） ====================
			firewall := authorized.Group("/firewall")
			firewall.Use(middleware.AdminMiddleware())
			{
				firewall.GET("/status", handler.GetFirewallStatus)
				firewall.GET("/policy", handler.GetFirewallPolicy)
				firewall.PUT("/policy", handler.SaveFirewallPolicy)
				firewall.POST("/preview", handler.PreviewFirewallPolicy)
				firewall.POST("/apply", handler.ApplyFirewallPolicy)
				firewall.POST("/disable", handler.DisableFirewall)
				firewall.POST("/rollback", handler.RollbackFirewall)
				firewall.POST("/geoip/import", handler.ImportFirewallRegion)
				firewall.POST("/geoip/update", handler.UpdateFirewallGeoIP)
				firewall.PUT("/port-forward", handler.SetPortForwardFirewall)
				firewall.GET("/host/status", handler.GetHostFirewallStatus)
				firewall.POST("/host/enable/preview", handler.PreviewEnableHostFirewall)
				firewall.POST("/host/enable", handler.EnableHostFirewall)
				firewall.POST("/host/disable", handler.DisableHostFirewall)
				firewall.GET("/host/rules", handler.ListHostFirewallRules)
				firewall.POST("/host/rules", handler.CreateHostFirewallRule)
				firewall.PUT("/host/rules/:id", handler.UpdateHostFirewallRule)
				firewall.DELETE("/host/rules/:id", handler.DeleteHostFirewallRule)
				firewall.POST("/host/rules/vnc-default", handler.AddHostFirewallVNCDefaultRule)
				firewall.GET("/host/connections/preview", handler.PreviewHostFirewallConnections)
				firewall.POST("/host/connections/close", handler.CloseHostFirewallConnections)
			}

			// ==================== OVS 网络诊断（管理员） ====================
			ovs := authorized.Group("/ovs")
			ovs.Use(middleware.AdminMiddleware())
			{
				ovs.GET("/status", handler.GetOVSStatus)
				ovs.GET("/ports", handler.GetOVSPorts)
				ovs.GET("/leases", handler.GetOVSLeases)
				ovs.POST("/check", handler.CheckOVSNetwork)
				ovs.POST("/repair", handler.RepairOVSNetwork)
				ovs.GET("/port-security/status", handler.GetPortSecurityStatus)                 // 获取端口安全状态与端口诊断
				ovs.POST("/port-security/preflight", handler.PreflightPortSecurity)             // 只读预检端口安全能力和配置
				ovs.POST("/port-security/enable", handler.EnablePortSecurity)                   // 异步启用端口安全
				ovs.POST("/port-security/disable", handler.DisablePortSecurity)                 // 异步停用端口安全
				ovs.POST("/port-security/reconcile", handler.ReconcilePortSecurity)             // 异步协调全部端口策略
				ovs.POST("/port-security/ports/:port/isolate", handler.IsolatePortSecurityPort) // 异步隔离指定 OVS 端口
				ovs.POST("/port-security/ports/:port/release", handler.ReleasePortSecurityPort) // 异步释放指定 OVS 端口
				ovs.GET("/port-mirror/options", handler.GetPortMirrorOptions)                   // 获取端口镜像源接口和目标空交换机
				ovs.GET("/port-mirror/status", handler.GetPortMirrorStatus)                     // 获取端口镜像实时状态与计数
				ovs.POST("/port-mirror/enable", handler.EnablePortMirror)                       // 异步启用端口镜像
				ovs.POST("/port-mirror/disable", handler.DisablePortMirror)                     // 异步停用端口镜像并清理运行态
			}

			// ==================== 存储池管理 ====================
			storagePool := authorized.Group("/storage-pool")
			storagePool.Use(middleware.ElasticCloudOnlyMiddleware())
			{
				storagePool.GET("/list", middleware.AdminMiddleware(), handler.GetStoragePoolList)
				storagePool.GET("/all-isos", handler.GetAllISOs)
				storagePool.GET("/vm-targets", handler.GetVMStorageTargets)
				storagePool.GET("/:id", middleware.AdminMiddleware(), handler.GetStoragePoolDetail)
				storagePool.PUT("/:id/config", middleware.AdminMiddleware(), handler.UpdateStoragePoolConfig)
				storagePool.POST("/:id/default", middleware.AdminMiddleware(), handler.SetDefaultStoragePool)
				storagePool.POST("/:id/format-mount", middleware.AdminMiddleware(), handler.FormatMountStoragePool)
				storagePool.POST("/:id/create-partition", middleware.AdminMiddleware(), handler.CreateStoragePartition)
				storagePool.POST("/:id/delete-partitions", middleware.AdminMiddleware(), handler.DeleteStoragePartitions)
				storagePool.GET("/pv-targets", middleware.AdminMiddleware(), handler.GetAvailablePVTargets)
				storagePool.POST("/create-volume", middleware.AdminMiddleware(), handler.CreateStorageVolume)
				storagePool.POST("/delete-volume", middleware.AdminMiddleware(), handler.DeleteStorageVolume)
			}

			// ==================== 节点管理（管理员） ====================
			nodes := authorized.Group("/nodes")
			nodes.Use(middleware.AdminMiddleware())
			{
				nodes.GET("", handler.ListHostNodes)
				nodes.POST("", handler.CreateHostNode)
				nodes.GET("/:id/migration-options", handler.GetNodeMigrationOptions)
				nodes.PUT("/:id", handler.UpdateHostNode)
				nodes.DELETE("/:id", handler.DeleteHostNode)
				nodes.POST("/:id/probe", handler.ProbeHostNode)
			}

			migration := authorized.Group("/migration")
			migration.Use(middleware.AdminMiddleware())
			{
				migration.POST("/adopt-vm", handler.AdoptMigratedVM)
			}

			// ==================== 用户管理（管理员） ====================
			user := authorized.Group("/user")
			user.Use(middleware.AdminMiddleware())
			{
				user.GET("/list", handler.GetUserList)
				user.POST("", handler.CreateUser)                         // 创建邀请用户或可直接登录用户
				user.PUT("/:username/account", handler.UpdateUserAccount) // 更新用户邮箱和密码
				user.PUT("/:username/vms", handler.AssignVMs)
				user.POST("/:username/lightweight-registrations", handler.CreateLightweightVMRegistrations)
				user.PUT("/:username/lightweight-vm-quota", handler.UpdateLightweightVMQuota)
				user.DELETE("/:username/lightweight-vm/:vmName", handler.RemoveLightweightVMRegistrationByVMName)
				user.POST("/:username/lightweight-vm/:vmName/delete", handler.DeleteLightweightVM) // 删除轻量云 VM
				user.DELETE("/:username/lightweight-registrations/:id", handler.DeleteLightweightVMRegistration)
				user.PUT("/:username/quota", handler.UpdateUserQuota)
				user.PUT("/:username/status", handler.UpdateUserStatus)
				user.GET("/:username/quota", handler.GetUserQuotaUsage)
				user.PUT("/:username/ssh", handler.ToggleUserSSH)
				user.POST("/:username/resend-invite", handler.ResendInvite)
				user.POST("/:username/traffic/reset", handler.ResetUserTraffic)
				user.DELETE("/:username", handler.DeleteUser)
			}

			// ==================== 用户自助（所有登录用户可用） ====================
			self := authorized.Group("/self")
			{
				self.GET("/quota", handler.GetSelfQuota)                                                          // 查看自己的配额
				self.GET("/vms", handler.GetSelfVMs)                                                              // 查看自己的VM列表
				self.GET("/vms/sse", handler.GetSelfVMsSSE)                                                       // SSE实时推送VM列表
				self.GET("/lightweight-registrations", handler.GetSelfLightweightVMRegistrations)                 // 轻量云待确认服务器
				self.POST("/lightweight-registrations/:id/confirm", handler.ConfirmSelfLightweightVMRegistration) // 确认开通轻量云服务器
				self.POST("/vm/clone", middleware.ElasticCloudOnlyMiddleware(), handler.SelfCloneVm)              // 从模板克隆VM
				self.POST("/vm/create", middleware.ElasticCloudOnlyMiddleware(), handler.SelfCreateVm)            // 普通创建VM
				self.DELETE("/vm/:name", middleware.ElasticCloudOnlyMiddleware(), handler.SelfDeleteVm)           // 删除自己的VM
				self.GET("/vm/:name/qcow2-disks", handler.GetVmQcow2Disks)                                        // 获取qcow2磁盘列表

				// 虚拟机导出/导入
				self.GET("/vm/:name/export-options", handler.GetVMExportOptionsHandler)                                          // 获取虚拟机可导出磁盘
				self.POST("/vm/export", middleware.ElasticCloudOnlyMiddleware(), handler.ExportVMHandler)                        // 导出虚拟机
				self.POST("/vm/import", middleware.ElasticCloudOnlyMiddleware(), handler.ImportVMHandler)                        // 导入虚拟机
				self.POST("/vm/import-appliance/inspect", middleware.ElasticCloudOnlyMiddleware(), handler.InspectSelfAppliance) // 检查我的存储 OVF/OVA
				self.POST("/vm/import-appliance", middleware.ElasticCloudOnlyMiddleware(), handler.ImportSelfAppliance)          // 从 OVF/OVA 导入虚拟机

				// 用户存储池
				self.GET("/storage/info", middleware.ElasticCloudOnlyMiddleware(), handler.GetUserStorageInfo)                              // 存储池信息
				self.POST("/storage/init", middleware.ElasticCloudOnlyMiddleware(), handler.InitUserStorageHandler)                         // 初始化存储池
				self.GET("/storage/files/:category", middleware.ElasticCloudOnlyMiddleware(), handler.ListUserStorageFiles)                 // 列出文件
				self.POST("/storage/upload/init", middleware.ElasticCloudOnlyMiddleware(), handler.UserStorageUploadInit)                   // 分片上传-初始化/秒传
				self.POST("/storage/upload/chunk", middleware.ElasticCloudOnlyMiddleware(), handler.UserStorageUploadChunk)                 // 分片上传-单片
				self.POST("/storage/upload/complete", middleware.ElasticCloudOnlyMiddleware(), handler.UserStorageUploadComplete)           // 分片上传-完成校验
				self.GET("/storage/upload/status", middleware.ElasticCloudOnlyMiddleware(), handler.UserStorageUploadStatus)                // 分片上传-进度查询(续传)
				self.DELETE("/storage/upload", middleware.ElasticCloudOnlyMiddleware(), handler.UserStorageUploadCancel)                    // 分片上传-取消
				self.GET("/storage/upload/pending", middleware.ElasticCloudOnlyMiddleware(), handler.UserStorageUploadPending)              // 分片上传-未完成会话(主动恢复)
				self.DELETE("/storage/file/:category/:filename", middleware.ElasticCloudOnlyMiddleware(), handler.DeleteUserStorageFile)    // 删除文件
				self.GET("/storage/download/:category/:filename", middleware.ElasticCloudOnlyMiddleware(), handler.DownloadUserStorageFile) // 下载文件
				self.GET("/storage/isos", middleware.ElasticCloudOnlyMiddleware(), handler.GetUserISOsForVM)                                // 用户ISO列表（VM创建用）
				self.GET("/storage/mounts", middleware.ElasticCloudOnlyMiddleware(), handler.ListUserMounts)                                // 用户所有VM的挂载列表
				self.POST("/storage/mount", middleware.ElasticCloudOnlyMiddleware(), handler.MountStorageToVM)                              // 挂载存储池到VM
				self.DELETE("/storage/mount/:vmName/:tag", middleware.ElasticCloudOnlyMiddleware(), handler.UnmountStorageFromVM)           // 卸载存储池

			}

			// ==================== 宿主机监控 ====================
			host := authorized.Group("/host")
			{
				host.GET("/stats", handler.GetHostStats)
				host.GET("/stats/sse", handler.GetHostStatsSSE)
				host.GET("/stats/history", handler.GetHostStatsHistory)
				host.GET("/cpus", handler.GetHostCPUCores)
				host.GET("/cpu/hardware", middleware.AdminMiddleware(), handler.GetHostCPUHardware)     // 获取宿主机 CPU 硬件信息与每核使用率
				host.GET("/memory/modules", middleware.AdminMiddleware(), handler.GetHostMemoryModules) // 获取宿主机内存条信息
				host.GET("/disks", handler.GetHostDisks)
				host.GET("/kvm-intel-unrestricted-guest", middleware.AdminMiddleware(), handler.GetHostKVMIntelUnrestrictedGuestStatus)
				host.PUT("/kvm-intel-unrestricted-guest", middleware.AdminMiddleware(), handler.UpdateHostKVMIntelUnrestrictedGuest)
				host.GET("/ksm", middleware.AdminMiddleware(), handler.GetHostKSMStatus)
				host.PUT("/ksm", middleware.AdminMiddleware(), handler.UpdateHostKSMProfile)
				host.GET("/zram", middleware.AdminMiddleware(), handler.GetHostZRAMStatus)
				host.PUT("/zram", middleware.AdminMiddleware(), handler.UpdateHostZRAMProfile)
				// 硬件直通
				host.GET("/hardware-passthrough/status", middleware.AdminMiddleware(), handler.GetHardwarePassthroughStatus)
				host.POST("/hardware-passthrough/enable-iommu", middleware.AdminMiddleware(), handler.EnableIommu)
				host.POST("/hardware-passthrough/load-vfio", middleware.AdminMiddleware(), handler.LoadVfioPci)
				// 硬件直通设备管理
				host.GET("/passthrough", handler.GetPassthroughDevices)
				host.POST("/passthrough/bind", middleware.AdminMiddleware(), handler.BindPCIDevice)
				host.POST("/passthrough/unbind", middleware.AdminMiddleware(), handler.UnbindPCIDevice)
			}

			// ==================== 任务队列 ====================
			task := authorized.Group("/task")
			{
				task.GET("/list", handler.GetTaskList)
				task.GET("/sse", handler.SSETaskProgress)
				task.GET("/:id", handler.GetTaskDetail)
				task.POST("/:id/cancel", handler.CancelTask)
				task.DELETE("/clear", handler.ClearFinishedTasks)
			}

			// ==================== 调度事件中心（管理员） ====================
			scheduler := authorized.Group("/scheduler")
			scheduler.Use(middleware.AdminMiddleware())
			{
				scheduler.GET("/list", handler.GetSchedulerList)
				scheduler.GET("/events", handler.GetSchedulerEventList)
				scheduler.GET("/events/sse", handler.SSESchedulerEvents)
			}

			// ==================== CPU 亲和性预设（所有登录用户可读） ====================
			authorized.GET("/cpu-affinity-presets", handler.GetCPUAffinityPresets)

			// ==================== 系统运行环境信息（需登录） ====================
			authorized.GET("/system-info", handler.GetPublicSystemInfo)

		}
	}

	// ==================== 前端静态文件服务（生产环境） ====================
	setupStaticFileServing(r)

	return r
}

// setupStaticFileServing 配置前端静态文件服务
// 当 web-dist 目录存在时，自动提供前端文件，支持 Vue SPA 路由回退
func setupStaticFileServing(r *gin.Engine) {
	// 获取可执行文件所在目录
	execPath, err := os.Executable()
	if err != nil {
		return
	}
	execDir := filepath.Dir(execPath)
	webDistDir := filepath.Join(execDir, "web-dist")

	// 检查 web-dist 目录是否存在
	if _, err := os.Stat(webDistDir); os.IsNotExist(err) {
		// 也尝试相对于工作目录查找
		webDistDir = "web-dist"
		if _, err := os.Stat(webDistDir); os.IsNotExist(err) {
			logger.App.Info("未找到 web-dist 目录，跳过前端静态文件服务（开发环境请使用 vite dev）")
			return
		}
	}

	absWebDistDir, _ := filepath.Abs(webDistDir)
	logger.App.Info("启用前端静态文件服务", "dir", absWebDistDir)

	// 提供静态资源文件（CSS/JS/图片等）—— 带 hash 的资源可长缓存
	r.Use(func(c *gin.Context) {
		if strings.HasPrefix(c.Request.URL.Path, "/assets/") {
			c.Header("Cache-Control", "public, max-age=31536000, immutable")
		}
		c.Next()
	})
	r.Static("/assets", filepath.Join(absWebDistDir, "assets"))

	// 提供根目录下的静态文件（favicon 等）
	r.StaticFile("/favicon.svg", filepath.Join(absWebDistDir, "favicon.svg"))
	r.StaticFile("/icons.svg", filepath.Join(absWebDistDir, "icons.svg"))

	// SPA 回退：所有非 API 路由都返回 index.html
	r.NoRoute(func(c *gin.Context) {
		path := c.Request.URL.Path

		// API 路由不回退
		if strings.HasPrefix(path, "/api") {
			c.JSON(http.StatusNotFound, gin.H{"code": 404, "message": "Not Found"})
			return
		}

		// 安全路径校验：null byte 检查（必须在 Clean 之前）
		if strings.ContainsRune(path, 0) {
			c.JSON(http.StatusForbidden, gin.H{"error": "非法路径"})
			return
		}
		cleanPath := filepath.Clean(path)
		safePath := filepath.Join(absWebDistDir, cleanPath)
		if !strings.HasPrefix(safePath, absWebDistDir+string(filepath.Separator)) && safePath != absWebDistDir {
			c.JSON(http.StatusForbidden, gin.H{"error": "非法路径"})
			return
		}

		// 尝试提供静态文件
		if _, err := os.Stat(safePath); err == nil {
			c.File(safePath)
			return
		}

		// SPA 回退到 index.html
		c.Header("Cache-Control", "no-cache, no-store, must-revalidate")
		c.Header("Pragma", "no-cache")
		c.Header("Expires", "0")
		c.File(filepath.Join(absWebDistDir, "index.html"))
	})
}
