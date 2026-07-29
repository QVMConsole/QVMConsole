package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"

	"kvm_console/config"
	"kvm_console/logger"
	"kvm_console/model"
	"kvm_console/router"
	"kvm_console/service"
	clonepkg "kvm_console/service/clone"
	"kvm_console/service/guest_agent"
	guestautomation "kvm_console/service/guest_automation"
	"kvm_console/service/libvirt_rpc"
	netpkg "kvm_console/service/network"
	"kvm_console/service/snapshot"
	vmmemory "kvm_console/service/vm/memory"
	vmmigration "kvm_console/service/vm/migration"
	vmimport "kvm_console/service/vm/vmimport"
	"kvm_console/taskqueue"
	"kvm_console/utils"
)

// Version 版本号，通过 ldflags 在构建时注入
// 构建命令: go build -ldflags="-s -w -X main.Version=v1.0.0"
var Version = "dev"

func main() {
	if len(os.Args) > 1 && os.Args[1] == "host-zram-apply" {
		if err := service.ApplyHostZRAMPersistentProfile(); err != nil {
			log.Fatalf("恢复 zRAM 失败: %v", err)
		}
		return
	}

	// 避免 /tmp 为 tmpfs 时大文件上传因空间不足失败（tmpfs 通常仅几 GB）
	// 优先使用环境变量 KVM_TMPDIR 指定的目录，否则回退到服务器工作目录下的 tmp 目录
	ensureLargeTempDir()

	// 初始化配置
	config.Init()

	// 初始化日志系统
	logger.InitWithConsoleConfig(
		config.GlobalConfig.LogDir,
		config.GlobalConfig.LogLevel,
		config.GlobalConfig.LogMaxDays,
		config.GlobalConfig.LogCompress,
		config.GlobalConfig.LogConsole,
		config.GlobalConfig.LogConsoleTypes,
		config.GlobalConfig.LogConsoleLevel,
		config.GlobalConfig.LogMaxSizeMB,
		config.GlobalConfig.LogMaxBackups,
	)
	defer logger.Close()

	logger.App.Info("配置初始化完成")

	// 初始化数据库
	model.InitDB()

	// 从数据库加载持久化的系统设置（覆盖环境变量默认值）
	if savedSettings, err := model.GetAllSettings(); err == nil && len(savedSettings) > 0 {
		config.GlobalConfig.LoadFromDB(savedSettings)
		logger.App.Info("已从数据库加载持久化系统设置", "count", len(savedSettings))
	}

	// 初始化 go-libvirt RPC 连接（连接失败阻止启动）
	if err := libvirt_rpc.InitLibvirtRPC(); err != nil {
		log.Fatal("go-libvirt 连接失败，程序无法启动: ", err)
	}
	defer libvirt_rpc.CloseLibvirt()

	if err := service.BootstrapVMCacheFromHost(); err != nil {
		logger.App.Warn("启动时同步虚拟机缓存失败，已保留数据库旧缓存", "error", err)
	} else {
		logger.App.Info("启动时虚拟机缓存同步完成")
	}

	// 安全检查（数据库设置加载完成后）
	config.ValidateSecurity()

	// 初始化 clone 子包依赖
	initCloneDeps()

	// 注册任务处理器
	registerTaskHandlers()

	// 启动任务队列（3 个 Worker）
	taskqueue.Start(3)

	// 启动资源采集器（后台定时采集VM资源数据）
	service.StartStatsCollector()
	vmmemory.StartMemoryBalloonScheduler()
	service.StartSchedulerEventCleanup()
	service.StartVMScheduleRunner()
	service.StartJWTSecretRotator()
	service.StartExpiredUploadSessionCleanup() // 清理过期分片上传会话
	service.StartPasswordBreachScheduler()

	// 同步 SSH 拒绝配置（确保与数据库状态一致）
	service.SyncSSHDenyConfig()
	service.EnsureAllActiveUsersDefaultSecurityGroup()
	if err := service.EnsureSystemBaseNetwork(); err != nil {
		logger.App.Warn("创建系统基础网络交换机失败", "error", err)
	}
	if err := service.EnsureAllNetworkBridgesRuntime(); err != nil {
		logger.App.Warn("恢复桥接网桥失败", "error", err)
	}
	if err := netpkg.RestorePortForwardRules(); err != nil {
		logger.App.Warn("恢复端口转发规则失败", "error", err)
	}
	if err := service.EnsureAllVPCSwitchRuntime(); err != nil {
		logger.App.Warn("恢复 VPC 网络运行态失败", "error", err)
	}
	if err := service.RestorePublicIPRules(); err != nil {
		logger.App.Warn("恢复公网 IP 规则失败", "error", err)
	}

	// 设置路由
	r := router.Setup()

	// 启动服务
	addr := fmt.Sprintf(":%d", config.GlobalConfig.Port)
	logger.App.Info("QVMConsole 服务启动", "addr", addr)
	if err := r.Run(addr); err != nil {
		logger.App.Error("服务启动失败", "error", err)
		os.Exit(1)
	}
}

