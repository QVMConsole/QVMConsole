#!/bin/bash
# =============================================================================
# QVMConsole 管理脚本 (qvmc-manage)
# 用于直接在服务器上管理 QVMConsole 的账户与安全设置
# =============================================================================
set -euo pipefail

# ---- 颜色定义 ----
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
CYAN='\033[0;36m'
BOLD='\033[1m'
NC='\033[0m' # No Color

# ---- 自动检测项目目录 ----
detect_project_dir() {
    local script_dir
    script_dir="$(cd "$(dirname "$0")" && pwd)"

    # 1. 脚本自身所在目录（如果包含 server/main.go 或 go.mod）
    if [ -f "$script_dir/server/main.go" ] || [ -f "$script_dir/go.mod" ]; then
        echo "$script_dir"
        return
    fi

    # 2. 常见安装路径
    for dir in "/opt/project/QVMConsole" "/opt/kvm-console"; do
        if [ -d "$dir" ]; then
            echo "$dir"
            return
        fi
    done

    # 3. 回退到脚本目录
    echo "$script_dir"
}

PROJECT_DIR="$(detect_project_dir)"

# ---- 加载 .env 文件 ----
detect_env_file() {
    local candidates=()
    [ -n "${KVM_ENV_FILE:-}" ] && candidates+=("$KVM_ENV_FILE")
    candidates+=(
        "${PROJECT_DIR}/.env"
        "${PROJECT_DIR}/server/.env"
        "${PROJECT_DIR}/../.env"
        "${PROJECT_DIR}/../../.env"
        "/opt/kvm-console/.env"
        "/opt/project/QVMConsole/.env"
        "/opt/project/QVMConsole/server/.env"
    )
    local candidate
    for candidate in "${candidates[@]}"; do
        if [ -f "$candidate" ]; then
            printf '%s\n' "$candidate"
            return
        fi
    done
    printf '%s\n' "${PROJECT_DIR}/.env"
}

ENV_FILE="$(detect_env_file)"

load_env() {
    if [ -f "$ENV_FILE" ]; then
        set -a
        # shellcheck disable=SC1090
        source "$ENV_FILE"
        set +a
    fi
}
load_env

env_get() {
    local key="$1"
    if [ -f "$ENV_FILE" ]; then
        awk -F= -v k="$key" '$1 == k { sub(/^[^=]*=/, ""); print; exit }' "$ENV_FILE"
    fi
}

env_set() {
    local key="$1"
    local value="$2"
    mkdir -p "$(dirname "$ENV_FILE")"
    touch "$ENV_FILE"
    chmod 600 "$ENV_FILE" 2>/dev/null || true
    if grep -q "^${key}=" "$ENV_FILE"; then
        sed -i "s|^${key}=.*|${key}=${value}|" "$ENV_FILE"
    else
        printf '%s=%s\n' "$key" "$value" >> "$ENV_FILE"
    fi
}

current_public_access_enabled() {
    local configured="${KVM_PUBLIC_ACCESS_ENABLED:-}"
    if [ -n "$configured" ]; then
        printf '%s\n' "$configured"
        return
    fi
    if [ -f "$DB_PATH" ]; then
        local stored
        stored=$(sqlite3 "$DB_PATH" "SELECT value FROM system_settings WHERE key='public_access_enabled' LIMIT 1;" 2>/dev/null || true)
        [ -n "$stored" ] && { printf '%s\n' "$stored"; return; }
    fi
    printf 'false\n'
}

# ---- 自动检测数据库路径 ----
detect_db_path() {
    # 1. 环境变量优先
    if [ -n "${KVM_DB_PATH:-}" ] && [ -f "$KVM_DB_PATH" ]; then
        echo "$KVM_DB_PATH"
        return
    fi

    # 2. 检查项目目录下的常见位置
    local candidates=(
        "${PROJECT_DIR}/data/kvm_console.db"
        "${PROJECT_DIR}/server/data/kvm_console.db"
        "/opt/project/QVMConsole/data/kvm_console.db"
        "/opt/kvm-console/data/kvm_console.db"
    )
    for path in "${candidates[@]}"; do
        if [ -f "$path" ]; then
            echo "$path"
            return
        fi
    done

    # 3. 默认路径（可能不存在）
    echo "${PROJECT_DIR}/data/kvm_console.db"
}

DB_PATH="$(detect_db_path)"

# ---- 默认管理员配置 ----
ADMIN_USER="${KVM_ADMIN_USER:-admin}"
ADMIN_PASS="${KVM_ADMIN_PASS:-admin123}"

