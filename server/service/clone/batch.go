package clone

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"

	"kvm_console/config"
	"kvm_console/logger"
	"kvm_console/taskqueue"
	"kvm_console/utils"
)

// BatchCloneVM 批量克隆（支持取消）
func BatchCloneVM(ctx context.Context, params *BatchCloneParams, progressFn func(int, string)) ([]CloneResult, error) {
	if len(params.HostDevices) > 0 && !params.IsAdmin {
		return nil, fmt.Errorf("仅管理员可配置硬件直通设备")
	}
	if params.Count > 1 && len(params.HostDevices) > 0 {
		return nil, fmt.Errorf("批量克隆不能复用同一组物理直通设备，请改为单台克隆")
	}
	maxConcurrency := config.GlobalConfig.BatchCloneMaxConcurrency
	if maxConcurrency <= 0 {
		maxConcurrency = 10
	}

	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

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
			wg.Wait()
			return results[:completed], taskqueue.ErrTaskCanceled
		default:
		}

		wg.Add(1)
		select {
		case sem <- struct{}{}:
		case <-ctx.Done():
			wg.Done()
			wg.Wait()
			return results[:completed], taskqueue.ErrTaskCanceled
		}

		go func(index int) {
			defer utils.RecoverAndLog("clone-batch")
			defer wg.Done()
			defer func() { <-sem }()

			if atomic.LoadInt32(&cancelled) == 1 {
				return
			}

			select {
			case <-ctx.Done():
				return
			default:
			}

			vmName := fmt.Sprintf("%s-%s", params.Prefix, padNum(params.StartNum+index))

			vmPassword := params.Password
			if vmPassword == "" {
				vmPassword = GenerateRandomStrongPassword()
			}

			cloneParams := &CloneParams{
				Owner:                params.Owner,
				Name:                 vmName,
				Template:             params.Template,
				TemplateType:         params.TemplateType,
				CloneMode:            params.CloneMode,
				VCPU:                 params.VCPU,
				MaxVCPU:              params.MaxVCPU,
				RAM:                  params.RAM,
				DiskSize:             params.DiskSize,
				Network:              params.Network,
				Hostname:             params.Hostname,
				User:                 params.User,
				Password:             vmPassword,
				Autostart:            params.Autostart,
				Freeze:               params.Freeze,
				APIC:                 params.APIC,
				PAE:                  params.PAE,
				RTCOffset:            params.RTCOffset,
				RTCStartDate:         params.RTCStartDate,
				GuestAgent:           params.GuestAgent,
				SMBIOS1:              params.SMBIOS1,
				UEFI:                 params.UEFI,
				TemplateRootPass:     params.TemplateRootPass,
				TemplateUser:         params.TemplateUser,
				VideoModel:           params.VideoModel,
				SpiceEnabled:         params.SpiceEnabled,
				DiskBus:              params.DiskBus,
				NicModel:             params.NicModel,
				StoragePoolID:        params.StoragePoolID,
				CPUTopologyMode:      params.CPUTopologyMode,
				CPULimitPercent:      params.CPULimitPercent,
				CPUAffinity:          params.CPUAffinity,
				FirstBootRebootMode:  params.FirstBootRebootMode,
				SwitchID:             params.SwitchID,
				SecurityGroupID:      params.SecurityGroupID,
				AllowedIPv4Addresses: params.AllowedIPv4Addresses,
				AllowedIPv6Addresses: params.AllowedIPv6Addresses,
				ExtraDisks:           params.ExtraDisks,
				HostDevices:          params.HostDevices,
				IsAdmin:              params.IsAdmin,
				DisableSystemInit:    params.DisableSystemInit,
				StaticIP:             params.StaticIP,
				Gateway:              params.Gateway,
				DNS:                  params.DNS,
				PCIERootPorts:        params.PCIERootPorts,
				NestedVirt:           params.NestedVirt,
				KVMHidden:            params.KVMHidden,
				VendorID:             params.VendorID,
			}

			subProgress := func(_ int, msg string) {
				logger.App.Info("批量克隆", "vm", vmName, "msg", msg)
			}

			result, err := CloneVM(ctx, cloneParams, subProgress)
			if err != nil {
				if err == taskqueue.ErrTaskCanceled {
					atomic.StoreInt32(&cancelled, 1)
					cancel()
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
