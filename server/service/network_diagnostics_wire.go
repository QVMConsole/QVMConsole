package service

import (
	"context"

	diag "kvm_console/service/network/diagnostics"
	ovspkg "kvm_console/service/ovs"
	"kvm_console/utils"
)

// init wires network/diagnostics package function variables to service root implementations.
// This breaks the circular dependency: diagnostics package cannot import service,
// so it exposes function variables that we set here.
func init() {
	// HookGetVMNetworkRuntimeStatus: 调用 ovs 子包获取 VM 网络运行时状态，
	// 然后将 ovs 类型转换为 diagnostics 镜像类型
	diag.HookGetVMNetworkRuntimeStatus = func(vmName string) (*diag.VMNetworkRuntimeStatus, error) {
		status, err := GetVMNetworkRuntimeStatus(vmName)
		if err != nil {
			return nil, err
		}
		return convertOVSStatusToDiag(status), nil
	}

	// HookListLivePortForwardsFromIPTables: 调用 network 子包获取 iptables 端口转发规则，
	// 然后将 network 类型转换为 diagnostics 镜像类型
	diag.HookListLivePortForwardsFromIPTables = func() ([]diag.PortForwardRule, error) {
		rules, err := listLivePortForwardsFromIPTables()
		if err != nil {
			return nil, err
		}
		return convertNetPortForwardToDiag(rules), nil
	}

	// HookExecCommand: 封装 utils.ExecCommand 为 diagnostics.ExecResult
	diag.HookExecCommand = func(name string, args ...string) diag.ExecResult {
		result := utils.ExecCommand(name, args...)
		return diag.ExecResult{
			Stdout:   result.Stdout,
			Stderr:   result.Stderr,
			ExitCode: result.ExitCode,
			Error:    result.Error,
		}
	}
}

// ── Type aliases ──

type NetworkDiagnosticFilter = diag.NetworkDiagnosticFilter
type NetworkCaptureRequest = diag.NetworkCaptureRequest
type NetworkCaptureParams = diag.NetworkCaptureParams
type NetworkDiagnosticTemplate = diag.NetworkDiagnosticTemplate
type VMNetworkDiagnostics = diag.VMNetworkDiagnostics
type NetworkCaptureSession = diag.NetworkCaptureSession

// ── Exported delegates (used by handler and other service files) ──

// GetVMNetworkDiagnostics delegates to diagnostics.GetVMNetworkDiagnostics
func GetVMNetworkDiagnostics(vmName string) (*VMNetworkDiagnostics, error) {
	return diag.GetVMNetworkDiagnostics(vmName)
}

// InitNetworkCaptureSession delegates to diagnostics.InitNetworkCaptureSession
func InitNetworkCaptureSession(taskID uint, vmName string, req NetworkCaptureRequest, createdBy string) {
	diag.InitNetworkCaptureSession(taskID, vmName, req, createdBy)
}

// GetNetworkCaptureSession delegates to diagnostics.GetNetworkCaptureSession
func GetNetworkCaptureSession(taskID uint) (*NetworkCaptureSession, bool) {
	return diag.GetNetworkCaptureSession(taskID)
}

// DeleteNetworkCaptureFile delegates to diagnostics.DeleteNetworkCaptureFile
func DeleteNetworkCaptureFile(taskID uint) error {
	return diag.DeleteNetworkCaptureFile(taskID)
}

// NetworkCaptureFilePath delegates to diagnostics.CaptureFilePathAbs
func NetworkCaptureFilePath(taskID uint) (string, string, error) {
	return diag.CaptureFilePathAbs(taskID)
}

// ExecuteNetworkCapture delegates to diagnostics.ExecuteNetworkCapture
func ExecuteNetworkCapture(ctx context.Context, taskID uint, params NetworkCaptureParams, progress func(int, string)) (string, error) {
	return diag.ExecuteNetworkCapture(ctx, taskID, params, progress)
}

// ParseNetworkCaptureParams delegates to diagnostics.ParseNetworkCaptureParams
func ParseNetworkCaptureParams(raw string) (NetworkCaptureParams, error) {
	return diag.ParseNetworkCaptureParams(raw)
}

// BuildNetworkCaptureBPF delegates to diagnostics.BuildNetworkCaptureBPF
func BuildNetworkCaptureBPF(filter NetworkDiagnosticFilter) (string, error) {
	return diag.BuildNetworkCaptureBPF(filter)
}

// ── Helper functions ──

// convertOVSStatusToDiag 将 ovs.VMNetworkRuntimeStatus 转换为 diagnostics.VMNetworkRuntimeStatus
func convertOVSStatusToDiag(status *ovspkg.VMNetworkRuntimeStatus) *diag.VMNetworkRuntimeStatus {
	if status == nil {
		return nil
	}
	result := &diag.VMNetworkRuntimeStatus{
		VMName:              status.VMName,
		State:               status.State,
		Bridge:              status.Bridge,
		PortSecurityEnabled: status.PortSecurityEnabled,
		Issues:              append([]string{}, status.Issues...),
	}
	result.Interfaces = make([]diag.VMNetworkInterface, len(status.Interfaces))
	for i, iface := range status.Interfaces {
		result.Interfaces[i] = diag.VMNetworkInterface{
			InterfaceType:   iface.InterfaceType,
			Target:          iface.Target,
			SourceBridge:    iface.SourceBridge,
			SourceNetwork:   iface.SourceNetwork,
			Model:           iface.Model,
			MAC:             iface.MAC,
			VirtualPortType: iface.VirtualPortType,
			OFPort:          iface.OFPort,
			IP:              iface.IP,
			IPSource:        iface.IPSource,
			Issues:          append([]string{}, iface.Issues...),
		}
		if iface.PortSecurity != nil {
			result.Interfaces[i].PortSecurity = &diag.VMPortSecurityStatus{
				Mode:                  iface.PortSecurity.Mode,
				AllowedIPv4Addresses:  append([]string{}, iface.PortSecurity.AllowedIPv4Addresses...),
				AllowedIPv6Addresses:  append([]string{}, iface.PortSecurity.AllowedIPv6Addresses...),
				NeighborMeterID:       iface.PortSecurity.NeighborMeterID,
				BroadcastMeterID:      iface.PortSecurity.BroadcastMeterID,
				PolicingKpps:          iface.PortSecurity.PolicingKpps,
				PolicingBurstKPackets: iface.PortSecurity.PolicingBurstKPackets,
				DropPackets:           iface.PortSecurity.DropPackets,
				NeighborDropPackets:   iface.PortSecurity.NeighborDropPackets,
				BroadcastDropPackets:  iface.PortSecurity.BroadcastDropPackets,
				Applied:               iface.PortSecurity.Applied,
				Isolated:              iface.PortSecurity.Isolated,
				LastError:             iface.PortSecurity.LastError,
			}
		}
	}
	return result
}

// convertNetPortForwardToDiag 将 service.PortForwardRule（即 network.PortForwardRule）转换为 diagnostics.PortForwardRule
func convertNetPortForwardToDiag(rules []PortForwardRule) []diag.PortForwardRule {
	result := make([]diag.PortForwardRule, len(rules))
	for i, r := range rules {
		result[i] = diag.PortForwardRule{
			Protocol: r.Protocol,
			HostPort: r.HostPort,
			DestIP:   r.DestIP,
			DestPort: r.DestPort,
			RuleKey:  r.StableKey(),
		}
	}
	return result
}