# ---- 工具函数 ----

# 使用 Python 生成 bcrypt 哈希（兼容 Go 的 golang.org/x/crypto/bcrypt）
hash_password() {
    local password="$1"
    local python_script

    # Python bcrypt 方案（与 Go bcrypt 兼容）
    python_script='
import sys
try:
    import bcrypt
    # Go 使用 cost=10, 前缀 $2a$
    hashed = bcrypt.hashpw(sys.argv[1].encode(), bcrypt.gensalt(rounds=10, prefix=b"2a"))
    print(hashed.decode())
except ImportError:
    # 回退：无 bcrypt 库时尝试用 crypt（部分系统）
    import crypt
    salt = crypt.mksalt(crypt.METHOD_BCRYPT)
    print(crypt.crypt(sys.argv[1], salt))
'
    if python3 -c "$python_script" "$password" 2>/dev/null; then
        return 0
    fi

    # 如果以上方案都不可用，提示安装 bcrypt
    echo -e "${RED}错误: 需要 Python bcrypt 库来生成密码哈希${NC}" >&2
    echo -e "${YELLOW}请运行: pip3 install bcrypt${NC}" >&2
    return 1
}

# 生成定时泄露检测使用的 HIBP 前缀与 HMAC 指纹，并执行在线检查。
password_security_info() {
    local password="$1"
    local secret="${KVM_SECURITY_SECRET:-}"
    python3 -c '
import hashlib
import hmac
import sys
import urllib.request

password = sys.argv[1]
secret = sys.argv[2]
full_hash = hashlib.sha1(password.encode()).hexdigest().upper()
prefix, suffix = full_hash[:5], full_hash[5:]
digest = hmac.new(secret.encode(), full_hash.encode(), hashlib.sha256).hexdigest().upper() if secret else ""
status, count = "ok", 0
try:
    req = urllib.request.Request(
        "https://api.pwnedpasswords.com/range/" + prefix,
        headers={"User-Agent": "QVMConsole-Manage-PasswordCheck"},
    )
    with urllib.request.urlopen(req, timeout=5) as response:
        for raw_line in response.read().decode("utf-8", "replace").splitlines():
            parts = raw_line.strip().split(":", 1)
            if len(parts) == 2 and parts[0].upper() == suffix:
                status = "breached"
                count = int(parts[1])
                break
except Exception:
    status = "unavailable"
print(f"{prefix}|{digest}|{status}|{count}")
' "$password" "$secret"
}

