package diagnostics

import "fmt"

// GetVMNetworkDiagnostics 获取虚拟机网络诊断信息
func GetVMNetworkDiagnostics(vmName string) (*VMNetworkDiagnostics, error) {
	if HookGetVMNetworkRuntimeStatus == nil {
		return nil, fmt.Errorf("HookGetVMNetworkRuntimeStatus 未注入")
	}
	status, err := HookGetVMNetworkRuntimeStatus(vmName)
	if err != nil {
		return nil, err
	}
	diag := &VMNetworkDiagnostics{
		VMName:              status.VMName,
		State:               status.State,
		Interfaces:          convertInterfaces(status.Interfaces),
		PortSecurityEnabled: status.PortSecurityEnabled,
		Issues:              append([]string{}, status.Issues...),
	}
	for _, iface := range diag.Interfaces {
		if isUsableCaptureInterface(iface) {
			diag.DefaultInterface = iface.Target
			diag.DefaultIP = iface.IP
			break
		}
	}
	if diag.DefaultInterface != "" {
		diag.Neighbors = readNetworkNeighbors(diag.DefaultInterface)
	}
	diag.PortForwards = portForwardsForVMInterfaces(diag.Interfaces)
	diag.Templates = BuildNetworkDiagnosticTemplates(diag.DefaultIP, diag.PortForwards)
	if diag.DefaultInterface == "" {
		diag.Issues = append(diag.Issues, "未找到可抓包的运行中 vnet/tap 接口")
	}
	return diag, nil
}

// convertInterfaces 将 ovs 镜像类型 []VMNetworkInterface 转为内部类型
// （在 Hook 注入时已经由 register 转换，此处直接使用）
func convertInterfaces(ifaces []VMNetworkInterface) []VMNetworkInterface {
	result := make([]VMNetworkInterface, len(ifaces))
	copy(result, ifaces)
	return result
}
