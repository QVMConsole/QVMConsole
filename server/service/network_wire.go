package service

import (
	"kvm_console/model"
	netpkg "kvm_console/service/network"
	ovspkg "kvm_console/service/ovs"
)

// ── Type aliases ──

type PortForwardRule = netpkg.PortForwardRule
type PortForwardAddParams = netpkg.PortForwardAddParams
type PortForwardUpdateParams = netpkg.PortForwardUpdateParams
type PortForwardAutoAddParams = netpkg.PortForwardAutoAddParams

// init wires network package function variables to service root implementations.
// This breaks the circular dependency: network package cannot import service,
// so it exposes function variables that we set here.
func init() {
	// ── OVS / Network hooks — now delegate to ovs subpackage ──
	netpkg.HookEnsureOVSNetworkReady = ovspkg.EnsureOVSNetworkReady
	netpkg.HookListOVSStaticHosts = func() ([]netpkg.OVSStaticHost, error) {
		hosts, err := ovspkg.ListOVSStaticHosts()
		if err != nil {
			return nil, err
		}
		result := make([]netpkg.OVSStaticHost, len(hosts))
		for i, h := range hosts {
			result[i] = netpkg.OVSStaticHost{VMName: h.VMName, MAC: h.MAC, IP: h.IP}
		}
		return result, nil
	}
	netpkg.HookWriteOVSStaticHosts = WriteOVSStaticHostsForNetwork
	netpkg.HookReloadOVSDNSMasq = ovspkg.ReloadOVSDNSMasq
	netpkg.HookUseOVSNetwork = ovspkg.UseOVSNetwork
	netpkg.HookOvsSubnetPrefix = ovspkg.OvsSubnetPrefix
	netpkg.HookUpsertOVSStaticHost = ovspkg.UpsertOVSStaticHost
	netpkg.HookRemoveOVSStaticHost = ovspkg.RemoveOVSStaticHost
	netpkg.HookGetOVSStaticHostByVMName = func(vmName string) (netpkg.OVSStaticHost, bool) {
		host, ok := ovspkg.GetOVSStaticHostByVMName(vmName)
		return netpkg.OVSStaticHost{VMName: host.VMName, MAC: host.MAC, IP: host.IP}, ok
	}
	netpkg.HookGetOVSStaticIPByMAC = ovspkg.GetOVSStaticIPByMAC
	netpkg.HookNormalizeIPForOVS = ovspkg.NormalizeIPForOVS
	netpkg.HookListOVSDHCPLeases = func() ([]netpkg.OVSDHCPLease, error) {
		leases, err := ovspkg.ListOVSDHCPLeases()
		if err != nil {
			return nil, err
		}
		result := make([]netpkg.OVSDHCPLease, len(leases))
		for i, l := range leases {
			result[i] = netpkg.OVSDHCPLease{
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
	netpkg.HookNewerOVSDHCPLease = func(current, candidate netpkg.OVSDHCPLease) netpkg.OVSDHCPLease {
		a := ovspkg.OVSDHCPLease{
			ExpiryTime: current.ExpiryTime,
			ExpiryUnix: current.ExpiryUnix,
			MAC:        current.MAC,
			IP:         current.IP,
			Hostname:   current.Hostname,
			ClientID:   current.ClientID,
		}
		b := ovspkg.OVSDHCPLease{
			ExpiryTime: candidate.ExpiryTime,
			ExpiryUnix: candidate.ExpiryUnix,
			MAC:        candidate.MAC,
			IP:         candidate.IP,
			Hostname:   candidate.Hostname,
			ClientID:   candidate.ClientID,
		}
		winner := ovspkg.NewerOVSDHCPLease(a, b)
		return netpkg.OVSDHCPLease{
			ExpiryTime: winner.ExpiryTime,
			ExpiryUnix: winner.ExpiryUnix,
			MAC:        winner.MAC,
			IP:         winner.IP,
			Hostname:   winner.Hostname,
			ClientID:   winner.ClientID,
		}
	}
	netpkg.HookCleanOVSDHCPLease = ovspkg.CleanOVSDHCPLease
	netpkg.HookBuildOVSInterfaceXML = ovspkg.BuildOVSInterfaceXML
	netpkg.HookBuildOVSInterfaceXMLWithVLAN = ovspkg.BuildOVSInterfaceXMLWithVLAN
	netpkg.HookBuildOVSStaticHostsForUpsert = func(hosts []netpkg.OVSStaticHost, target netpkg.OVSStaticHost) ([]netpkg.OVSStaticHost, error) {
		converted := make([]ovspkg.OVSStaticHost, len(hosts))
		for i, h := range hosts {
			converted[i] = ovspkg.OVSStaticHost{VMName: h.VMName, MAC: h.MAC, IP: h.IP}
		}
		result, err := ovspkg.BuildOVSStaticHostsForUpsert(converted, ovspkg.OVSStaticHost{VMName: target.VMName, MAC: target.MAC, IP: target.IP})
		if err != nil {
			return nil, err
		}
		out := make([]netpkg.OVSStaticHost, len(result))
		for i, h := range result {
			out[i] = netpkg.OVSStaticHost{VMName: h.VMName, MAC: h.MAC, IP: h.IP}
		}
		return out, nil
	}
	netpkg.HookParseOVSDHCPLeasesText = func(text string) []netpkg.OVSDHCPLease {
		leases := ovspkg.ParseOVSDHCPLeasesText(text)
		result := make([]netpkg.OVSDHCPLease, len(leases))
		for i, l := range leases {
			result[i] = netpkg.OVSDHCPLease{
				ExpiryTime: l.ExpiryTime,
				ExpiryUnix: l.ExpiryUnix,
				MAC:        l.MAC,
				IP:         l.IP,
				Hostname:   l.Hostname,
				ClientID:   l.ClientID,
			}
		}
		return result
	}

	// ── VPC-related hooks ──
	netpkg.HookGetVPCSwitchForVM = func(vmName string) (*model.VPCSwitch, bool) {
		return getVPCSwitchForVM(vmName)
	}
	netpkg.HookGetVPCLeaseIPForVM = ovspkg.GetVPCLeaseIPForVM
	netpkg.HookCleanVPCDHCPLease = ovspkg.CleanVPCDHCPLease
	netpkg.HookListVPCStaticHosts = func(switchID uint) ([]netpkg.OVSStaticHost, error) {
		hosts, err := ovspkg.ListVPCStaticHosts(switchID)
		if err != nil {
			return nil, err
		}
		result := make([]netpkg.OVSStaticHost, len(hosts))
		for i, h := range hosts {
			result[i] = netpkg.OVSStaticHost{VMName: h.VMName, MAC: h.MAC, IP: h.IP}
		}
		return result, nil
	}
	netpkg.HookListAllVPCStaticHosts = func() ([]netpkg.OVSStaticHost, error) {
		hosts, err := ovspkg.ListAllVPCStaticHosts()
		if err != nil {
			return nil, err
		}
		result := make([]netpkg.OVSStaticHost, len(hosts))
		for i, h := range hosts {
			result[i] = netpkg.OVSStaticHost{VMName: h.VMName, MAC: h.MAC, IP: h.IP}
		}
		return result, nil
	}
	netpkg.HookWriteVPCStaticHosts = func(switchID uint, hosts []netpkg.OVSStaticHost) error {
		converted := make([]ovspkg.OVSStaticHost, len(hosts))
		for i, h := range hosts {
			converted[i] = ovspkg.OVSStaticHost{VMName: h.VMName, MAC: h.MAC, IP: h.IP}
		}
		return ovspkg.WriteVPCStaticHosts(switchID, converted)
	}
	netpkg.HookListVPCDHCPLeases = func() ([]netpkg.OVSDHCPLease, error) {
		leases, err := ovspkg.ListVPCDHCPLeases()
		if err != nil {
			return nil, err
		}
		result := make([]netpkg.OVSDHCPLease, len(leases))
		for i, l := range leases {
			result[i] = netpkg.OVSDHCPLease{
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
	netpkg.HookListVPCDHCPLeasesForSwitch = func(switchID uint) ([]netpkg.OVSDHCPLease, error) {
		leases, err := ovspkg.ListVPCDHCPLeasesForSwitch(switchID)
		if err != nil {
			return nil, err
		}
		result := make([]netpkg.OVSDHCPLease, len(leases))
		for i, l := range leases {
			result[i] = netpkg.OVSDHCPLease{
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
	netpkg.HookReloadVPCDNSMasq = ReloadVPCDNSMasq

	// ── Firewall hooks (now injected from firewall_wire.go) ──

	// ── VM / User hooks ──
	netpkg.HookGetUserVMList = GetUserVMList
	netpkg.HookFindVMOwner = FindVMOwner

	// ── Utility hooks ──
	netpkg.HookWriteFileIfChanged = ovspkg.WriteFileIfChanged
}

// ── Delegates ──

// listLivePortForwardsFromIPTables delegates to network.ListLivePortForwardsFromIPTables
func listLivePortForwardsFromIPTables() ([]PortForwardRule, error) {
	return netpkg.ListLivePortForwardsFromIPTables()
}

// ListPortForwards delegates to network.ListPortForwards
func ListPortForwards() ([]PortForwardRule, error) {
	return netpkg.ListPortForwards()
}

// findLivePortForwardByStableKey delegates to network.FindLivePortForwardByStableKey
func findLivePortForwardByStableKey(ruleKey string) (*PortForwardRule, error) {
	return netpkg.FindLivePortForwardByStableKey(ruleKey)
}

// AddPortForward delegates to network.AddPortForward
func AddPortForward(params *PortForwardAddParams) error {
	return netpkg.AddPortForward(params)
}

// getHostIP delegates to network.GetHostIP
func getHostIP() string {
	return netpkg.GetHostIP()
}

// buildPortForwardAccessAddress delegates to network.BuildPortForwardAccessAddress
func buildPortForwardAccessAddress(hostIP, hostPort string) string {
	return netpkg.BuildPortForwardAccessAddress(hostIP, hostPort)
}

// GetUserPortForwardUsage delegates to network.GetUserPortForwardUsage
func GetUserPortForwardUsage(username string) int {
	return netpkg.GetUserPortForwardUsage(username)
}

// RemoveVPCPortForwardAcceptRules delegates to network.RemoveVPCPortForwardAcceptRules
func RemoveVPCPortForwardAcceptRules() {
	netpkg.RemoveVPCPortForwardAcceptRules()
}

// SavePortForwardRules delegates to network.SavePortForwardRules
func SavePortForwardRules() error {
	return netpkg.SavePortForwardRules()
}

// removePortForwardsForCIDR delegates to network.RemovePortForwardsForCIDR
func removePortForwardsForCIDR(cidr string) {
	netpkg.RemovePortForwardsForCIDR(cidr)
}

// cleanupOVSStaticHostsForVMs delegates to network.CleanupOVSStaticHostsForVMs
func cleanupOVSStaticHostsForVMs(vmNames []string) {
	netpkg.CleanupOVSStaticHostsForVMs(vmNames)
}