// registerTaskHandlers 注册异步任务处理器
func registerTaskHandlers() {
	// 克隆任务（支持取消）
	taskqueue.RegisterHandler(model.TaskTypeClone, func(ctx context.Context, task *model.Task, progress func(int, string)) (string, error) {
		params, err := service.ParseCloneParams(task.Params)
		if err != nil {
			return "", fmt.Errorf("解析参数失败: %w", err)
		}
		result, err := service.CloneVM(ctx, params, progress)
		if err != nil {
			return "", err
		}
		if err := bindTaskVMToVPC(task.CreatedBy, params.Name, params.SwitchID, params.SecurityGroupID); err != nil {
			return "", fmt.Errorf("克隆完成，但绑定 VPC 网络失败: %w", err)
		}
		attachTaskExtraNICs(params.Name, task.Params)
		// 应用 IOPS 限制
		applyCloneIOPS(params)
		if err := configureClonedGuestDisks(ctx, params.Name, params.TemplateType, params.ExtraDisks, progress); err != nil {
			partial := &guestautomation.OperationResult{
				HostStage:  guestautomation.StageResult{Status: "success", Message: "虚拟机与额外磁盘已创建"},
				GuestStage: guestautomation.StageResult{Status: "failed", Message: err.Error()}, Retryable: true,
			}
			return guestautomation.BuildResultJSON(partial), fmt.Errorf("克隆已完成，但来宾磁盘自动挂载失败: %w", err)
		}
		if saveErr := service.SaveVMCredential(params.Name, params.User, params.Password, "clone", task.CreatedBy, false); saveErr != nil {
			logger.App.Error("保存虚拟机克隆凭据失败", "vm", params.Name, "error", saveErr)
		}
		// 克隆完成后重新分配用户带宽
		if task.CreatedBy != "" && task.CreatedBy != "admin" {
			go func() {
				defer utils.RecoverAndLog("main-clone-rebalance")
				if err := service.RebalanceUserBandwidth(task.CreatedBy); err != nil {
					logger.App.Warn("克隆完成后重新分配用户带宽失败", "user", task.CreatedBy, "error", err)
				}
			}()
		}
		refreshVMCacheAfterTask(params.Name)
		resultJSON, _ := json.Marshal(result)
		return string(resultJSON), nil
	})

	// 原生链式克隆任务（支持取消）
	taskqueue.RegisterHandler(model.TaskTypeLinkedClone, func(ctx context.Context, task *model.Task, progress func(int, string)) (string, error) {
		params, err := service.ParseLinkedCloneParams(task.Params)
		if err != nil {
			return "", fmt.Errorf("解析参数失败: %w", err)
		}
		result, err := service.LinkedCloneVM(ctx, params, progress)
		if err != nil {
			return "", err
		}
		if err := bindTaskVMToVPC(task.CreatedBy, params.Name, params.SwitchID, params.SecurityGroupID); err != nil {
			return "", fmt.Errorf("原生链式克隆完成，但绑定 VPC 网络失败: %w", err)
		}
		attachTaskExtraNICs(params.Name, task.Params)
		// 应用 IOPS 限制
		applyLinkedCloneIOPS(params)
		if err := configureClonedGuestDisks(ctx, params.Name, params.TemplateType, params.ExtraDisks, progress); err != nil {
			partial := &guestautomation.OperationResult{
				HostStage:  guestautomation.StageResult{Status: "success", Message: "虚拟机与额外磁盘已创建"},
				GuestStage: guestautomation.StageResult{Status: "failed", Message: err.Error()}, Retryable: true,
			}
			return guestautomation.BuildResultJSON(partial), fmt.Errorf("链式克隆已完成，但来宾磁盘自动挂载失败: %w", err)
		}
		refreshVMCacheAfterTask(params.Name)
		resultJSON, _ := json.Marshal(result)
		return string(resultJSON), nil
	})

	// 转为独立虚拟机任务（脱离链式克隆 backing chain）
	taskqueue.RegisterHandler(model.TaskTypeMakeVMIndependent, func(ctx context.Context, task *model.Task, progress func(int, string)) (string, error) {
		params, err := service.ParseMakeVMIndependentParams(task.Params)
		if err != nil {
			return "", fmt.Errorf("解析参数失败: %w", err)
		}
		if err := service.MakeVMIndependent(ctx, params, progress); err != nil {
			return "", err
		}
		refreshVMCacheAfterTask(params.VMName)
		return `{"status":"ok"}`, nil
	})

	// 批量克隆任务（支持取消）
	taskqueue.RegisterHandler(model.TaskTypeBatch, func(ctx context.Context, task *model.Task, progress func(int, string)) (string, error) {
		params, err := service.ParseBatchCloneParams(task.Params)
		if err != nil {
			return "", fmt.Errorf("解析参数失败: %w", err)
		}
		results, err := service.BatchCloneVM(ctx, params, progress)
		if err != nil {
			return "", err
		}
		for resultIndex := range results {
			result := &results[resultIndex]
			if result.Error != "" {
				continue
			}
			if err := bindTaskVMToVPC(task.CreatedBy, result.VMName, params.SwitchID, params.SecurityGroupID); err != nil {
				logger.App.Warn("批量克隆绑定 VPC 失败", "vm", result.VMName, "error", err)
			}
			attachTaskExtraNICs(result.VMName, task.Params)
			if guestErr := configureClonedGuestDisks(ctx, result.VMName, params.TemplateType, params.ExtraDisks, func(_ int, message string) {
				logger.App.Info("批量克隆来宾磁盘配置", "vm", result.VMName, "message", message)
			}); guestErr != nil {
				result.Error = "虚拟机已创建，来宾磁盘自动挂载失败: " + guestErr.Error()
			}
			// 每台 VM 可能使用独立随机密码，优先用 result.Password
			credPassword := result.Password
			if credPassword == "" {
				credPassword = params.Password
			}
			if saveErr := service.SaveVMCredential(result.VMName, params.User, credPassword, "batch_clone", task.CreatedBy, false); saveErr != nil {
				logger.App.Warn("批量克隆保存凭据失败", "vm", result.VMName, "error", saveErr)
			}
			refreshVMCacheAfterTask(result.VMName)
		}
		resultJSON, _ := json.Marshal(results)
		return string(resultJSON), nil
	})

	// 重装系统任务
	taskqueue.RegisterHandler(model.TaskTypeReinstall, func(ctx context.Context, task *model.Task, progress func(int, string)) (string, error) {
		params, err := service.ParseReinstallParams(task.Params)
		if err != nil {
			return "", fmt.Errorf("解析参数失败: %w", err)
		}
		if strings.TrimSpace(params.Operator) == "" {
			params.Operator = task.CreatedBy
		}
		if err := service.ReinstallVM(ctx, params, progress); err != nil {
			return "", err
		}
		refreshVMCacheAfterTask(params.Name)
		return "", nil
	})

	// 模板制作任务
	taskqueue.RegisterHandler(model.TaskTypePrepare, func(ctx context.Context, task *model.Task, progress func(int, string)) (string, error) {
		var params service.PrepareTemplateParams
		if err := json.Unmarshal([]byte(task.Params), &params); err != nil {
			return "", fmt.Errorf("解析参数失败: %w", err)
		}
		progress(10, "开始制作模板...")
		err := service.PrepareTemplate(&params, progress)
		if err != nil {
			return "", err
		}
		progress(100, "模板制作完成")
		return fmt.Sprintf(`{"template":"%s"}`, params.TemplateName), nil
	})

	// 已导入 Linux 模板预处理任务
	taskqueue.RegisterHandler(model.TaskTypeTemplateLinuxPrepare, func(ctx context.Context, task *model.Task, progress func(int, string)) (string, error) {
		var params struct {
			TemplateName string `json:"template_name"`
		}
		if err := json.Unmarshal([]byte(task.Params), &params); err != nil {
			return "", fmt.Errorf("解析参数失败: %w", err)
		}
		if err := service.PrepareImportedLinuxTemplate(params.TemplateName, progress); err != nil {
			return "", err
		}
		return fmt.Sprintf(`{"template":"%s","linux_init_status":"ready"}`, params.TemplateName), nil
	})

	// 模板导出任务
	taskqueue.RegisterHandler(model.TaskTypeTemplateExport, func(ctx context.Context, task *model.Task, progress func(int, string)) (string, error) {
		var params service.ExportTemplateParams
		if err := json.Unmarshal([]byte(task.Params), &params); err != nil {
			return "", fmt.Errorf("解析参数失败: %w", err)
		}

		result, err := service.ExportTemplate(ctx, &params, progress)
		if err != nil {
			return "", err
		}

		resultJSON, _ := json.Marshal(result)
		return string(resultJSON), nil
	})

	// 模板导入任务
	taskqueue.RegisterHandler(model.TaskTypeTemplateImport, func(ctx context.Context, task *model.Task, progress func(int, string)) (string, error) {
		var params service.ImportTemplateParams
		if err := json.Unmarshal([]byte(task.Params), &params); err != nil {
			return "", fmt.Errorf("解析参数失败: %w", err)
		}

		result, err := service.ImportTemplate(ctx, &params, progress)
		if err != nil {
			return "", err
		}

		resultJSON, _ := json.Marshal(result)
		return string(resultJSON), nil
	})

	// 删除模板任务
	taskqueue.RegisterHandler(model.TaskTypeDeleteTemplate, func(ctx context.Context, task *model.Task, progress func(int, string)) (string, error) {
		var params service.DeleteTemplateParams
		if err := json.Unmarshal([]byte(task.Params), &params); err != nil {
			return "", fmt.Errorf("解析参数失败: %w", err)
		}

		result, err := service.DeleteTemplateWithVMs(&params, progress)
		if err != nil {
			return "", err
		}

		resultJSON, _ := json.Marshal(result)
		return string(resultJSON), nil
	})

	// 普通创建虚拟机任务
	taskqueue.RegisterHandler(model.TaskTypeCreate, func(ctx context.Context, task *model.Task, progress func(int, string)) (string, error) {
		params, err := service.ParseCreateVMParams(task.Params)
		if err != nil {
			return "", fmt.Errorf("解析参数失败: %w", err)
		}
		diskPath, err := service.CreateVM(params, progress)
		if err != nil {
			return "", err
		}
		if err := bindTaskVMToVPC(task.CreatedBy, params.Name, params.SwitchID, params.SecurityGroupID); err != nil {
			return "", fmt.Errorf("虚拟机创建完成，但绑定 VPC 网络失败: %w", err)
		}
		attachTaskExtraNICs(params.Name, task.Params)
		refreshVMCacheAfterTask(params.Name)
		resultJSON, _ := json.Marshal(map[string]string{
			"vm_name":   params.Name,
			"disk_path": diskPath,
		})
		return string(resultJSON), nil
	})

	// 轻量云注册 VM 开通任务
	taskqueue.RegisterHandler(model.TaskTypeLightweightVMProvision, func(ctx context.Context, task *model.Task, progress func(int, string)) (string, error) {
		params, err := service.ParseLightweightVMProvisionParams(task.Params)
		if err != nil {
			return "", fmt.Errorf("解析参数失败: %w", err)
		}
		result, err := service.ProvisionLightweightVMRegistration(ctx, params, progress)
		if err != nil {
			return "", err
		}
		refreshVMCacheAfterTask(result.VMName)
		resultJSON, _ := json.Marshal(result)
		return string(resultJSON), nil
	})

	// 跨节点虚拟机迁移任务
	taskqueue.RegisterHandler(model.TaskTypeVMMigrate, func(ctx context.Context, task *model.Task, progress func(int, string)) (string, error) {
		params, err := vmmigration.ParseVMMigrationTaskParams(task.Params)
		if err != nil {
			return "", fmt.Errorf("解析参数失败: %w", err)
		}
		result, err := vmmigration.ExecuteVMMigration(ctx, params, progress)
		if err != nil {
			return "", err
		}
		resultJSON, _ := json.Marshal(result)
		return string(resultJSON), nil
	})

	// 本机虚拟机硬盘迁移任务
	taskqueue.RegisterHandler(model.TaskTypeVMDiskMigrate, func(ctx context.Context, task *model.Task, progress func(int, string)) (string, error) {
		params, err := service.ParseVMDiskMigrationTaskParams(task.Params)
		if err != nil {
			return "", fmt.Errorf("解析参数失败: %w", err)
		}
		result, err := service.ExecuteVMDiskMigration(ctx, params, progress)
		if err != nil {
			return "", err
		}
		resultJSON, _ := json.Marshal(result)
		return string(resultJSON), nil
	})

	// 宿主机硬盘格式化并挂载任务
	taskqueue.RegisterHandler(model.TaskTypeStorageFormat, func(ctx context.Context, task *model.Task, progress func(int, string)) (string, error) {
		var params struct {
			ID     string `json:"id"`
			FSType string `json:"fstype"`
		}
		if err := json.Unmarshal([]byte(task.Params), &params); err != nil {
			return "", fmt.Errorf("解析参数失败: %w", err)
		}
		if params.ID == "" {
			return "", fmt.Errorf("存储池设备 ID 不能为空")
		}
		if err := service.FormatAndMountStoragePool(ctx, params.ID, params.FSType, progress); err != nil {
			return "", err
		}
		return fmt.Sprintf(`{"storage_pool_id":"%s"}`, params.ID), nil
	})

	// 宿主机硬盘创建分区任务
	taskqueue.RegisterHandler(model.TaskTypeStorageCreatePartition, func(ctx context.Context, task *model.Task, progress func(int, string)) (string, error) {
		var params struct {
			ID     string `json:"id"`
			SizeGB int    `json:"size_gb"`
		}
		if err := json.Unmarshal([]byte(task.Params), &params); err != nil {
			return "", fmt.Errorf("解析参数失败: %w", err)
		}
		if params.ID == "" {
			return "", fmt.Errorf("存储池设备 ID 不能为空")
		}
		if err := service.CreatePartitionOnDisk(ctx, params.ID, params.SizeGB, progress); err != nil {
			return "", err
		}
		return fmt.Sprintf(`{"storage_pool_id":"%s"}`, params.ID), nil
	})

	// 宿主机硬盘删除所有分区任务
	taskqueue.RegisterHandler(model.TaskTypeStorageDeletePartitions, func(ctx context.Context, task *model.Task, progress func(int, string)) (string, error) {
		var params struct {
			ID string `json:"id"`
		}
		if err := json.Unmarshal([]byte(task.Params), &params); err != nil {
			return "", fmt.Errorf("解析参数失败: %w", err)
		}
		if params.ID == "" {
			return "", fmt.Errorf("存储池设备 ID 不能为空")
		}
		if err := service.DeleteAllPartitionsOnDisk(ctx, params.ID, progress); err != nil {
			return "", err
		}
		return fmt.Sprintf(`{"storage_pool_id":"%s"}`, params.ID), nil
	})

	// 创建 LVM 存储卷任务
	taskqueue.RegisterHandler(model.TaskTypeStorageCreateLVMVolume, func(ctx context.Context, task *model.Task, progress func(int, string)) (string, error) {
		var params struct {
			Params string `json:"params"`
		}
		if err := json.Unmarshal([]byte(task.Params), &params); err != nil {
			return "", fmt.Errorf("解析参数失败: %w", err)
		}
		var req service.LVMVolumeRequest
		if err := json.Unmarshal([]byte(params.Params), &req); err != nil {
			return "", fmt.Errorf("解析 LVM 存储卷参数失败: %w", err)
		}
		if err := service.CreateLVMVolume(ctx, req, progress); err != nil {
			return "", err
		}
		return fmt.Sprintf(`{"vg_name":"%s","lv_name":"%s"}`, req.VGName, req.LVName), nil
	})

	// 删除 LVM 存储卷任务
	taskqueue.RegisterHandler(model.TaskTypeStorageDeleteLVMVolume, func(ctx context.Context, task *model.Task, progress func(int, string)) (string, error) {
		var params struct {
			VGName string `json:"vg_name"`
		}
		if err := json.Unmarshal([]byte(task.Params), &params); err != nil {
			return "", fmt.Errorf("解析参数失败: %w", err)
		}
		if err := service.DeleteLVMVolume(ctx, params.VGName, progress); err != nil {
			return "", err
		}
		return fmt.Sprintf(`{"vg_name":"%s"}`, params.VGName), nil
	})

	// 删除虚拟机任务
	taskqueue.RegisterHandler(model.TaskTypeDelete, func(ctx context.Context, task *model.Task, progress func(int, string)) (string, error) {
		var params struct {
			Name          string   `json:"name"`
			DeleteDisks   []string `json:"delete_disks"`
			TransferDisks []string `json:"transfer_disks"`
			TransferUser  string   `json:"transfer_user"`
			Action        string   `json:"action"`
		}
		if err := json.Unmarshal([]byte(task.Params), &params); err != nil {
			return "", fmt.Errorf("解析参数失败: %w", err)
		}

		// 强制删除模式：绕过磁盘和快照检查，处理僵尸虚拟机
		if params.Action == "force_delete" {
			progress(10, "开始强制删除虚拟机...")
			if err := service.ForceDeleteVM(params.Name); err != nil {
				return "", err
			}
			if err := service.DeleteVMCredential(params.Name); err != nil {
				logger.App.Warn("主动删除VM凭据失败", "vm", params.Name, "error", err)
			}
			if err := model.DeleteVMLock(params.Name); err != nil {
				logger.App.Warn("主动删除VM锁失败", "vm", params.Name, "error", err)
			}
			markVMCacheMissingAfterTask(params.Name)
			if task.CreatedBy != "" && task.CreatedBy != "admin" {
				go func() {
					defer utils.RecoverAndLog("main-force-delete-rebalance")
					if err := service.RebalanceUserBandwidth(task.CreatedBy); err != nil {
						logger.App.Warn("强制删除VM后重新分配用户带宽失败", "user", task.CreatedBy, "error", err)
					}
				}()
			}
			progress(100, "虚拟机已强制删除")
			return fmt.Sprintf(`{"vm_name":"%s","action":"force_delete"}`, params.Name), nil
		}

		progress(10, "开始删除虚拟机...")

		var err error
		if len(params.DeleteDisks) > 0 || len(params.TransferDisks) > 0 {
			err = service.DeleteVMWithDisks(params.Name, params.DeleteDisks, params.TransferDisks, params.TransferUser)
		} else {
			err = service.DeleteVM(params.Name)
		}
		if err != nil {
			return "", err
		}
		if err := service.DeleteVMCredential(params.Name); err != nil {
			logger.App.Warn("主动删除VM凭据失败", "vm", params.Name, "error", err)
		}
		if err := model.DeleteVMLock(params.Name); err != nil {
			logger.App.Warn("主动删除VM锁失败", "vm", params.Name, "error", err)
		}
		markVMCacheMissingAfterTask(params.Name)
		// 删除完成后重新分配用户带宽
		if task.CreatedBy != "" && task.CreatedBy != "admin" {
			go func() {
				defer utils.RecoverAndLog("main-delete-rebalance")
				if err := service.RebalanceUserBandwidth(task.CreatedBy); err != nil {
					logger.App.Warn("删除VM后重新分配用户带宽失败", "user", task.CreatedBy, "error", err)
				}
			}()
		}
		progress(100, "虚拟机已删除")
		return fmt.Sprintf(`{"vm_name":"%s"}`, params.Name), nil
	})

	// 虚拟机定时任务动作
	taskqueue.RegisterHandler(model.TaskTypeVMScheduleAction, func(ctx context.Context, task *model.Task, progress func(int, string)) (string, error) {
		return service.RunVMScheduledAction(ctx, task, progress)
	})

	// 快照操作任务（创建/恢复/删除）
	taskqueue.RegisterHandler(model.TaskTypeSnapshot, func(ctx context.Context, task *model.Task, progress func(int, string)) (string, error) {
		var params struct {
			VmName                 string `json:"vm_name"`
			SnapName               string `json:"snap_name"`
			Description            string `json:"description"`
			IncludeMemory          bool   `json:"include_memory"`
			AutoFixNVRAM           bool   `json:"auto_fix_nvram"`
			PauseForMemorySnapshot *bool  `json:"pause_for_memory_snapshot"`
			Action                 string `json:"action"`
		}
		if err := json.Unmarshal([]byte(task.Params), &params); err != nil {
			return "", fmt.Errorf("解析参数失败: %w", err)
		}

		switch params.Action {
		case "create":
			progress(10, fmt.Sprintf("正在为 %s 创建快照 %s ...", params.VmName, params.SnapName))
			pauseForMemorySnapshot := true
			if params.PauseForMemorySnapshot != nil {
				pauseForMemorySnapshot = *params.PauseForMemorySnapshot
			}
			err := snapshot.CreateSnapshotWithOptions(params.VmName, params.SnapName, params.Description, params.IncludeMemory, params.AutoFixNVRAM, pauseForMemorySnapshot, progress)
			if err != nil {
				return "", err
			}
			progress(100, fmt.Sprintf("快照 %s 创建成功", params.SnapName))
			return fmt.Sprintf(`{"vm_name":"%s","snap_name":"%s","action":"create"}`, params.VmName, params.SnapName), nil

		case "revert":
			progress(10, fmt.Sprintf("正在将 %s 恢复到快照 %s ...", params.VmName, params.SnapName))
			err := snapshot.RevertSnapshot(params.VmName, params.SnapName)
			if err != nil {
				return "", err
			}
			progress(100, fmt.Sprintf("已恢复到快照 %s", params.SnapName))
			return fmt.Sprintf(`{"vm_name":"%s","snap_name":"%s","action":"revert"}`, params.VmName, params.SnapName), nil

		case "delete":
			progress(10, fmt.Sprintf("正在删除 %s 的快照 %s ...", params.VmName, params.SnapName))
			err := snapshot.DeleteSnapshot(params.VmName, params.SnapName)
			if err != nil {
				return "", err
			}
			progress(100, fmt.Sprintf("快照 %s 已删除", params.SnapName))
			return fmt.Sprintf(`{"vm_name":"%s","snap_name":"%s","action":"delete"}`, params.VmName, params.SnapName), nil

		case "delete_all":
			progress(10, fmt.Sprintf("正在删除 %s 的全部快照 ...", params.VmName))
			deleted, err := snapshot.DeleteAllSnapshots(params.VmName, progress)
			if err != nil {
				return "", err
			}
			progress(100, fmt.Sprintf("已删除 %d 个快照", deleted))
			return fmt.Sprintf(`{"vm_name":"%s","deleted":%d,"action":"delete_all"}`, params.VmName, deleted), nil

		default:
			return "", fmt.Errorf("未知的快照操作: %s", params.Action)
		}
	})

	// 删除用户任务（级联删除所有资产）
	taskqueue.RegisterHandler(model.TaskTypeDeleteUser, func(ctx context.Context, task *model.Task, progress func(int, string)) (string, error) {
		var params struct {
			Username string `json:"username"`
		}
		if err := json.Unmarshal([]byte(task.Params), &params); err != nil {
			return "", fmt.Errorf("解析参数失败: %w", err)
		}
		progress(5, fmt.Sprintf("开始删除用户 %s 及其所有资产...", params.Username))
		err := service.DeleteSystemUser(params.Username, progress)
		if err != nil {
			return "", err
		}
		return fmt.Sprintf(`{"username":"%s"}`, params.Username), nil
	})

	// 封禁用户任务（关闭其运行中的虚拟机）
	taskqueue.RegisterHandler(model.TaskTypeDisableUser, func(ctx context.Context, task *model.Task, progress func(int, string)) (string, error) {
		var params struct {
			Username string `json:"username"`
		}
		if err := json.Unmarshal([]byte(task.Params), &params); err != nil {
			return "", fmt.Errorf("解析参数失败: %w", err)
		}

		result, err := service.DisableUserAccount(params.Username, progress)
		if err != nil {
			return "", err
		}

		resultJSON, _ := json.Marshal(result)
		return string(resultJSON), nil
	})
	taskqueue.RegisterHandler(model.TaskTypeRuntimeQuotaShutdown, func(ctx context.Context, task *model.Task, progress func(int, string)) (string, error) {
		var params struct {
			Username string `json:"username"`
		}
		if err := json.Unmarshal([]byte(task.Params), &params); err != nil {
			return "", fmt.Errorf("解析参数失败: %w", err)
		}

		result, err := service.EnforceUserRuntimeQuotaShutdown(params.Username, progress)
		if err != nil {
			return "", err
		}

		resultJSON, _ := json.Marshal(result)
		return string(resultJSON), nil
	})
	taskqueue.RegisterHandler(model.TaskTypeLightweightRuntimeQuotaShutdown, func(ctx context.Context, task *model.Task, progress func(int, string)) (string, error) {
		var params struct {
			VMName string `json:"vm_name"`
		}
		if err := json.Unmarshal([]byte(task.Params), &params); err != nil {
			return "", fmt.Errorf("解析参数失败: %w", err)
		}

		result, err := service.EnforceLightweightVMRuntimeQuotaShutdown(params.VMName, progress)
		if err != nil {
			return "", err
		}

		resultJSON, _ := json.Marshal(result)
		return string(resultJSON), nil
	})

	// 导出虚拟机任务
	taskqueue.RegisterHandler(model.TaskTypeExport, func(ctx context.Context, task *model.Task, progress func(int, string)) (string, error) {
		var params service.ExportVMParams
		if err := json.Unmarshal([]byte(task.Params), &params); err != nil {
			return "", fmt.Errorf("解析参数失败: %w", err)
		}
		result, err := service.ExportVM(ctx, &params, progress)
		if err != nil {
			return "", err
		}
		resultJSON, _ := json.Marshal(result)
		return string(resultJSON), nil
	})

	// 导入虚拟机任务
	taskqueue.RegisterHandler(model.TaskTypeImport, func(ctx context.Context, task *model.Task, progress func(int, string)) (string, error) {
		params, err := vmimport.ParseImportVMParams(task.Params)
		if err != nil {
			return "", fmt.Errorf("解析参数失败: %w", err)
		}
		result, err := vmimport.ImportVM(ctx, params, progress)
		if err != nil {
			return "", err
		}
		if err := bindTaskVMToVPC(params.Username, params.Name, params.SwitchID, params.SecurityGroupID); err != nil {
			return "", fmt.Errorf("导入完成，但绑定 VPC 网络失败: %w", err)
		}
		attachTaskExtraNICs(params.Name, task.Params)
		if saveErr := service.SaveVMCredential(params.Name, params.User, params.Password, "import", task.CreatedBy, false); saveErr != nil {
			logger.App.Warn("保存虚拟机导入凭据失败", "vm", params.Name, "error", saveErr)
		}
		refreshVMCacheAfterTask(params.Name)
		resultJSON, _ := json.Marshal(result)
		return string(resultJSON), nil
	})
	// 管理员通过绝对路径导入磁盘任务
	taskqueue.RegisterHandler(model.TaskTypeImportDisk, func(ctx context.Context, task *model.Task, progress func(int, string)) (string, error) {
		params, err := vmimport.ParseImportDiskByPathParams(task.Params)
		if err != nil {
			return "", fmt.Errorf("解析参数失败: %w", err)
		}
		result, err := vmimport.ImportDiskByPath(ctx, params, progress)
		if err != nil {
			return "", err
		}
		if err := bindTaskVMToVPC(params.Username, params.Name, params.SwitchID, params.SecurityGroupID); err != nil {
			return "", fmt.Errorf("导入完成，但绑定 VPC 网络失败: %w", err)
		}
		attachTaskExtraNICs(params.Name, task.Params)
		// 应用 IOPS 限制
		applyImportDiskIOPS(params)
		if saveErr := service.SaveVMCredential(params.Name, params.User, params.Password, "import_disk", task.CreatedBy, false); saveErr != nil {
			logger.App.Warn("保存虚拟机导入磁盘凭据失败", "vm", params.Name, "error", saveErr)
		}
		refreshVMCacheAfterTask(params.Name)
		resultJSON, _ := json.Marshal(result)
		return string(resultJSON), nil
	})
	// 管理员为已有虚拟机导入磁盘任务
	taskqueue.RegisterHandler(model.TaskTypeImportDiskAttach, func(ctx context.Context, task *model.Task, progress func(int, string)) (string, error) {
		params, err := vmimport.ParseImportDiskForExistingVMParams(task.Params)
		if err != nil {
			return "", fmt.Errorf("解析参数失败: %w", err)
		}
		dev, err := vmimport.ImportDiskForExistingVM(ctx, params, progress)
		if err != nil {
			return "", err
		}
		if params.GuestMount.Enabled {
			guestResult, guestErr := guestautomation.RunDiskOperation(ctx, &guestautomation.DiskOperationParams{
				Action: "guest_mount", VMName: params.VMName, Device: dev, GuestType: params.GuestType,
				ExistingDisk: true, GuestMount: params.GuestMount,
			}, progress)
			guestResult.HostStage = guestautomation.StageResult{Status: "success", Message: "磁盘已导入并连接到虚拟机"}
			return guestautomation.BuildResultJSON(guestResult), guestErr
		}
		resultJSON, _ := json.Marshal(map[string]string{"device": dev})
		return string(resultJSON), nil
	})
	// 磁盘转移任务（将磁盘文件转移到用户存储）
	taskqueue.RegisterHandler(model.TaskTypeDiskTransfer, func(ctx context.Context, task *model.Task, progress func(int, string)) (string, error) {
		var params struct {
			DiskPath string `json:"disk_path"`
			Username string `json:"username"`
			Device   string `json:"device"`
		}
		if err := json.Unmarshal([]byte(task.Params), &params); err != nil {
			return "", fmt.Errorf("解析参数失败: %w", err)
		}
		progress(10, fmt.Sprintf("正在转移磁盘 %s 到用户存储...", params.Device))
		if err := service.TransferDiskFile(params.DiskPath, params.Username); err != nil {
			return "", err
		}
		progress(100, fmt.Sprintf("磁盘 %s 已转移到「我的存储-虚拟磁盘」", params.Device))
		return fmt.Sprintf(`{"device":"%s","disk_path":"%s"}`, params.Device, params.DiskPath), nil
	})

	// 救援系统任务（启动/关闭救援模式）
	taskqueue.RegisterHandler(model.TaskTypeRescue, func(ctx context.Context, task *model.Task, progress func(int, string)) (string, error) {
		var params struct {
			VmName string `json:"vm_name"`
			Action string `json:"action"`
		}
		if err := json.Unmarshal([]byte(task.Params), &params); err != nil {
			return "", fmt.Errorf("解析参数失败: %w", err)
		}

		switch params.Action {
		case "start":
			rescueISO := config.GlobalConfig.RescueISO
			progress(5, fmt.Sprintf("正在为 %s 启动救援系统...", params.VmName))
			if err := service.StartRescue(params.VmName, rescueISO, progress); err != nil {
				return "", err
			}
			refreshVMCacheAfterTask(params.VmName)
			return fmt.Sprintf(`{"vm_name":"%s","action":"start"}`, params.VmName), nil
		case "stop":
			progress(5, fmt.Sprintf("正在为 %s 关闭救援系统...", params.VmName))
			if err := service.StopRescue(params.VmName, progress); err != nil {
				return "", err
			}
			refreshVMCacheAfterTask(params.VmName)
			return fmt.Sprintf(`{"vm_name":"%s","action":"stop"}`, params.VmName), nil
		default:
			return "", fmt.Errorf("未知的救援操作: %s", params.Action)
		}
	})
	taskqueue.RegisterHandler(model.TaskTypeResetVMPassword, func(ctx context.Context, task *model.Task, progress func(int, string)) (string, error) {
		params, err := service.ParseResetLinuxPasswordParams(task.Params)
		if err != nil {
			return "", fmt.Errorf("解析参数失败: %w", err)
		}
		if err := service.ResetLinuxPassword(ctx, params, progress); err != nil {
			return "", err
		}
		resultJSON, _ := json.Marshal(map[string]string{
			"vm_name":  params.VMName,
			"username": params.Username,
		})
		return string(resultJSON), nil
	})
	registerGuestDiskHandler := func(taskType string) {
		taskqueue.RegisterHandler(taskType, func(ctx context.Context, task *model.Task, progress func(int, string)) (string, error) {
			params, err := guestautomation.ParseDiskOperationParams(task.Params)
			if err != nil {
				return "", fmt.Errorf("解析磁盘任务参数失败: %w", err)
			}
			result, runErr := guestautomation.RunDiskOperation(ctx, params, progress)
			return guestautomation.BuildResultJSON(result), runErr
		})
	}
	registerGuestDiskHandler(model.TaskTypeVMDiskResize)
	registerGuestDiskHandler(model.TaskTypeVMDiskProvision)
	registerGuestDiskHandler(model.TaskTypeVMDiskGuestMount)
	taskqueue.RegisterHandler(model.TaskTypeApplyFirewall, func(ctx context.Context, task *model.Task, progress func(int, string)) (string, error) {
		var policy service.FirewallPolicy
		if err := json.Unmarshal([]byte(task.Params), &policy); err != nil {
			return "", fmt.Errorf("解析参数失败: %w", err)
		}
		if err := service.ApplyFirewallPolicy(&policy, progress); err != nil {
			return "", err
		}
		return `{"action":"apply"}`, nil
	})
	taskqueue.RegisterHandler(model.TaskTypeDisableFirewall, func(ctx context.Context, task *model.Task, progress func(int, string)) (string, error) {
		if err := service.DisableFirewall(progress); err != nil {
			return "", err
		}
		return `{"action":"disable"}`, nil
	})
	taskqueue.RegisterHandler(model.TaskTypeRollbackFirewall, func(ctx context.Context, task *model.Task, progress func(int, string)) (string, error) {
		if err := service.RollbackFirewall(progress); err != nil {
			return "", err
		}
		return `{"action":"rollback"}`, nil
	})
	taskqueue.RegisterHandler(model.TaskTypeUpdateFirewallGeoIP, func(ctx context.Context, task *model.Task, progress func(int, string)) (string, error) {
		var params service.FirewallGeoUpdateParams
		if err := json.Unmarshal([]byte(task.Params), &params); err != nil {
			return "", fmt.Errorf("解析参数失败: %w", err)
		}
		if err := service.UpdateFirewallGeoIP(ctx, params, progress); err != nil {
			return "", err
		}
		return `{"action":"update_geoip"}`, nil
	})
	taskqueue.RegisterHandler(model.TaskTypeEnableHostFirewall, func(ctx context.Context, task *model.Task, progress func(int, string)) (string, error) {
		var params service.HostFirewallEnableRequest
		if err := json.Unmarshal([]byte(task.Params), &params); err != nil {
			return "", fmt.Errorf("解析参数失败: %w", err)
		}
		if err := service.EnableHostFirewall(params, progress); err != nil {
			return "", err
		}
		return `{"action":"enable_host_firewall"}`, nil
	})
	taskqueue.RegisterHandler(model.TaskTypeDisableHostFirewall, func(ctx context.Context, task *model.Task, progress func(int, string)) (string, error) {
		if err := service.DisableHostFirewall(progress); err != nil {
			return "", err
		}
		return `{"action":"disable_host_firewall"}`, nil
	})
	taskqueue.RegisterHandler(model.TaskTypeOVSRepair, func(ctx context.Context, task *model.Task, progress func(int, string)) (string, error) {
		return service.RepairOVSNetwork(ctx, progress)
	})
	taskqueue.RegisterHandler(model.TaskTypeNetworkCapture, func(ctx context.Context, task *model.Task, progress func(int, string)) (string, error) {
		params, err := service.ParseNetworkCaptureParams(task.Params)
		if err != nil {
			return "", fmt.Errorf("解析参数失败: %w", err)
		}
		return service.ExecuteNetworkCapture(ctx, task.ID, params, progress)
	})
	taskqueue.RegisterHandler(model.TaskTypePublicIPApply, func(ctx context.Context, task *model.Task, progress func(int, string)) (string, error) {
		params, err := service.ParsePublicIPOperationParams(task.Params)
		if err != nil {
			return "", fmt.Errorf("解析参数失败: %w", err)
		}
		return service.ExecutePublicIPOperation(ctx, params, progress)
	})
	taskqueue.RegisterHandler(model.TaskTypeEnterMaintenanceMode, func(ctx context.Context, task *model.Task, progress func(int, string)) (string, error) {
		var params service.MaintenanceModeTaskParams
		if err := json.Unmarshal([]byte(task.Params), &params); err != nil {
			return "", fmt.Errorf("解析参数失败: %w", err)
		}
		result, err := service.EnterMaintenanceMode(ctx, &params, progress)
		if err != nil {
			return "", err
		}
		resultJSON, _ := json.Marshal(result)
		return string(resultJSON), nil
	})
	taskqueue.RegisterHandler(model.TaskTypeExitMaintenanceMode, func(ctx context.Context, task *model.Task, progress func(int, string)) (string, error) {
		var params service.MaintenanceModeTaskParams
		if err := json.Unmarshal([]byte(task.Params), &params); err != nil {
			return "", fmt.Errorf("解析参数失败: %w", err)
		}
		result, err := service.ExitMaintenanceMode(ctx, &params, progress)
		if err != nil {
			return "", err
		}
		resultJSON, _ := json.Marshal(result)
		return string(resultJSON), nil
	})
	taskqueue.RegisterHandler(model.TaskTypePasswordBreachScan, func(ctx context.Context, task *model.Task, progress func(int, string)) (string, error) {
		var params service.PasswordBreachScanParams
		if err := json.Unmarshal([]byte(task.Params), &params); err != nil {
			return "", fmt.Errorf("解析参数失败: %w", err)
		}
		result, err := service.ExecutePasswordBreachScan(ctx, params, progress)
		if err != nil {
			return "", err
		}
		return service.EncodePasswordBreachTaskResult(result), nil
	})
	taskqueue.RegisterHandler(model.TaskTypePasswordBreachNotify, func(ctx context.Context, task *model.Task, progress func(int, string)) (string, error) {
		var params service.PasswordBreachNotifyParams
		if err := json.Unmarshal([]byte(task.Params), &params); err != nil {
			return "", fmt.Errorf("解析参数失败: %w", err)
		}
		return service.ExecutePasswordBreachNotification(ctx, params, progress)
	})
	logger.App.Info("任务处理器注册完成")
}

