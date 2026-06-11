package service

import (
	"kvm_console/model"
	fwpkg "kvm_console/service/firewall"
	netpkg "kvm_console/service/network"
	vpcpkg "kvm_console/service/network/vpc"
	ovspkg "kvm_console/service/ovs"
	vmpkg "kvm_console/service/vm"
)

// vpc_register.go — 将 service 根包函数注册到 vpc 子包的 Hook 变量，
// 并将 vpc 子包的导出函数注册到 service 根包的 Hook 变量。
// vpc 子包通过 Hook 调用 service 根包函数，避免循环 import。

// ── Type aliases（向后兼容，让 service 根包和外部调用方可直接使用类型名） ──

type VPCSwitchRequest = vpcpkg.VPCSwitchRequest
type VPCSecurityGroupRequest = vpcpkg.VPCSecurityGroupRequest
type VPCSecurityGroupRuleRequest = vpcpkg.VPCSecurityGroupRuleRequest
type AddVMInterfaceRequest = vpcpkg.AddVMInterfaceRequest
type VPCQuotaInfo = vpcpkg.VPCQuotaInfo
type VPCBindingInfo = vpcpkg.VPCBindingInfo
type VMInterfaceInfo = vpcpkg.VMInterfaceInfo

// ── Wrapper functions（向后兼容，让 service 根包和外部调用方仍通过 service.Xxx 调用） ──

// Switch CRUD
func ListVPCSwitches(operator, role, requestedUsername string) ([]model.VPCSwitch, error) {
	return vpcpkg.ListVPCSwitches(operator, role, requestedUsername)
}
func CreateVPCSwitch(operator, role string, req VPCSwitchRequest) (*model.VPCSwitch, error) {
	return vpcpkg.CreateVPCSwitch(operator, role, req)
}
func UpdateVPCSwitch(operator, role string, id uint, req VPCSwitchRequest) (*model.VPCSwitch, error) {
	return vpcpkg.UpdateVPCSwitch(operator, role, id, req)
}
func DeleteVPCSwitch(operator, role string, id uint) error {
	return vpcpkg.DeleteVPCSwitch(operator, role, id)
}
func ResetVPCSwitchTraffic(operator, role string, id uint) error {
	return vpcpkg.ResetVPCSwitchTraffic(operator, role, id)
}

// Switch runtime
func EnsureVPCSwitchRuntime(sw model.VPCSwitch) error {
	return vpcpkg.EnsureVPCSwitchRuntime(sw)
}
func EnsureAllVPCSwitchRuntime() error {
	return vpcpkg.EnsureAllVPCSwitchRuntime()
}
func VPCGatewayPortName(id uint) string {
	return vpcpkg.VPCGatewayPortName(id)
}

// Switch bandwidth
func ApplyVPCSwitchBandwidth(sw model.VPCSwitch) error {
	return vpcpkg.ApplyVPCSwitchBandwidth(sw)
}

// Security group CRUD
func ListVPCSecurityGroups(operator, role, requestedUsername string) ([]model.VPCSecurityGroup, error) {
	return vpcpkg.ListVPCSecurityGroups(operator, role, requestedUsername)
}
func CreateVPCSecurityGroup(operator, role string, req VPCSecurityGroupRequest) (*model.VPCSecurityGroup, error) {
	return vpcpkg.CreateVPCSecurityGroup(operator, role, req)
}
func UpdateVPCSecurityGroup(operator, role string, id uint, req VPCSecurityGroupRequest) (*model.VPCSecurityGroup, error) {
	return vpcpkg.UpdateVPCSecurityGroup(operator, role, id, req)
}
func DeleteVPCSecurityGroup(operator, role string, id uint) error {
	return vpcpkg.DeleteVPCSecurityGroup(operator, role, id)
}
func AddVPCSecurityGroupRule(operator, role string, groupID uint, req VPCSecurityGroupRuleRequest) (*model.VPCSecurityGroupRule, error) {
	return vpcpkg.AddVPCSecurityGroupRule(operator, role, groupID, req)
}
func DeleteVPCSecurityGroupRule(operator, role string, ruleID uint) error {
	return vpcpkg.DeleteVPCSecurityGroupRule(operator, role, ruleID)
}

// ACL
func PreviewVPCACLRules() (string, error) {
	return vpcpkg.PreviewVPCACLRules()
}
func ApplyVPCACLRules() error {
	return vpcpkg.ApplyVPCACLRules()
}

