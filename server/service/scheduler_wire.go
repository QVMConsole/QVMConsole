package service

import (
	"kvm_console/model"
	schedpkg "kvm_console/service/scheduler"
)

// ── Type aliases（向后兼容，让 service 根包和外部调用方可直接使用类型名） ──

type SchedulerDefinition = schedpkg.SchedulerDefinition
type SchedulerListItem = schedpkg.SchedulerListItem
type SchedulerEventMessage = schedpkg.SchedulerEventMessage
type SchedulerEventStartInput = schedpkg.SchedulerEventStartInput

// ── Exported delegates ──

func RegisterScheduler(def SchedulerDefinition) {
	schedpkg.RegisterScheduler(def)
}

func ListSchedulers() ([]SchedulerListItem, error) {
	return schedpkg.ListSchedulers()
}

func StartSchedulerEvent(input SchedulerEventStartInput) (*model.SchedulerEvent, error) {
	return schedpkg.StartSchedulerEvent(input)
}

func FinishSchedulerEventSuccess(event *model.SchedulerEvent, resultMessage string) error {
	return schedpkg.FinishSchedulerEventSuccess(event, resultMessage)
}

func FinishSchedulerEventFailed(event *model.SchedulerEvent, errorMessage string) error {
	return schedpkg.FinishSchedulerEventFailed(event, errorMessage)
}

func RegisterSchedulerSSEClient(ch chan SchedulerEventMessage) {
	schedpkg.RegisterSchedulerSSEClient(ch)
}

func UnregisterSchedulerSSEClient(ch chan SchedulerEventMessage) {
	schedpkg.UnregisterSchedulerSSEClient(ch)
}

func StartSchedulerEventCleanup() {
	schedpkg.StartSchedulerEventCleanup()
}
