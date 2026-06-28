package clone

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"

	"kvm_console/config"
	"kvm_console/logger"
	"kvm_console/taskqueue"
)

// BatchCloneVM 批量克隆（支持取消）
func BatchCloneVM(ctx context.Context, params *BatchCloneParams, progressFn func(int, string)) ([]CloneResult, error) {
	maxConcurrency := config.GlobalConfig.BatchCloneMaxConcurrency
	if maxConcurrency <= 0 {
		maxConcurrency = 10
	}

	results := make([]CloneResult, params.Count)
	var mu sync.Mutex
	var wg sync.WaitGroup
	sem := make(chan struct{}, maxConcurrency)
	var completed int32
	var cancelled int32

	progressFn(0, fmt.Sprintf("开始批量克隆 %d 台虚拟机（最大并发 %d）...", params.Count, maxConcurrency))

	for i := 0; i < params.Count; i++ {
		select {
		case <-ctx.Done():
			return results[:completed], taskqueue.ErrTaskCanceled
		default:
		}

		wg.Add(1)
		sem <- struct{}{}

		go func(index int) {
			defer wg.Done()
			defer func() { <-sem }()

			if atomic.LoadInt32(&cancelled) == 1 {
				return
			}

			vmName := fmt.Sprintf("%s-%s", params.Prefix, padNum(params.StartNum+index))

			vmPassword := params.Password
			if vmPassword == "" {
				vmPassword = GenerateRandomStrongPassword()
			}

			cloneParams := &CloneParams{
				Name:                vmName,
				Template:            params.Template,
				TemplateType:        params.TemplateType,
				CloneMode:           params.CloneMode,
				VCPU:                params.VCPU,
				MaxVCPU:             params.MaxVCPU,
				RAM:                 params.RAM,
				DiskSize:            params.DiskSize,
				Network:             params.Network,
				Hostname:            params.Hostname,
				User:                params.User,
				Password:            vmPassword,
				Autostart:           params.Autostart,
				Freeze:              params.Freeze,
				APIC:                params.APIC,
				PAE:                 params.PAE,
				RTCOffset:           params.RTCOffset,
				RTCStartDate:        params.RTCStartDate,
				GuestAgent:          params.GuestAgent,
				SMBIOS1:             params.SMBIOS1,
				UEFI:                params.UEFI,
				TemplateRootPass:    params.TemplateRootPass,
				TemplateUser:        params.TemplateUser,
				VideoModel:          params.VideoModel,
				SpiceEnabled:        params.SpiceEnabled,
				DiskBus:             params.DiskBus,
				NicModel:            params.NicModel,
				StoragePoolID:       params.StoragePoolID,
				CPUTopologyMode:     params.CPUTopologyMode,
				CPULimitPercent:     params.CPULimitPercent,
				CPUAffinity:         params.CPUAffinity,
				FirstBootRebootMode: params.FirstBootRebootMode,
				SwitchID:            params.SwitchID,
				SecurityGroupID:     params.SecurityGroupID,
				IsAdmin:             params.IsAdmin,
				DisableSystemInit:   params.DisableSystemInit,
				StaticIP:            params.StaticIP,
				Gateway:             params.Gateway,
				DNS:                 params.DNS,
			}

			subProgress := func(_ int, msg string) {
				logger.App.Info("批量克隆", "vm", vmName, "msg", msg)
			}

			result, err := CloneVM(ctx, cloneParams, subProgress)
			if err != nil {
				if err == taskqueue.ErrTaskCanceled {
					atomic.StoreInt32(&cancelled, 1)
					return
				}
			}

			mu.Lock()
			if err != nil {
				results[index] = CloneResult{VMName: vmName, Error: err.Error()}
			} else {
				if result.Password == "" {
					result.Password = vmPassword
				}
				results[index] = *result
			}
			atomic.AddInt32(&completed, 1)
			done := atomic.LoadInt32(&completed)
			progressFn(int(done*100/int32(params.Count)), fmt.Sprintf("已完成 %d/%d 台", done, params.Count))
			mu.Unlock()
		}(i)
	}

	wg.Wait()

	if atomic.LoadInt32(&cancelled) == 1 {
		return results, taskqueue.ErrTaskCanceled
	}

	return results, nil
}