// Binding
func BindVMToVPC(username, vmName string, switchID, securityGroupID uint) error {
	return vpcpkg.BindVMToVPC(username, vmName, switchID, securityGroupID)
}
func BindVMToVPCAsAdmin(vmName string, switchID, securityGroupID uint) error {
	return vpcpkg.BindVMToVPCAsAdmin(vmName, switchID, securityGroupID)
}
func GetVPCBindingInfo(operator, role, vmName string) (*VPCBindingInfo, error) {
	return vpcpkg.GetVPCBindingInfo(operator, role, vmName)
}
func SwitchVMSecurityGroup(operator, role, vmName string, securityGroupID uint) error {
	return vpcpkg.SwitchVMSecurityGroup(operator, role, vmName, securityGroupID)
}
func ApplyVPCSwitchToDomainXML(vmXML string, switchID uint) (string, error) {
	return vpcpkg.ApplyVPCSwitchToDomainXML(vmXML, switchID)
}

// Interface management
func AddVMInterface(vmName string, req AddVMInterfaceRequest) (*VMInterfaceInfo, error) {
	return vpcpkg.AddVMInterface(vmName, req)
}
func RemoveVMInterface(vmName string, interfaceOrder int) error {
	return vpcpkg.RemoveVMInterface(vmName, interfaceOrder)
}
func AttachExtraNICs(vmName string, extraNics []AddVMInterfaceRequest) {
	vpcpkg.AttachExtraNICs(vmName, extraNics)
}
func ListVMInterfaces(vmName string) ([]VMInterfaceInfo, error) {
	return vpcpkg.ListVMInterfaces(vmName)
}

// Helpers
func EnsureDefaultSecurityGroup(username string) (*model.VPCSecurityGroup, error) {
	return vpcpkg.EnsureDefaultSecurityGroup(username)
}
func EnsureDefaultVPCSwitch(username string) (*model.VPCSwitch, error) {
	return vpcpkg.EnsureDefaultVPCSwitch(username)
}
func EnsureAllActiveUsersDefaultSecurityGroup() {
	vpcpkg.EnsureAllActiveUsersDefaultSecurityGroup()
}
func EnsureSystemBaseNetwork() error {
	return vpcpkg.EnsureSystemBaseNetwork()
}
func GetVPCQuota(username string) (*VPCQuotaInfo, error) {
	return vpcpkg.GetVPCQuota(username)
}
func IPInCIDR(ipText, cidrText string) bool {
	return vpcpkg.IPInCIDR(ipText, cidrText)
}
func IsVPCManagedIP(ipText string) bool {
	return vpcpkg.IsVPCManagedIP(ipText)
}

// Runtime apply
func ApplyVPCBindingRuntime(vmName string) error {
	return vpcpkg.ApplyVPCBindingRuntime(vmName)
}
func ApplyVPCSwitchRuntime(vmName string, sw model.VPCSwitch) error {
	return vpcpkg.ApplyVPCSwitchRuntime(vmName, sw)
}
func ApplyAllVPCBindingsRuntime() error {
	return vpcpkg.ApplyAllVPCBindingsRuntime()
}

// VM create helpers
func EnsureVPCForVMCreate(username string, switchID, securityGroupID uint) error {
	return vpcpkg.EnsureVPCForVMCreate(username, switchID, securityGroupID)
}
func ResolveVPCForVMCreate(username string, switchID, securityGroupID uint) (uint, uint, error) {
	return vpcpkg.ResolveVPCForVMCreate(username, switchID, securityGroupID)
}
func EnsureSecurityGroupAllowsPortForward(vmName, protocol, portText string) error {
	return vpcpkg.EnsureSecurityGroupAllowsPortForward(vmName, protocol, portText)
}
func RemoveSecurityGroupAllowsPortForwardIfUnused(destIP, protocol, portText string) error {
	return vpcpkg.RemoveSecurityGroupAllowsPortForwardIfUnused(destIP, protocol, portText)
}

// Traffic
func CheckAllVPCSwitchTrafficQuota() {
	vpcpkg.CheckAllVPCSwitchTrafficQuota()
}
func ResetAllVPCSwitchMonthlyTraffic() {
	vpcpkg.ResetAllVPCSwitchMonthlyTraffic()
}
func CheckVPCSwitchTrafficAfterQuotaUpdate(switchID uint) {
	vpcpkg.CheckVPCSwitchTrafficAfterQuotaUpdate(switchID)
}
func AggregateSwitchMonthlyTraffic(switchID uint) (int64, int64) {
	return vpcpkg.AggregateSwitchMonthlyTraffic(switchID)
}
func IsVPCSwitchTrafficLimited(switchID uint) (bool, bool) {
	return vpcpkg.IsVPCSwitchTrafficLimited(switchID)
}

