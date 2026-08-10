package vpc

import (
	"os"

	"kvm_console/model"
)

// deps.go — 子包通过 hook 变量调用 service 根包函数，避免循环 import。
// service 根包在 vpc_register.go 的 init() 中为这些变量赋值。

// ── Equivalent types (避免直接引用 service 根包) ──

// StaticHost 等价于 service.OVSStaticHost
type StaticHost struct {
	VMName string
	MAC    string
	IP     string
}

// DHCPLease 等价于 service.OVSDHCPLease
type DHCPLease struct {
	ExpiryTime string
	ExpiryUnix int64
	MAC        string
	IP         string
	Hostname   string
	ClientID   string
}

// RuntimeInterface 等价于 service.ovsRuntimeInterface
type RuntimeInterface struct {
	Name   string
	Type   string
	Source string
	Model  string
	MAC    string
}

// PortForwardRuleBrief 等价于 service.PortForwardRule 中 VPC 所需字段
type PortForwardRuleBrief struct {
	DestIP   string
	Protocol string
	DestPort string
}

// ── Constants (与 service 根包一致，避免循环 import) ──

const (
	BridgeModeNAT    = "nat"
	BridgeModeDirect = "bridge"
	UserStatusActive = "active"
)

// ── OVS / Network hooks ──

var (
	HookOvsBridgeName    func() string
	HookOvsUplink        func() string
	HookEnsureOVSNetwork func() error

	HookEnsureOVSBridgeExists      func(bridge string) error
	HookEnsureOVSBridgeDirect      func(bridge, uplink string, migrateHostIP bool, hostAddrs, hostGW, hostMetric string) error
	HookGetOVSBridgePhysicalUplink func(bridge string) string

	HookBridgeNameForSwitch    func(sw model.VPCSwitch) string
	HookSwitchUsesDirectBridge func(sw model.VPCSwitch) bool
	HookBridgeModeForSwitch    func(sw model.VPCSwitch) string
	HookNormalizeBridgeMode    func(mode string) string

	HookAddOVSBandwidthMeter     func(bridge string, meterID uint32, rateKbit int) error
	HookGetOVSInterfaceOfPort    func(vnetIF string) string
	HookApplyTCVPCSwitchDownlink func(gwPort string, downMbps int)
	HookClearTCVPCSwitchDownlink func(gwPort string)

	HookEnsureIPTablesRule      func(checkCmd, addCmd, label string) error
	HookCleanupStaleNATRules    func(cidr, internalIF, currentUplink string)
	HookEnsureLocalDNSMasqInput func(iface string) error
	HookRemoveLocalDNSMasqInput func(iface string)
	HookWriteFileIfChanged      func(path string, content []byte, perm os.FileMode) (bool, error)

	HookParseVirshDomiflist          func(text string) []RuntimeInterface
	HookIsPortSecurityEnabled        func() bool
	HookTriggerPortSecurityReconcile func()
	HookReconcileVMPortSecurity      func(vmName string) error
)

// ── OVS Static Host / DHCP hooks ──

var (
	HookGetOVSStaticHostByVMName func(vmName string) (StaticHost, bool)
	HookGetOVSStaticIPByMAC      func(mac string) string
	HookRemoveOVSStaticHost      func(vmName, mac string) (string, error)
	HookUpsertVPCStaticHost      func(sw model.VPCSwitch, vmName, mac, ipAddr string) error
	HookRemoveVPCStaticHost      func(switchID uint, vmName, mac string) (string, error)
	HookGetVPCStaticIPByMAC      func(switchID uint, mac string) string
	HookListVPCStaticHosts       func(switchID uint) ([]StaticHost, error)
	HookListVPCDHCPLeases        func(switchID uint) ([]DHCPLease, error)

	HookCleanOVSDHCPLease     func(mac, ipAddr string)
	HookCleanAllVPCDHCPLeases func(mac, ipAddr string)
)

// ── VM / User hooks ──

var (
	HookFindVMOwner            func(vmName string) string
	HookIsLightweightCloudUser func(username string) bool
	HookGetUserVMList          func(username string) []string
	HookUserOwnsVM             func(username, vmName string) bool
	HookGetLightweightVMQuota  func(vmName string) (*model.LightweightVMQuota, error)
	HookClearVMBandwidth       func(vmName string) error

	HookListAllVMNames             func() []string
	HookGetFirewallVMIP            func(vmName string) string
	HookPublicIPNATPrivateIPsForVM func(vmName string) []string
	HookGetVMMACByOrder            func(vmName string, order int) string

	HookAttachVMInterface func(vmName string, sw model.VPCSwitch, nicModel string, interfaceOrder int) error
	HookDetachVMInterface func(vmName string, interfaceOrder int) error
)

// ── Port forward / Firewall hooks ──

var (
	HookRemoveVPCPortForwardAcceptRules func()
	HookSavePortForwardRules            func() error
	HookRemovePortForwardsForCIDR       func(cidr string)
	HookListLivePortForwards            func() ([]PortForwardRuleBrief, error)
	HookCleanupOVSStaticHostsForVMs     func(vmNames []string)
)

// ── Traffic / Bandwidth hooks ──

var (
	HookGetGlobalEffectiveBandwidth func() (downMbps, upMbps int)
	HookFormatTrafficBytes          func(bytes int64) string
)

// ── Utility hooks ──

var (
	HookFirstNonEmpty func(values ...string) string
)
