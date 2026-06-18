package service

import (
	"kvm_console/model"
	bridge "kvm_console/service/network/bridge"
)

// init wires network/bridge package function variables to service root implementations.
// This breaks the circular dependency: bridge package cannot import service,
// so it exposes function variables that we set here.
func init() {
	bridge.HookEnsureOVSNetworkReady = EnsureOVSNetworkReady
	bridge.HookEnsureAllVPCSwitchRuntime = EnsureAllVPCSwitchRuntime
	bridge.HookWriteFileIfChanged = writeFileIfChanged
}

// ── Type aliases ──

type HostInterfaceInfo = bridge.HostInterfaceInfo
type NetworkBridgeInfo = bridge.NetworkBridgeInfo
type NetworkBridgeRequest = bridge.NetworkBridgeRequest
type InterfaceConfigInfo = bridge.InterfaceConfigInfo
type SetInterfaceConfigRequest = bridge.SetInterfaceConfigRequest

// ── Constant aliases ──

const BridgeModeNAT = bridge.BridgeModeNAT

// ── Exported delegates (used by handler and other service files) ──

// ListHostPhysicalInterfaces delegates to bridge.ListHostPhysicalInterfaces
func ListHostPhysicalInterfaces() ([]HostInterfaceInfo, error) {
	return bridge.ListHostPhysicalInterfaces()
}

// ListNetworkBridges delegates to bridge.ListNetworkBridges
func ListNetworkBridges() ([]NetworkBridgeInfo, error) {
	return bridge.ListNetworkBridges()
}

// CreateNetworkBridge delegates to bridge.CreateNetworkBridge
func CreateNetworkBridge(req NetworkBridgeRequest) (*model.NetworkBridge, error) {
	return bridge.CreateNetworkBridge(req)
}

// DeleteNetworkBridge delegates to bridge.DeleteNetworkBridge
func DeleteNetworkBridge(id uint) error {
	return bridge.DeleteNetworkBridge(id)
}

// DeleteNetworkBridgeByName delegates to bridge.DeleteNetworkBridgeByName
func DeleteNetworkBridgeByName(name string) error {
	return bridge.DeleteNetworkBridgeByName(name)
}

// GetInterfaceConfig delegates to bridge.GetInterfaceConfig
func GetInterfaceConfig(name string) (*InterfaceConfigInfo, error) {
	return bridge.GetInterfaceConfig(name)
}

// SetInterfaceConfig delegates to bridge.SetInterfaceConfig
func SetInterfaceConfig(req SetInterfaceConfigRequest) (*InterfaceConfigInfo, error) {
	return bridge.SetInterfaceConfig(req)
}

// EnsureAllNetworkBridgesRuntime delegates to bridge.EnsureAllNetworkBridgesRuntime
func EnsureAllNetworkBridgesRuntime() error {
	return bridge.EnsureAllNetworkBridgesRuntime()
}

// EnsureOVSBridgeDirect delegates to bridge.EnsureOVSBridgeDirect
func EnsureOVSBridgeDirect(bridgeName, uplink string, migrateHostIP bool, cfg bridge.HostIPConfig) error {
	return bridge.EnsureOVSBridgeDirect(bridgeName, uplink, migrateHostIP, cfg)
}

// BridgeModeForSwitch delegates to bridge.BridgeModeForSwitch
func BridgeModeForSwitch(sw model.VPCSwitch) string {
	return bridge.BridgeModeForSwitch(sw)
}

// BridgeNameForSwitch delegates to bridge.BridgeNameForSwitch
func BridgeNameForSwitch(sw model.VPCSwitch) string {
	return bridge.BridgeNameForSwitch(sw)
}

// SwitchUsesDirectBridge delegates to bridge.SwitchUsesDirectBridge
func SwitchUsesDirectBridge(sw model.VPCSwitch) bool {
	return bridge.SwitchUsesDirectBridge(sw)
}

// BuildOVSInterfaceXMLForBridge delegates to bridge.BuildOVSInterfaceXMLForBridge
func BuildOVSInterfaceXMLForBridge(mac, modelName, bridgeName string) string {
	return bridge.BuildOVSInterfaceXMLForBridge(mac, modelName, bridgeName)
}

// BuildOVSVirtInstallNetworkArgForBridge delegates to bridge.BuildOVSVirtInstallNetworkArgForBridge
func BuildOVSVirtInstallNetworkArgForBridge(modelName, bridgeName string) string {
	return bridge.BuildOVSVirtInstallNetworkArgForBridge(modelName, bridgeName)
}

// ── Unexported delegates (used internally by service root package) ──

// normalizeBridgeMode delegates to bridge.NormalizeBridgeMode
// Kept unexported for backward compatibility with vpc_register.go
func normalizeBridgeMode(mode string) string {
	return bridge.NormalizeBridgeMode(mode)
}