// configureClonedGuestDisks 在模板克隆父任务中等待 QGA，并依次初始化启用了自动挂载的额外磁盘。
func configureClonedGuestDisks(ctx context.Context, vmName, guestType string, extraDisks []service.ExtraDiskParam, progress func(int, string)) error {
	configured := false
	for _, item := range extraDisks {
		if item.GuestMount.Enabled {
			configured = true
			break
		}
	}
	if !configured {
		return nil
	}
	guestType = strings.ToLower(strings.TrimSpace(guestType))
	if guestType == "" {
		if vmInfo, err := service.GetVM(vmName); err == nil {
			guestType = strings.ToLower(strings.TrimSpace(vmInfo.OSType))
		}
	}
	deadline := time.Now().Add(guest_agent.DiskTimeout)
	for {
		preflightCtx, cancel := context.WithTimeout(ctx, guestautomationTimeoutForMain())
		err := guestautomation.Preflight(preflightCtx, vmName, guestType, nil, false)
		cancel()
		if err == nil {
			break
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("等待 QEMU Guest Agent 就绪超时: %w", err)
		}
		select {
		case <-ctx.Done():
			return taskqueue.ErrTaskCanceled
		case <-time.After(2 * time.Second):
		}
	}
	if guestType == "" {
		osCtx, cancel := context.WithTimeout(ctx, guestautomationTimeoutForMain())
		osInfo, osErr := guest_agent.NewClient(vmName).OSInfo(osCtx)
		cancel()
		if osErr != nil {
			return fmt.Errorf("识别来宾系统类型失败: %w", osErr)
		}
		if strings.EqualFold(osInfo.ID, "mswindows") {
			guestType = "windows"
		} else {
			guestType = "linux"
		}
	}
	disks, err := service.ListDisks(vmName)
	if err != nil {
		return err
	}
	dataDisks := make([]service.DiskInfo, 0)
	for _, item := range disks {
		if item.DeviceType == "disk" && !item.IsSystem {
			dataDisks = append(dataDisks, item)
		}
	}
	if len(dataDisks) < len(extraDisks) {
		return fmt.Errorf("额外磁盘数量与来宾映射不一致")
	}
	for index, item := range extraDisks {
		if !item.GuestMount.Enabled {
			continue
		}
		config := guestautomation.GuestMountConfig(item.GuestMount)
		if err := guestautomation.ValidateGuestMount(&config, guestType); err != nil {
			return fmt.Errorf("额外磁盘 %d 配置无效: %w", index+1, err)
		}
		result, err := guestautomation.RunDiskOperation(ctx, &guestautomation.DiskOperationParams{
			Action: "guest_mount", VMName: vmName, Device: dataDisks[index].Device,
			GuestType: guestType, ExistingDisk: false, GuestMount: config,
		}, progress)
		if err != nil {
			return fmt.Errorf("额外磁盘 %d（%s）配置失败: %s", index+1, dataDisks[index].Device, result.GuestStage.Message)
		}
	}
	return nil
}

