package bridge

import (
	"fmt"
	"os"
	"strings"

	"kvm_console/logger"
	"kvm_console/utils"
)

// networkdOverridePath 返回 networkd override 文件路径
func networkdOverridePath(iface string) string {
	return fmt.Sprintf("/etc/systemd/network/01-kvm-console-%s.network", iface)
}

// disableNetworkdDHCPForPort 为已加入 OVS bridge 的物理网卡写入 networkd 覆盖配置，
// 禁用 DHCP 以防止 systemd-networkd 周期性发送 DHCP Discover 干扰 OVS 数据通道导致丢包。
func disableNetworkdDHCPForPort(iface string) {
	iface = strings.TrimSpace(iface)
	if iface == "" {
		return
	}
	// 仅当 systemd-networkd 在运行时处理（探测类命令，失败仅记 DEBUG）
	if utils.ExecCommandQuiet("systemctl", "is-active", "--quiet", "systemd-networkd").Error != nil {
		return
	}
	content := fmt.Sprintf(`[Match]
Name=%s

[Link]
Unmanaged=yes

[Network]
DHCP=no
LinkLocalAddressing=no
`, iface)
	path := networkdOverridePath(iface)
	changed, err := HookWriteFileIfChanged(path, []byte(content), 0644)
	if err != nil {
		logger.App.Warn("写入 networkd 覆盖配置失败", "iface", iface, "error", err)
		return
	}
	if changed {
		utils.ExecCommand("networkctl", "reload")
		logger.App.Info("已禁用 networkd 对 OVS 端口的 DHCP 管理", "iface", iface)
	}
}

// removeNetworkdDHCPOverrideForPort 删除物理网卡从 OVS bridge 移除后不再需要的 networkd 覆盖配置。
func removeNetworkdDHCPOverrideForPort(iface string) {
	iface = strings.TrimSpace(iface)
	if iface == "" {
		return
	}
	path := networkdOverridePath(iface)
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return
	}
	if err := os.Remove(path); err != nil {
		logger.App.Warn("删除 networkd 覆盖配置失败", "iface", iface, "error", err)
		return
	}
	if utils.ExecCommandQuiet("systemctl", "is-active", "--quiet", "systemd-networkd").Error == nil {
		utils.ExecCommand("networkctl", "reload")
		logger.App.Info("已恢复 networkd 对端口的管理", "iface", iface)
	}
}