// Inference
func InferVPCSwitchForVM(vmName string) (*model.VPCSwitch, bool) {
	return vpcpkg.InferVPCSwitchForVM(vmName)
}

// Cleanup
func CleanupUserNetworkResources(username string, vmNames []string) error {
	return vpcpkg.CleanupUserNetworkResources(username, vmNames)
}
func CleanupVMVPCBinding(vmName string) {
	vpcpkg.CleanupVMVPCBinding(vmName)
}

// Traffic helpers
func CurrentTrafficMonth() string {
	return vpcpkg.CurrentTrafficMonth()
}
func ClampTrafficBytes(value int64) int64 {
	return vpcpkg.ClampTrafficBytes(value)
}
func TrafficQuotaBytes(gb float64) float64 {
	return vpcpkg.TrafficQuotaBytes(gb)
}

// Config / Path helpers
var VPCConfigDir = vpcpkg.VPCConfigDir // const from vpc sub-package, exposed as var for backward compat
func VPCDHCPHostsPath(id uint) string {
	return vpcpkg.VPCDHCPHostsPath(id)
}
func ReloadVPCDNSMasq(id uint) {
	vpcpkg.ReloadVPCDNSMasq(id)
}
func SetFirstOVSInterfaceVLANTag(xmlText string, vlanID int) (string, bool) {
	return vpcpkg.SetFirstOVSInterfaceVLANTag(xmlText, vlanID)
}
func SafeVMXMLFileName(vmName string) string {
	return vpcpkg.SafeVMXMLFileName(vmName)
}
func StripRuntimeOnlyInterfaceElements(block string) string {
	return vpcpkg.StripRuntimeOnlyInterfaceElements(block)
}

// ── Hook registration ──

