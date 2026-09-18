package ovs

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"kvm_console/model"
	"kvm_console/service/ip_resolver"
	netpkg "kvm_console/service/network"
	vpcpkg "kvm_console/service/network/vpc"
)

// ListOVSDHCPLeases reads and parses the OVS DHCP leases file.
func ListOVSDHCPLeases() ([]OVSDHCPLease, error) {
	data, err := os.ReadFile(OVSLeasesFile)
	if err != nil {
		if os.IsNotExist(err) {
			return []OVSDHCPLease{}, nil
		}
		return nil, err
	}
	return ParseOVSDHCPLeasesText(string(data)), nil
}

// ListVPCDHCPLeases reads and parses all VPC DHCP leases files.
func ListVPCDHCPLeases() ([]OVSDHCPLease, error) {
	files, err := filepath.Glob(filepath.Join(vpcpkg.VPCConfigDir, "leases-*"))
	if err != nil {
		return nil, err
	}
	var leases []OVSDHCPLease
	for _, file := range files {
		data, err := os.ReadFile(file)
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return nil, err
		}
		leases = append(leases, ParseOVSDHCPLeasesText(string(data))...)
	}
	return leases, nil
}

// ListVPCDHCPLeasesForSwitch reads and parses the DHCP leases file for a specific VPC switch.
func ListVPCDHCPLeasesForSwitch(switchID uint) ([]OVSDHCPLease, error) {
	data, err := os.ReadFile(filepath.Join(vpcpkg.VPCConfigDir, fmt.Sprintf("leases-%d", switchID)))
	if err != nil {
		if os.IsNotExist(err) {
			return []OVSDHCPLease{}, nil
		}
		return nil, err
	}
	return ParseOVSDHCPLeasesText(string(data)), nil
}

// ParseOVSDHCPLeasesText parses the text content of a DHCP leases file.
func ParseOVSDHCPLeasesText(text string) []OVSDHCPLease {
	var leases []OVSDHCPLease
	for _, line := range strings.Split(text, "\n") {
		fields := strings.Fields(line)
		if len(fields) < 3 {
			continue
		}
		lease := OVSDHCPLease{
			ExpiryTime: formatOVSLeaseExpiry(fields[0]),
			ExpiryUnix: parseOVSLeaseExpiryUnix(fields[0]),
			MAC:        strings.ToLower(fields[1]),
			IP:         fields[2],
		}
		if len(fields) >= 4 && fields[3] != "*" {
			lease.Hostname = fields[3]
		}
		if len(fields) >= 5 && fields[4] != "*" {
			lease.ClientID = fields[4]
		}
		leases = append(leases, lease)
	}
	return leases
}

func parseOVSLeaseExpiryUnix(raw string) int64 {
	sec, err := strconv.ParseInt(strings.TrimSpace(raw), 10, 64)
	if err != nil {
		return 0
	}
	return sec
}

func formatOVSLeaseExpiry(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	sec, err := strconv.ParseInt(raw, 10, 64)
	if err != nil {
		return raw
	}
	if sec == 0 {
		return "永久"
	}
	return time.Unix(sec, 0).Local().Format("2006-01-02 15:04:05")
}

// NewerOVSDHCPLease returns the lease with the later expiry time.
func NewerOVSDHCPLease(current, candidate OVSDHCPLease) OVSDHCPLease {
	if current.IP == "" {
		return candidate
	}
	currentExpiry := current.ExpiryUnix
	candidateExpiry := candidate.ExpiryUnix
	if currentExpiry == 0 {
		currentExpiry = 1<<63 - 1
	}
	if candidateExpiry == 0 {
		candidateExpiry = 1<<63 - 1
	}
	if candidateExpiry >= currentExpiry {
		return candidate
	}
	return current
}

// CleanOVSDHCPLease removes DHCP lease entries matching the given MAC or IP.
func CleanOVSDHCPLease(mac, ipAddr string) {
	data, err := os.ReadFile(OVSLeasesFile)
	if err != nil {
		return
	}
	mac = strings.ToLower(strings.TrimSpace(mac))
	var lines []string
	for _, line := range strings.Split(string(data), "\n") {
		fields := strings.Fields(line)
		if len(fields) >= 3 {
			if (mac != "" && strings.EqualFold(fields[1], mac)) || (ipAddr != "" && fields[2] == ipAddr) {
				continue
			}
		}
		if strings.TrimSpace(line) != "" {
			lines = append(lines, line)
		}
	}
	_ = os.WriteFile(OVSLeasesFile, []byte(strings.Join(lines, "\n")+"\n"), 0644)
}

