package service

import (
	"context"
	"fmt"
	"sync"
	"time"

	"kvm_console/config"
	"kvm_console/logger"
	"kvm_console/model"
	"kvm_console/service/storage/quota"
	"kvm_console/taskqueue"
	"kvm_console/utils"
)

const storageTrimSchedulerKey = "storage_trim_daily"

var (
	storageTrimSchedulerOnce sync.Once
	storageTrimSubmitMu      sync.Mutex
)

// StartStorageTrimScheduler 启动用户存储自动回收调度器
// 每天本地时间 02:00 自动执行一次存储回收（fstrim + fallocate --dig-holes）
func StartStorageTrimScheduler() {
	RegisterScheduler(SchedulerDefinition{
		Key: storageTrimSchedulerKey, Name: "用户存储自动回收", Group: "存储维护",
		Description: "每天本地时间 02:00 自动回收用户存储稀疏文件中的未使用空间",
		Enabled: func() bool {
			return config.GlobalConfig != nil && config.GlobalConfig.ScheduledStorageTrimEnabled
		},
	})
	storageTrimSchedulerOnce.Do(func() {
		go func() {
			defer utils.RecoverAndLog("storage-trim-scheduler")
			for {
				now := time.Now()
				next := time.Date(now.Year(), now.Month(), now.Day()+1, 2, 0, 0, 0, now.Location())
				timer := time.NewTimer(time.Until(next))
				<-timer.C
				if config.GlobalConfig != nil && config.GlobalConfig.ScheduledStorageTrimEnabled {
					if _, _, err := SubmitStorageTrim("system:scheduler", "每日 02:00 自动回收"); err != nil {
						logger.App.Warn("提交用户存储自动回收任务失败", "error", err)
					}
				}
			}
		}()
	})
}

// SubmitStorageTrim 提交用户存储回收任务（已有运行中任务则复用）
type StorageTrimTaskParams struct {
	TriggerReason string `json:"trigger_reason"`
}

func SubmitStorageTrim(createdBy, triggerReason string) (*model.Task, bool, error) {
	storageTrimSubmitMu.Lock()
	defer storageTrimSubmitMu.Unlock()
	if active, ok := taskqueue.GetActiveTask(model.TaskTypeStorageTrim); ok {
		return active, true, nil
	}
	if createdBy == "" {
		createdBy = "system:scheduler"
	}
	if triggerReason == "" {
		triggerReason = "用户存储回收"
	}
	task, err := taskqueue.SubmitWithStruct(model.TaskTypeStorageTrim, StorageTrimTaskParams{
		TriggerReason: triggerReason,
	}, createdBy)
	return task, false, err
}

// ExecuteStorageTrim 执行用户存储回收（任务队列入口）
// 用户存储文件系统未挂载时跳过本次执行，不视为失败
func ExecuteStorageTrim(ctx context.Context, params StorageTrimTaskParams, progress func(int, string)) (*quota.TrimStorageResult, error) {
	triggerReason := params.TriggerReason
	if triggerReason == "" {
		triggerReason = "用户存储回收"
	}
	event, _ := StartSchedulerEvent(SchedulerEventStartInput{
		SchedulerKey: storageTrimSchedulerKey, SchedulerName: "用户存储自动回收",
		SchedulerGroup: "存储维护", TriggerReason: triggerReason,
	})
	if !quota.IsStorageFilesystemMounted() {
		if event != nil {
			_ = FinishSchedulerEventSuccess(event, "用户存储文件系统未挂载，本次跳过")
		}
		return nil, nil
	}
	if progress != nil {
		progress(20, "正在执行用户存储回收")
	}
	result, err := quota.TrimStorage(ctx)
	if event != nil {
		if err != nil {
			_ = FinishSchedulerEventFailed(event, err.Error())
		} else {
			_ = FinishSchedulerEventSuccess(event, fmt.Sprintf("释放空间 %s", result.TrimmedHuman))
		}
	}
	if progress != nil {
		progress(100, "用户存储回收完成")
	}
	return result, err
}
