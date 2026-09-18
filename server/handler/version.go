package handler

import (
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"kvm_console/config"
	"kvm_console/service/arch"
	"kvm_console/service/ovs"
)

// GetVersion 返回系统版本信息
func GetVersion(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"code": 200,
		"data": gin.H{
			"version":    Version,
			"build_time": BuildTime,
			"site_title": config.GlobalConfig.SiteTitle,
		},
	})
}

// GetPublicSystemInfo 返回系统运行环境信息（需登录认证）
func GetPublicSystemInfo(c *gin.Context) {
	hostname, _ := os.Hostname()
	osInfo := getOSReleaseInfo()
	pkgMgr := detectPackageManager()
	ovsDep := getOVSDependencyInfo()

	c.JSON(http.StatusOK, gin.H{
		"code": 200,
		"data": gin.H{
			"go_version":          runtime.Version(),
			"os":                  runtime.GOOS,
			"distro":              getDistroName(),
			"os_id":               osInfo.ID,
			"os_id_like":          osInfo.IDLike,
			"pkg_manager":         pkgMgr,
			"arch":                arch.GetHostArchDisplayName(),
			"num_cpu":             runtime.NumCPU(),
			"hostname":            hostname,
			"num_goroutine":       runtime.NumGoroutine(),
			"kernel":              getKernelVersion(),
			"uptime":              getSystemUptime(),
			"libvirt":             getLibvirtVersion(),
			"qemu":                getQEMUVersion(),
			"qemu_spice":          CheckQEMUSPICESupport(),
			"ovs_package":         ovsDep.PackageName,
			"ovs_service":         ovsDep.ServiceName,
			"ovs_installed":       ovsDep.Installed,
			"ovs_install_command": ovsDep.InstallCommand,
		},
	})
}

// ── OS / 包管理器 / OVS 依赖检测 ──

type osReleaseInfo struct {
	ID      string
	IDLike  string
	Name    string
	Version string
}

func getOSReleaseInfo() osReleaseInfo {
	info := osReleaseInfo{}
	data, err := os.ReadFile("/etc/os-release")
	if err != nil {
		return info
	}
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "ID=") && info.ID == "" {
			info.ID = strings.Trim(strings.TrimPrefix(line, "ID="), "\"")
		}
		if strings.HasPrefix(line, "ID_LIKE=") && info.IDLike == "" {
			info.IDLike = strings.Trim(strings.TrimPrefix(line, "ID_LIKE="), "\"")
		}
		if strings.HasPrefix(line, "PRETTY_NAME=") && info.Name == "" {
			info.Name = strings.Trim(strings.TrimPrefix(line, "PRETTY_NAME="), "\"")
		}
		if strings.HasPrefix(line, "VERSION_ID=") && info.Version == "" {
			info.Version = strings.Trim(strings.TrimPrefix(line, "VERSION_ID="), "\"")
		}
	}
	return info
}

// detectPackageManager 检测系统包管理器，返回 "apt" / "dnf" / "yum" / "unknown"
func detectPackageManager() string {
	osInfo := getOSReleaseInfo()
	id := strings.ToLower(osInfo.ID)
	idLike := strings.ToLower(osInfo.IDLike)

	// Debian/Ubuntu 系列
	if id == "ubuntu" || id == "debian" || strings.Contains(idLike, "debian") || strings.Contains(idLike, "ubuntu") {
		if _, err := exec.LookPath("apt-get"); err == nil {
			return "apt"
		}
	}
	// 已知 RPM 系发行版
	rpmDistros := []string{"kylin", "neokylin", "openeuler", "centos", "rhel", "anolis", "rocky", "alma", "fedora"}
	for _, d := range rpmDistros {
		if id == d || strings.Contains(idLike, d) {
			if _, err := exec.LookPath("dnf"); err == nil {
				return "dnf"
			}
			if _, err := exec.LookPath("yum"); err == nil {
				return "yum"
			}
		}
	}
	// 通用回退
	if _, err := exec.LookPath("dnf"); err == nil {
		return "dnf"
	}
	if _, err := exec.LookPath("yum"); err == nil {
		return "yum"
	}
	if _, err := exec.LookPath("apt-get"); err == nil {
		return "apt"
	}
	return "unknown"
}

type ovsDependencyInfo struct {
	PackageName    string
	ServiceName    string
	Installed      bool
	InstallCommand string
}

