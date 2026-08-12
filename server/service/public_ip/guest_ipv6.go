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
	guestIPv6ApplyPath = "/usr/local/sbin/qvm-public-ipv6-apply"
	guestIPv6UnitPath  = "/etc/systemd/system/qvm-public-ipv6.service"
)

type guestIPv6Desired struct {
	BindingIDs []uint
	Addresses  []string
	Gateway    string
	MAC        string
}

var guestIPv6AppliedHashes sync.Map

// reconcilePublicIPv6GuestVMs 将路由型公网 IPv6 同步到运行中的来宾系统。
// QGA 暂不可用时保留 pending 状态，由前缀监控器继续重试。
func reconcilePublicIPv6GuestVMs(ctx context.Context, vmNames []string, force bool) {
	seen := map[string]bool{}
	for _, vmName := range vmNames {
		vmName = strings.TrimSpace(vmName)
		if vmName == "" || seen[vmName] {
			continue
		}
		seen[vmName] = true
		if err := reconcilePublicIPv6Guest(ctx, vmName, force); err != nil {
			logger.App.Warn("同步来宾公网 IPv6 配置待重试", "vm", vmName, "error", err)
		}
	}
}

// ReconcilePendingPublicIPv6Guests 重试仍有待下发状态的绑定。
func ReconcilePendingPublicIPv6Guests(ctx context.Context) {
	if model.DB == nil {
		return
	}
	var bindings []model.PublicIPBinding
	model.DB.Where("mode = ? AND public_ip LIKE ?", PublicIPModeClassicRoute, "%:%").Find(&bindings)
	vmNames := make([]string, 0, len(bindings))
	for _, binding := range bindings {
		vmNames = append(vmNames, binding.VMName)
	}
	reconcilePublicIPv6GuestVMs(ctx, vmNames, false)
}

func reconcilePublicIPv6Guest(ctx context.Context, vmName string, force bool) error {
	desired, err := loadGuestIPv6Desired(vmName)
	if err != nil {
		return err
	}
	hash := fmt.Sprintf("%x", sha256.Sum256([]byte(strings.Join(desired.Addresses, ",")+"|"+desired.Gateway+"|"+desired.MAC)))
	if !force {
		if value, ok := guestIPv6AppliedHashes.Load(vmName); ok && value == hash {
			return nil
		}
	}

	state, stateErr := libvirt_rpc.GetDomainStateRPC(vmName)
	if stateErr != nil || !strings.EqualFold(strings.TrimSpace(state), "running") {
		updateGuestIPv6Status(desired.BindingIDs, "pending", "虚拟机启动且 Guest Agent 就绪后将自动配置 IPv6")
		if stateErr != nil {
			return fmt.Errorf("读取虚拟机状态失败: %w", stateErr)
		}
		return nil
	}

	client := guest_agent.NewClient(vmName)
	if err := client.Ping(ctx); err != nil {
		updateGuestIPv6Status(desired.BindingIDs, "pending", "等待 QEMU Guest Agent 连接后自动配置 IPv6")
		return nil
	}
	if !client.Supports(ctx, "guest-exec") || !client.Supports(ctx, "guest-exec-status") {
		updateGuestIPv6Status(desired.BindingIDs, "manual", "Guest Agent 未开放命令执行，请按配置提示手动设置 IPv6")
		return nil
	}
	osInfo, err := client.OSInfo(ctx)
	if err != nil {
		updateGuestIPv6Status(desired.BindingIDs, "pending", "等待 Guest Agent 返回来宾系统类型")
		return fmt.Errorf("读取来宾系统类型失败: %w", err)
	}
	if strings.Contains(strings.ToLower(osInfo.ID+" "+osInfo.Name), "windows") {
		updateGuestIPv6Status(desired.BindingIDs, "manual", "Windows 来宾请按配置提示设置 IPv6 地址与默认网关")
		return nil
	}

	applyScript := buildLinuxGuestIPv6ApplyScript(desired)
	command := buildLinuxGuestIPv6InstallCommand(applyScript, len(desired.Addresses) > 0)
	result, err := client.Execute(ctx, "/bin/sh", []string{"-c", command}, 0)
	if err != nil {
		updateGuestIPv6Status(desired.BindingIDs, "failed", compactGuestIPv6Message(err.Error()))
		return err
	}
	if result.ExitCode != 0 {
		message := strings.TrimSpace(result.Stderr)
		if message == "" {
			message = strings.TrimSpace(result.Stdout)
		}
		if message == "" {
			message = fmt.Sprintf("来宾 IPv6 配置命令退出码 %d", result.ExitCode)
		}
		updateGuestIPv6Status(desired.BindingIDs, "failed", compactGuestIPv6Message(message))
		return fmt.Errorf("来宾 IPv6 配置失败: %s", message)
	}
	guestIPv6AppliedHashes.Store(vmName, hash)
	if len(desired.Addresses) == 0 {
		guestIPv6AppliedHashes.Delete(vmName)
		return nil
	}
	updateGuestIPv6Status(desired.BindingIDs, "applied", "已通过 QEMU Guest Agent 配置并持久化")
	return nil
}

