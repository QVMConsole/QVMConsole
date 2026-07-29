package service

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"kvm_console/config"
	"kvm_console/logger"
	"kvm_console/model"
	securitypkg "kvm_console/service/security"
	"kvm_console/taskqueue"
	"kvm_console/utils"
)

const passwordBreachSchedulerKey = "password_breach_daily"

type PasswordBreachScanParams struct {
	Source string `json:"source"`
}

type PasswordBreachNotifyParams struct {
	UserID uint `json:"user_id"`
}

type PasswordBreachScanResult = securitypkg.PasswordBreachScanResult
type PasswordBreachStatus = securitypkg.PasswordBreachStatus
type PasswordEnrollmentResult = securitypkg.PasswordEnrollmentResult

var (
	passwordBreachSchedulerOnce sync.Once
	passwordBreachSubmitMu      sync.Mutex
)

func StartPasswordBreachScheduler() {
	RegisterScheduler(SchedulerDefinition{
		Key: passwordBreachSchedulerKey, Name: "密码泄露定时检测", Group: "账户安全",
		Description: "每天本地时间 00:00 检测已登记的账户密码指纹",
		Enabled: func() bool {
			return config.GlobalConfig != nil && config.GlobalConfig.ScheduledPasswordBreachCheckEnabled
		},
	})
	passwordBreachSchedulerOnce.Do(func() {
		go func() {
			defer utils.RecoverAndLog("password-breach-scheduler")
			for {
				now := time.Now()
				next := time.Date(now.Year(), now.Month(), now.Day()+1, 0, 0, 0, 0, now.Location())
				timer := time.NewTimer(time.Until(next))
				<-timer.C
				if config.GlobalConfig != nil && config.GlobalConfig.ScheduledPasswordBreachCheckEnabled {
					if _, _, err := SubmitPasswordBreachScan(securitypkg.PasswordBreachScanSourceScheduled, "system:scheduler"); err != nil {
						logger.App.Warn("提交密码泄露定时检测失败", "error", err)
					}
				}
			}
		}()
	})
}

func SubmitPasswordBreachScan(source, createdBy string) (*model.Task, bool, error) {
	passwordBreachSubmitMu.Lock()
	defer passwordBreachSubmitMu.Unlock()
	if active, ok := taskqueue.GetActiveTask(model.TaskTypePasswordBreachScan); ok {
		return active, true, nil
	}
	task, err := taskqueue.SubmitWithStruct(model.TaskTypePasswordBreachScan, PasswordBreachScanParams{Source: source}, createdBy)
	return task, false, err
}

func SubmitPasswordBreachNotification(userID uint) {
	_, err := taskqueue.SubmitWithStruct(model.TaskTypePasswordBreachNotify, PasswordBreachNotifyParams{UserID: userID}, "system:security")
	if err != nil {
		logger.App.Warn("提交密码泄露通知任务失败", "user_id", userID, "error", err)
	}
}

func ExecutePasswordBreachScan(ctx context.Context, params PasswordBreachScanParams, progress func(int, string)) (PasswordBreachScanResult, error) {
	if params.Source == "" {
		params.Source = securitypkg.PasswordBreachScanSourceManual
	}
	var event *model.SchedulerEvent
	if params.Source == securitypkg.PasswordBreachScanSourceScheduled {
		event, _ = StartSchedulerEvent(SchedulerEventStartInput{
			SchedulerKey: passwordBreachSchedulerKey, SchedulerName: "密码泄露定时检测",
			SchedulerGroup: "账户安全", TriggerReason: "每日 00:00 自动检测",
		})
	}
	result, err := securitypkg.RunPasswordBreachScan(ctx, params.Source, progress)
	if event != nil {
		if err != nil {
			_ = FinishSchedulerEventFailed(event, err.Error())
		} else {
			_ = FinishSchedulerEventSuccess(event, fmt.Sprintf("管理员 %d 个，普通用户 %d 个", result.BreachedAdmins, result.BreachedUsers))
		}
	}
	return result, err
}

func ExecutePasswordBreachNotification(_ context.Context, params PasswordBreachNotifyParams, progress func(int, string)) (string, error) {
	if progress != nil {
		progress(30, "正在发送泄露密码通知")
	}
	if err := securitypkg.SendPendingPasswordBreachNotifications(false); err != nil {
		return "", err
	}
	if progress != nil {
		progress(100, "泄露密码通知已发送")
	}
	return fmt.Sprintf("账户 %d 的泄露密码通知已处理", params.UserID), nil
}

func GetPasswordBreachStatus() (PasswordBreachStatus, *model.Task, error) {
	status, err := securitypkg.GetPasswordBreachStatus(config.GlobalConfig != nil && config.GlobalConfig.ScheduledPasswordBreachCheckEnabled)
	if err != nil {
		return status, nil, err
	}
	active, _ := taskqueue.GetActiveTask(model.TaskTypePasswordBreachScan)
	return status, active, nil
}

func EnrollAndCheckPassword(user *model.User, password string, checkNow bool) (PasswordEnrollmentResult, error) {
	return securitypkg.EnrollAndCheckPassword(user, password, checkNow)
}

func EncodePasswordBreachTaskResult(result PasswordBreachScanResult) string {
	raw, _ := json.Marshal(result)
	return string(raw)
}
