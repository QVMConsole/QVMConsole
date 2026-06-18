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
			settings.POST("/log/delete", handler.DeleteLogs)
			settings.POST("/log/export", handler.ExportLogs)
			settings.POST("/storage/trim", handler.TrimUserStorage)
		}

		// ==================== 需要认证的路由 ====================
		authorized := api.Group("")
		authorized.Use(middleware.AuthMiddleware())
		{
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
				// 多网口管理（仅管理员）
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

				// QEMU Monitor（普通用户仅开放安全子集）
				vm.GET("/:name/monitor/status", handler.GetVMMonitorStatus)
				vm.POST("/:name/monitor/command", handler.ExecuteVMMonitorCommand)

				// 磁盘管理
				vm.GET("/:name/disks", handler.GetDiskList)
				vm.GET("/:name/disk-migration/options", middleware.AdminMiddleware(), handler.GetDiskMigrationOptions)
				vm.POST("/:name/disk", middleware.ElasticCloudOnlyMiddleware(), handler.AddDisk)
				vm.POST("/:name/disk/:dev/resize", middleware.ElasticCloudOnlyMiddleware(), handler.ResizeDisk)
				vm.PUT("/:name/disk/:dev/bus", middleware.ElasticCloudOnlyMiddleware(), handler.ChangeDiskBus)
				vm.POST("/:name/disk/attach", middleware.ElasticCloudOnlyMiddleware(), handler.AttachDisk)
				vm.POST("/:name/disk/import", middleware.ElasticCloudOnlyMiddleware(), middleware.AdminMiddleware(), handler.ImportDiskForVM)
				vm.POST("/:name/disk/:dev/migrate", middleware.AdminMiddleware(), handler.MigrateDisk)
				vm.DELETE("/:name/disk/:dev", middleware.ElasticCloudOnlyMiddleware(), handler.DeleteDisk)
				vm.GET("/:name/disk/:dev/iops", handler.GetDiskIOPS)
				vm.PUT("/:name/disk/:dev/iops", middleware.AdminMiddleware(), handler.SetDiskIOPS)

				// CD/DVD 管理
				vm.POST("/:name/cdrom", middleware.ElasticCloudOnlyMiddleware(), handler.ChangeCDROM)
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
				network.DELETE("/port-forward/by-key/:rule_key", handler.DeletePortForwardByRuleKey)
				network.POST("/port-forward/batch-delete", handler.BatchDeletePortForward)
				network.POST("/port-forward/save", handler.SavePortForwardRules)
				network.POST("/port-forward/probe/run", handler.RunPortForwardHTTPProbe)
				network.GET("/port-forward/whitelist/summary", handler.GetPortForwardWhitelistSummary)
				network.GET("/port-forward/whitelist", middleware.AdminMiddleware(), handler.GetPortForwardWhitelistList)
				network.POST("/port-forward/whitelist/user", middleware.AdminMiddleware(), handler.AddPortForwardUserWhitelist)
				network.DELETE("/port-forward/whitelist/user/:username", middleware.AdminMiddleware(), handler.DeletePortForwardUserWhitelist)
				network.POST("/port-forward/whitelist/vm", middleware.AdminMiddleware(), handler.AddPortForwardVMWhitelist)
				network.DELETE("/port-forward/whitelist/vm/:vm_name", middleware.AdminMiddleware(), handler.DeletePortForwardVMWhitelist)

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
				vpc.POST("/switches/:id/traffic/reset", middleware.ElasticCloudOnlyMiddleware(), handler.ResetVPCSwitchTraffic)
				vpc.DELETE("/switches/:id", middleware.ElasticCloudOnlyMiddleware(), handler.DeleteVPCSwitch)
				vpc.GET("/switches/:id/vms", handler.GetVPCSwitchVMs)
				vpc.GET("/security-groups", handler.ListVPCSecurityGroups)
				vpc.POST("/security-groups", middleware.ElasticCloudOnlyMiddleware(), handler.CreateVPCSecurityGroup)
				vpc.PUT("/security-groups/:id", middleware.ElasticCloudOnlyMiddleware(), handler.UpdateVPCSecurityGroup)
				vpc.DELETE("/security-groups/:id", middleware.ElasticCloudOnlyMiddleware(), handler.DeleteVPCSecurityGroup)
				vpc.POST("/security-groups/:id/rules", handler.AddVPCSecurityGroupRule)
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
				user.POST("", handler.CreateUser)
				user.PUT("/:username/vms", handler.AssignVMs)
				user.POST("/:username/lightweight-registrations", handler.CreateLightweightVMRegistrations)
				user.PUT("/:username/lightweight-vm-quota", handler.UpdateLightweightVMQuota)
				user.DELETE("/:username/lightweight-vm/:vmName", handler.RemoveLightweightVMRegistrationByVMName)
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
				self.POST("/vm/export", middleware.ElasticCloudOnlyMiddleware(), handler.ExportVMHandler) // 导出虚拟机
				self.POST("/vm/import", middleware.ElasticCloudOnlyMiddleware(), handler.ImportVMHandler) // 导入虚拟机

				// 用户存储池
				self.GET("/storage/info", middleware.ElasticCloudOnlyMiddleware(), handler.GetUserStorageInfo)                              // 存储池信息
				self.POST("/storage/init", middleware.ElasticCloudOnlyMiddleware(), handler.InitUserStorageHandler)                         // 初始化存储池
				self.GET("/storage/files/:category", middleware.ElasticCloudOnlyMiddleware(), handler.ListUserStorageFiles)                 // 列出文件
				self.POST("/storage/upload/:category", middleware.ElasticCloudOnlyMiddleware(), handler.UploadUserStorageFile)              // 上传文件
				self.GET("/storage/upload-check", middleware.ElasticCloudOnlyMiddleware(), handler.CheckLargeUpload)                        // 大文件上传检测
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
				host.GET("/disks", handler.GetHostDisks)
				host.GET("/kvm-intel-unrestricted-guest", middleware.AdminMiddleware(), handler.GetHostKVMIntelUnrestrictedGuestStatus)
				host.PUT("/kvm-intel-unrestricted-guest", middleware.AdminMiddleware(), handler.UpdateHostKVMIntelUnrestrictedGuest)
				host.GET("/ksm", middleware.AdminMiddleware(), handler.GetHostKSMStatus)
				host.PUT("/ksm", middleware.AdminMiddleware(), handler.UpdateHostKSMProfile)
				host.GET("/zram", middleware.AdminMiddleware(), handler.GetHostZRAMStatus)
				host.PUT("/zram", middleware.AdminMiddleware(), handler.UpdateHostZRAMProfile)
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
