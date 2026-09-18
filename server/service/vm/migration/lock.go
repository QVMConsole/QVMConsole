package migration

import (
	"encoding/json"
	"fmt"
	"strings"

	"kvm_console/model"
	"kvm_console/service"
	"kvm_console/taskqueue"
)

// IsVMUnderMigration 判断 VM 是否存在等待中或运行中的迁移任务。
func IsVMUnderMigration(vmName string) bool {
	vmName = strings.TrimSpace(vmName)
	if vmName == "" {
		return false
	}
	if taskqueue.HasActiveTask(model.TaskTypeVMMigrate, func(raw string) bool {
		var params VMMigrationTaskParams
		if err := json.Unmarshal([]byte(raw), &params); err != nil {
			return false
		}
		return strings.TrimSpace(params.VMName) == vmName
	}) {
		return true
	}
	return taskqueue.HasActiveTask(model.TaskTypeVMDiskMigrate, func(raw string) bool {
		var params service.VMDiskMigrationTaskParams
		if err := json.Unmarshal([]byte(raw), &params); err != nil {
			return false
		}
		return strings.TrimSpace(params.VMName) == vmName
	})
}

// HasActiveReinstallTask 判断 VM 是否已有等待中或执行中的重装任务。
func HasActiveReinstallTask(vmName string) bool {
	vmName = strings.TrimSpace(vmName)
	if vmName == "" {
		return false
	}
	return taskqueue.HasActiveTask(model.TaskTypeReinstall, func(raw string) bool {
		return isReinstallTaskForVM(raw, vmName)
	})
}

func isReinstallTaskForVM(raw, vmName string) bool {
	var params service.ReinstallParams
	if err := json.Unmarshal([]byte(raw), &params); err != nil {
		return false
	}
	return strings.TrimSpace(params.Name) == strings.TrimSpace(vmName)
}

// EnsureVMNotMigrating 在执行用户侧 VM 操作前阻止迁移中的 VM 被修改。
func EnsureVMNotMigrating(vmName, action string) error {
	if !IsVMUnderMigration(vmName) {
		return nil
	}
	action = strings.TrimSpace(action)
	if action == "" {
		action = "该操作"
	}
	return fmt.Errorf("虚拟机正在迁移中，暂不能执行%s", action)
}

// ApplyVMUnderMigrationStatus sets the VM status to migrating if applicable.
func ApplyVMUnderMigrationStatus(vm *service.VmInfo) {
	if vm == nil || !IsVMUnderMigration(vm.Name) {
		return
	}
	vm.Status = VMStatusMigrating
}