func guestautomationTimeoutForMain() time.Duration {
	return 15 * time.Second
}

func bindTaskVMToVPC(owner, vmName string, switchID, securityGroupID uint) error {
	if owner == "admin" && switchID > 0 {
		if err := service.BindVMToVPCAsAdmin(vmName, switchID, securityGroupID); err != nil {
			return err
		}
		logger.App.Info("管理员 VM 绑定 VPC", "vm", vmName, "switch", switchID, "sg", securityGroupID)
		return nil
	}
	if owner == "" || owner == "admin" {
		owner = service.FindVMOwner(vmName)
	}
	if owner == "" || owner == "admin" {
		logger.App.Info("VM 未找到普通用户归属，跳过自动绑定", "vm", vmName)
		return nil
	}
	if switchID == 0 || securityGroupID == 0 {
		// 未指定交换机/安全组，不再自动解析；若无网口则虚拟机将无网络
		logger.App.Info("VM 未指定交换机，跳过自动绑定", "vm", vmName)
		return nil
	}
	if err := service.BindVMToVPC(owner, vmName, switchID, securityGroupID); err != nil {
		return err
	}
	logger.App.Info("VM 自动绑定 VPC", "vm", vmName, "user", owner, "switch", switchID, "sg", securityGroupID)
	return nil
}

