package public_ip

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"net/netip"
	"sort"
	"strings"
	"sync"

	"kvm_console/logger"
	"kvm_console/model"
	"kvm_console/service/guest_agent"
	"kvm_console/service/libvirt_rpc"
	"kvm_console/utils"
)

const (
	guestIPv4ApplyPath = "/usr/local/sbin/qvm-public-ipv4-apply"
	guestIPv4UnitPath  = "/etc/systemd/system/qvm-public-ipv4.service"
)

type guestIPv4Desired struct {
	Addresses []string
	MAC       string
}

var guestIPv4AppliedHashes sync.Map

// reconcilePublicIPv4GuestVMs 将路由型公网 IPv4 同步到运行中的 Linux 来宾。
// 来宾保留私网 DHCP 地址作为宿主机下一跳，公网地址仅以 /32 附加到主网卡。
func reconcilePublicIPv4GuestVMs(ctx context.Context, vmNames []string, force bool) {
	seen := map[string]bool{}
	for _, vmName := range vmNames {
		vmName = strings.TrimSpace(vmName)
		if vmName == "" || seen[vmName] {
			continue
		}
		seen[vmName] = true
		if err := reconcilePublicIPv4Guest(ctx, vmName, force); err != nil {
			logger.App.Warn("同步来宾公网 IPv4 配置待重试", "vm", vmName, "error", err)
		}
	}
}

// ReconcilePendingPublicIPv4Guests 重试已绑定路由型公网 IPv4 的来宾同步。
func ReconcilePendingPublicIPv4Guests(ctx context.Context) {
	if model.DB == nil {
		return
	}
	var bindings []model.PublicIPBinding
	model.DB.Where("mode = ? AND public_ip NOT LIKE ?", PublicIPModeClassicRoute, "%:%").Find(&bindings)
	vmNames := make([]string, 0, len(bindings))
	for _, binding := range bindings {
		vmNames = append(vmNames, binding.VMName)
	}
	reconcilePublicIPv4GuestVMs(ctx, vmNames, false)
}

func reconcilePublicIPv4Guest(ctx context.Context, vmName string, force bool) error {
	desired, err := loadGuestIPv4Desired(vmName)
	if err != nil {
		return err
	}
	hash := fmt.Sprintf("%x", sha256.Sum256([]byte(strings.Join(desired.Addresses, ",")+"|"+desired.MAC)))
	if !force {
		if value, ok := guestIPv4AppliedHashes.Load(vmName); ok && value == hash {
			return nil
		}
	}

	state, stateErr := libvirt_rpc.GetDomainStateRPC(vmName)
	if stateErr != nil {
		return fmt.Errorf("读取虚拟机状态失败: %w", stateErr)
	}
	if !strings.EqualFold(strings.TrimSpace(state), "running") {
		return nil
	}

	client := guest_agent.NewClient(vmName)
	if err := client.Ping(ctx); err != nil {
		return nil
	}
	if !client.Supports(ctx, "guest-exec") || !client.Supports(ctx, "guest-exec-status") {
		return nil
	}
	osInfo, err := client.OSInfo(ctx)
	if err != nil {
		return fmt.Errorf("读取来宾系统类型失败: %w", err)
	}
	if strings.Contains(strings.ToLower(osInfo.ID+" "+osInfo.Name), "windows") {
		return nil
	}

	command := buildLinuxGuestIPv4InstallCommand(buildLinuxGuestIPv4ApplyScript(desired), len(desired.Addresses) > 0)
	result, err := client.Execute(ctx, "/bin/sh", []string{"-c", command}, 0)
	if err != nil {
		return err
	}
	if result.ExitCode != 0 {
		message := strings.TrimSpace(result.Stderr)
		if message == "" {
			message = strings.TrimSpace(result.Stdout)
		}
		return fmt.Errorf("来宾 IPv4 配置失败: %s", message)
	}
	if len(desired.Addresses) == 0 {
		guestIPv4AppliedHashes.Delete(vmName)
		return nil
	}
	guestIPv4AppliedHashes.Store(vmName, hash)
	return nil
}

func loadGuestIPv4Desired(vmName string) (guestIPv4Desired, error) {
	desired := guestIPv4Desired{}
	if model.DB == nil {
		return desired, fmt.Errorf("数据库尚未初始化")
	}
	var bindings []model.PublicIPBinding
	if err := model.DB.Where("vm_name = ? AND mode = ?", vmName, PublicIPModeClassicRoute).Order("public_ip ASC").Find(&bindings).Error; err != nil {
		return desired, err
	}
	seen := map[string]bool{}
	for _, binding := range bindings {
		address, err := netip.ParseAddr(strings.TrimSpace(binding.PublicIP))
		if err != nil || !address.Is4() || seen[address.String()] {
			continue
		}
		seen[address.String()] = true
		desired.Addresses = append(desired.Addresses, address.String())
	}
	sort.Strings(desired.Addresses)
	for _, iface := range HookParseVirshDomiflistOutput(utils.ExecCommand("virsh", "domiflist", vmName).Stdout) {
		if strings.TrimSpace(iface.MAC) != "" {
			desired.MAC = strings.ToLower(strings.TrimSpace(iface.MAC))
			break
		}
	}
	return desired, nil
}