func loadGuestIPv6Desired(vmName string) (guestIPv6Desired, error) {
	desired := guestIPv6Desired{}
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
		if err != nil || address.Is4() || seen[address.String()] {
			continue
		}
		seen[address.String()] = true
		desired.Addresses = append(desired.Addresses, address.String())
		desired.BindingIDs = append(desired.BindingIDs, binding.ID)
	}
	sort.Strings(desired.Addresses)
	if len(desired.Addresses) > 0 {
		desired.Gateway = publicIPv6GatewayLinkLocal(vmName)
		if desired.Gateway == "" {
			return desired, fmt.Errorf("未解析到宿主机 IPv6 链路本地网关")
		}
	}
	for _, iface := range HookParseVirshDomiflistOutput(utils.ExecCommand("virsh", "domiflist", vmName).Stdout) {
		if strings.TrimSpace(iface.MAC) != "" {
			desired.MAC = strings.ToLower(strings.TrimSpace(iface.MAC))
			break
		}
	}
	return desired, nil
}

func buildLinuxGuestIPv6ApplyScript(desired guestIPv6Desired) string {
	var addresses strings.Builder
	for _, address := range desired.Addresses {
		addresses.WriteString(address)
		addresses.WriteByte('\n')
	}
	return fmt.Sprintf(`#!/bin/sh
set -eu
STATE_DIR=/var/lib/qvm-console
STATE_FILE="$STATE_DIR/public-ipv6-addresses"
GATEWAY_FILE="$STATE_DIR/public-ipv6-gateway"
MAC=%s
DESIRED=%s
GATEWAY=%s
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
if [ -f "$STATE_FILE" ]; then
  while IFS= read -r OLD_ADDRESS; do
    [ -n "$OLD_ADDRESS" ] || continue
    ip -6 addr del "$OLD_ADDRESS/128" dev "$IFACE" 2>/dev/null || true
  done < "$STATE_FILE"
fi
if [ -f "$GATEWAY_FILE" ]; then
  OLD_GATEWAY="$(cat "$GATEWAY_FILE")"
  [ -z "$OLD_GATEWAY" ] || ip -6 route del default via "$OLD_GATEWAY" dev "$IFACE" 2>/dev/null || true
fi
: > "$STATE_FILE"
for ADDRESS in $DESIRED; do
  ip -6 addr replace "$ADDRESS/128" dev "$IFACE"
  printf '%%s\n' "$ADDRESS" >> "$STATE_FILE"
done
if [ -n "$GATEWAY" ] && [ -s "$STATE_FILE" ]; then
  ip -6 route replace default via "$GATEWAY" dev "$IFACE" onlink metric 1024
  printf '%%s\n' "$GATEWAY" > "$GATEWAY_FILE"
else
  rm -f "$GATEWAY_FILE"
fi
`, shellGuestIPv6Value(desired.MAC), shellGuestIPv6Value(strings.TrimSpace(addresses.String())), shellGuestIPv6Value(desired.Gateway))
}

func buildLinuxGuestIPv6InstallCommand(applyScript string, enabled bool) string {
	encodedScript := base64.StdEncoding.EncodeToString([]byte(applyScript))
	unit := `[Unit]
Description=QVM Console public IPv6 configuration
After=network-online.target
Wants=network-online.target

[Service]
Type=oneshot
ExecStart=/usr/local/sbin/qvm-public-ipv6-apply
RemainAfterExit=yes

[Install]
WantedBy=multi-user.target
`
	encodedUnit := base64.StdEncoding.EncodeToString([]byte(unit))
	command := fmt.Sprintf("printf '%%s' '%s' | base64 -d > %s\nprintf '%%s' '%s' | base64 -d > %s\nchmod 0755 %s\n%s\n",
		encodedScript, guestIPv6ApplyPath, encodedUnit, guestIPv6UnitPath, guestIPv6ApplyPath, guestIPv6ApplyPath)
	if enabled {
		command += "if command -v systemctl >/dev/null 2>&1; then systemctl daemon-reload; systemctl enable --now qvm-public-ipv6.service >/dev/null 2>&1 || true; fi\n"
	} else {
		command += "if command -v systemctl >/dev/null 2>&1; then systemctl disable qvm-public-ipv6.service >/dev/null 2>&1 || true; systemctl daemon-reload; fi\n"
		command += fmt.Sprintf("rm -f %s %s\n", guestIPv6ApplyPath, guestIPv6UnitPath)
	}
	return command
}

func shellGuestIPv6Value(value string) string {
	return "'" + strings.ReplaceAll(value, "'", "'\"'\"'") + "'"
}

func updateGuestIPv6Status(bindingIDs []uint, status, message string) {
	if model.DB == nil || len(bindingIDs) == 0 {
		return
	}
	model.DB.Model(&model.PublicIPBinding{}).Where("id IN ?", bindingIDs).Updates(map[string]interface{}{
		"guest_ipv6_status":  status,
		"guest_ipv6_message": compactGuestIPv6Message(message),
	})
}

func compactGuestIPv6Message(value string) string {
	value = strings.TrimSpace(value)
	if len([]rune(value)) <= 120 {
		return value
	}
	return string([]rune(value)[:120])
}