// attachTaskExtraNICs 从任务参数中提取额外网口配置并附加到虚拟机
func attachTaskExtraNICs(vmName string, paramsJSON string) {
	var raw struct {
		ExtraNics []service.AddVMInterfaceRequest `json:"extra_nics"`
	}
	if err := json.Unmarshal([]byte(paramsJSON), &raw); err != nil || len(raw.ExtraNics) == 0 {
		return
	}
	service.AttachExtraNICs(vmName, raw.ExtraNics)
}

func refreshVMCacheAfterTask(vmName string) {
	service.RefreshVMCacheByNameAsync(vmName)
}

func markVMCacheMissingAfterTask(vmName string) {
	service.MarkVMCacheMissingAsync(vmName)
}

func applyCloneIOPS(params *service.CloneParams) {
	if params.SystemDiskIOPS != nil && (params.SystemDiskIOPS.TotalIopsSec > 0 || params.SystemDiskIOPS.ReadIopsSec > 0 || params.SystemDiskIOPS.WriteIopsSec > 0) {
		if dev := getFirstDiskDevice(params.Name); dev != "" {
			if err := service.SetDiskIOPSTune(params.Name, dev, params.SystemDiskIOPS); err != nil {
				logger.App.Warn("克隆系统盘 IOPS 设置失败", "vm", params.Name, "error", err)
			}
		}
	}
	for i, ed := range params.ExtraDisks {
		if ed.IOPSTotal > 0 || ed.IOPSRead > 0 || ed.IOPSWrite > 0 {
			if dev := getNthDiskDevice(params.Name, i+2); dev != "" {
				if err := service.SetDiskIOPSTune(params.Name, dev, &service.DiskIOPSTune{
					TotalIopsSec: ed.IOPSTotal, ReadIopsSec: ed.IOPSRead, WriteIopsSec: ed.IOPSWrite,
				}); err != nil {
					logger.App.Warn("克隆额外磁盘 IOPS 设置失败", "vm", params.Name, "disk", i+1, "error", err)
				}
			}
		}
	}
}