# 检查 sqlite3 是否可用
check_deps() {
    local missing=()

    if ! command -v sqlite3 &>/dev/null; then
        missing+=("sqlite3")
    fi
    if ! command -v python3 &>/dev/null; then
        missing+=("python3")
    fi

    if [ ${#missing[@]} -gt 0 ]; then
        echo -e "${RED}缺少必要依赖: ${missing[*]}${NC}"
        echo -e "${YELLOW}请安装后重试。例如 (Debian/Ubuntu): sudo apt install ${missing[*]} python3-bcrypt${NC}"
        exit 1
    fi
}

# 检查数据库是否可访问
check_db() {
    if [ ! -f "$DB_PATH" ]; then
        echo -e "${RED}错误: 数据库文件不存在: ${DB_PATH}${NC}"
        echo -e "${YELLOW}提示: 请确认 QVMConsole 已至少启动过一次，或设置 KVM_DB_PATH 环境变量${NC}"
        exit 1
    fi
    if ! sqlite3 "$DB_PATH" "SELECT 1;" &>/dev/null; then
        echo -e "${RED}错误: 无法读取数据库: ${DB_PATH}${NC}"
        exit 1
    fi
}

# SQLite 字符串转义（转义单引号）
sqlite_escape() {
    local val="$1"
    echo "${val//\'/''}"
}

# 安全确认
confirm_action() {
    local prompt="$1"
    echo -ne "${YELLOW}${prompt} (输入 yes 确认): ${NC}"
    read -r confirm
    if [ "$confirm" != "yes" ]; then
        echo -e "${CYAN}操作已取消${NC}"
        return 1
    fi
    return 0
}

# 按任意键继续
press_enter() {
    echo ""
    echo -ne "${CYAN}按 Enter 键返回菜单...${NC}"
    read -r
}

# ---- 功能 4: 修改服务端口 ----
change_port() {
    echo ""
    echo -e "${BOLD}========================================${NC}"
    echo -e "${BOLD}   修改服务端口${NC}"
    echo -e "${BOLD}========================================${NC}"
    echo ""

    # 获取当前端口
    local current_port="${KVM_PORT:-8080}"
    echo -e "当前端口: ${CYAN}${current_port}${NC}"
    echo ""

    # 输入新端口
    echo -ne "${CYAN}请输入新端口号 (1-65535): ${NC}"
    read -r new_port

    # 验证端口
    if [ -z "$new_port" ]; then
        echo -e "${RED}端口号不能为空${NC}"
        press_enter
        return
    fi

    if ! [[ "$new_port" =~ ^[0-9]+$ ]] || [ "$new_port" -lt 1 ] || [ "$new_port" -gt 65535 ]; then
        echo -e "${RED}无效的端口号: $new_port，请输入 1-65535 之间的数字${NC}"
        press_enter
        return
    fi

    if [ "$new_port" = "$current_port" ]; then
        echo -e "${YELLOW}新端口与当前端口相同，无需修改${NC}"
        press_enter
        return
    fi

    echo ""
    echo -e "当前端口: ${RED}${current_port}${NC} → 新端口: ${GREEN}${new_port}${NC}"
    echo -e "${YELLOW}注意: 修改后需要重启服务才能生效${NC}"
    echo ""

    # UFW 检测
    local ufw_active=false
    if command -v ufw &>/dev/null; then
        if ufw status 2>/dev/null | grep -q "Status: active"; then
            ufw_active=true
            echo -e "${YELLOW}检测到 UFW 防火墙已启用，将自动更新防火墙规则${NC}"
        else
            echo -e "${YELLOW}UFW 已安装但未启用，跳过防火墙规则更新${NC}"
        fi
    else
        echo -e "${YELLOW}未检测到 UFW 防火墙${NC}"
    fi
    echo ""

    if ! confirm_action "确认修改端口?"; then
        return
    fi

    # 更新 .env 文件中的 KVM_PORT
    if [ ! -f "$ENV_FILE" ]; then
        echo -e "${RED}错误: .env 文件不存在: ${ENV_FILE}${NC}"
        press_enter
        return
    fi

    echo ""
    echo -ne "正在更新 .env 文件..."
    if grep -q "^KVM_PORT=" "$ENV_FILE"; then
        sed -i "s/^KVM_PORT=.*/KVM_PORT=${new_port}/" "$ENV_FILE"
    else
        echo "KVM_PORT=${new_port}" >> "$ENV_FILE"
    fi
    echo -e " ${GREEN}完成${NC}"
    echo -e "${GREEN}✓ KVM_PORT 已更新为 ${new_port}${NC}"

    # UFW 规则更新
    if $ufw_active; then
        echo ""
        # 检查 root 权限（UFW 需要 root）
        if [ "$(id -u)" -ne 0 ]; then
            echo -e "${YELLOW}当前非 root 用户，无法自动操作 UFW 防火墙${NC}"
            echo -e "${YELLOW}请手动执行以下命令:${NC}"
            echo -e "${CYAN}  sudo ufw allow ${new_port}/tcp${NC}"
            if [ "$current_port" != "$new_port" ]; then
                echo -e "${CYAN}  sudo ufw delete allow ${current_port}/tcp${NC}"
            fi
        else
            # 先添加新端口规则（避免锁定自身）
            echo -ne "正在添加新端口 ${new_port}/tcp 的 UFW 规则..."
            if ufw allow "${new_port}/tcp" &>/dev/null; then
                echo -e " ${GREEN}完成${NC}"
            else
                echo -e " ${RED}失败${NC}"
                echo -e "${RED}请手动执行: ufw allow ${new_port}/tcp${NC}"
            fi

            # 删除旧端口规则（如果存在且不同于新端口）
            if [ "$current_port" != "$new_port" ]; then
                if ufw status | grep -qE "^${current_port}/tcp\s"; then
                    echo -ne "正在删除旧端口 ${current_port}/tcp 的 UFW 规则..."
                    if ufw delete allow "${current_port}/tcp" &>/dev/null; then
                        echo -e " ${GREEN}完成${NC}"
                    else
                        echo -e " ${YELLOW}删除失败，请手动检查: ufw delete allow ${current_port}/tcp${NC}"
                    fi
                else
                    echo -e "${YELLOW}未找到旧端口 ${current_port}/tcp 的 UFW 规则，跳过删除${NC}"
                fi
            fi
        fi
    fi

    echo ""
    echo -e "${GREEN}========================================${NC}"
    echo -e "${GREEN}  端口修改完成！${NC}"
    echo -e "${GREEN}  新端口: ${new_port}${NC}"
    echo -e "${GREEN}========================================${NC}"
    echo ""

    # 询问是否立即重启服务
    local service_name="${KVM_SERVICE_UNIT_NAME:-kvm-console.service}"
    echo -e "${YELLOW}端口修改需要重启服务才能生效${NC}"
    echo ""

    if confirm_action "是否立即重启服务?"; then
        echo ""
        echo -ne "正在重启服务 ${service_name}..."

        # 检查 root 权限（systemctl restart 需要 root）
        if [ "$(id -u)" -eq 0 ]; then
            if systemctl restart "$service_name" &>/dev/null; then
                echo -e " ${GREEN}完成${NC}"
                echo -e "${GREEN}✓ 服务已重启，新端口 ${new_port} 已生效${NC}"
                echo ""
                # 等待服务完全启动
                echo -ne "等待服务就绪..."
                sleep 3
                if systemctl is-active --quiet "$service_name" 2>/dev/null; then
                    echo -e " ${GREEN}服务运行中${NC}"
                    echo -e "${GREEN}✓ 现在可通过 http://<IP>:${new_port} 访问面板${NC}"
                else
                    echo -e " ${RED}服务未正常运行${NC}"
                    echo -e "${YELLOW}请检查: systemctl status ${service_name}${NC}"
                fi
            else
                echo -e " ${RED}重启失败${NC}"
                echo -e "${YELLOW}请手动执行: systemctl restart ${service_name}${NC}"
            fi
        else
            echo -e "${YELLOW}当前非 root 用户，尝试使用 sudo 重启...${NC}"
            if sudo systemctl restart "$service_name" &>/dev/null; then
                echo -e " ${GREEN}完成${NC}"
                echo -e "${GREEN}✓ 服务已重启，新端口 ${new_port} 已生效${NC}"
                echo ""
                sleep 3
                if systemctl is-active --quiet "$service_name" 2>/dev/null; then
                    echo -e "${GREEN}✓ 现在可通过 http://<IP>:${new_port} 访问面板${NC}"
                else
                    echo -e "${YELLOW}请检查: systemctl status ${service_name}${NC}"
                fi
            else
                echo -e " ${RED}重启失败${NC}"
                echo -e "${YELLOW}请手动执行: sudo systemctl restart ${service_name}${NC}"
            fi
        fi
    else
        echo ""
        echo -e "${YELLOW}请在方便时手动重启服务:${NC}"
        echo -e "${CYAN}  systemctl restart ${service_name}${NC}"
    fi
    echo ""

    press_enter
}

# ---- 功能 1: 重置默认管理员密码 ----
reset_admin_password() {
    echo ""
    echo -e "${BOLD}========================================${NC}"
    echo -e "${BOLD}   重置默认管理员密码${NC}"
    echo -e "${BOLD}========================================${NC}"
    echo ""

    local admin_exists safe_user
    safe_user=$(sqlite_escape "$ADMIN_USER")
    admin_exists=$(sqlite3 "$DB_PATH" "SELECT COUNT(*) FROM users WHERE username='$safe_user' AND deleted_at IS NULL;")

    if [ "$admin_exists" -eq 0 ]; then
        echo -e "${YELLOW}管理员账号 '${ADMIN_USER}' 不存在，将新建${NC}"
    else
        echo -e "${YELLOW}管理员账号 '${ADMIN_USER}' 已存在，将重置密码并清除安全绑定${NC}"

        local totp_enabled email_bound
        totp_enabled=$(sqlite3 "$DB_PATH" "SELECT totp_enabled FROM users WHERE username='$safe_user' AND deleted_at IS NULL;")
        email_bound=$(sqlite3 "$DB_PATH" "SELECT email FROM users WHERE username='$safe_user' AND deleted_at IS NULL;")

        if [ "$totp_enabled" = "1" ]; then
            echo -e "  - TOTP 令牌: ${RED}已绑定${NC} → 将被清除"
        fi
        if [ -n "$email_bound" ]; then
            echo -e "  - 邮箱绑定: ${RED}${email_bound}${NC} → 将被清除"
        fi
    fi

    echo ""
    echo -e "目标管理员: ${CYAN}${ADMIN_USER}${NC}"
    echo -e "新密码:     ${CYAN}${ADMIN_PASS}${NC}"

    if ! confirm_action "确认执行重置操作?"; then
        return
    fi

    if [ "$(current_public_access_enabled)" = "true" ]; then
        if ! disable_public_access_for_safety; then
            return 1
        fi
    fi

    echo ""
    echo -ne "正在生成密码哈希..."

    local hashed_password
    if ! hashed_password=$(hash_password "$ADMIN_PASS"); then
        echo ""
        return 1
    fi
    local safe_pass
    safe_pass=$(sqlite_escape "$hashed_password")
    echo -e " ${GREEN}完成${NC}"

    if [ "$admin_exists" -eq 0 ]; then
        # 创建新管理员
        sqlite3 "$DB_PATH" "INSERT INTO users (username, password_hash, role, status, cloud_type, created_at, updated_at) VALUES ('$safe_user', '$safe_pass', 'admin', 'active', 'elastic', datetime('now'), datetime('now'));"
        echo -e "${GREEN}✓ 管理员账号 '${ADMIN_USER}' 已创建${NC}"
    else
        # 更新密码并清除安全绑定
        sqlite3 "$DB_PATH" "UPDATE users SET password_hash='$safe_pass', totp_enabled=0, totp_secret_enc='', totp_recovery_codes_enc='', totp_bound_at=NULL, email='', email_verified_at=NULL, updated_at=datetime('now') WHERE username='$safe_user' AND deleted_at IS NULL;"
        echo -e "${GREEN}✓ 管理员密码已重置${NC}"
        if [ "$totp_enabled" = "1" ]; then
            echo -e "${GREEN}✓ TOTP 令牌已清除${NC}"
        fi
        if [ -n "$email_bound" ]; then
            echo -e "${GREEN}✓ 邮箱绑定已清除${NC}"
        fi
    fi

    press_enter
}

# ---- 功能 5: 修改管理员密码（保留邮箱与 TOTP） ----
change_admin_password() {
    echo ""
    echo -e "${BOLD}========================================${NC}"
    echo -e "${BOLD}   修改管理员密码${NC}"
    echo -e "${BOLD}========================================${NC}"
    echo ""

    local admins
    admins=$(sqlite3 "$DB_PATH" -separator '|' "SELECT id, username, totp_enabled, email FROM users WHERE role='admin' AND deleted_at IS NULL ORDER BY id;")
    if [ -z "$admins" ]; then
        echo -e "${YELLOW}数据库中没有可修改的管理员账户${NC}"
        press_enter
        return
    fi

    local -a admins_array=()
    local index=1
    while IFS='|' read -r id username totp email; do
        local display="${username}"
        [ "$totp" = "1" ] && display="${display} [2FA 已绑定]"
        [ -n "$email" ] && display="${display} [${email}]"
        admins_array+=("$id|$username")
        echo -e "  ${BOLD}${index}${NC}. ${display}"
        ((index++))
    done <<< "$admins"

    echo ""
    echo -e "  ${BOLD}0${NC}. 返回主菜单"
    echo -ne "${CYAN}请选择要修改密码的管理员 (1-$((index-1))): ${NC}"
    read -r choice
    if [ "$choice" = "0" ] || [ -z "$choice" ]; then
        return
    fi
    if ! [[ "$choice" =~ ^[0-9]+$ ]] || [ "$choice" -lt 1 ] || [ "$choice" -ge "$index" ]; then
        echo -e "${RED}无效选择${NC}"
        press_enter
        return
    fi

    local selected sel_id sel_username
    selected="${admins_array[$((choice-1))]}"
    sel_id=$(echo "$selected" | cut -d'|' -f1)
    sel_username=$(echo "$selected" | cut -d'|' -f2)

    local new_password confirm_password security_info prefix fingerprint breach_status breach_count
    while true; do
        echo ""
        echo -ne "${CYAN}请输入新密码（至少 12 位）: ${NC}"
        read -rs new_password
        echo ""
        echo -ne "${CYAN}请再次输入新密码: ${NC}"
        read -rs confirm_password
        echo ""
        if [ "$new_password" != "$confirm_password" ]; then
            echo -e "${RED}两次输入的密码不一致${NC}"
            continue
        fi
        if [ "${#new_password}" -lt 12 ]; then
            echo -e "${RED}密码长度不能少于 12 位${NC}"
            continue
        fi

        echo -ne "正在进行泄露密码检测..."
        security_info=$(password_security_info "$new_password")
        IFS='|' read -r prefix fingerprint breach_status breach_count <<< "$security_info"
        if [ "$breach_status" = "breached" ]; then
            echo -e " ${RED}失败${NC}"
            echo -e "${RED}该密码已在公开泄露数据库中出现 ${breach_count} 次，请更换密码${NC}"
            continue
        fi
        if [ "$breach_status" = "unavailable" ]; then
            echo -e " ${YELLOW}服务暂时不可用${NC}"
            if ! confirm_action "在线检测失败，确认仍使用此密码?"; then
                continue
            fi
        else
            echo -e " ${GREEN}通过${NC}"
        fi
        break
    done

    echo ""
    echo -e "目标管理员: ${CYAN}${sel_username}${NC}"
    echo -e "邮箱和 TOTP 绑定将保持不变。"
    if ! confirm_action "确认修改管理员密码?"; then
        return
    fi

    local hashed_password safe_pass safe_prefix safe_fingerprint
    if ! hashed_password=$(hash_password "$new_password"); then
        return 1
    fi
    safe_pass=$(sqlite_escape "$hashed_password")
    safe_prefix=$(sqlite_escape "$prefix")
    safe_fingerprint=$(sqlite_escape "$fingerprint")

    sqlite3 "$DB_PATH" "UPDATE users SET password_hash='$safe_pass', password_breach_prefix='$safe_prefix', password_breach_hmac='$safe_fingerprint', password_breached=0, password_breach_count=0, password_breach_checked_at=NULL, password_breach_detected_at=NULL, password_breach_user_notified_at=NULL, password_breach_admin_notified_at=NULL, force_password_change=0, force_password_change_reason='', login_verified_until=NULL, high_risk_verified_until=NULL, security_updated_at=datetime('now'), updated_at=datetime('now') WHERE id=$sel_id AND role='admin' AND deleted_at IS NULL;"
    unset new_password confirm_password hashed_password

    echo -e "${GREEN}✓ 管理员 '${sel_username}' 的密码已修改，泄露锁定状态已清除${NC}"
    press_enter
}

# ---- 功能 2: 清除 TOTP 令牌认证 ----
clear_totp() {
    echo ""
    echo -e "${BOLD}========================================${NC}"
    echo -e "${BOLD}   清除 TOTP 令牌认证${NC}"
    echo -e "${BOLD}========================================${NC}"
    echo ""

    # 查询所有启用了 TOTP 的用户
    local totp_users
    totp_users=$(sqlite3 "$DB_PATH" -separator '|' "SELECT id, username, role, email FROM users WHERE totp_enabled=1 AND deleted_at IS NULL ORDER BY id;")

    if [ -z "$totp_users" ]; then
        echo -e "${GREEN}当前没有已绑定 TOTP 的用户${NC}"
        press_enter
        return
    fi

    # 将结果保存为数组
    local -a users_array=()
    local index=1
    while IFS='|' read -r id username role email; do
        local display="${username}"
        [ "$role" = "admin" ] && display="${display} (管理员)"
        [ -n "$email" ] && display="${display} [${email}]"
        users_array+=("$id|$username|$role")
        echo -e "  ${BOLD}${index}${NC}. ${display}"
        ((index++))
    done <<< "$totp_users"

    echo ""
    echo -e "  ${BOLD}0${NC}. 返回主菜单"
    echo ""

    echo -ne "${CYAN}请选择要清除 TOTP 的账户 (1-$((index-1))): ${NC}"
    read -r choice

    if [ "$choice" = "0" ] || [ -z "$choice" ]; then
        return
    fi

    if ! [[ "$choice" =~ ^[0-9]+$ ]] || [ "$choice" -lt 1 ] || [ "$choice" -ge "$index" ]; then
        echo -e "${RED}无效选择${NC}"
        press_enter
        return
    fi

    local selected
    selected="${users_array[$((choice-1))]}"
    local sel_id sel_username sel_role safe_username
    sel_id=$(echo "$selected" | cut -d'|' -f1)
    sel_username=$(echo "$selected" | cut -d'|' -f2)
    sel_role=$(echo "$selected" | cut -d'|' -f3)
    safe_username=$(sqlite_escape "$sel_username")

    echo ""
    echo -e "将清除以下账户的 TOTP 令牌:"
    echo -e "  用户名: ${CYAN}${sel_username}${NC}"
    echo -e "  角色:   ${CYAN}${sel_role}${NC}"

    if ! confirm_action "确认清除 TOTP?"; then
        return
    fi

    if [ "$sel_role" = "admin" ] && [ "$(current_public_access_enabled)" = "true" ]; then
        if ! disable_public_access_for_safety; then
            return 1
        fi
    fi

    sqlite3 "$DB_PATH" "UPDATE users SET totp_enabled=0, totp_secret_enc='', totp_recovery_codes_enc='', totp_bound_at=NULL, updated_at=datetime('now') WHERE id=$sel_id AND deleted_at IS NULL;"

    echo -e "${GREEN}✓ 账户 '${sel_username}' 的 TOTP 令牌已清除${NC}"
    press_enter
}

# ---- 功能 6: 开启公网访问 ----
set_public_access_db() {
    local enabled="$1"
    local revoke_keys="${2:-0}"
    local now_sql="$(date '+%Y-%m-%d %H:%M:%S')"
    if [ "$revoke_keys" = "1" ]; then
        sqlite3 "$DB_PATH" <<SQL
BEGIN IMMEDIATE;
INSERT INTO system_settings(key, value) VALUES ('public_access_enabled', '$enabled')
ON CONFLICT(key) DO UPDATE SET value=excluded.value;
UPDATE user_api_keys
SET revoked_at='$now_sql', updated_at='$now_sql'
WHERE revoked_at IS NULL AND user_id IN (SELECT id FROM users WHERE role='admin');
COMMIT;
SQL
    else
        sqlite3 "$DB_PATH" "INSERT INTO system_settings(key, value) VALUES ('public_access_enabled', '$enabled') ON CONFLICT(key) DO UPDATE SET value=excluded.value;"
    fi
}

restart_panel_service() {
    local service_name="${KVM_SERVICE_UNIT_NAME:-kvm-console.service}"
    if [ "$(id -u)" -ne 0 ]; then
        echo -e "${RED}错误: 更新公网访问后需要 root 权限重启 ${service_name}${NC}"
        return 1
    fi
    if ! systemctl restart "$service_name"; then
        echo -e "${RED}错误: 重启服务失败，请检查: systemctl status ${service_name}${NC}"
        return 1
    fi
    sleep 3
    if ! systemctl is-active --quiet "$service_name"; then
        echo -e "${RED}错误: 服务重启后未正常运行，请检查日志${NC}"
        return 1
    fi
    return 0
}

disable_public_access_for_safety() {
    echo -e "${YELLOW}检测到公网访问已开启。为避免安全信息被清除后留下不满足要求的状态，将先关闭公网访问。${NC}"
    if ! set_public_access_db false 0; then
        echo -e "${RED}错误: 写入公网访问状态失败${NC}"
        return 1
    fi
    env_set "KVM_PUBLIC_ACCESS_ENABLED" "false"
    KVM_PUBLIC_ACCESS_ENABLED="false"
    if ! restart_panel_service; then
        return 1
    fi
    echo -e "${GREEN}✓ 公网访问已关闭${NC}"
    return 0
}

enable_public_access() {
    echo ""
    echo -e "${BOLD}========================================${NC}"
    echo -e "${BOLD}   开启公网访问${NC}"
    echo -e "${BOLD}========================================${NC}"
    echo ""

    if [ "$(id -u)" -ne 0 ]; then
        echo -e "${RED}错误: 开启公网访问必须使用 root 运行此脚本${NC}"
        press_enter
        return
    fi
    if [ "$(current_public_access_enabled)" = "true" ]; then
        echo -e "${GREEN}公网访问当前已开启${NC}"
        press_enter
        return
    fi

    local admins missing=0
    admins=$(sqlite3 "$DB_PATH" -separator '|' "SELECT id, username, status, totp_enabled, COALESCE(email,'') FROM users WHERE role='admin' AND status='active' AND deleted_at IS NULL ORDER BY id;")
    if [ -z "$admins" ]; then
        echo -e "${RED}错误: 未找到有效管理员，不能开启公网访问${NC}"
        press_enter
        return
    fi
    echo -e "${CYAN}当前有效管理员安全状态:${NC}"
    while IFS='|' read -r id username status totp email; do
        [ -n "$username" ] || continue
        local display="${username}（状态: ${status}）"
        if [ "$totp" = "1" ]; then
            display+=" [2FA 已绑定]"
        else
            display+=" [2FA 未绑定]"
            missing=1
        fi
        [ -n "$email" ] && display+=" [${email}]"
        echo "  - ${display}"
    done <<< "$admins"

    if [ "$missing" -eq 1 ]; then
        echo ""
        echo -e "${YELLOW}普通开启要求所有有效管理员先绑定 2FA。${NC}"
        echo -e "${YELLOW}仅 root 可使用固定确认短语应急绕过；绕过后相关管理员仍必须尽快完成 2FA。${NC}"
        local phrase
        read -rp "如确认应急绕过，请输入 ENABLE-PUBLIC-ACCESS（直接回车取消）: " phrase
        if [ "$phrase" != "ENABLE-PUBLIC-ACCESS" ]; then
            echo -e "${CYAN}已取消开启公网访问${NC}"
            press_enter
            return
        fi
    fi

    if ! confirm_action "确认开启公网访问?"; then
        return
    fi
    if ! set_public_access_db true 1; then
        echo -e "${RED}错误: 更新数据库公网访问状态失败${NC}"
        press_enter
        return 1
    fi
    env_set "KVM_PUBLIC_ACCESS_ENABLED" "true"
    KVM_PUBLIC_ACCESS_ENABLED="true"
    echo -e "${GREEN}✓ 公网访问状态已写入，现有管理员 API Key 已撤销${NC}"
    if restart_panel_service; then
        echo -e "${GREEN}✓ 公网访问已开启并已重启服务${NC}"
    fi
    press_enter
}

# ---- 功能 3: 查看所有用户 ----
list_users() {
    echo ""
    echo -e "${BOLD}========================================${NC}"
    echo -e "${BOLD}   用户列表${NC}"
    echo -e "${BOLD}========================================${NC}"
    echo ""

    local users
    users=$(sqlite3 "$DB_PATH" -separator '|' "SELECT id, username, role, status, totp_enabled, email FROM users WHERE deleted_at IS NULL ORDER BY id;")

    if [ -z "$users" ]; then
        echo -e "${YELLOW}数据库中没有用户${NC}"
        press_enter
        return
    fi

    printf "  ${BOLD}%-4s %-20s %-8s %-8s %-6s %s${NC}\n" "ID" "用户名" "角色" "状态" "TOTP" "邮箱"
    echo "  ----------------------------------------------------------------"
    while IFS='|' read -r id username role status totp email; do
        local totp_display="否"
        [ "$totp" = "1" ] && totp_display="${GREEN}是${NC}"
        [ -z "$email" ] && email="-"
        printf "  %-4s %-20s %-8s %-8s " "$id" "$username" "$role" "$status"
        echo -ne "$totp_display"
        printf "  %s\n" "$email"
    done <<< "$users"

    press_enter
}

# ---- 主菜单 ----
show_menu() {
    clear
    echo ""
    echo -e "${BOLD}${CYAN}╔══════════════════════════════════════════════╗${NC}"
    echo -e "${BOLD}${CYAN}║       QVMConsole 管理工具 v1.0              ║${NC}"
    echo -e "${BOLD}${CYAN}╚══════════════════════════════════════════════╝${NC}"
    echo ""
    echo -e "  项目目录: ${CYAN}${PROJECT_DIR}${NC}"
    echo -e "  数据库:   ${CYAN}${DB_PATH}${NC}"
    echo -e "  管理员:   ${CYAN}${ADMIN_USER}${NC}"
    echo ""
    echo -e "  ${BOLD}${GREEN}1${NC}. 重置默认管理员密码 (并清除 TOTP/邮箱绑定)"
    echo -e "  ${BOLD}${GREEN}2${NC}. 清除 TOTP 令牌认证 (选择账户)"
    echo -e "  ${BOLD}${GREEN}3${NC}. 查看所有用户"
    echo -e "  ${BOLD}${GREEN}4${NC}. 修改服务端口 (自动更新 UFW 防火墙规则)"
    echo -e "  ${BOLD}${GREEN}5${NC}. 修改管理员密码 (保留 TOTP/邮箱绑定)"
    echo -e "  ${BOLD}${GREEN}6${NC}. 开启公网访问（检查管理员 2FA，撤销现有管理员 API Key）"
    echo ""
    echo -e "  ${BOLD}${RED}0${NC}. 退出"
    echo ""
    echo -ne "${CYAN}请输入选项 [0-6]: ${NC}"
}

# ---- 主流程 ----
main() {
    check_deps
    check_db

    while true; do
        show_menu
        read -r choice

        case "${choice:-}" in
            1) reset_admin_password ;;
            2) clear_totp ;;
            3) list_users ;;
            4) change_port ;;
            5) change_admin_password ;;
            6) enable_public_access ;;
            0)
                echo ""
                echo -e "${GREEN}再见!${NC}"
                exit 0
                ;;
            *)
                echo -e "${RED}无效选项，请重新输入${NC}"
                sleep 1
                ;;
        esac
    done
}

main "$@"
