#!/bin/bash
# ============================================================
# QVMConsole 安装 / 更新 / 卸载脚本
# ============================================================

set -Eeuo pipefail

RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
CYAN='\033[0;36m'
NC='\033[0m'

info() { echo -e "${GREEN}[INFO]${NC} $1"; }
warn() { echo -e "${YELLOW}[WARN]${NC} $1"; }
error() { echo -e "${RED}[ERROR]${NC} $1"; }
success() { echo -e "${GREEN}[✓]${NC} $1"; }

APP_NAME="QVMConsole"
INSTALL_DIR="/opt/kvm-console"
SERVICE_NAME="kvm-console"
SERVICE_FILE="/etc/systemd/system/${SERVICE_NAME}.service"
ENV_FILE="${INSTALL_DIR}/.env"
GITHUB_REPO="yxsj245/kvm_console"
GITHUB_API="https://api.github.com/repos/${GITHUB_REPO}/releases/latest"

STORAGE_IMG="/var/lib/kvm-user-storage.img"
STORAGE_MOUNT="/var/lib/kvm-user-storage"
OVS_CONFIG_DIR="/etc/kvm-console/ovs"
OVS_STATE_DIR="/var/lib/kvm-console/ovs"
OVS_DNSMASQ_UNIT="kvm-console-ovs-dnsmasq.service"
OVS_DNSMASQ_SERVICE_FILE="/etc/systemd/system/${OVS_DNSMASQ_UNIT}"
PORT_FORWARD_DIR="/etc/kvm-portforward"
VM_ACCESS_DIR="/etc/libvirt/vm-access"
FIREWALL_DIR="/etc/kvm-console/firewall"
VPC_CONFIG_DIR="/etc/kvm-console/vpc"

MODE=""
KVM_PORT=""
RELEASE_SOURCE_DIR=""

APT_DEPS=(
    "ca-certificates"
    "curl"
    "tar"
    "gzip"
    "qemu-utils"
    "libvirt-daemon-system"
    "libvirt-daemon-driver-qemu"
    "libvirt-clients"
    "openvswitch-switch"
    "dnsmasq-base"
    "virtinst"
    "libguestfs-tools"
    "ntfs-3g"
    "genisoimage"
    "sshpass"
    "cloud-image-utils"
    "lvm2"
    "cloud-guest-utils"
    "quota"
    "e2fsprogs"
    "util-linux"
    "nftables"
    "iproute2"
    "iptables"
    "tcpdump"
    "ufw"
    "nmap"
    "arp-scan"
    "conntrack"
    "openssh-client"
    "openssh-server"
    "parted"
)

# 架构特有依赖：在 check_and_install_deps 中根据 $ARCH 动态追加
QEMU_PKG_X86="qemu-system-x86"
EFI_PKG_X86="ovmf"
QEMU_PKG_ARM="qemu-system-arm"
EFI_PKG_ARM="qemu-efi-aarch64"

COMMAND_CHECKS=(
    "virsh"
    "qemu-img"
    "virt-install"
    "virt-filesystems"
    "virt-customize"
    "guestfish"
    "virt-win-reg"
    "ntfsclone"
    "ntfsfix"
    "ntfsresize"
    "genisoimage"
    "sshpass"
    "ovs-vsctl"
    "ovs-ofctl"
    "dnsmasq"
    "nft"
    "ip"
    "iptables"
    "tcpdump"
    "tc"
    "setquota"
    "repquota"
    "chattr"
    "mkfs.ext4"
    "lsblk"
    "findmnt"
    "blkid"
    "wipefs"
    "mount"
    "growpart"
    "parted"
    "partprobe"
)

cleanup_tmp() {
    if [ -n "${TMP_RELEASE_DIR:-}" ] && [ -d "$TMP_RELEASE_DIR" ]; then
        rm -rf "$TMP_RELEASE_DIR"
    fi
}
trap cleanup_tmp EXIT

check_root() {
    if [ "$(id -u)" -ne 0 ]; then
        error "请使用 root 用户或 sudo 运行此脚本"
        exit 1
    fi
}

check_os() {
    if [ ! -f /etc/os-release ]; then
        error "无法识别操作系统，本脚本仅适配 Debian / Ubuntu 系列"
        exit 1
    fi
    . /etc/os-release
    if [[ "${ID:-}" != "ubuntu" && "${ID:-}" != "debian" && "${ID_LIKE:-}" != *"debian"* && "${ID_LIKE:-}" != *"ubuntu"* ]]; then
        error "当前系统为 ${PRETTY_NAME:-unknown}，请使用 Debian / Ubuntu 系列系统安装"
        exit 1
    fi
    info "检测到系统: ${PRETTY_NAME:-unknown}"
}

detect_arch() {
    ARCH=$(uname -m)
    case "$ARCH" in
        x86_64|amd64)
            ARCH="x86_64"
            ;;
        aarch64|arm64)
            ARCH="aarch64"
            ;;
        *)
            error "不支持的 CPU 架构: $ARCH，仅支持 x86_64 / aarch64"
            exit 1
            ;;
    esac
    info "检测到 CPU 架构: ${ARCH}"
}

check_arch() {
    detect_arch
}

check_kvm_hardware() {
    info "检测 KVM 硬件虚拟化能力..."
    if [ ! -r /proc/cpuinfo ]; then
        error "无法读取 /proc/cpuinfo，不能确认硬件虚拟化能力"
        exit 1
    fi
    if [ "$ARCH" = "x86_64" ]; then
        if ! awk -F: '/^(flags|Features)[[:space:]]*:/ { if ($2 ~ /(^|[[:space:]])(vmx|svm)([[:space:]]|$)/) found=1 } END { exit found ? 0 : 1 }' /proc/cpuinfo; then
            error "未检测到 CPU 硬件虚拟化标记（Intel VT-x/vmx 或 AMD-V/svm），请先在 BIOS/UEFI 中开启虚拟化后再安装"
            exit 1
        fi
    elif [ "$ARCH" = "aarch64" ]; then
        if [ ! -e /dev/kvm ]; then
            error "未检测到 /dev/kvm，ARM 虚拟化可能未启用或内核不支持 KVM"
            exit 1
        fi
    fi
    success "CPU 已开启硬件虚拟化标记"
}

