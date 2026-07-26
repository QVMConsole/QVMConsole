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
