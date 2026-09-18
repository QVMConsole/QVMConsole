package ovs

import (
	"fmt"
	"net"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	vpcpkg "kvm_console/service/network/vpc"
)

// ListOVSStaticHosts reads and parses the OVS DHCP static hosts file.
func ListOVSStaticHosts() ([]OVSStaticHost, error) {
	data, err := os.ReadFile(OVSDHCPHostsFile)
	if err != nil {
		if os.IsNotExist(err) {
			return []OVSStaticHost{}, nil
		}
		return nil, err
	}
	return ParseOVSStaticHostsText(string(data)), nil
}

// ParseOVSStaticHostsText parses the text content of a DHCP static hosts file.
func ParseOVSStaticHostsText(text string) []OVSStaticHost {
	var hosts []OVSStaticHost
	for _, line := range strings.Split(text, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		parts := strings.Split(line, ",")
		if len(parts) < 2 {
			continue
		}
		host := OVSStaticHost{MAC: strings.ToLower(strings.TrimSpace(parts[0]))}
		for _, part := range parts[1:] {
			part = strings.TrimSpace(part)
			if net.ParseIP(part) != nil {
				host.IP = part
				continue
			}
			if !strings.HasPrefix(part, "set:") && host.VMName == "" {
				host.VMName = part
			}
		}
		if host.MAC != "" && host.IP != "" {
			hosts = append(hosts, host)
		}
	}
	return hosts
}

func writeStaticHostsFile(path string, hosts []OVSStaticHost) error {
	sort.Slice(hosts, func(i, j int) bool {
		if hosts[i].VMName == hosts[j].VMName {
			return hosts[i].MAC < hosts[j].MAC
		}
		return hosts[i].VMName < hosts[j].VMName
	})
	var lines []string
	for _, host := range hosts {
		host.MAC = strings.ToLower(strings.TrimSpace(host.MAC))
		host.IP = strings.TrimSpace(host.IP)
		host.VMName = strings.TrimSpace(host.VMName)
		if host.MAC == "" || host.IP == "" {
			continue
		}
		if host.VMName == "" {
			lines = append(lines, fmt.Sprintf("%s,%s", host.MAC, host.IP))
		} else {
			lines = append(lines, fmt.Sprintf("%s,%s,%s", host.MAC, host.IP, host.VMName))
		}
	}
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}
	return os.WriteFile(path, []byte(strings.Join(lines, "\n")+"\n"), 0644)
}

func writeOVSStaticHosts(hosts []OVSStaticHost) error {
	return writeStaticHostsFile(OVSDHCPHostsFile, hosts)
}

// WriteOVSStaticHosts exports writeOVSStaticHosts for external callers.
func WriteOVSStaticHosts(hosts []OVSStaticHost) error {
	return writeOVSStaticHosts(hosts)
}

// ListVPCStaticHosts reads and parses the VPC DHCP static hosts file for a switch.
func ListVPCStaticHosts(switchID uint) ([]OVSStaticHost, error) {
	data, err := os.ReadFile(vpcpkg.VPCDHCPHostsPath(switchID))
	if err != nil {
		if os.IsNotExist(err) {
			return []OVSStaticHost{}, nil
		}
		return nil, err
	}
	return ParseOVSStaticHostsText(string(data)), nil
}

// ListAllVPCStaticHosts reads and parses all VPC DHCP static hosts files.
func ListAllVPCStaticHosts() ([]OVSStaticHost, error) {
	files, err := filepath.Glob(filepath.Join(vpcpkg.VPCConfigDir, "dhcp-hosts-*"))
	if err != nil {
		return nil, err
	}
	var hosts []OVSStaticHost
	for _, file := range files {
		data, err := os.ReadFile(file)
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return nil, err
		}
		hosts = append(hosts, ParseOVSStaticHostsText(string(data))...)
	}
	return hosts, nil
}

// WriteVPCStaticHosts exports writeVPCStaticHosts for external callers.
func WriteVPCStaticHosts(switchID uint, hosts []OVSStaticHost) error {
	return writeStaticHostsFile(vpcpkg.VPCDHCPHostsPath(switchID), hosts)
}