ensure_kvm_runtime() {
    info "检测 /dev/kvm 运行环境..."
    if [ "$ARCH" = "x86_64" ]; then
        local vendor_module="kvm"
        if grep -q "GenuineIntel" /proc/cpuinfo 2>/dev/null; then
            vendor_module="kvm_intel"
        elif grep -q "AuthenticAMD" /proc/cpuinfo 2>/dev/null; then
            vendor_module="kvm_amd"
        fi
        modprobe kvm 2>/dev/null || true
        modprobe "$vendor_module" 2>/dev/null || true
    elif [ "$ARCH" = "aarch64" ]; then
        # ARM 平台 KVM 通常内置在内核中（builtin），也可能以模块存在
        modprobe kvm 2>/dev/null || true
    fi

    if [ ! -e /dev/kvm ]; then
        error "未检测到 /dev/kvm。通常是 BIOS/UEFI 未开启虚拟化、宿主机未开放嵌套虚拟化，或内核 KVM 模块无法加载"
        exit 1
    fi
    success "/dev/kvm 可用"
}

detect_existing_install() {
    if [ -x "${INSTALL_DIR}/kvm-console" ] || [ -f "$SERVICE_FILE" ]; then
        return 0
    fi
    return 1
}

choose_mode() {
    if detect_existing_install; then
        echo ""
        echo -e "${CYAN}检测到已安装的 ${APP_NAME}${NC}"
        echo -e "  ${CYAN}1.${NC} 更新"
        echo -e "  ${CYAN}2.${NC} 卸载"
        echo -e "  ${CYAN}3.${NC} 修复配置文件（重置 .env 为默认值）"
        echo ""
        local choice
        read -rp "请选择操作 [1/2/3，默认 1]: " choice
        choice=${choice:-1}
        case "$choice" in
            1)
                MODE="update"
                info "将执行更新，并重新检测/修复运行地基"
                ;;
            2)
                MODE="uninstall"
                info "将执行卸载"
                ;;
            3)
                MODE="repair"
                info "将重置配置文件为默认值"
                ;;
            *)
                error "无效的选择: $choice"
                exit 1
                ;;
        esac
    else
        MODE="install"
        info "未检测到已安装的 ${APP_NAME}，将执行首次安装"
    fi
}

is_pkg_installed() {
    dpkg-query -W -f='${Status}' "$1" 2>/dev/null | grep -q "install ok installed"
}

install_optional_polkit() {
    if command -v pkaction >/dev/null 2>&1 || systemctl list-unit-files 2>/dev/null | grep -q '^polkit\.service'; then
        return
    fi
    info "补充安装 polkit 组件..."
    if apt-cache show polkitd >/dev/null 2>&1; then
        apt-get install -y polkitd
    elif apt-cache show policykit-1 >/dev/null 2>&1; then
        apt-get install -y policykit-1
    else
        warn "未找到 polkitd / policykit-1 包，用户级 libvirt 授权可能需要手动检查"
    fi
}

find_kvm_stat_binary() {
    if command -v kvm_stat >/dev/null 2>&1; then
        command -v kvm_stat
        return 0
    fi

    local found
    found=$(find /usr/lib/linux-tools -name kvm_stat -type f 2>/dev/null | sort -V | tail -n1 || true)
    if [ -n "$found" ]; then
        printf '%s\n' "$found"
        return 0
    fi
    return 1
}

check_optional_kvm_stat() {
    local kvm_stat_path
    if kvm_stat_path=$(find_kvm_stat_binary); then
        success "可选辅助指标 kvm_stat 已可用: $kvm_stat_path"
        return
    fi

    info "未检测到可用的 kvm_stat，跳过 kvm_page_fault 辅助指标；热迁移仍会使用 libvirt dirty-rate 判断"
}