func init() {
	// ── Register vpc sub-package hooks INTO service root ──
	HookApplyVPCBindingRuntime = vpcpkg.ApplyVPCBindingRuntime

	// ── OVS / Network hooks ──
	vpcpkg.HookOvsBridgeName = ovspkg.OvsBridgeName
	vpcpkg.HookOvsUplink = ovspkg.OvsUplink
	vpcpkg.HookEnsureOVSNetwork = ovspkg.EnsureOVSNetworkReady
	vpcpkg.HookEnsureOVSBridgeExists = EnsureOVSBridgeExists
	vpcpkg.HookEnsureOVSBridgeDirect = EnsureOVSBridgeDirect
	vpcpkg.HookBridgeNameForSwitch = BridgeNameForSwitch
	vpcpkg.HookSwitchUsesDirectBridge = SwitchUsesDirectBridge
	vpcpkg.HookBridgeModeForSwitch = BridgeModeForSwitch
	vpcpkg.HookNormalizeBridgeMode = normalizeBridgeMode
	vpcpkg.HookAddOVSBandwidthMeter = addOVSBandwidthMeter
	vpcpkg.HookGetOVSInterfaceOfPort = getOVSInterfaceOfPort
	vpcpkg.HookApplyTCVPCSwitchDownlink = applyTCVPCSwitchDownlinkLimit
	vpcpkg.HookClearTCVPCSwitchDownlink = clearTCVPCSwitchDownlinkLimit
	vpcpkg.HookEnsureIPTablesRule = ovspkg.EnsureIPTablesRule
	vpcpkg.HookEnsureLocalDNSMasqInput = ovspkg.EnsureLocalDNSMasqInputRules
	vpcpkg.HookRemoveLocalDNSMasqInput = func(iface string) {
		ovspkg.RemoveLocalDNSMasqInputRules(iface)
	}
	vpcpkg.HookWriteFileIfChanged = ovspkg.WriteFileIfChanged
	vpcpkg.HookParseVirshDomiflist = func(text string) []vpcpkg.RuntimeInterface {
		ifaceList := parseVirshDomiflistOutput(text)
		result := make([]vpcpkg.RuntimeInterface, len(ifaceList))
		for i, iface := range ifaceList {
			result[i] = vpcpkg.RuntimeInterface{
				Name:   iface.Name,
				Type:   iface.Type,
				Source: iface.Source,
				Model:  iface.Model,
				MAC:    iface.MAC,
			}
		}
		return result
	}

	// ── OVS Static Host / DHCP hooks ──
	vpcpkg.HookGetOVSStaticHostByVMName = func(vmName string) (vpcpkg.StaticHost, bool) {
		host, ok := ovspkg.GetOVSStaticHostByVMName(vmName)
		return vpcpkg.StaticHost{VMName: host.VMName, MAC: host.MAC, IP: host.IP}, ok
	}
	vpcpkg.HookGetOVSStaticIPByMAC = ovspkg.GetOVSStaticIPByMAC
	vpcpkg.HookRemoveOVSStaticHost = ovspkg.RemoveOVSStaticHost
	vpcpkg.HookUpsertVPCStaticHost = netpkg.UpsertVPCStaticHost
	vpcpkg.HookRemoveVPCStaticHost = netpkg.RemoveVPCStaticHost
	vpcpkg.HookGetVPCStaticIPByMAC = netpkg.GetVPCStaticIPByMAC
	vpcpkg.HookListVPCStaticHosts = func(switchID uint) ([]vpcpkg.StaticHost, error) {
		hosts, err := ovspkg.ListVPCStaticHosts(switchID)
		if err != nil {
			return nil, err
		}
		result := make([]vpcpkg.StaticHost, len(hosts))
		for i, h := range hosts {
			result[i] = vpcpkg.StaticHost{VMName: h.VMName, MAC: h.MAC, IP: h.IP}
		}
		return result, nil
	}
	vpcpkg.HookListVPCDHCPLeases = func(switchID uint) ([]vpcpkg.DHCPLease, error) {
		leases, err := ovspkg.ListVPCDHCPLeasesForSwitch(switchID)
		if err != nil {
			return nil, err
		}
		result := make([]vpcpkg.DHCPLease, len(leases))
		for i, l := range leases {
			result[i] = vpcpkg.DHCPLease{
				ExpiryTime: l.ExpiryTime,
				ExpiryUnix: l.ExpiryUnix,
				MAC:        l.MAC,
				IP:         l.IP,
				Hostname:   l.Hostname,
				ClientID:   l.ClientID,
			}
		}
		return result, nil
	}
	vpcpkg.HookCleanOVSDHCPLease = ovspkg.CleanOVSDHCPLease
	vpcpkg.HookCleanAllVPCDHCPLeases = ovspkg.CleanAllVPCDHCPLeases

	// ── VM / User hooks ──
	vpcpkg.HookFindVMOwner = FindVMOwner
	vpcpkg.HookIsLightweightCloudUser = IsLightweightCloudUser
	vpcpkg.HookGetUserVMList = GetUserVMList
	vpcpkg.HookUserOwnsVM = UserOwnsVM
	vpcpkg.HookGetLightweightVMQuota = GetLightweightVMQuota
	vpcpkg.HookClearVMBandwidth = ClearVMBandwidth
	vpcpkg.HookListAllVMNames = fwpkg.ListAllVMNames
	vpcpkg.HookGetFirewallVMIP = getFirewallVMIP
	vpcpkg.HookPublicIPNATPrivateIPsForVM = PublicIPNATPrivateIPsForVM
	vpcpkg.HookGetVMMACByOrder = GetVMMACByOrder
	vpcpkg.HookAttachVMInterface = vmpkg.AttachVMInterface
	vpcpkg.HookDetachVMInterface = vmpkg.DetachVMInterface

	// ── Port forward / Firewall hooks ──
	vpcpkg.HookRemoveVPCPortForwardAcceptRules = RemoveVPCPortForwardAcceptRules
	vpcpkg.HookSavePortForwardRules = SavePortForwardRules
	vpcpkg.HookRemovePortForwardsForCIDR = removePortForwardsForCIDR
	vpcpkg.HookListLivePortForwards = func() ([]vpcpkg.PortForwardRuleBrief, error) {
		rules, err := listLivePortForwardsFromIPTables()
		if err != nil {
			return nil, err
		}
		result := make([]vpcpkg.PortForwardRuleBrief, len(rules))
		for i, r := range rules {
			result[i] = vpcpkg.PortForwardRuleBrief{
				DestIP:   r.DestIP,
				Protocol: r.Protocol,
				DestPort: r.DestPort,
			}
		}
		return result, nil
	}
	vpcpkg.HookCleanupOVSStaticHostsForVMs = cleanupOVSStaticHostsForVMs

	// ── Traffic / Bandwidth hooks ──
	vpcpkg.HookGetGlobalEffectiveBandwidth = getGlobalEffectiveBandwidth

	// ── Utility hooks ──
	vpcpkg.HookFirstNonEmpty = FirstNonEmpty
}