// CleanVPCDHCPLease removes DHCP lease entries from a specific VPC switch.
func CleanVPCDHCPLease(switchID uint, mac, ipAddr string) {
	path := filepath.Join(vpcpkg.VPCConfigDir, fmt.Sprintf("leases-%d", switchID))
	cleanVPCDHCPLeaseFile(path, mac, ipAddr)
}

// CleanAllVPCDHCPLeases removes DHCP lease entries from all VPC switches matching the given MAC or IP.
func CleanAllVPCDHCPLeases(mac, ipAddr string) {
	entries, err := os.ReadDir(vpcpkg.VPCConfigDir)
	if err != nil {
		return
	}
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasPrefix(entry.Name(), "leases-") {
			continue
		}
		cleanVPCDHCPLeaseFile(filepath.Join(vpcpkg.VPCConfigDir, entry.Name()), mac, ipAddr)
	}
}

func cleanVPCDHCPLeaseFile(path, mac, ipAddr string) {
	data, err := os.ReadFile(path)
	if err != nil {
		return
	}
	mac = strings.ToLower(strings.TrimSpace(mac))
	var lines []string
	for _, line := range strings.Split(string(data), "\n") {
		fields := strings.Fields(line)
		if len(fields) >= 3 {
			if (mac != "" && strings.EqualFold(fields[1], mac)) || (ipAddr != "" && fields[2] == ipAddr) {
				continue
			}
		}
		if strings.TrimSpace(line) != "" {
			lines = append(lines, line)
		}
	}
	_ = os.WriteFile(path, []byte(strings.Join(lines, "\n")+"\n"), 0644)
}

// GetOVSLeaseIPByMAC finds the latest DHCP lease IP for the given MAC across OVS and VPC.
func GetOVSLeaseIPByMAC(mac string) string {
	leases, err := ListOVSDHCPLeases()
	if err != nil {
		leases = []OVSDHCPLease{}
	}
	if vpcLeases, vpcErr := ListVPCDHCPLeases(); vpcErr == nil {
		leases = append(leases, vpcLeases...)
	}
	var latest OVSDHCPLease
	for _, lease := range leases {
		if strings.EqualFold(lease.MAC, mac) {
			latest = NewerOVSDHCPLease(latest, lease)
		}
	}
	return latest.IP
}

// GetVPCLeaseIPForVMByMAC finds the VPC DHCP lease IP for a VM by MAC address (multi-NIC scenario).
func GetVPCLeaseIPForVMByMAC(vmName, mac string) string {
	vmName = strings.TrimSpace(vmName)
	mac = strings.ToLower(strings.TrimSpace(mac))
	if vmName == "" || mac == "" || model.DB == nil {
		return ""
	}
	var bindings []model.VPCVMBinding
	if err := model.DB.Where("vm_name = ?", vmName).Order("interface_order ASC").Find(&bindings).Error; err != nil || len(bindings) == 0 {
		return ""
	}
	for _, binding := range bindings {
		if ip := netpkg.GetVPCStaticIPByMAC(binding.SwitchID, mac); ip != "" {
			return ip
		}
		leasesPath := filepath.Join(vpcpkg.VPCConfigDir, fmt.Sprintf("leases-%d", binding.SwitchID))
		data, err := os.ReadFile(leasesPath)
		if err != nil {
			continue
		}
		leases := ParseOVSDHCPLeasesText(string(data))
		var latest OVSDHCPLease
		for _, lease := range leases {
			if strings.EqualFold(lease.MAC, mac) {
				latest = NewerOVSDHCPLease(latest, lease)
			}
		}
		if latest.IP != "" {
			return latest.IP
		}
	}
	return ""
}

// GetVPCLeaseIPForVM finds the VPC DHCP lease IP for a VM (first interface).
func GetVPCLeaseIPForVM(vmName string) string {
	vmName = strings.TrimSpace(vmName)
	if vmName == "" || model.DB == nil {
		return ""
	}
	var binding model.VPCVMBinding
	if err := model.DB.Where("vm_name = ?", vmName).First(&binding).Error; err != nil {
		return ""
	}
	mac := ip_resolver.GetFirstVMMAC(vmName)
	if mac == "" {
		return ""
	}
	if ip := netpkg.GetVPCStaticIPByMAC(binding.SwitchID, mac); ip != "" {
		return ip
	}
	data, err := os.ReadFile(filepath.Join(vpcpkg.VPCConfigDir, fmt.Sprintf("leases-%d", binding.SwitchID)))
	if err != nil {
		return ""
	}
	leases := ParseOVSDHCPLeasesText(string(data))
	var latest OVSDHCPLease
	for _, lease := range leases {
		if strings.EqualFold(lease.MAC, mac) {
			latest = NewerOVSDHCPLease(latest, lease)
		}
	}
	return latest.IP
}
