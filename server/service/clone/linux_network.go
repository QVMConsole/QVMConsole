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
	return fmt.Sprintf(`
if [ -d /etc/netplan ]; then
  for qvm_netplan in /etc/netplan/*.yaml /etc/netplan/*.yml; do
    [ -f "$qvm_netplan" ] || continue
    if grep -qE '^[[:space:]]*macaddress:[[:space:]]*["'"']?[0-9A-Fa-f:]+["'"']?[[:space:]]*$' "$qvm_netplan"; then
      sed -E -i '0,/^([[:space:]]*macaddress:[[:space:]]*)["'"']?[0-9A-Fa-f:]+["'"']?[[:space:]]*$/s//\1"%s"/' "$qvm_netplan"
      break
    fi
  done
fi`, mac)
}