// UpsertOVSStaticHost adds or updates an OVS static host binding.
func UpsertOVSStaticHost(vmName, mac, ipAddr string) error {
	if err := EnsureOVSNetworkReady(); err != nil {
		return err
	}
	mac = strings.ToLower(strings.TrimSpace(mac))
	vmName = strings.TrimSpace(vmName)
	ipAddr = strings.TrimSpace(ipAddr)
	hosts, err := ListOVSStaticHosts()
	if err != nil {
		return err
	}
	next, err := BuildOVSStaticHostsForUpsert(hosts, OVSStaticHost{VMName: vmName, MAC: mac, IP: ipAddr})
	if err != nil {
		return err
	}
	if err := writeOVSStaticHosts(next); err != nil {
		return fmt.Errorf("写入 OVS 静态 IP 绑定失败: %w", err)
	}
	CleanOVSDHCPLease(mac, ipAddr)
	ReloadOVSDNSMasq()
	return nil
}

// BuildOVSStaticHostsForUpsert builds the updated static hosts list for an upsert operation.
func BuildOVSStaticHostsForUpsert(hosts []OVSStaticHost, target OVSStaticHost) ([]OVSStaticHost, error) {
	target.MAC = strings.ToLower(strings.TrimSpace(target.MAC))
	target.IP = strings.TrimSpace(target.IP)
	target.VMName = strings.TrimSpace(target.VMName)
	if target.MAC == "" {
		return nil, fmt.Errorf("MAC 地址不能为空")
	}
	if target.IP == "" {
		return nil, fmt.Errorf("IP 地址不能为空")
	}

	var next []OVSStaticHost
	for _, host := range hosts {
		host.MAC = strings.ToLower(strings.TrimSpace(host.MAC))
		host.IP = strings.TrimSpace(host.IP)
		host.VMName = strings.TrimSpace(host.VMName)

		sameVM := target.VMName != "" && host.VMName == target.VMName
		sameMAC := strings.EqualFold(host.MAC, target.MAC)
		sameIP := host.IP == target.IP

		if sameVM {
			continue
		}
		if sameMAC {
			return nil, fmt.Errorf("MAC 地址 %s 已绑定到虚拟机 %s，不能重复绑定", target.MAC, host.VMName)
		}
		if sameIP {
			return nil, fmt.Errorf("IP 地址 %s 已绑定到虚拟机 %s（MAC: %s），不能重复绑定", target.IP, host.VMName, host.MAC)
		}
		next = append(next, host)
	}
	next = append(next, target)
	return next, nil
}

// RemoveOVSStaticHost removes an OVS static host binding.
func RemoveOVSStaticHost(vmName, mac string) (string, error) {
	hosts, err := ListOVSStaticHosts()
	if err != nil {
		return "", err
	}
	var removedIP string
	var next []OVSStaticHost
	for _, host := range hosts {
		match := strings.EqualFold(host.MAC, mac) || (vmName != "" && host.VMName == vmName)
		if match {
			removedIP = host.IP
			continue
		}
		next = append(next, host)
	}
	if removedIP == "" {
		return "", fmt.Errorf("该虚拟机没有静态绑定")
	}
	if err := writeOVSStaticHosts(next); err != nil {
		return "", fmt.Errorf("删除 OVS 静态 IP 绑定失败: %w", err)
	}
	ReloadOVSDNSMasq()
	return removedIP, nil
}

// GetOVSStaticIPByMAC returns the static IP bound to the given MAC address.
func GetOVSStaticIPByMAC(mac string) string {
	hosts, err := ListOVSStaticHosts()
	if err != nil {
		return ""
	}
	for _, host := range hosts {
		if strings.EqualFold(host.MAC, mac) {
			return host.IP
		}
	}
	return ""
}

// GetOVSStaticHostByVMName returns the static host binding for the given VM name.
func GetOVSStaticHostByVMName(vmName string) (OVSStaticHost, bool) {
	hosts, err := ListOVSStaticHosts()
	if err != nil {
		return OVSStaticHost{}, false
	}
	vmName = strings.TrimSpace(vmName)
	for _, host := range hosts {
		if strings.TrimSpace(host.VMName) == vmName {
			return host, true
		}
	}
	return OVSStaticHost{}, false
}

// NormalizeIPForOVS normalizes an IP address for OVS use.
func NormalizeIPForOVS(ipAddr string) string {
	ipAddr = strings.TrimSpace(ipAddr)
	if matched, _ := regexp.MatchString(`^\d+$`, ipAddr); matched {
		return OvsSubnetPrefix() + "." + ipAddr
	}
	return ipAddr
}