func getOVSDependencyInfo() ovsDependencyInfo {
	info := ovsDependencyInfo{
		PackageName: "openvswitch-switch",
		ServiceName: ovs.DetectOpenvswitchServiceName(),
	}
	// 检查 ovs-vsctl 是否已安装
	if _, err := exec.LookPath("ovs-vsctl"); err == nil {
		info.Installed = true
	}
	// 根据包管理器生成安装命令
	pkgMgr := detectPackageManager()
	switch pkgMgr {
	case "apt":
		info.PackageName = "openvswitch-switch"
		info.InstallCommand = "sudo apt install -y openvswitch-switch"
	case "dnf":
		info.PackageName = "openvswitch"
		info.InstallCommand = "sudo dnf install -y openvswitch"
	case "yum":
		info.PackageName = "openvswitch"
		info.InstallCommand = "sudo yum install -y openvswitch"
	default:
		info.InstallCommand = "# 请根据系统包管理器手动安装 OVS"
	}
	return info
}

// ── 系统信息辅助函数 ──

func getKernelVersion() string {
	out, err := exec.Command("uname", "-r").Output()
	if err != nil {
		return "-"
	}
	return strings.TrimSpace(string(out))
}

func getSystemUptime() string {
	data, err := os.ReadFile("/proc/uptime")
	if err != nil {
		return "-"
	}
	var uptimeSeconds float64
	fmt.Sscanf(string(data), "%f", &uptimeSeconds)
	d := time.Duration(uptimeSeconds * float64(time.Second))
	days := int(d.Hours()) / 24
	hours := int(d.Hours()) % 24
	if days > 0 {
		return fmt.Sprintf("%d 天 %d 小时", days, hours)
	}
	return fmt.Sprintf("%d 小时", hours)
}

func getLibvirtVersion() string {
	out, err := exec.Command("libvirtd", "--version").Output()
	if err != nil {
		return "-"
	}
	return strings.TrimSpace(string(out))
}

func getQEMUVersion() string {
	out, err := exec.Command("qemu-system-x86_64", "--version").Output()
	if err != nil {
		out, err = exec.Command("qemu-kvm", "--version").Output()
		if err != nil {
			return "-"
		}
	}
	lines := strings.SplitN(strings.TrimSpace(string(out)), "\n", 2)
	if len(lines) > 0 {
		return lines[0]
	}
	return "-"
}

func getDistroName() string {
	data, err := os.ReadFile("/etc/os-release")
	if err != nil {
		return "-"
	}
	name := ""
	version := ""
	for _, line := range strings.Split(string(data), "\n") {
		if strings.HasPrefix(line, "PRETTY_NAME=") {
			name = strings.Trim(strings.TrimPrefix(line, "PRETTY_NAME="), "\"")
			break
		}
		if strings.HasPrefix(line, "NAME=") && name == "" {
			name = strings.Trim(strings.TrimPrefix(line, "NAME="), "\"")
		}
		if strings.HasPrefix(line, "VERSION=") && version == "" {
			version = strings.Trim(strings.TrimPrefix(line, "VERSION="), "\"")
		}
	}
	if name != "" {
		return name
	}
	if version != "" {
		return version
	}
	return "-"
}

// CheckQEMUSPICESupport 检测 QEMU 是否编译了 SPICE 支持
// 通过检查 qemu-system-x86_64 -spice help 是否成功来判断
func CheckQEMUSPICESupport() bool {
	// 方法1: 检查 -spice help
	out, err := exec.Command("qemu-system-x86_64", "-spice", "help").Output()
	if err == nil && strings.Contains(strings.ToLower(string(out)), "spice") {
		return true
	}
	// 方法2: 检查帮助信息中是否包含 spice 选项
	out, err = exec.Command("qemu-system-x86_64", "--help").Output()
	if err == nil {
		helpText := strings.ToLower(string(out))
		if strings.Contains(helpText, "-spice") {
			return true
		}
	}
	// 方法3: 检查 qemu-kvm
	out, err = exec.Command("qemu-kvm", "-spice", "help").Output()
	if err == nil && strings.Contains(strings.ToLower(string(out)), "spice") {
		return true
	}
	return false
}

// Version 通过 ldflags 在构建时注入，格式: -X kvm_console/handler.Version=v1.0.0
var Version = "dev"

// BuildTime 通过 ldflags 在构建时注入，格式: -X kvm_console/handler.BuildTime=2025-01-01T00:00:00Z
var BuildTime = ""
