package ovs

// deps.go — ovs 子包通过 Hook 变量调用 service 根包函数，避免循环 import。
// service 根包在 ovs_register.go 的 init() 中为这些变量赋值。

var (
	// HookEnsureOVSBridgeExists checks whether an OVS bridge already exists.
	HookEnsureOVSBridgeExists func(bridge string) error

	// HookBuildOVSVirtInstallNetworkArgForBridge builds the --network argument for virt-install.
	HookBuildOVSVirtInstallNetworkArgForBridge func(modelName, bridge string) string

	// HookBuildOVSInterfaceXMLForBridge builds a <interface> XML snippet for the given bridge.
	HookBuildOVSInterfaceXMLForBridge func(mac, modelName, bridge string) string

	// HookGetVMPortSecurityStatus 获取虚拟机逐端口防护状态。
	HookGetVMPortSecurityStatus func(vmName string) (bool, map[string]VMPortSecurityStatus, error)
)