func buildLinuxGuestIPv4ApplyScript(desired guestIPv4Desired) string {
	return fmt.Sprintf(`#!/bin/sh
set -eu
STATE_DIR=/var/lib/qvm-console
STATE_FILE="$STATE_DIR/public-ipv4-addresses"
MAC=%s
DESIRED=%s
mkdir -p "$STATE_DIR"
IFACE=""
if [ -n "$MAC" ]; then
  for ITEM in /sys/class/net/*; do
    [ -f "$ITEM/address" ] || continue
    [ "$(tr 'A-F' 'a-f' < "$ITEM/address")" = "$MAC" ] || continue
    IFACE="${ITEM##*/}"
    break
  done
fi
if [ -z "$IFACE" ]; then
  IFACE="$(ip -4 route show default | awk '{print $5; exit}')"
fi
if [ -z "$IFACE" ]; then
  echo "未找到来宾主网卡" >&2
  exit 1
fi
DEFAULT_LINE="$(ip -4 route show default dev "$IFACE" | head -n1)"
GATEWAY="$(printf '%%s\n' "$DEFAULT_LINE" | awk '{for (i=1;i<=NF;i++) if ($i=="via") {print $(i+1); exit}}')"
METRIC="$(printf '%%s\n' "$DEFAULT_LINE" | awk '{for (i=1;i<=NF;i++) if ($i=="metric") {print $(i+1); exit}}')"
[ -n "$GATEWAY" ] || { echo "未找到来宾默认网关" >&2; exit 1; }
[ -n "$METRIC" ] || METRIC=200
if [ -f "$STATE_FILE" ]; then
  while IFS= read -r OLD_ADDRESS; do
    [ -n "$OLD_ADDRESS" ] || continue
    ip -4 addr del "$OLD_ADDRESS/32" dev "$IFACE" 2>/dev/null || true
  done < "$STATE_FILE"
fi
: > "$STATE_FILE"
PRIMARY=""
for ADDRESS in $DESIRED; do
  ip -4 addr replace "$ADDRESS/32" dev "$IFACE"
  printf '%%s\n' "$ADDRESS" >> "$STATE_FILE"
  [ -n "$PRIMARY" ] || PRIMARY="$ADDRESS"
done
if [ -n "$PRIMARY" ]; then
  ip -4 route replace default via "$GATEWAY" dev "$IFACE" src "$PRIMARY" metric "$METRIC"
else
  ip -4 route replace default via "$GATEWAY" dev "$IFACE" metric "$METRIC"
fi
`, shellGuestIPv6Value(desired.MAC), shellGuestIPv6Value(strings.Join(desired.Addresses, " ")))
}

func buildLinuxGuestIPv4InstallCommand(applyScript string, enabled bool) string {
	encodedScript := base64.StdEncoding.EncodeToString([]byte(applyScript))
	unit := `[Unit]
Description=QVM Console public IPv4 routed address configuration
After=network-online.target
Wants=network-online.target

[Service]
Type=oneshot
ExecStart=/usr/local/sbin/qvm-public-ipv4-apply
RemainAfterExit=yes

[Install]
WantedBy=multi-user.target
`
	encodedUnit := base64.StdEncoding.EncodeToString([]byte(unit))
	command := fmt.Sprintf("printf '%%s' '%s' | base64 -d > %s\nprintf '%%s' '%s' | base64 -d > %s\nchmod 0755 %s\n%s\n",
		encodedScript, guestIPv4ApplyPath, encodedUnit, guestIPv4UnitPath, guestIPv4ApplyPath, guestIPv4ApplyPath)
	if enabled {
		command += "if command -v systemctl >/dev/null 2>&1; then systemctl daemon-reload; systemctl enable --now qvm-public-ipv4.service >/dev/null 2>&1 || true; fi\n"
	} else {
		command += "if command -v systemctl >/dev/null 2>&1; then systemctl disable qvm-public-ipv4.service >/dev/null 2>&1 || true; systemctl daemon-reload; fi\n"
		command += fmt.Sprintf("rm -f %s %s\n", guestIPv4ApplyPath, guestIPv4UnitPath)
	}
	return command
}
