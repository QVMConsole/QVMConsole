package clone

import (
	"fmt"
	"strings"

	"kvm_console/utils"
)

// resolveLinuxCloneTargetUser 解析最终登录用户，显式选择 root 时不得回退到模板用户。
func resolveLinuxCloneTargetUser(user, templateUser string) string {
	targetUser := strings.TrimSpace(user)
	if targetUser != "" {
		return targetUser
	}
	return strings.TrimSpace(templateUser)
}

// linuxPermitRootLoginValue 返回 SSH 的 root 登录策略。
func linuxPermitRootLoginValue(targetUser string) string {
	if targetUser == "root" {
		return "yes"
	}
	return "no"
}

// buildLinuxSSHPasswordAuthCommand 生成 SSH 密码认证与 root 登录策略命令。
func buildLinuxSSHPasswordAuthCommand(targetUser string) string {
	permitRoot := linuxPermitRootLoginValue(targetUser)
	return fmt.Sprintf(`set -eu
SSHD_CFG=/etc/ssh/sshd_config
if [ -f "$SSHD_CFG" ]; then
  if grep -qE "^\s*#?\s*PermitRootLogin" "$SSHD_CFG"; then
    sed -i "s/^\s*#\?\s*PermitRootLogin.*/PermitRootLogin %s/" "$SSHD_CFG"
  else
    printf 'PermitRootLogin %s\n' >> "$SSHD_CFG"
  fi
  if grep -qE "^\s*#?\s*PasswordAuthentication" "$SSHD_CFG"; then
    sed -i "s/^\s*#\?\s*PasswordAuthentication.*/PasswordAuthentication yes/" "$SSHD_CFG"
  else
    printf 'PasswordAuthentication yes\n' >> "$SSHD_CFG"
  fi
fi
if [ -d /etc/ssh/sshd_config.d ]; then
  for SSHD_DROPIN in /etc/ssh/sshd_config.d/*.conf; do
    [ -f "$SSHD_DROPIN" ] || continue
    sed -i "s/^\s*PermitRootLogin.*/PermitRootLogin %s/" "$SSHD_DROPIN"
    sed -i "s/^\s*PasswordAuthentication.*/PasswordAuthentication yes/" "$SSHD_DROPIN"
  done
fi`, permitRoot, permitRoot, permitRoot)
}

// buildEnsureLinuxCloneUserCommand 生成离线账户准备命令。
// 模板元数据只描述预期旧用户名，是否存在必须以克隆磁盘内的账户为准。
func buildEnsureLinuxCloneUserCommand(templateUser, targetUser string) string {
	return fmt.Sprintf(`set -eu
OLD=%s
NEW=%s

if ! id "$NEW" >/dev/null 2>&1; then
  if [ -n "$OLD" ] && [ "$OLD" != "root" ] && [ "$OLD" != "$NEW" ] && id "$OLD" >/dev/null 2>&1; then
    usermod -l "$NEW" "$OLD"
    if getent group "$OLD" >/dev/null 2>&1 && ! getent group "$NEW" >/dev/null 2>&1; then
      groupmod -n "$NEW" "$OLD"
    fi
    usermod -d "/home/$NEW" -m "$NEW"
    if [ -d /etc/sudoers.d ]; then
      find /etc/sudoers.d -type f -exec sed -i "s/$OLD/$NEW/g" {} \;
    fi
  else
    useradd -m -s /bin/bash "$NEW"
  fi
fi

usermod -s /bin/bash "$NEW"
if getent group sudo >/dev/null 2>&1; then
  usermod -aG sudo "$NEW"
elif getent group wheel >/dev/null 2>&1; then
  usermod -aG wheel "$NEW"
fi

USER_HOME=$(getent passwd "$NEW" | cut -d: -f6)
USER_GROUP=$(id -gn "$NEW")
if [ -n "$USER_HOME" ]; then
  install -d -m 0755 -o "$NEW" -g "$USER_GROUP" "$USER_HOME"
fi`, utils.ShellSingleQuote(templateUser), utils.ShellSingleQuote(targetUser))
}