func applyLinkedCloneIOPS(params *service.LinkedCloneParams) {
	if params.SystemDiskIOPS != nil && (params.SystemDiskIOPS.TotalIopsSec > 0 || params.SystemDiskIOPS.ReadIopsSec > 0 || params.SystemDiskIOPS.WriteIopsSec > 0) {
		if dev := getFirstDiskDevice(params.Name); dev != "" {
			if err := service.SetDiskIOPSTune(params.Name, dev, params.SystemDiskIOPS); err != nil {
				logger.App.Warn("链式克隆系统盘 IOPS 设置失败", "vm", params.Name, "error", err)
			}
		}
	}
	for i, ed := range params.ExtraDisks {
		if ed.IOPSTotal > 0 || ed.IOPSRead > 0 || ed.IOPSWrite > 0 {
			if dev := getNthDiskDevice(params.Name, i+2); dev != "" {
				if err := service.SetDiskIOPSTune(params.Name, dev, &service.DiskIOPSTune{
					TotalIopsSec: ed.IOPSTotal, ReadIopsSec: ed.IOPSRead, WriteIopsSec: ed.IOPSWrite,
				}); err != nil {
					logger.App.Warn("链式克隆额外磁盘 IOPS 设置失败", "vm", params.Name, "disk", i+1, "error", err)
				}
			}
		}
	}
}