check_and_install_deps() {
    info "检查宿主机依赖包..."
    local missing=()
    local pkg

    # 根据架构动态确定依赖列表
    local deps=("${APT_DEPS[@]}")
    if [ "$ARCH" = "x86_64" ]; then
        deps+=("$QEMU_PKG_X86" "$EFI_PKG_X86")
        info "架构: x86_64，QEMU 包: ${QEMU_PKG_X86}，EFI 包: ${EFI_PKG_X86}"
    elif [ "$ARCH" = "aarch64" ]; then
        deps+=("$QEMU_PKG_ARM" "$EFI_PKG_ARM")
        info "架构: aarch64，QEMU 包: ${QEMU_PKG_ARM}，EFI 包: ${EFI_PKG_ARM}"
    fi

    for pkg in "${deps[@]}"; do
        if is_pkg_installed "$pkg"; then
            success "$pkg 已安装"
        else
            missing+=("$pkg")
        fi
    done

    if [ ${#missing[@]} -gt 0 ]; then
        warn "发现缺失依赖: ${missing[*]}"
        read -rp "是否立即安装缺失依赖? [Y/n]: " confirm
        confirm=${confirm:-Y}
        if [[ ! "$confirm" =~ ^[Yy]$ ]]; then
            error "缺少必要依赖，无法保证面板功能完整运行"
            exit 1
        fi
        info "更新 apt 包索引..."
        apt-get update
        info "安装缺失依赖..."
        DEBIAN_FRONTEND=noninteractive apt-get install -y "${missing[@]}"
    fi

    install_optional_polkit
    check_optional_kvm_stat
    ensure_required_commands
    ensure_core_services
}

ensure_required_commands() {
    info "校验功能所需系统命令..."
    local missing_cmds=()
    local cmd
    for cmd in "${COMMAND_CHECKS[@]}"; do
        if ! command -v "$cmd" >/dev/null 2>&1; then
            missing_cmds+=("$cmd")
        fi
    done
    if [ ${#missing_cmds[@]} -gt 0 ]; then
        error "以下命令不可用: ${missing_cmds[*]}。请检查 apt 源或依赖安装结果"
        exit 1
    fi
    success "系统命令校验完成"
}

ensure_core_services() {
    info "检查核心服务..."
    systemctl enable --now libvirtd 2>/dev/null || systemctl enable --now libvirt-daemon 2>/dev/null || true
    systemctl enable --now openvswitch-switch 2>/dev/null || true
    systemctl enable ssh 2>/dev/null || systemctl enable sshd 2>/dev/null || true

    if ! systemctl is-active --quiet libvirtd 2>/dev/null && ! systemctl is-active --quiet libvirt-daemon 2>/dev/null; then
        error "libvirt 服务未运行，请检查 libvirt-daemon-system 安装状态"
        exit 1
    fi
    if ! systemctl is-active --quiet openvswitch-switch 2>/dev/null; then
        warn "openvswitch-switch 当前未运行，面板会在网络修复时再次尝试启动"
    fi
    success "核心服务检查完成"
}

detect_root_size() {
    # 优先使用更稳健的 df --output 方式获取根分区大小
    local root_size_gb
    root_size_gb=$(df --output=size -BG / 2>/dev/null | awk 'NR==2{gsub(/[^0-9]/,"",$1); print $1}')
    if [ -n "$root_size_gb" ] && [ "$root_size_gb" -gt 0 ] 2>/dev/null; then
        echo "${root_size_gb}G"
        return
    fi
    # 回退方案：解析 df -k 输出
    local root_size_kb
    root_size_kb=$(df -k / 2>/dev/null | awk 'NR==2{print $2}')
    if [ -n "$root_size_kb" ] && [ "$root_size_kb" -gt 0 ] 2>/dev/null; then
        echo "$((root_size_kb / 1024 / 1024))G"
        return
    fi
    echo "100G"
}

ensure_storage_fstab() {
    touch /etc/fstab
    if ! grep -Fq "$STORAGE_IMG $STORAGE_MOUNT ext4 loop,prjquota" /etc/fstab 2>/dev/null; then
        echo "${STORAGE_IMG} ${STORAGE_MOUNT} ext4 loop,prjquota 0 0" >> /etc/fstab
        success "已写入用户存储挂载配置到 /etc/fstab"
    fi
}

setup_quota() {
    info "检查用户存储 Project Quota 文件系统..."
    mkdir -p "$STORAGE_MOUNT"
    touch /etc/projects /etc/projid

    if mountpoint -q "$STORAGE_MOUNT"; then
        quotaon -P "$STORAGE_MOUNT" 2>/dev/null || true
        ensure_storage_fstab
        success "用户存储文件系统已挂载"
        return
    fi

    if [ -f "$STORAGE_IMG" ]; then
        info "检测到已有用户存储镜像，正在挂载..."
        if mount -o loop,prjquota "$STORAGE_IMG" "$STORAGE_MOUNT" 2>/dev/null; then
            quotaon -P "$STORAGE_MOUNT" 2>/dev/null || true
            ensure_storage_fstab
            success "用户存储文件系统已挂载"
            return
        fi
        # 挂载失败（可能是之前创建时损坏的镜像），允许重新创建
        warn "现有镜像挂载失败，可能为损坏文件。"
        read -rp "是否删除现有镜像并重新创建? [Y/n]: " recreate
        recreate=${recreate:-Y}
        if [[ "$recreate" =~ ^[Yy]$ ]]; then
            umount "$STORAGE_MOUNT" 2>/dev/null || true
            rm -f "$STORAGE_IMG"
            warn "已删除损坏的镜像，将重新创建"
        else
            error "无法挂载用户存储文件系统，请手动检查: $STORAGE_IMG"
            exit 1
        fi
    fi

    local storage_size
    local default_size
    default_size=$(detect_root_size)
    echo ""
    info "用户存储配额需要创建专用 ext4 project quota 稀疏镜像"
    while true; do
        read -rp "存储文件系统最大容量 [默认 ${default_size}]: " storage_size
        storage_size=${storage_size:-$default_size}
        # 校验格式：必须为数字+可选单位（K/M/G/T），不区分大小写
        if [[ "$storage_size" =~ ^[0-9]+[kKmMgGtT]?$ ]]; then
            # 确保有单位后缀（无后缀时默认当作 G）
            if [[ "$storage_size" =~ ^[0-9]+$ ]]; then
                storage_size="${storage_size}G"
            fi
            break
        fi
        warn "无效的大小格式: ${storage_size}，请输入数字+单位，如 300G、1024M"
    done

    read -rp "是否创建用户存储文件系统? [Y/n]: " confirm
    confirm=${confirm:-Y}
    if [[ ! "$confirm" =~ ^[Yy]$ ]]; then
        error "已取消创建用户存储文件系统。该文件系统是"我的存储"配额的基础，请创建后再继续安装"
        exit 1
    fi

    info "创建用户存储镜像: $STORAGE_IMG ($storage_size)"
    # 使用 truncate 创建稀疏镜像文件，大小格式已在上方循环中校验
    truncate -s "$storage_size" "$STORAGE_IMG"
    mkfs.ext4 -q -O project,quota "$STORAGE_IMG"
    mount -o loop,prjquota "$STORAGE_IMG" "$STORAGE_MOUNT"
    quotaon -P "$STORAGE_MOUNT" 2>/dev/null || true
    ensure_storage_fstab
    success "用户存储 Project Quota 文件系统已创建"
}

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
    if grep -q "^${key}=" "$ENV_FILE"; then
        sed -i "s|^${key}=.*|${key}=${value}|" "$ENV_FILE"
    else
        echo "${key}=${value}" >> "$ENV_FILE"
    fi
}

env_default() {
    local key="$1"
    local value="$2"
    if [ -z "$(env_get "$key")" ] && ! grep -q "^${key}=" "$ENV_FILE" 2>/dev/null; then
        env_set "$key" "$value"
    fi
}

random_secret() {
    local secret
    secret=$(tr -dc 'a-zA-Z0-9' </dev/urandom | head -c 48 || true)
    printf '%s' "$secret"
}

configure_port() {
    local default_port="8080"
    local existing_port
    existing_port=$(env_get "KVM_PORT")
    if [ -n "$existing_port" ]; then
        read -rp "请输入网页访问端口 [默认保持 ${existing_port}]: " input_port
        KVM_PORT=${input_port:-$existing_port}
    else
        read -rp "请输入网页访问端口 [默认 ${default_port}]: " input_port
        KVM_PORT=${input_port:-$default_port}
    fi

    if ! [[ "$KVM_PORT" =~ ^[0-9]+$ ]] || [ "$KVM_PORT" -lt 1 ] || [ "$KVM_PORT" -gt 65535 ]; then
        error "无效的端口号: $KVM_PORT，请输入 1-65535 之间的数字"
        exit 1
    fi
    success "网页端口设置为: $KVM_PORT"
}

write_env() {
    info "写入并补齐环境配置..."
    mkdir -p "$INSTALL_DIR"
    touch "$ENV_FILE"
    chmod 600 "$ENV_FILE"

    # === 关键配置：任何模式下都必须写入或补齐 ===
    env_set "KVM_PORT" "$KVM_PORT"
    env_default "KVM_DB_PATH" "${INSTALL_DIR}/data/kvm_console.db"
    env_default "KVM_JWT_SECRET" "$(random_secret)"
    env_default "KVM_JWT_SECRET_ROTATE_HOURS" "24"

    if [ "$MODE" = "install" ] || [ "$MODE" = "repair" ]; then
        env_default "KVM_VM_CREDENTIAL_SECRET" "$(random_secret)"
        env_default "KVM_SECURITY_SECRET" "$(random_secret)"
    else
        # 旧版本升级时保持空值，让程序继续回退到 KVM_JWT_SECRET，避免历史加密数据无法解密。
        env_default "KVM_VM_CREDENTIAL_SECRET" ""
        env_default "KVM_SECURITY_SECRET" ""
    fi

    env_default "KVM_JWT_EXPIRE_HOURS" "24"
    env_default "KVM_PORTFORWARD_DIR" "$PORT_FORWARD_DIR"
    env_default "KVM_VM_ACCESS_DIR" "$VM_ACCESS_DIR"
    env_default "KVM_ADMIN_USER" "admin"
    env_default "KVM_ADMIN_PASS" "admin123"
    env_default "KVM_SERVICE_UNIT_NAME" "${SERVICE_NAME}.service"
    env_default "KVM_SMTP_PASSWORD_ENC" ""

    # === 以下为可配置项：仅首次安装或修复时写入默认值 ===
    # 更新时跳过，保持 .env 现有内容不动，面板保存设置时会同步写 .env
    if [ "$MODE" = "install" ] || [ "$MODE" = "repair" ]; then
        env_default "KVM_TEMPLATE_DIR" "/var/lib/libvirt/images/templates"
        env_default "KVM_TEMPLATE_IMPORT_DIR" "/var/lib/libvirt/images/templates/_imports"
        env_default "KVM_TEMPLATE_EXPORT_DIR" "/var/lib/libvirt/images/templates/_exports"
        env_default "KVM_CLONE_DIR" "/var/lib/libvirt/images"
        env_default "KVM_ISO_DIR" "/var/lib/libvirt/images/ISO"
        env_default "KVM_DEFAULT_NETWORK" "default"
        env_default "KVM_NETWORK_BACKEND" "ovs"
        env_default "KVM_OVS_BRIDGE" "br-ovs"
        env_default "KVM_OVS_UPLINK" ""
        env_default "KVM_OVS_DHCP_START" ""
        env_default "KVM_OVS_DHCP_END" ""
        env_default "KVM_SUBNET_PREFIX" "192.168.122"
        env_default "KVM_AUTO_PORT_START" "10000"
        env_default "KVM_AUTO_PORT_END" "20000"
        env_default "KVM_HOST_IP" ""
        env_default "KVM_EXTERNAL_NIC" ""
        env_default "KVM_MAX_BURST_INBOUND" "0"
        env_default "KVM_MAX_BURST_OUTBOUND" "0"
        env_default "KVM_RESCUE_ISO" ""
        env_default "KVM_PUBLIC_BASE_URL" ""
        env_default "KVM_SITE_TITLE" "QVMConsole"
        env_default "KVM_DEVELOPMENT_MODE" "false"
        env_default "KVM_MAINTENANCE_MODE" "false"
        env_default "KVM_MAINTENANCE_SERVICE_UNITS" "kvm-console.service,libvirtd.service,libvirtd.socket,libvirtd-ro.socket,libvirtd-admin.socket"
        env_default "KVM_MAINTENANCE_VM_SHUTDOWN_TIMEOUT_SECONDS" "40"
        env_default "KVM_SMTP_HOST" ""
        env_default "KVM_SMTP_PORT" "587"
        env_default "KVM_SMTP_USERNAME" ""
        env_default "KVM_SMTP_FROM_NAME" "QVMConsole"
        env_default "KVM_SMTP_FROM_ADDRESS" ""
        env_default "KVM_SMTP_SECURITY" "starttls"
        env_default "KVM_SMTP_TIMEOUT_SECONDS" "15"
        env_default "KVM_DYNAMIC_MEMORY_SCHEDULER_ENABLED" "true"
        env_default "KVM_DYNAMIC_MEMORY_INTERVAL_SECONDS" "30"
        env_default "KVM_DYNAMIC_MEMORY_HOST_RESERVE_MB" "2048"
        env_default "KVM_DYNAMIC_MEMORY_HOST_RESERVE_PERCENT" "20"
        env_default "KVM_DYNAMIC_MEMORY_INCREASE_THRESHOLD_PERCENT" "15"
        env_default "KVM_DYNAMIC_MEMORY_RECLAIM_THRESHOLD_PERCENT" "35"
        env_default "KVM_DYNAMIC_MEMORY_COOLDOWN_SECONDS" "120"
        env_default "KVM_DYNAMIC_MEMORY_OBSERVATION_HOURS" "24"
        env_default "KVM_SCHEDULER_EVENT_RETENTION_HOURS" "168"
        env_default "KVM_VPC_SUBNET_PREFIX" "10.200"
        env_default "KVM_VPC_VLAN_START" "100"
        env_default "KVM_VPC_VLAN_END" "4094"
        env_default "KVM_VPC_DNS" "223.5.5.5,223.6.6.6"
        env_default "KVM_VPC_ACL_TABLE" "kvm_console_vpc_acl"
        env_default "KVM_PORT_FORWARD_HTTP_PROBE_ENABLED" "true"
        env_default "KVM_PORT_FORWARD_HTTP_PROBE_INTERVAL_MINUTES" "60"
        env_default "KVM_PORT_FORWARD_HTTP_PROBE_TIMEOUT_SECONDS" "3"
        env_default "KVM_DEFAULT_DISK_IOPS_TOTAL" "0"
        env_default "KVM_DEFAULT_DISK_IOPS_READ" "0"
        env_default "KVM_DEFAULT_DISK_IOPS_WRITE" "0"
        env_default "KVM_BATCH_CLONE_MAX_CONCURRENCY" "10"
    fi

    success "配置文件已准备: $ENV_FILE"
}

load_env_file() {
    if [ -f "$ENV_FILE" ]; then
        set -a
        # shellcheck disable=SC1090
        . "$ENV_FILE"
        set +a
    fi
}

ensure_directories() {
    info "补齐运行目录..."
    load_env_file

    local template_dir="${KVM_TEMPLATE_DIR:-/var/lib/libvirt/images/templates}"
    local import_dir="${KVM_TEMPLATE_IMPORT_DIR:-${template_dir}/_imports}"
    local export_dir="${KVM_TEMPLATE_EXPORT_DIR:-${template_dir}/_exports}"
    local clone_dir="${KVM_CLONE_DIR:-/var/lib/libvirt/images}"
    local iso_dir="${KVM_ISO_DIR:-/var/lib/libvirt/images/ISO}"

    mkdir -p \
        "${INSTALL_DIR}/data" \
        "$template_dir" \
        "$import_dir" \
        "$export_dir" \
        "$clone_dir" \
        "$iso_dir" \
        "$PORT_FORWARD_DIR/backups" \
        "$VM_ACCESS_DIR" \
        "$FIREWALL_DIR/backups" \
        "$VPC_CONFIG_DIR" \
        "$OVS_CONFIG_DIR" \
        "$OVS_STATE_DIR" \
        "$STORAGE_MOUNT" \
        "/etc/ssh/sshd_config.d"

    touch "$OVS_CONFIG_DIR/dhcp-hosts"
    touch /etc/projects /etc/projid

    if getent group vmoperator >/dev/null 2>&1; then
        true
    else
        groupadd -f vmoperator
    fi

    local qemu_user=""
    if id libvirt-qemu >/dev/null 2>&1; then
        qemu_user="libvirt-qemu"
    elif id qemu >/dev/null 2>&1; then
        qemu_user="qemu"
    fi
    if [ -n "$qemu_user" ] && getent group kvm >/dev/null 2>&1; then
        chown "$qemu_user:kvm" "$template_dir" "$import_dir" "$export_dir" "$clone_dir" "$iso_dir" 2>/dev/null || true
        chmod 775 "$template_dir" "$import_dir" "$export_dir" "$clone_dir" "$iso_dir" 2>/dev/null || true
        find "$template_dir" -type f \( -name '*.qcow2' -o -name '*.img' -o -name '*.raw' \) -exec chown "$qemu_user:kvm" {} + 2>/dev/null || true
        find "$template_dir" -type f \( -name '*.qcow2' -o -name '*.img' -o -name '*.raw' \) -exec chmod u+rw {} + 2>/dev/null || true
    fi

    success "运行目录已补齐"
}

ensure_apparmor_storage_access() {
    if [ ! -d /sys/module/apparmor ] || [ ! -d /etc/apparmor.d ]; then
        return 0
    fi

    info "配置 libvirt 自定义存储 AppArmor 访问规则..."
    load_env_file
    mkdir -p /etc/apparmor.d/local /etc/apparmor.d/abstractions/libvirt-qemu.d

    local marker="# BEGIN kvm_console managed storage access"
    local marker_end="# END kvm_console managed storage access"
    local helper_file="/etc/apparmor.d/local/usr.lib.libvirt.virt-aa-helper"
    local qemu_file="/etc/apparmor.d/abstractions/libvirt-qemu.d/kvm-console-storage"
    local storage_root="/var/lib/kvm-storage"
    local template_dir="${KVM_TEMPLATE_DIR:-/var/lib/libvirt/images/templates}"
    local user_storage_root="$STORAGE_MOUNT"

    touch "$helper_file" "$qemu_file"

    write_managed_apparmor_block() {
        local file="$1"
        local permission="$2"
        local tmp
        tmp="$(mktemp)"
        awk -v begin="$marker" -v end="$marker_end" '
            $0 == begin { skip = 1; next }
            $0 == end { skip = 0; next }
            !skip { print }
        ' "$file" >"$tmp"

        {
            cat "$tmp"
            printf '\n%s\n' "$marker"
            for root in "$storage_root" "$user_storage_root" "$template_dir"; do
                root="${root%/}"
                [ -n "$root" ] || continue
                printf '%s/ r,\n' "$root"
                printf '%s/**/ r,\n' "$root"
                printf '%s/** %s,\n' "$root" "$permission"
            done
            printf '%s\n' "$marker_end"
        } >"$file"

        rm -f "$tmp"
    }

    write_managed_apparmor_block "$helper_file" "r"
    write_managed_apparmor_block "$qemu_file" "rwk"

    if command -v apparmor_parser >/dev/null 2>&1 && [ -f /etc/apparmor.d/usr.lib.libvirt.virt-aa-helper ]; then
        apparmor_parser -r /etc/apparmor.d/usr.lib.libvirt.virt-aa-helper 2>/dev/null || warn "virt-aa-helper AppArmor 规则重载失败，后续启动 VM 时会再次尝试修复"
    fi
}

detect_default_uplink() {
    ip route show default 2>/dev/null | awk '{print $5; exit}'
}

ensure_sysctl_network() {
    info "启用 IPv4 转发..."
    cat >/etc/sysctl.d/99-kvm-console-network.conf <<'EOF'
net.ipv4.ip_forward=1
EOF
    sysctl -p /etc/sysctl.d/99-kvm-console-network.conf >/dev/null || true
}

ensure_local_dnsmasq_input_rules() {
    local iface="$1"
    [ -n "$iface" ] || return 0

    local rule proto port
    for rule in "udp 67" "udp 53" "tcp 53"; do
        proto="${rule%% *}"
        port="${rule##* }"
        iptables -C INPUT -i "$iface" -p "$proto" --dport "$port" -j ACCEPT 2>/dev/null || \
            iptables -I INPUT 1 -i "$iface" -p "$proto" --dport "$port" -j ACCEPT
    done
}

ensure_existing_vpc_dnsmasq_input_rules() {
    command -v ovs-vsctl >/dev/null 2>&1 || return 0

    local iface
    ovs-vsctl --format=csv --data=bare --no-heading --columns=name find Interface type=internal 2>/dev/null | while IFS= read -r iface; do
        case "$iface" in
            vpcsw*) ensure_local_dnsmasq_input_rules "$iface" ;;
        esac
    done
}

wait_unit_active() {
    local unit="$1"
    local max_wait="${2:-6}"
    local i
    for ((i = 0; i < max_wait; i++)); do
        if systemctl is-active --quiet "$unit" 2>/dev/null; then
            return 0
        fi
        sleep 1
    done
    return 1
}

restart_ovs_dnsmasq_service() {
    if systemctl restart "$OVS_DNSMASQ_UNIT" >/dev/null 2>&1; then
        success "OVS DHCP 服务已启动"
        return 0
    fi

    # dnsmasq 旧进程释放监听地址可能略慢，systemd 的 Restart=on-failure 会自动重试。
    if wait_unit_active "$OVS_DNSMASQ_UNIT" 8; then
        success "OVS DHCP 服务已在 systemd 自动重试后启动"
        return 0
    fi

    warn "OVS DHCP 服务暂未启动成功，可在面板 OVS 诊断中执行修复，或查看: journalctl -u ${OVS_DNSMASQ_UNIT} -n 80 --no-pager"
    return 0
}

setup_ovs_foundation() {
    info "准备 OVS 网络地基..."
    load_env_file
    local bridge="${KVM_OVS_BRIDGE:-br-ovs}"
    local subnet="${KVM_SUBNET_PREFIX:-192.168.122}"
    local gateway="${subnet}.1"
    local dhcp_start="${KVM_OVS_DHCP_START:-${subnet}.2}"
    local dhcp_end="${KVM_OVS_DHCP_END:-${subnet}.254}"
    local uplink="${KVM_OVS_UPLINK:-}"

    if [ -z "$uplink" ]; then
        uplink=$(detect_default_uplink)
    fi
    if [ -z "$uplink" ]; then
        warn "未检测到默认出口网卡，OVS NAT 将在面板网络修复时再次尝试。也可在 $ENV_FILE 配置 KVM_OVS_UPLINK"
    fi

    systemctl enable --now openvswitch-switch 2>/dev/null || true
    ovs-vsctl --may-exist add-br "$bridge"
    ip link set "$bridge" up
    if ! ip -4 addr show dev "$bridge" | grep -q "${gateway}/24"; then
        ip addr flush dev "$bridge" 2>/dev/null || true
        ip addr add "${gateway}/24" dev "$bridge"
    fi
    ensure_local_dnsmasq_input_rules "$bridge"
    ensure_existing_vpc_dnsmasq_input_rules

    cat >"${OVS_CONFIG_DIR}/dnsmasq.conf" <<EOF
interface=${bridge}
bind-interfaces
except-interface=lo
dhcp-authoritative
dhcp-range=${dhcp_start},${dhcp_end},255.255.255.0,12h
dhcp-option=option:router,${gateway}
dhcp-option=option:dns-server,223.5.5.5,223.6.6.6
dhcp-hostsfile=${OVS_CONFIG_DIR}/dhcp-hosts
dhcp-leasefile=${OVS_STATE_DIR}/dnsmasq.leases
pid-file=/run/kvm-console-ovs-dnsmasq.pid
log-dhcp
EOF

    cat >"${OVS_CONFIG_DIR}/prepare-bridge.sh" <<EOF
#!/bin/bash
set -e
BRIDGE="${bridge}"
GATEWAY="${gateway}/24"
ovs-vsctl --may-exist add-br "\$BRIDGE"
ip link set "\$BRIDGE" up
if ! ip -4 addr show dev "\$BRIDGE" | grep -q "\$GATEWAY"; then
  ip addr flush dev "\$BRIDGE" 2>/dev/null || true
  ip addr add "\$GATEWAY" dev "\$BRIDGE"
fi
for rule in "udp 67" "udp 53" "tcp 53"; do
  proto="\${rule%% *}"
  port="\${rule##* }"
  iptables -C INPUT -i "\$BRIDGE" -p "\$proto" --dport "\$port" -j ACCEPT 2>/dev/null || \\
    iptables -I INPUT 1 -i "\$BRIDGE" -p "\$proto" --dport "\$port" -j ACCEPT
done
EOF
    chmod +x "${OVS_CONFIG_DIR}/prepare-bridge.sh"

    cat >"$OVS_DNSMASQ_SERVICE_FILE" <<EOF
[Unit]
Description=KVM Console OVS DHCP/DNS service
After=network-online.target openvswitch-switch.service
Wants=network-online.target openvswitch-switch.service

[Service]
Type=forking
PIDFile=/run/kvm-console-ovs-dnsmasq.pid
ExecStartPre=/bin/bash ${OVS_CONFIG_DIR}/prepare-bridge.sh
ExecStart=/usr/sbin/dnsmasq --conf-file=${OVS_CONFIG_DIR}/dnsmasq.conf
ExecReload=/bin/kill -HUP \$MAINPID
Restart=on-failure

[Install]
WantedBy=multi-user.target
EOF

    systemctl daemon-reload
    systemctl enable "$OVS_DNSMASQ_UNIT" >/dev/null 2>&1 || true
    restart_ovs_dnsmasq_service

    if [ -n "$uplink" ]; then
        iptables -t nat -C POSTROUTING -s "${subnet}.0/24" -o "$uplink" -j MASQUERADE 2>/dev/null || \
            iptables -t nat -A POSTROUTING -s "${subnet}.0/24" -o "$uplink" -j MASQUERADE
        iptables -C FORWARD -i "$bridge" -o "$uplink" -j ACCEPT 2>/dev/null || \
            iptables -A FORWARD -i "$bridge" -o "$uplink" -j ACCEPT
        iptables -C FORWARD -i "$uplink" -o "$bridge" -m conntrack --ctstate RELATED,ESTABLISHED -j ACCEPT 2>/dev/null || \
            iptables -A FORWARD -i "$uplink" -o "$bridge" -m conntrack --ctstate RELATED,ESTABLISHED -j ACCEPT
    fi

    if virsh net-info default >/dev/null 2>&1; then
        virsh net-destroy default >/dev/null 2>&1 || true
        virsh net-autostart default --disable >/dev/null 2>&1 || true
    fi
    success "OVS 网络地基已准备"
}

setup_sshd_foundation() {
    if [ -f /etc/ssh/sshd_config ] && ! grep -q 'Include /etc/ssh/sshd_config.d/' /etc/ssh/sshd_config; then
        sed -i '1i Include /etc/ssh/sshd_config.d/*.conf' /etc/ssh/sshd_config
    fi
    systemctl reload sshd 2>/dev/null || systemctl reload ssh 2>/dev/null || true
}

extract_tarball() {
    local tarball_path="$1"
    info "正在解压发行包: $tarball_path"
    TMP_RELEASE_DIR=$(mktemp -d)
    tar -xzf "$tarball_path" -C "$TMP_RELEASE_DIR"

    local found_bin
    found_bin=$(find "$TMP_RELEASE_DIR" -maxdepth 3 -name "kvm-console" -type f -perm /111 2>/dev/null | head -1)
    if [ -z "$found_bin" ]; then
        error "发行包中未找到 kvm-console 可执行文件"
        exit 1
    fi
    RELEASE_SOURCE_DIR=$(dirname "$found_bin")
    if [ ! -d "${RELEASE_SOURCE_DIR}/web-dist" ]; then
        error "发行包中未找到 web-dist 前端文件"
        exit 1
    fi
    success "发行包解压完成"
}

get_release() {
    local script_dir
    script_dir="$(cd "$(dirname "$0")" && pwd)"
    if [ -f "${script_dir}/kvm-console" ] && [ -d "${script_dir}/web-dist" ]; then
        info "检测到本地发行目录，使用本地文件"
        RELEASE_SOURCE_DIR="$script_dir"
        return
    fi

    local local_tarball_name
    if [ "$ARCH" = "x86_64" ]; then
        local_tarball_name="kvm-console-linux-amd64.tar.gz"
    elif [ "$ARCH" = "aarch64" ]; then
        local_tarball_name="kvm-console-linux-arm64.tar.gz"
    fi
    local local_tarball=""
    if [ -f "$(pwd)/${local_tarball_name}" ]; then
        local_tarball="$(pwd)/${local_tarball_name}"
        read -rp "检测到本地发行包 ${local_tarball}，是否使用? [Y/n]: " use_local
        use_local=${use_local:-Y}
        if [[ "$use_local" =~ ^[Yy]$ ]]; then
            extract_tarball "$local_tarball"
            return
        fi
    fi

    echo ""
    echo -e "  ${CYAN}1.${NC} 输入本地 tar.gz 文件路径"
    echo -e "  ${CYAN}2.${NC} 从 GitHub Releases 下载最新版本"
    echo ""
    local install_choice
    read -rp "请选择安装包来源 [1/2，默认 2]: " install_choice
    install_choice=${install_choice:-2}

    if [ "$install_choice" = "1" ]; then
        local user_tarball
        read -rp "请输入 tar.gz 文件完整路径: " user_tarball
        user_tarball="${user_tarball/#\~/$HOME}"
        if [ ! -f "$user_tarball" ]; then
            error "文件不存在: $user_tarball"
            exit 1
        fi
        extract_tarball "$user_tarball"
        return
    fi

    info "从 GitHub 获取最新版本信息..."
    local release_info
    release_info=$(curl -fsSL "$GITHUB_API") || {
        error "无法连接 GitHub API，请检查网络或使用离线发行包"
        exit 1
    }
    local arch_suffix
    if [ "$ARCH" = "x86_64" ]; then
        arch_suffix="amd64"
    elif [ "$ARCH" = "aarch64" ]; then
        arch_suffix="arm64"
    fi
    local download_url
    local tag_name
    download_url=$(printf '%s' "$release_info" | awk -F'"' -v s="linux-${arch_suffix}.tar.gz" '/browser_download_url/ && $0 ~ s {print $4; exit}')
    tag_name=$(printf '%s' "$release_info" | awk -F'"' '/tag_name/ {print $4; exit}')
    if [ -z "$download_url" ]; then
        error "未找到 linux-${arch_suffix}.tar.gz 发行包"
        exit 1
    fi

    info "最新版本: ${tag_name:-unknown}"
    TMP_RELEASE_DIR=$(mktemp -d)
    curl -L --progress-bar -o "${TMP_RELEASE_DIR}/kvm-console-linux-amd64.tar.gz" "$download_url"
    extract_tarball "${TMP_RELEASE_DIR}/kvm-console-linux-amd64.tar.gz"
}

install_files() {
    if [ "$MODE" = "update" ]; then
        info "停止 ${APP_NAME} 服务..."
        systemctl stop "$SERVICE_NAME" 2>/dev/null || true
    fi

    mkdir -p "$INSTALL_DIR/data"
    info "安装后端程序..."
    cp -f "${RELEASE_SOURCE_DIR}/kvm-console" "${INSTALL_DIR}/kvm-console"
    chmod +x "${INSTALL_DIR}/kvm-console"

    info "安装前端静态文件..."
    rm -rf "${INSTALL_DIR}/web-dist"
    cp -r "${RELEASE_SOURCE_DIR}/web-dist" "${INSTALL_DIR}/web-dist"
    success "程序文件已安装"
}

setup_service() {
    info "配置 systemd 服务..."
    cat >"$SERVICE_FILE" <<EOF
[Unit]
Description=${APP_NAME} 虚拟机管理平台
After=network-online.target libvirtd.service openvswitch-switch.service
Wants=network-online.target libvirtd.service openvswitch-switch.service

[Service]
Type=simple
WorkingDirectory=${INSTALL_DIR}
EnvironmentFile=${ENV_FILE}
ExecStart=${INSTALL_DIR}/kvm-console
Restart=on-failure
RestartSec=5
LimitNOFILE=65536
StandardOutput=journal
StandardError=journal
SyslogIdentifier=kvm-console

[Install]
WantedBy=multi-user.target
EOF
    systemctl daemon-reload
    systemctl enable "$SERVICE_NAME"
    success "systemd 服务已配置"
}

start_service() {
    info "启动 ${APP_NAME} 服务..."
    systemctl restart "$SERVICE_NAME"
    sleep 2
    if systemctl is-active --quiet "$SERVICE_NAME"; then
        success "${APP_NAME} 服务启动成功"
    else
        error "服务启动失败，请查看日志: journalctl -u $SERVICE_NAME -f"
        exit 1
    fi
}

uninstall_app() {
    echo ""
    warn "卸载不会删除已有虚拟机磁盘、模板、libvirt 定义和用户存储镜像，除非你手动清理。"
    read -rp "确认卸载 ${APP_NAME}? 请输入 UNINSTALL 确认: " confirm
    if [ "$confirm" != "UNINSTALL" ]; then
        warn "已取消卸载"
        return
    fi

    systemctl stop "$SERVICE_NAME" 2>/dev/null || true
    systemctl disable "$SERVICE_NAME" 2>/dev/null || true
    rm -f "$SERVICE_FILE"

    read -rp "是否同时停用 OVS DHCP 辅助服务? [Y/n]: " stop_ovs
    stop_ovs=${stop_ovs:-Y}
    if [[ "$stop_ovs" =~ ^[Yy]$ ]]; then
        systemctl disable --now "$OVS_DNSMASQ_UNIT" 2>/dev/null || true
        rm -f "$OVS_DNSMASQ_SERVICE_FILE"
    fi

    systemctl daemon-reload

    read -rp "是否删除安装目录 ${INSTALL_DIR}（包含数据库和配置）? [y/N]: " purge
    purge=${purge:-N}
    if [[ "$purge" =~ ^[Yy]$ ]]; then
        rm -rf "$INSTALL_DIR"
        success "安装目录已删除"
    else
        rm -f "${INSTALL_DIR}/kvm-console"
        rm -rf "${INSTALL_DIR}/web-dist"
        warn "已保留 ${INSTALL_DIR}/data 与 ${ENV_FILE}"
    fi

    success "${APP_NAME} 已卸载"
}

show_info() {
    local host_ip
    host_ip=$(hostname -I 2>/dev/null | awk '{print $1}')
    host_ip=${host_ip:-localhost}

    echo ""
    echo -e "${CYAN}╔══════════════════════════════════════════════════╗${NC}"
    if [ "$MODE" = "install" ]; then
        echo -e "${CYAN}║       ${APP_NAME} 安装完成！                     ║${NC}"
    else
        echo -e "${CYAN}║       ${APP_NAME} 更新完成！                     ║${NC}"
    fi
    echo -e "${CYAN}╠══════════════════════════════════════════════════╣${NC}"
    echo -e "${CYAN}║${NC}  访问地址: ${GREEN}http://${host_ip}:${KVM_PORT}${NC}"
    echo -e "${CYAN}║${NC}  安装目录: ${GREEN}${INSTALL_DIR}${NC}"
    echo -e "${CYAN}║${NC}  配置文件: ${GREEN}${ENV_FILE}${NC}"
    if [ "$MODE" = "install" ]; then
        echo -e "${CYAN}║${NC}  默认账号: ${GREEN}admin${NC} / ${GREEN}admin123${NC}"
    fi
    echo -e "${CYAN}╠══════════════════════════════════════════════════╣${NC}"
    echo -e "${CYAN}║${NC}  查看状态: systemctl status $SERVICE_NAME"
    echo -e "${CYAN}║${NC}  查看日志: journalctl -u $SERVICE_NAME -f"
    echo -e "${CYAN}║${NC}  重启服务: systemctl restart $SERVICE_NAME"
    echo -e "${CYAN}╚══════════════════════════════════════════════════╝${NC}"
    echo ""
}

run_install_or_update() {
    check_kvm_hardware
    check_and_install_deps
    ensure_kvm_runtime
    setup_quota
    configure_port
    get_release
    install_files
    write_env
    ensure_directories
    ensure_apparmor_storage_access
    ensure_sysctl_network
    setup_ovs_foundation
    setup_sshd_foundation
    setup_service
    start_service
    show_info
}

# 修复配置文件：将 .env 重置为默认值并重启服务
repair_config() {
    echo ""
    warn "修复配置文件将把 ${ENV_FILE} 重置为默认值，已有的自定义配置将被覆盖。"
    read -rp "确认重置配置文件? [y/N]: " confirm
    confirm=${confirm:-N}
    if [[ ! "$confirm" =~ ^[Yy]$ ]]; then
        warn "已取消修复"
        return
    fi

    write_env
    success "配置文件已重置为默认值"
    info "重启面板服务使配置生效..."
    systemctl restart "$SERVICE_NAME"
    sleep 2
    if systemctl is-active --quiet "$SERVICE_NAME"; then
        success "面板服务已重启，配置文件已修复"
    else
        warn "服务启动异常，请查看日志: journalctl -u $SERVICE_NAME -f"
    fi
}

main() {
    echo ""
    echo -e "${CYAN}╔══════════════════════════════════════════════════╗${NC}"
    echo -e "${CYAN}║         ${APP_NAME} 安装 / 更新 / 卸载脚本        ║${NC}"
    echo -e "${CYAN}╚══════════════════════════════════════════════════╝${NC}"
    echo ""

    check_root
    check_os
    check_arch
    choose_mode

    case "$MODE" in
        install|update)
            run_install_or_update
            ;;
        repair)
            repair_config
            ;;
        uninstall)
            uninstall_app
            ;;
        *)
            error "未知模式: $MODE"
            exit 1
            ;;
    esac
}

main "$@"
