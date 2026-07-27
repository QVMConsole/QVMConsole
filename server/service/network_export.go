package service

// Network export adapters - export unexported functions for network subpackage Hook injection
import (
	netpkg "kvm_console/service/network"
	ovspkg "kvm_console/service/ovs"
)

// WriteOVSStaticHostsForNetwork exports writeOVSStaticHosts for network Hook
func WriteOVSStaticHostsForNetwork(hosts []netpkg.OVSStaticHost) error {
	converted := make([]ovspkg.OVSStaticHost, len(hosts))
	for i, h := range hosts {
		converted[i] = ovspkg.OVSStaticHost{VMName: h.VMName, MAC: h.MAC, IP: h.IP}
	}
	return ovspkg.WriteOVSStaticHosts(converted)
}

// SetPortForwardFirewallExemptionForNetwork wraps SetPortForwardFirewallExemption
// converting service.FirewallPolicy to network.FirewallPolicy
// NOTE: The actual hook injection is now in firewall_register.go.
// This function is kept as a convenience wrapper for any remaining direct callers.
func SetPortForwardFirewallExemptionForNetwork(key string, exempt bool) (*netpkg.FirewallPolicy, error) {
	policy, err := SetPortForwardFirewallExemption(key, exempt)
	if err != nil {
		return nil, err
	}
	if policy == nil {
		return nil, nil
	}
	return &netpkg.FirewallPolicy{
		PortForwardExemptions: policy.PortForwardExemptions,
	}, nil
}
