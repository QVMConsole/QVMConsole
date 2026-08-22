package network

import (
	"fmt"
	"os"
	"path/filepath"

	"kvm_console/model"
	vpcpkg "kvm_console/service/network/vpc"
)

// deps.go — network 子包通过 hook 变量调用 service 根包函数，避免循环 import。
// service 根包在 network_register.go 的 init() 中为这些变量赋值。

// ── Equivalent types (避免直接引用 service 根包) ──

// OVSStaticHost 等价于 service.OVSStaticHost
type OVSStaticHost struct {
	VMName string
	MAC    string
	IP     string
}

// OVSDHCPLease 等价于 service.OVSDHCPLease
type OVSDHCPLease struct {
	ExpiryTime string
	ExpiryUnix int64
	MAC        string
	IP         string
	Hostname   string
	ClientID   string
}

// FirewallPolicy 镜像 service.FirewallPolicy（仅含 network 包所需字段）
type FirewallPolicy struct {
	PortForwardExemptions map[string]bool
}

// ── VPC constants (与 vpc/types.go 一致，避免循环 import) ──

const vpcConfigDir = "/etc/kvm-console/vpc"

// vpcDHCPHostsPath 返回 VPC 交换机的 DHCP 静态绑定文件路径。
func vpcDHCPHostsPath(id uint) string {
	return filepath.Join(vpcConfigDir, fmt.Sprintf("dhcp-hosts-%d", id))
}

// ── OVS / Network hooks ──

var (
	HookEnsureOVSNetworkReady        func() error
	HookListOVSStaticHosts           func() ([]OVSStaticHost, error)
	HookWriteOVSStaticHosts          func(hosts []OVSStaticHost) error
	HookReloadOVSDNSMasq             func()
	HookUseOVSNetwork                func() bool
	HookOvsSubnetPrefix              func() string
	HookUpsertOVSStaticHost          func(vmName, mac, ipAddr string) error
	HookRemoveOVSStaticHost          func(vmName, mac string) (string, error)
	HookGetOVSStaticHostByVMName     func(vmName string) (OVSStaticHost, bool)
	HookGetOVSStaticIPByMAC          func(mac string) string
	HookNormalizeIPForOVS            func(ipAddr string) string
	HookListOVSDHCPLeases            func() ([]OVSDHCPLease, error)
	HookNewerOVSDHCPLease            func(current, candidate OVSDHCPLease) OVSDHCPLease
	HookCleanOVSDHCPLease            func(mac, ipAddr string)
	HookBuildOVSInterfaceXML         func(mac, nicModel string) string
	HookBuildOVSInterfaceXMLWithVLAN func(mac, nicModel string, vlanID int) string
	HookBuildOVSStaticHostsForUpsert func(hosts []OVSStaticHost, target OVSStaticHost) ([]OVSStaticHost, error)
	HookParseOVSDHCPLeasesText       func(text string) []OVSDHCPLease
)

// ── VPC-related hooks (service root delegates to VPC package) ──

var (
	HookGetVPCSwitchForVM          func(vmName string) (*model.VPCSwitch, bool)
	HookGetVPCLeaseIPForVM         func(vmName string) string
	HookCleanVPCDHCPLease          func(switchID uint, mac, ipAddr string)
	HookListVPCStaticHosts         func(switchID uint) ([]OVSStaticHost, error)
	HookListAllVPCStaticHosts      func() ([]OVSStaticHost, error)
	HookWriteVPCStaticHosts        func(switchID uint, hosts []OVSStaticHost) error
	HookListVPCDHCPLeases          func() ([]OVSDHCPLease, error)
	HookListVPCDHCPLeasesForSwitch func(switchID uint) ([]OVSDHCPLease, error)
	HookReloadVPCDNSMasq           func(switchID uint)
)

// ── Firewall hooks ──

var (
	HookGetFirewallPolicy                 func() (*FirewallPolicy, error)
	HookSetPortForwardFirewallExemption   func(key string, exempt bool) (*FirewallPolicy, error)
	HookClearPortForwardFirewallExemption func(key string) error
	HookEnsureHostFirewallPortForwardRule func(hostPort, protocol, comment string) error
	HookDeleteHostFirewallPortForwardRule func(hostPort, protocol string) error
)

// ── VM / User hooks ──

var (
	HookGetUserVMList func(username string) []string
	HookFindVMOwner   func(vmName string) string
)

// ── Utility hooks ──

var (
	HookWriteFileIfChanged func(path string, content []byte, perm os.FileMode) (bool, error)
)

// ── VPC sub-package direct calls ──
// network 可以 import vpc（vpc 是 network 的子包，不会循环依赖）。

func isVPCManagedIP(ip string) bool {
	return vpcpkg.IsVPCManagedIP(ip)
}

func ipInCIDR(ip, cidr string) bool {
	return vpcpkg.IPInCIDR(ip, cidr)
}

func removeSecurityGroupAllowsPortForwardIfUnused(destIP, protocol, portText, sourceIP string) error {
	return vpcpkg.RemoveSecurityGroupAllowsPortForwardIfUnused(destIP, protocol, portText, sourceIP)
}

// syncSecurityGroupPortForwardRules 对账安全组自动放行规则与实时端口转发规则
func syncSecurityGroupPortForwardRules() error {
	return vpcpkg.SyncSecurityGroupPortForwardRules()
}

func applyVPCSwitchRuntime(vmName string, sw model.VPCSwitch) error {
	return vpcpkg.ApplyVPCSwitchRuntime(vmName, sw)
}
