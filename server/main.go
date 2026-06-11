package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"strings"

	"kvm_console/config"
	"kvm_console/logger"
	"kvm_console/model"
	"kvm_console/router"
	"kvm_console/service"
	clonepkg "kvm_console/service/clone"
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
	service.StartPortForwardHTTPProbeScheduler()
	service.StartVMScheduleRunner()
	service.StartJWTSecretRotator()

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
		if saveErr := service.SaveVMCredential(params.Name, params.User, params.Password, "clone", task.CreatedBy, false); saveErr != nil {
			logger.App.Error("保存虚拟机克隆凭据失败", "vm", params.Name, "error", saveErr)
		}
		// 克隆完成后重新分配用户带宽
		if task.CreatedBy != "" && task.CreatedBy != "admin" {
			go func() {
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
		refreshVMCacheAfterTask(params.Name)
		resultJSON, _ := json.Marshal(result)
		return string(resultJSON), nil
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
		for _, result := range results {
			if result.Error != "" {
				continue
			}
			if err := bindTaskVMToVPC(task.CreatedBy, result.VMName, params.SwitchID, params.SecurityGroupID); err != nil {
				logger.App.Warn("批量克隆绑定 VPC 失败", "vm", result.VMName, "error", err)
			}
			attachTaskExtraNICs(result.VMName, task.Params)
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
		err := service.PrepareTemplate(&params)
		if err != nil {
			return "", err
		}
		progress(100, "模板制作完成")
		return fmt.Sprintf(`{"template":"%s"}`, params.TemplateName), nil
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
			ID string `json:"id"`
		}
		if err := json.Unmarshal([]byte(task.Params), &params); err != nil {
			return "", fmt.Errorf("解析参数失败: %w", err)
		}
		if params.ID == "" {
			return "", fmt.Errorf("存储池设备 ID 不能为空")
		}
		if err := service.FormatAndMountStoragePool(ctx, params.ID, progress); err != nil {
			return "", err
		}
		return fmt.Sprintf(`{"storage_pool_id":"%s"}`, params.ID), nil
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
			_ = service.DeleteVMCredential(params.Name)
			_ = model.DeleteVMLock(params.Name)
			markVMCacheMissingAfterTask(params.Name)
			if task.CreatedBy != "" && task.CreatedBy != "admin" {
				go func() {
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
		_ = service.DeleteVMCredential(params.Name)
		_ = model.DeleteVMLock(params.Name)
		markVMCacheMissingAfterTask(params.Name)
		// 删除完成后重新分配用户带宽
		if task.CreatedBy != "" && task.CreatedBy != "admin" {
			go func() {
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
	taskqueue.RegisterHandler(model.TaskTypePortForwardHTTPProbe, func(ctx context.Context, task *model.Task, progress func(int, string)) (string, error) {
		var params service.PortForwardHTTPProbeTaskParams
		if err := json.Unmarshal([]byte(task.Params), &params); err != nil {
			return "", fmt.Errorf("解析参数失败: %w", err)
		}
		return service.ExecuteManualPortForwardHTTPProbe(ctx, &params, task.CreatedBy, progress)
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
	logger.App.Info("任务处理器注册完成")
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
		resolvedSwitchID, resolvedSecurityGroupID, err := service.ResolveVPCForVMCreate(owner, switchID, securityGroupID)
		if err != nil {
			return err
		}
		switchID = resolvedSwitchID
		securityGroupID = resolvedSecurityGroupID
	}
	if switchID == 0 || securityGroupID == 0 {
		logger.App.Info("VM 未解析到交换机或安全组，跳过自动绑定", "vm", vmName)
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
	})
}