func applyImportDiskIOPS(params *vmimport.ImportDiskByPathParams) {
	if params.SystemDiskIOPS != nil && (params.SystemDiskIOPS.TotalIopsSec > 0 || params.SystemDiskIOPS.ReadIopsSec > 0 || params.SystemDiskIOPS.WriteIopsSec > 0) {
		if dev := getFirstDiskDevice(params.Name); dev != "" {
			if err := service.SetDiskIOPSTune(params.Name, dev, params.SystemDiskIOPS); err != nil {
				logger.App.Warn("导入系统盘 IOPS 设置失败", "vm", params.Name, "error", err)
			}
		}
	}
	for i, ed := range params.ExtraImportDisks {
		if ed.IOPSTotal > 0 || ed.IOPSRead > 0 || ed.IOPSWrite > 0 {
			if dev := getNthDiskDevice(params.Name, i+2); dev != "" {
				if err := service.SetDiskIOPSTune(params.Name, dev, &service.DiskIOPSTune{
					TotalIopsSec: ed.IOPSTotal, ReadIopsSec: ed.IOPSRead, WriteIopsSec: ed.IOPSWrite,
				}); err != nil {
					logger.App.Warn("导入额外磁盘 IOPS 设置失败", "vm", params.Name, "disk", i+1, "error", err)
				}
			}
		}
	}
}

func getFirstDiskDevice(vmName string) string {
	return getNthDiskDevice(vmName, 1)
}

func getNthDiskDevice(vmName string, n int) string {
	result := utils.ExecCommand("virsh", "domblklist", vmName)
	if result.Error != nil {
		return ""
	}
	lines := strings.Split(result.Stdout, "\n")
	count := 0
	for _, line := range lines {
		fields := strings.Fields(line)
		if len(fields) < 2 || fields[0] == "Target" || strings.HasPrefix(line, "-") {
			continue
		}
		path := fields[1]
		if path == "" || path == "-" {
			continue
		}
		count++
		if count == n {
			return fields[0]
		}
	}
	return ""
}

