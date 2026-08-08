package clone

import (
	"fmt"
	"strings"

	"kvm_console/utils"
)

// cloneFnOS FnOS 克隆逻辑（离线注入首个管理员账号和初始化状态）
func cloneFnOS(params *CloneParams, cloneDisk string, progressFn func(int, string)) error {
	if strings.TrimSpace(params.User) == "" {
		params.User = "admin"
	}
	if params.Password == "" {
		return fmt.Errorf("FnOS 模板克隆需要设置登录密码")
	}

	progressFn(27, "注入 FnOS 首次登录信息...")

	identityCommand := buildFnOSIdentityResetCommand()
	if strings.TrimSpace(params.FnOSDeviceID) != "" {
		command, err := buildFnOSCustomDeviceIDCommand(params.FnOSDeviceID)
		if err != nil {
			return err
		}
		identityCommand = command
	} else if params.PreserveFnOSDeviceID {
		identityCommand = buildFnOSIdentityPreservationCommand()
	}

	customizeArgs := []string{
		"-a", cloneDisk,
		"--no-network",
		"--run-command", buildFnOSUserProvisionCommand(params.User),
		"--run-command", buildFnOSPasswordCommand(params.User, params.Password),
		"--run-command", buildFnOSHostnameCommand(params.Hostname),
		"--run-command", buildFnOSHostsCommand(params.Hostname),
		"--run-command", "mkdir -p /usr/trim/etc && date +%s > /usr/trim/etc/system_inited_timestamp",
		"--run-command", identityCommand,
		"--run-command", "rm -f /var/lib/dhcp/*.leases 2>/dev/null || true",
		"--run-command", "rm -f /var/lib/NetworkManager/*.lease 2>/dev/null || true",
		"--run-command", "rm -f /var/lib/systemd/network/*.lease 2>/dev/null || true",
		"--run-command", "rm -rf /run/systemd/netif/leases/* 2>/dev/null || true",
		"--selinux-relabel",
		"--quiet",
	}

	// virt-customize 需要读写整个磁盘镜像，属于大 IO 操作，不设置自动超时
	result := utils.ExecCommandNoTimeout("virt-customize", customizeArgs...)
	if result.Error != nil {
		return fmt.Errorf("FnOS 首次登录信息注入失败: %s", result.Stderr)
	}

	return nil
}

func buildFnOSUserProvisionCommand(username string) string {
	quotedUser := utils.ShellSingleQuote(username)
	quotedHome := utils.ShellSingleQuote("/home/" + username)
	return fmt.Sprintf(`TARGET_USER=%s
TARGET_HOME=%s
if ! getent group Users >/dev/null 2>&1; then
  echo '缺少 Users 组' >&2
  exit 1
fi
if ! getent group Administrators >/dev/null 2>&1; then
  echo '缺少 Administrators 组' >&2
  exit 1
fi
if id -u "$TARGET_USER" >/dev/null 2>&1; then
  usermod -g Users -G Administrators -s /bin/bash -d "$TARGET_HOME" -m "$TARGET_USER"
else
  useradd -m -N -g Users -G Administrators -s /bin/bash "$TARGET_USER"
fi
passwd -u "$TARGET_USER" 2>/dev/null || true`, quotedUser, quotedHome)
}

func buildFnOSPasswordCommand(username, password string) string {
	// 过滤控制字符，防止通过 shell 注入
	cleanPassword := strings.Map(func(r rune) rune {
		if r < 32 || r == 127 {
			return -1
		}
		return r
	}, password)
	return fmt.Sprintf("printf '%%s:%%s\\n' %s %s | chpasswd",
		utils.ShellSingleQuote(username),
		utils.ShellSingleQuote(cleanPassword),
	)
}

func buildFnOSHostnameCommand(hostname string) string {
	return fmt.Sprintf("printf '%%s\\n' %s > /etc/hostname", utils.ShellSingleQuote(hostname))
}

func buildFnOSHostsCommand(hostname string) string {
	return fmt.Sprintf(`TARGET_HOSTNAME=%s
if grep -q '^127\.0\.1\.1[[:space:]]' /etc/hosts; then
  sed -i "s/^127\.0\.1\.1[[:space:]].*/127.0.1.1\t${TARGET_HOSTNAME}/" /etc/hosts
else
  printf '127.0.1.1\t%%s\n' "$TARGET_HOSTNAME" >> /etc/hosts
fi`, utils.ShellSingleQuote(hostname))
}

func buildFnOSIdentityResetCommand() string {
	return "truncate -s 0 /etc/machine-id && rm -f /var/lib/dbus/machine-id 2>/dev/null || true"
}

func buildFnOSIdentityPreservationCommand() string {
	return strings.TrimSpace(`
if [ -s /etc/machine-id ] && [ ! -e /var/lib/dbus/machine-id ]; then
  mkdir -p /var/lib/dbus
  cp /etc/machine-id /var/lib/dbus/machine-id
fi`)
}

func buildFnOSCustomDeviceIDCommand(deviceID string) (string, error) {
	normalized, machineID, err := normalizeFnOSDeviceID(deviceID)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf(`CUSTOM_DEVICE_ID=%s
CUSTOM_MACHINE_ID=%s
mkdir -p /usr/trim/etc /var/lib/dbus
chattr -i /usr/trim/etc/machine_id /etc/device_id 2>/dev/null || true
printf '%%s\n' "$CUSTOM_DEVICE_ID" > /etc/device_id
printf '%%s\n' "$CUSTOM_MACHINE_ID" > /usr/trim/etc/machine_id
if [ -s /etc/machine-id ] && [ ! -e /var/lib/dbus/machine-id ]; then
  cp /etc/machine-id /var/lib/dbus/machine-id
fi
chattr +i /usr/trim/etc/machine_id /etc/device_id 2>/dev/null || true`, utils.ShellSingleQuote(normalized), utils.ShellSingleQuote(machineID)), nil
}

// NormalizeFnOSDeviceID exports normalizeFnOSDeviceID for external callers
func NormalizeFnOSDeviceID(deviceID string) (string, string, error) {
	return normalizeFnOSDeviceID(deviceID)
}

func normalizeFnOSDeviceID(deviceID string) (string, string, error) {
	normalized := strings.ToLower(strings.TrimSpace(deviceID))
	if !fnOSDeviceIDRegexp.MatchString(normalized) {
		return "", "", fmt.Errorf("FnOS 设备 ID 格式错误，请填写 32 位或 40 位十六进制字符串")
	}
	if len(normalized) == 40 {
		return normalized[:32], normalized, nil
	}
	return normalized, normalized + "00000000", nil
}

// ValidateFnOSDeviceID 校验 FnOS 设备 ID 格式
func ValidateFnOSDeviceID(deviceID string) error {
	_, _, err := normalizeFnOSDeviceID(deviceID)
	return err
}