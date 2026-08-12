package public_ip

import "os"

// OVSStaticHost mirrors service.OVSStaticHost for use within the public_ip package.
type OVSStaticHost struct {
	VMName string
	MAC    string
	IP     string
}

// VMNetworkRuntimeStatus mirrors the fields used from service.VMNetworkRuntimeStatus.
type VMNetworkRuntimeStatus struct {
	Interfaces []VMNetworkInterface
}

// VMNetworkInterface mirrors the fields used from service.VMNetworkInterface.
type VMNetworkInterface struct {
	IP string
}

// OVSRuntimeInterface mirrors service.ovsRuntimeInterface for use within the public_ip package.
type OVSRuntimeInterface struct {
	Name   string
	Type   string
	Source string
	Model  string
	MAC    string
}

var (
	// ── VM / User hooks ──
	HookFindVMOwner func(vmName string) string

	// ── OVS / Network hooks ──
	HookGetVPCLeaseIPForVM        func(vmName string) string
	HookGetOVSStaticHostByVMName  func(vmName string) (OVSStaticHost, bool)
	HookGetVMNetworkRuntimeStatus func(vmName string) (*VMNetworkRuntimeStatus, error)
	HookGetVMRouteInterface       func(vmName string) string
	HookIsVPCManagedIP            func(ipText string) bool
	HookApplyVPCACLRules          func() error
	HookReconcilePortSecurity     func() error
	HookOvsUplink                 func() string
	HookOvsBridgeName             func() string
	HookOvsGatewayIP              func() string
	HookGetOVSInterfaceOfPort     func(vnetIF string) string
	HookParseVirshDomiflistOutput func(text string) []OVSRuntimeInterface
	HookWriteFileIfChanged        func(path string, content []byte, perm os.FileMode) (bool, error)
)
