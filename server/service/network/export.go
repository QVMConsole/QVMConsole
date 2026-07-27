package network

// export.go — export unexported functions for service root backward compatibility

// FindLivePortForwardByStableKey exports findLivePortForwardByStableKey
func FindLivePortForwardByStableKey(ruleKey string) (*PortForwardRule, error) {
	return findLivePortForwardByStableKey(ruleKey)
}

// GetHostIP exports getHostIP
func GetHostIP() string {
	return getHostIP()
}

// BuildPortForwardAccessAddress exports buildPortForwardAccessAddress
func BuildPortForwardAccessAddress(hostIP, hostPort string) string {
	return buildPortForwardAccessAddress(hostIP, hostPort)
}

// RemovePortForwardsForCIDR exports removePortForwardsForCIDR
func RemovePortForwardsForCIDR(cidr string) {
	removePortForwardsForCIDR(cidr)
}

// CleanupOVSStaticHostsForVMs exports cleanupOVSStaticHostsForVMs
func CleanupOVSStaticHostsForVMs(vmNames []string) {
	cleanupOVSStaticHostsForVMs(vmNames)
}
