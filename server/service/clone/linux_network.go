package clone

import (
	"crypto/rand"
	"fmt"
	"strings"
)

// GenerateClonePrimaryMAC 生成 QEMU 本地管理 MAC，用于离线网络配置与最终域 XML。
func GenerateClonePrimaryMAC() (string, error) {
	var suffix [3]byte
	if _, err := rand.Read(suffix[:]); err != nil {
		return "", err
	}
	return fmt.Sprintf("52:54:00:%02x:%02x:%02x", suffix[0], suffix[1], suffix[2]), nil
}

// buildLinuxNetplanMACCompatCommand 仅替换首个固定 MAC，保留模板既有 DHCP、静态地址与路由配置。
func buildLinuxNetplanMACCompatCommand(mac string) string {
	mac = strings.ToLower(strings.TrimSpace(mac))
	if mac == "" {
		return ""
	}
	return fmt.Sprintf(`if [ -d /etc/netplan ]; then for qvm_netplan in /etc/netplan/*.yaml /etc/netplan/*.yml; do [ -f "$qvm_netplan" ] || continue; if grep -qE '^[[:space:]]*macaddress:' "$qvm_netplan"; then sed -E -i '0,/^([[:space:]]*)macaddress:.*/s//\1macaddress: "%s"/' "$qvm_netplan"; break; fi; done; fi`, mac)
}

// buildLinuxNetplanDHCPHotplugCompatCommand 使仅包含固定 MAC DHCP 配置的模板
// 在无主网口创建后仍能识别后续添加的网口。静态网络模板保持原状，避免改变其路由语义。
func buildLinuxNetplanDHCPHotplugCompatCommand() string {
	return `if [ -d /etc/netplan ]; then for qvm_netplan in /etc/netplan/*.yaml /etc/netplan/*.yml; do [ -f "$qvm_netplan" ] || continue; if grep -qE '^[[:space:]]*macaddress:' "$qvm_netplan" && grep -qE '^[[:space:]]*dhcp4:[[:space:]]*true[[:space:]]*$' "$qvm_netplan"; then sed -E -i '0,/^([[:space:]]*)macaddress:.*/s//\1name: "en*"/' "$qvm_netplan"; sed -E -i '/^[[:space:]]*set-name:[[:space:]]*[^[:space:]]+[[:space:]]*$/d' "$qvm_netplan"; break; fi; done; fi`
}

// buildLinuxNetworkdDHCPHotplugFallbackCommand 写入附加网口的 DHCP 兜底配置。
// Netplan 为主网口生成的 10-* 规则优先匹配；仅未被主规则匹配的 en* 网口会命中此规则。
func buildLinuxNetworkdDHCPHotplugFallbackCommand() string {
	return `mkdir -p /etc/systemd/network && cat > /etc/systemd/network/99-qvm-hotplug.network <<'EOF'
[Match]
Name=en*

[Network]
DHCP=yes
LinkLocalAddressing=ipv6

[DHCP]
RouteMetric=200
UseMTU=true
EOF`
}