// initCloneDeps 初始化 clone 子包的依赖注入
func initCloneDeps() {
	clonepkg.InitDeps(&clonepkg.Deps{
		// VM lifecycle
		StartVM:                     service.StartVM,
		StartVMPreserveRebootAction: service.StartVMPreserveRebootAction,
		GetVMInactiveDomainXML:      service.GetVMInactiveDomainXML,
		SetVMInactiveDomainXML:      service.SetVMInactiveDomainXML,
		SetVMRemark:                 service.SetVMRemark,
		SetVMFreeze:                 service.SetVMFreeze,
		FixOnReboot:                 service.FixOnReboot,

		// Template
		GetTemplateMeta:           service.GetTemplateMetaForClone,
		GetTemplateMinDiskSizeGB:  service.GetTemplateMinDiskSizeGB,
		EnsureTemplatePath:        service.EnsureTemplatePathExported,
		NormalizeTemplateBootType: service.NormalizeTemplateBootType,
		DetectTemplateBootType:    service.DetectTemplateBootType,
		ResolveCloneDiskSizeGB:    service.ResolveCloneDiskSizeGB,
		WriteVMTemplateSource:     service.WriteVMTemplateSource,

		// VM validation / normalization
		ValidateVMName:                 service.ValidateVMName,
		NormalizeVMNicModel:            service.NormalizeVMNicModel,
		NormalizeVMDiskBus:             service.NormalizeVMDiskBus,
		NormalizeVMCPUTopologyMode:     service.NormalizeVMCPUTopologyMode,
		NormalizeVMFirstBootRebootMode: service.NormalizeVMFirstBootRebootMode,
		VMCPULimitUnlimited:            service.VMCPULimitUnlimited,
		ValidateVMCPULimitPercent:      service.ValidateVMCPULimitPercent,

		// Resource checks
		ResolveVMStorageDir: service.ResolveVMStorageDir,
		CheckStorageSpace:   service.CheckStorageSpace,
		CheckDirWritable:    service.CheckDirWritable,
		CheckHostMemory:     service.CheckHostMemory,

		// Network / OVS
		EnsureOVSNetworkReady:         service.EnsureOVSNetworkReady,
		BuildOVSVirtInstallNetworkArg: service.BuildOVSVirtInstallNetworkArg,
		BuildOVSInterfaceXML:          service.BuildOVSInterfaceXML,
		GetOVSStaticIPByMAC:           service.GetOVSStaticIPByMAC,
		ListAllVPCStaticHosts:         service.ListAllVPCStaticHostsForClone,
		GetOVSLeaseIPByMAC:            service.GetOVSLeaseIPByMAC,

		// XML modification helpers
		ApplyRTCConfigToDomainXML:           service.ApplyRTCConfigToDomainXML,
		ApplyVMAPICToDomainXML:              service.ApplyVMAPICToDomainXML,
		ApplyCPUTopologyModeToDomainXML:     service.ApplyCPUTopologyModeToDomainXML,
		ApplyVMCPULimitToDomainXML:          service.ApplyVMCPULimitToDomainXML,
		ApplyCPUAffinityIfSet:               service.ApplyCPUAffinityIfSet,
		ApplyVPCSwitchToDomainXML:           service.ApplyVPCSwitchToDomainXML,
		ApplyFirstBootRebootModeToDomainXML: service.ApplyFirstBootRebootModeToDomainXML,
		EffectiveTopologyVCPU:               service.EffectiveTopologyVCPU,
		ShouldUseWindowsFirstBootColdReboot: service.ShouldUseWindowsFirstBootColdReboot,
		CompleteWindowsFirstBootColdReboot:  service.CompleteWindowsFirstBootColdReboot,
		BuildVCPUTag:                        service.BuildVCPUTag,
		ResolveRTCOffset:                    service.ResolveRTCOffset,
		NormalizeRTCStartDate:               service.NormalizeRTCStartDate,
		ParseRTCStartDateToEpoch:            service.ParseRTCStartDateToEpoch,
		VMRTCStartDateNow:                   service.VMRTCStartDateNow,
		VMRTCOffsetAbsolute:                 service.VMRTCOffsetAbsolute,
		InjectPCIERootPorts:                 service.InjectPCIERootPortsExported,

		// Disk / storage
		AddExtraDisksForVM: service.AddExtraDisksForVM,
		GetUserDiskDir:     service.GetUserDiskDir,
		CheckStorageQuota:  service.CheckStorageQuota,

		// VM credentials
		SaveVMCredential: service.SaveVMCredential,

		// VM cleanup
		DeleteVMStatsRecords:          service.DeleteVMStatsRecords,
		DeleteVMRuntimeRecord:         service.DeleteVMRuntimeRecord,
		CleanupVMVPCBinding:           service.CleanupVMVPCBinding,
		CleanupLightweightVMResources: service.CleanupLightweightVMResources,
		DeleteVMSchedules:             service.DeleteVMSchedules,

		// CPU affinity
		ParseCPUAffinity:            service.ParseCPUAffinity,
		ValidateCPUAffinity:         service.ValidateCPUAffinity,
		ApplyCPUAffinityToDomainXML: service.ApplyCPUAffinityToDomainXML,

		// Template boot type
		ResolveTemplateBootType: service.ResolveTemplateBootType,

		// VM first boot
		WaitForVMShutOff: service.WaitForVMShutOff,

		// Utility
		FirstNonEmpty: service.FirstNonEmpty,
		GetVMDiskInfo: service.GetVMDiskInfoForClone,

		// Disk expansion
		PrepareFnOSSystemDiskExpansion:    service.PrepareFnOSSystemDiskExpansionExported,
		PrepareWindowsSystemDiskExpansion: service.PrepareWindowsSystemDiskExpansionExported,

		// Migration hook
		HookEnsureVMNotMigrating: service.HookEnsureVMNotMigrating,

		// SPICE graphics（创建即带，默认本地监听）
		InjectSPICEGraphics:   service.InjectSPICEGraphicsToDomainXML,
		EnsureQXLVideo:        service.EnsureQXLVideo,
		SpiceEnabledByDefault: func() bool { return config.GlobalConfig.SpiceEnabledByDefault },
	})
}

// ensureLargeTempDir 检测 /tmp 是否为 tmpfs 且空间有限，若是则将 TMPDIR 重定向到磁盘目录。
// 避免大文件上传时因 Go multipart 解析将文件暂存到 tmpfs 导致空间不足。
// 设置 KVM_TMPDIR 环境变量可强制指定临时目录。
func ensureLargeTempDir() {
	// 1. 优先使用环境变量 KVM_TMPDIR
	if envDir := os.Getenv("KVM_TMPDIR"); envDir != "" {
		if err := os.MkdirAll(envDir, 0755); err == nil {
			os.Setenv("TMPDIR", envDir)
			utils.SetLargeUploadDiskMode(true)
			return
		}
	}

	// 2. 仅在 /tmp 为 tmpfs 且总空间小于 20GB 时才需要重定向
	//    （tmpfs 通常大小 = 物理内存一半，大文件上传极易耗尽）
	if !utils.IsTmpOnTmpfs() {
		return
	}
	tmpTotal := utils.GetTmpTotalBytes()
	if tmpTotal > 0 && tmpTotal > 20*1024*1024*1024 { // > 20GB，空间充裕
		return
	}

	// 3. 获取可执行文件所在目录作为回退
	execPath, err := os.Executable()
	if err != nil {
		return
	}
	execDir := filepath.Dir(execPath)

	// 4. 使用可执行文件同级的 tmp/multipart 目录
	tmpDir := filepath.Join(execDir, "tmp", "multipart")
	if err := os.MkdirAll(tmpDir, 0755); err != nil {
		return
	}
	os.Setenv("TMPDIR", tmpDir)
	utils.SetLargeUploadDiskMode(true)
}
