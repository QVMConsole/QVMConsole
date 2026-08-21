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
INSTALL_LAUNCH_DIR="$PWD"
COMPATIBILITY_CHECK_SCRIPT="check-system-compatibility.sh"
# 首次安装兼容性脚本下载地址，发布前填入正式地址。
COMPATIBILITY_CHECK_URL="https://download.xiaozhuhouses.asia/download/v1/links/qhnoBKgQhgqxdXFxnZIW95hjerBS3L7HBUAo0GNg8Do"
COMPATIBILITY_REPORT_DIR="${INSTALL_DIR}/logs/compatibility"
COMPATIBILITY_SCRIPT_PATH=""
COMPATIBILITY_DOWNLOAD_TMP=""
COMPATIBILITY_WARNING=0
COMPATIBILITY_SKIPPED=0
COMPATIBILITY_FAILURE_STAGE=""
COMPATIBILITY_CHECK_STATUS=0
COMPATIBILITY_INTERRUPTED=0
# 开源版官方下载源（按架构区分）
DOWNLOAD_URL_AMD64="https://download.xiaozhuhouses.asia/download/v1/links/YsxWkWgFPiZFrc8I0r2F8SpdLbhBA_O7PMnD0TDS0wM"
DOWNLOAD_URL_ARM64="https://download.xiaozhuhouses.asia/download/v1/links/SSr8OGj6KLbxHHKK746R_-CvpoFj1Skh9XIkjkNNzZ0"

STORAGE_IMG="/var/lib/kvm-user-storage.img"
STORAGE_MOUNT="/var/lib/kvm-user-storage"
STORAGE_DEFAULT_BACKING_DIR="/var/lib"
STORAGE_IMG_FILENAME="kvm-user-storage.img"
OVS_CONFIG_DIR="/etc/kvm-console/ovs"
OVS_STATE_DIR="/var/lib/kvm-console/ovs"
OVS_DNSMASQ_UNIT="kvm-console-ovs-dnsmasq.service"
OVS_DNSMASQ_SERVICE_FILE="/etc/systemd/system/${OVS_DNSMASQ_UNIT}"
DEFAULT_OVS_SUBNET_PREFIX="192.168.122"
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
    "dmidecode"
)

# 架构特有依赖：在 check_and_install_deps 中根据 $ARCH 动态追加
QEMU_PKG_X86="qemu-system-x86"
EFI_PKG_X86="ovmf"
QEMU_PKG_ARM="qemu-system-arm"
EFI_PKG_ARM="qemu-efi-aarch64"

# ==================== RPM 系发行版包名映射 ====================
# key = APT_DEPS 中的 Debian 包名，value = 对应的 RPM 包名
# 注意：openEuler/麒麟 的 libvirt 为单一包（含 daemon+client），非 Debian 拆分方式
declare -A RPM_PKG_MAP
RPM_PKG_MAP=(
    ["ca-certificates"]="ca-certificates"
    ["curl"]="curl"
    ["tar"]="tar"
    ["gzip"]="gzip"
    ["qemu-utils"]="qemu-img"
    ["libvirt-daemon-system"]="libvirt"
    ["libvirt-daemon-driver-qemu"]=""          # openEuler 上已包含在 libvirt 包中
    ["libvirt-clients"]="libvirt-client"
    ["openvswitch-switch"]="openvswitch"
    ["dnsmasq-base"]="dnsmasq"
    ["virtinst"]="virt-install"
    ["libguestfs-tools"]="libguestfs-tools"
    ["ntfs-3g"]="ntfs-3g"
    ["genisoimage"]="genisoimage"
    ["sshpass"]="sshpass"
    ["cloud-image-utils"]="cloud-utils"
    ["lvm2"]="lvm2"
    ["cloud-guest-utils"]="cloud-utils-growpart"
    ["quota"]="quota"
    ["e2fsprogs"]="e2fsprogs"
    ["util-linux"]="util-linux"
    ["nftables"]="nftables"
    ["iproute2"]="iproute"
    ["iptables"]="iptables"
    ["tcpdump"]="tcpdump"
    ["ufw"]="firewalld"
    ["nmap"]="nmap"
    ["arp-scan"]="arp-scan"
    ["conntrack"]="conntrack-tools"
    ["openssh-client"]="openssh-clients"
    ["openssh-server"]="openssh-server"
    ["parted"]="parted"
    ["dmidecode"]="dmidecode"
)

# RPM 系架构特有包名（openEuler 官方文档确认）
# QEMU: openEuler 24.03 用 qemu-kvm，部分旧版/麒麟可能只有 qemu，安装时自动回退
# UEFI: x86 用 edk2-ovmf，AArch64 用 edk2-aarch64
QEMU_PKG_X86_RPM="qemu-kvm"
QEMU_PKG_X86_RPM_FALLBACK="qemu"
EFI_PKG_X86_RPM="edk2-ovmf"
QEMU_PKG_ARM_RPM="qemu-kvm"
QEMU_PKG_ARM_RPM_FALLBACK="qemu"
EFI_PKG_ARM_RPM="edk2-aarch64"

# RPM 系中可能不存在的可选包（缺失时不报错，仅警告）
# 这些包在部分麒麟/openEuler 源中可能不存在或包名不同
# 注意：genisoimage 在 apt 系是 APT_DEPS 必装项，此处标记为 RPM 可选（可用 xorriso 替代）
RPM_PKG_SOFT=(
    "libguestfs-tools"
    "cloud-utils"
    "cloud-utils-growpart"
    "genisoimage"
    "arp-scan"
)

PKG_MGR=""

# ==================== 包管理器辅助函数 ====================

# detect_pkg_manager 检测当前系统的包管理器 (apt/dnf/yum)
detect_pkg_manager() {
    if [ -f /etc/os-release ]; then
        . /etc/os-release
        local os_id="${ID:-}"
        local os_like="${ID_LIKE:-}"
        os_id="${os_id,,}"
        os_like="${os_like,,}"

        # Debian/Ubuntu 系列
        if [[ "$os_id" == "ubuntu" ]] || [[ "$os_id" == "debian" ]]; then
            PKG_MGR="apt"
            return
        fi
        if [[ "$os_like" == *"debian"* ]] || [[ "$os_like" == *"ubuntu"* ]]; then
            if command -v apt-get &>/dev/null; then
                PKG_MGR="apt"
                return
            fi
        fi

        # 显式识别已知 RPM 系发行版
        case "$os_id" in
            kylin|neokylin|openEuler|centos|rhel|anolis|rocky|alma|fedora)
                if command -v dnf &>/dev/null; then
                    PKG_MGR="dnf"
                elif command -v yum &>/dev/null; then
                    PKG_MGR="yum"
                fi
                return
                ;;
        esac
        if [[ "$os_like" == *"rhel"* ]] || [[ "$os_like" == *"fedora"* ]] || [[ "$os_like" == *"kylin"* ]] || [[ "$os_like" == *"openeuler"* ]]; then
            if command -v dnf &>/dev/null; then
                PKG_MGR="dnf"
                return
            fi
            if command -v yum &>/dev/null; then
                PKG_MGR="yum"
                return
            fi
        fi

        # 通用 RPM 回退：优先 dnf，回退 yum
        if command -v dnf &>/dev/null; then
            PKG_MGR="dnf"
            return
        fi
        if command -v yum &>/dev/null; then
            PKG_MGR="yum"
            return
        fi
    fi

    # 最终回退：按命令可用性检测
    if command -v apt-get &>/dev/null; then
        PKG_MGR="apt"
    elif command -v dnf &>/dev/null; then
        PKG_MGR="dnf"
    elif command -v yum &>/dev/null; then
        PKG_MGR="yum"
    else
        error "未检测到支持的包管理器 (apt/dnf/yum)"
        exit 1
    fi
}

# pkg_name 将 Debian 包名转换为当前系统的 RPM 包名，Debian 系原样返回
pkg_name() {
    case "$PKG_MGR" in
        apt) echo "$1" ;;
        dnf|yum)
            local rpm_name="${RPM_PKG_MAP[$1]:-}"
            if [ -z "$rpm_name" ]; then
                # 无映射则跳过（该包在此发行版不可用）
                return 1
            fi
            echo "$rpm_name"
            ;;
    esac
}

# pkg_install 安装指定包
pkg_install() {
    case "$PKG_MGR" in
        apt) DEBIAN_FRONTEND=noninteractive apt-get install -y "$@" ;;
        dnf) dnf install -y "$@" ;;
        yum) yum install -y "$@" ;;
    esac
}

# pkg_update_index 更新包索引
pkg_update_index() {
    case "$PKG_MGR" in
        apt) apt-get update ;;
        dnf|yum) : ;;  # RPM 系通常不需要单独更新索引
    esac
}

# is_pkg_installed 检查指定包是否已安装
is_pkg_installed() {
    case "$PKG_MGR" in
        apt) dpkg-query -W -f='${Status}' "$1" 2>/dev/null | grep -q "install ok installed" ;;
        dnf|yum) rpm -q "$1" &>/dev/null ;;
    esac
}

# pkg_is_available 检查包在当前源中是否可用（仅 RPM 系）
# 注意：dnf repoquery 需要 dnf-plugins-core，缺失时自动安装
pkg_is_available() {
    case "$PKG_MGR" in
        apt) apt-cache show "$1" &>/dev/null ;;
        dnf)
            # dnf repoquery 需要 dnf-plugins-core，缺失时先安装
            if ! dnf repoquery --available "$1" &>/dev/null 2>&1; then
                if ! rpm -q dnf-plugins-core &>/dev/null 2>&1; then
                    dnf install -y dnf-plugins-core &>/dev/null 2>&1 || true
                fi
                dnf repoquery --available "$1" &>/dev/null
            fi
            ;;
        yum) yum list available "$1" &>/dev/null ;;
        *) return 1 ;;
    esac
}

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
    "ovsdb-client"
    "dnsmasq"
    "nft"
    "ip"
    "iptables"
    "ip6tables"
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
    if [ -n "${COMPATIBILITY_DOWNLOAD_TMP:-}" ] && [ -f "$COMPATIBILITY_DOWNLOAD_TMP" ]; then
        rm -f "$COMPATIBILITY_DOWNLOAD_TMP"
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
        error "无法识别操作系统"
        exit 1
    fi
    . /etc/os-release
    detect_pkg_manager
    info "检测到系统: ${PRETTY_NAME:-unknown}，包管理器: $PKG_MGR"
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

check_locale() {
    local lang="${LANG:-}"
    local lc_all="${LC_ALL:-}"
    local current="${lc_all:-$lang}"

    # 如果 LANG 和 LC_ALL 都为空，尝试用 locale 命令获取
    if [ -z "$current" ]; then
        current=$(locale 2>/dev/null | awk -F= '/^LANG=/ {print $2}' | tr -d '"' || true)
    fi

    if [[ "$current" =~ ^en_US\.UTF-8 ]] || [[ "$current" =~ ^C\.UTF-8 ]] || [[ "$current" =~ ^POSIX\.UTF-8 ]]; then
        info "系统语言环境: ${current}"
        return 0
    fi

    cat >&2 <<EOF

[ERROR] 系统语言环境不是英文 UTF-8。

当前检测到: LANG=${lang:-（空）}${lc_all:+ , LC_ALL=${lc_all}}

QVMConsole 大部分功能依赖命令返回的信息进行正确识别，
非英文环境下可能导致错误匹配逻辑失效。造成功能异常

请将系统语言环境设置为 en_US.UTF-8 后重启系统再执行安装，例如：

    sudo localectl set-locale LANG=en_US.UTF-8
    # 或
    export LANG=en_US.UTF-8

然后重新运行此安装脚本。
EOF
    exit 1
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

install_optional_polkit() {
    if command -v pkaction >/dev/null 2>&1 || systemctl list-unit-files 2>/dev/null | grep -q '^polkit\.service'; then
        return
    fi
    info "补充安装 polkit 组件..."
    case "$PKG_MGR" in
        apt)
            if apt-cache show polkitd >/dev/null 2>&1; then
                pkg_install polkitd
            elif apt-cache show policykit-1 >/dev/null 2>&1; then
                pkg_install policykit-1
            else
                warn "未找到 polkitd / policykit-1 包，用户级 libvirt 授权可能需要手动检查"
            fi
            ;;
        dnf|yum)
            pkg_install polkit 2>/dev/null || warn "未找到 polkit 包，用户级 libvirt 授权可能需要手动检查"
            ;;
    esac
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

# install_optional_virt_fw_vars 安装 UEFI 变量编辑工具。
# 该工具用于让 shim/fallback.efi 首次登记启动项后直接继续引导；缺失时不影响虚拟机创建。
install_optional_virt_fw_vars() {
    if command -v virt-fw-vars >/dev/null 2>&1 &&
       virt-fw-vars --help 2>&1 | grep -q -- '--set-fallback-no-reboot'; then
        success "virt-fw-vars 已可用"
        return
    fi

    info "尝试安装可选 UEFI 变量工具 virt-fw-vars..."
    case "$PKG_MGR" in
        apt)
            if pkg_is_available python3-virt-firmware 2>/dev/null; then
                pkg_install python3-virt-firmware >/dev/null 2>&1 || true
            fi
            ;;
        dnf|yum)
            pkg_install python3-virt-firmware >/dev/null 2>&1 ||
                pkg_install virt-firmware >/dev/null 2>&1 || true
            ;;
    esac

    if command -v virt-fw-vars >/dev/null 2>&1 &&
       virt-fw-vars --help 2>&1 | grep -q -- '--set-fallback-no-reboot'; then
        success "virt-fw-vars 安装成功"
    else
        warn "virt-fw-vars 不可用或版本过旧，UEFI 克隆首次启动时可能短暂显示启动项恢复界面"
    fi
}

check_and_install_deps() {
    info "检查宿主机依赖包..."
    local missing=()
    local pkg

    # 根据架构动态确定依赖列表
    local deps=("${APT_DEPS[@]}")
    local qemu_pkg_rpm=""
    local qemu_pkg_rpm_fallback=""
    if [ "$ARCH" = "x86_64" ]; then
        if [ "$PKG_MGR" = "apt" ]; then
            deps+=("$QEMU_PKG_X86" "$EFI_PKG_X86")
            info "架构: x86_64，QEMU 包: ${QEMU_PKG_X86}，EFI 包: ${EFI_PKG_X86}"
        else
            qemu_pkg_rpm="$QEMU_PKG_X86_RPM"
            qemu_pkg_rpm_fallback="$QEMU_PKG_X86_RPM_FALLBACK"
            deps+=("$qemu_pkg_rpm" "$EFI_PKG_X86_RPM")
            info "架构: x86_64，QEMU 包: ${qemu_pkg_rpm}，EFI 包: ${EFI_PKG_X86_RPM}"
        fi
    elif [ "$ARCH" = "aarch64" ]; then
        if [ "$PKG_MGR" = "apt" ]; then
            deps+=("$QEMU_PKG_ARM" "$EFI_PKG_ARM")
            info "架构: aarch64，QEMU 包: ${QEMU_PKG_ARM}，EFI 包: ${EFI_PKG_ARM}"
        else
            qemu_pkg_rpm="$QEMU_PKG_ARM_RPM"
            qemu_pkg_rpm_fallback="$QEMU_PKG_ARM_RPM_FALLBACK"
            deps+=("$qemu_pkg_rpm" "$EFI_PKG_ARM_RPM")
            info "架构: aarch64，QEMU 包: ${qemu_pkg_rpm}，EFI 包: ${EFI_PKG_ARM_RPM}"
        fi
    fi

    for pkg in "${deps[@]}"; do
        local mapped_pkg
        mapped_pkg=$(pkg_name "$pkg") || continue  # RPM 系无映射的包跳过
        if is_pkg_installed "$mapped_pkg"; then
            success "$pkg 已安装"
        else
            # 检查是否为 RPM 可选软性包
            local is_soft=0
            if [ "$PKG_MGR" != "apt" ]; then
                for soft in "${RPM_PKG_SOFT[@]}"; do
                    if [ "$mapped_pkg" = "$soft" ]; then
                        is_soft=1
                        break
                    fi
                done
            fi
            if [ "$is_soft" -eq 1 ]; then
                # RPM 软性包：跳过主安装流程，由 install_bundled_packages 在后处理中尝试安装
                warn "可选包 $mapped_pkg 跳过系统源安装（将由捆绑包机制处理）"
                continue
            fi
            missing+=("$mapped_pkg")
        fi
    done

    # QEMU 包回退：如果主包名（qemu-kvm）不可用，尝试回退包名（qemu）
    if [ "$PKG_MGR" != "apt" ] && [ -n "$qemu_pkg_rpm" ] && [ -n "$qemu_pkg_rpm_fallback" ]; then
        local qemu_rpm_mapped
        qemu_rpm_mapped=$(pkg_name "$qemu_pkg_rpm" 2>/dev/null || true)
        local qemu_fb_mapped
        qemu_fb_mapped=$(pkg_name "$qemu_pkg_rpm_fallback" 2>/dev/null || true)
        if [ -n "$qemu_rpm_mapped" ] && [ -n "$qemu_fb_mapped" ]; then
            # 检查主包是否在 missing 列表中且未安装
            local qemu_in_missing=0
            local i
            for i in "${!missing[@]}"; do
                if [ "${missing[$i]}" = "$qemu_rpm_mapped" ]; then
                    qemu_in_missing=1
                    # 检查回退包是否已安装或可用
                    if is_pkg_installed "$qemu_fb_mapped" 2>/dev/null; then
                        unset 'missing[i]'
                        success "QEMU 包回退: $qemu_rpm_mapped → $qemu_fb_mapped（已安装）"
                    elif pkg_is_available "$qemu_fb_mapped" 2>/dev/null; then
                        unset 'missing[i]'
                        missing+=("$qemu_fb_mapped")
                        warn "QEMU 包回退: $qemu_rpm_mapped 不可用，改用 $qemu_fb_mapped"
                    fi
                    break
                fi
            done
        fi
    fi

    if [ ${#missing[@]} -gt 0 ]; then
        warn "发现缺失依赖: ${missing[*]}"
        read -rp "是否立即安装缺失依赖? [Y/n]: " confirm
        confirm=${confirm:-Y}
        if [[ ! "$confirm" =~ ^[Yy]$ ]]; then
            error "缺少必要依赖，无法保证面板功能完整运行"
            exit 1
        fi
        info "更新包索引..."
        pkg_update_index
        info "安装缺失依赖..."
        pkg_install "${missing[@]}"
    fi

    # ISO 创建工具回退：genisoimage 不可用时尝试安装 xorriso
    if ! command -v genisoimage >/dev/null 2>&1 && \
       ! command -v xorriso >/dev/null 2>&1 && \
       ! command -v mkisofs >/dev/null 2>&1; then
        warn "未找到 genisoimage/xorriso/mkisofs，尝试安装替代工具..."
        pkg_install xorriso 2>/dev/null || pkg_install genisoimage 2>/dev/null || true
    fi

    install_optional_polkit
    check_optional_kvm_stat
    install_optional_virt_fw_vars
    ensure_required_commands
    ensure_core_services

    # 安装捆绑的 RPM 包（为 Kylin/openEuler 等源中缺失的包提供）
    install_bundled_packages
}

# extract_rpm_cmd 从捆绑 RPM 中提取单个命令到 /usr/local/bin
# 优先使用 rpm2cpio，回退 bsdtar；确保提取工具可用
extract_rpm_cmd() {
    local rpm_file="$1" cmd_name="$2"
    [ -f "$rpm_file" ] || return 1

    if ! command -v rpm2cpio >/dev/null 2>&1 && ! command -v bsdtar >/dev/null 2>&1; then
        dnf install -y rpm-build &>/dev/null 2>&1 || true
    fi

    local tmp
    tmp=$(mktemp -d)

    if command -v rpm2cpio >/dev/null 2>&1; then
        rpm2cpio "$rpm_file" 2>/dev/null | cpio -idm -D "$tmp" 2>/dev/null || { rm -rf "$tmp"; return 1; }
    elif command -v bsdtar >/dev/null 2>&1; then
        bsdtar xf "$rpm_file" -C "$tmp" 2>/dev/null || { rm -rf "$tmp"; return 1; }
    else
        rm -rf "$tmp"
        return 1
    fi

    if [ -f "$tmp/usr/bin/$cmd_name" ]; then
        cp "$tmp/usr/bin/$cmd_name" /usr/local/bin/
        chmod +x "/usr/local/bin/$cmd_name"
        rm -rf "$tmp"
        return 0
    fi
    rm -rf "$tmp"
    return 1
}

# install_bundled_packages 安装 release 中捆绑的 RPM 包，以及重试软性包安装
# 用于 Kylin/openEuler 等系统默认源中缺少的包，或在主流程中跳过的可选包
# 执行顺序：bundled → dnf soft retry → dnf provides fallback
install_bundled_packages() {
    local script_dir
    script_dir="$(cd "$(dirname "$0")" && pwd)"
    local bundled_dir="${script_dir}/bundled"

    # === Phase 1: 优先使用捆绑包（无需网络，快） ===
    if [ -d "$bundled_dir" ]; then
        info "检测到捆绑的 RPM 包目录，优先尝试本地包..."

        # arp-scan: 优先 dnf 安装（依赖少，可能成功）
        if ! command -v arp-scan >/dev/null 2>&1 && [ -f "$bundled_dir/arp-scan.rpm" ]; then
            if dnf install -y "$bundled_dir/arp-scan.rpm" 2>/dev/null || \
               yum install -y "$bundled_dir/arp-scan.rpm" 2>/dev/null; then
                success "arp-scan 安装成功"
            else
                warn "捆绑的 arp-scan 安装失败（可能缺少依赖），ARP 扫描功能将使用 nmap 替代"
            fi
        fi

        # libguestfs-tools-c: 直接提取二进制，跳过 dnf 依赖解析（libguestfs.so.0 已在系统）
        local -a lgft_bins=(virt-filesystems virt-customize guestfish guestmount \
            virt-sysprep virt-sparsify virt-builder virt-resize virt-inspector \
            virt-df virt-diff virt-edit virt-format virt-get-kernel virt-log \
            virt-ls virt-make-fs virt-rescue virt-tail virt-cat virt-alignment-scan)
        if ! command -v virt-filesystems >/dev/null 2>&1 && [ -f "$bundled_dir/libguestfs-tools-c.rpm" ]; then
            local extracted=0
            for cmd in "${lgft_bins[@]}"; do
                if ! command -v "$cmd" >/dev/null 2>&1 && \
                   extract_rpm_cmd "$bundled_dir/libguestfs-tools-c.rpm" "$cmd"; then
                    extracted=$((extracted + 1))
                fi
            done
            if command -v virt-filesystems >/dev/null 2>&1; then
                success "libguestfs-tools-c 提取完成（${extracted} 个工具）"
            fi
        fi

        # virt-win-reg: 从 libguestfs-tools noarch RPM 提取
        if ! command -v virt-win-reg >/dev/null 2>&1 && [ -f "$bundled_dir/libguestfs-tools.rpm" ]; then
            if extract_rpm_cmd "$bundled_dir/libguestfs-tools.rpm" virt-win-reg; then
                success "virt-win-reg 已提取到 /usr/local/bin"
            fi
        fi
    fi

    # === Phase 2: 重试之前在 RPM_PKG_SOFT 中跳过的包（可能在某些源中可用） ===
    if [ "$PKG_MGR" != "apt" ]; then
        local soft_pkg
        for soft_pkg in "${RPM_PKG_SOFT[@]}"; do
            if ! rpm -q "$soft_pkg" &>/dev/null 2>&1; then
                if dnf install -y "$soft_pkg" &>/dev/null 2>&1 || \
                   yum install -y "$soft_pkg" &>/dev/null 2>&1; then
                    success "$soft_pkg 安装成功"
                fi
            fi
        done
    fi

    # === Phase 3: dnf provides 最终回退（部分包可能在系统源中） ===
    if [ "$PKG_MGR" != "apt" ]; then
        local soft_cmd
        for soft_cmd in virt-filesystems virt-customize guestfish virt-win-reg growpart; do
            if ! command -v "$soft_cmd" >/dev/null 2>&1; then
                local providing_pkg
                providing_pkg=$(dnf provides "$soft_cmd" 2>/dev/null | awk -F: '/^[^ ]+ :/ {print $1; exit}' || true)
                if [ -n "$providing_pkg" ]; then
                    info "命令 $soft_cmd 由 $providing_pkg 提供，尝试安装..."
                    if dnf install -y "$providing_pkg" &>/dev/null 2>&1 || \
                       yum install -y "$providing_pkg" &>/dev/null 2>&1; then
                        success "$soft_cmd 安装成功"
                    else
                        warn "命令 $soft_cmd 的包 $providing_pkg 安装失败"
                    fi
                fi
            fi
        done
    fi
}

ensure_required_commands() {
    info "校验功能所需系统命令..."
    local missing_cmds=()
    local soft_missing_cmds=()
    local cmd
    # RPM 系上来自软性包的命令（缺失时仅警告不报错）
    local rpm_soft_cmds=("virt-customize" "guestfish" "virt-win-reg" "growpart" "virt-filesystems")
    for cmd in "${COMMAND_CHECKS[@]}"; do
        if ! command -v "$cmd" >/dev/null 2>&1; then
            # genisoimage 可由 xorriso 或 mkisofs 替代
            if [ "$cmd" = "genisoimage" ]; then
                if command -v xorriso >/dev/null 2>&1 || command -v mkisofs >/dev/null 2>&1; then
                    continue
                fi
                if [ "$PKG_MGR" != "apt" ]; then
                    soft_missing_cmds+=("genisoimage (或 xorriso/mkisofs)")
                    continue
                fi
                missing_cmds+=("genisoimage (或 xorriso/mkisofs)")
                continue
            fi
            # RPM 系软性命令：缺失时仅警告
            local is_soft=0
            if [ "$PKG_MGR" != "apt" ]; then
                for sc in "${rpm_soft_cmds[@]}"; do
                    if [ "$cmd" = "$sc" ]; then
                        is_soft=1
                        break
                    fi
                done
            fi
            if [ "$is_soft" -eq 1 ]; then
                soft_missing_cmds+=("$cmd")
            else
                missing_cmds+=("$cmd")
            fi
        fi
    done
    if [ ${#soft_missing_cmds[@]} -gt 0 ]; then
        warn "以下可选命令不可用（功能可能受限）: ${soft_missing_cmds[*]}"
    fi
    if [ ${#missing_cmds[@]} -gt 0 ]; then
        error "以下命令不可用: ${missing_cmds[*]}。请检查包管理器或依赖安装结果"
        exit 1
    fi
    success "系统命令校验完成"
}

ensure_core_services() {
    info "检查核心服务..."
    systemctl enable --now libvirtd 2>/dev/null || systemctl enable --now libvirt-daemon 2>/dev/null || \
        systemctl enable --now virtqemud 2>/dev/null || true
    systemctl enable --now openvswitch-switch 2>/dev/null || \
        systemctl enable --now openvswitch 2>/dev/null || true
    systemctl enable ssh 2>/dev/null || systemctl enable sshd 2>/dev/null || true

    if ! systemctl is-active --quiet libvirtd 2>/dev/null && \
       ! systemctl is-active --quiet libvirt-daemon 2>/dev/null && \
       ! systemctl is-active --quiet virtqemud 2>/dev/null; then
        # 尝试识别实际存在的服务名，给出更有用的错误信息
        local found_svc=""
        for svc in libvirtd libvirt-daemon virtqemud; do
            if systemctl list-unit-files 2>/dev/null | grep -q "^${svc}\.service"; then
                found_svc="$svc"
                break
            fi
        done
        if [ -n "$found_svc" ]; then
            error "libvirt 服务 ${found_svc} 已安装但未运行，请检查: systemctl status ${found_svc}"
        else
            error "未找到 libvirt 服务（libvirtd/libvirt-daemon/virtqemud），请检查 libvirt 安装状态"
        fi
        exit 1
    fi
    if ! systemctl is-active --quiet openvswitch-switch 2>/dev/null && \
       ! systemctl is-active --quiet openvswitch 2>/dev/null; then
        warn "openvswitch 当前未运行，面板会在网络修复时再次尝试启动"
    fi
    success "核心服务检查完成"
}

# print_ovs_dep_info 输出当前系统检测到的 OVS 依赖信息
print_ovs_dep_info() {
    local os_id="${ID:-unknown}"
    local os_pretty="${PRETTY_NAME:-$os_id}"
    local ovs_pkg="openvswitch-switch"
    local ovs_svc="openvswitch-switch"
    local install_cmd=""

    case "$PKG_MGR" in
        apt)
            ovs_pkg="openvswitch-switch"
            ovs_svc="openvswitch-switch"
            install_cmd="sudo apt install -y openvswitch-switch"
            ;;
        dnf)
            ovs_pkg="openvswitch"
            ovs_svc="openvswitch"
            install_cmd="sudo dnf install -y openvswitch"
            ;;
        yum)
            ovs_pkg="openvswitch"
            ovs_svc="openvswitch"
            install_cmd="sudo yum install -y openvswitch"
            ;;
    esac

    info "──────────────────────────────────────────"
    info "系统检测: ${os_pretty} (${PKG_MGR})"
    info "OVS 包名: ${ovs_pkg}"
    info "OVS 服务: ${ovs_svc}"
    if command -v ovs-vsctl &>/dev/null; then
        success "OVS 已安装 ($(ovs-vsctl --version 2>/dev/null | head -1))"
    else
        warn "OVS 未安装，安装命令: ${install_cmd}"
    fi
    info "──────────────────────────────────────────"
}

# configure_qemu_for_rpm 修复 openEuler/麒麟 上 QEMU 权限问题
# openEuler 默认 QEMU 以 qemu 用户运行，需确保 qemu.conf 配置允许访问虚拟机文件
configure_qemu_for_rpm() {
    [ "$PKG_MGR" = "apt" ] && return 0
    local qemu_conf="/etc/libvirt/qemu.conf"
    if [ ! -f "$qemu_conf" ]; then
        warn "未找到 $qemu_conf，跳过 QEMU 配置修复"
        return 0
    fi
    info "修复 openEuler/麒麟 QEMU 权限配置..."
    # 确保 user 和 group 设置为 root（面板以 root 运行，需要直接操控 QEMU 进程）
    if grep -qE '^#\s*user\s*=' "$qemu_conf"; then
        sed -i 's/^#\s*user\s*=.*/user = "root"/' "$qemu_conf"
    elif ! grep -qE '^user\s*=\s*"root"' "$qemu_conf"; then
        echo 'user = "root"' >> "$qemu_conf"
    fi
    if grep -qE '^#\s*group\s*=' "$qemu_conf"; then
        sed -i 's/^#\s*group\s*=.*/group = "root"/' "$qemu_conf"
    elif ! grep -qE '^group\s*=\s*"root"' "$qemu_conf"; then
        echo 'group = "root"' >> "$qemu_conf"
    fi
    # 重启 libvirtd 使配置生效
    systemctl restart libvirtd 2>/dev/null || systemctl restart libvirt-daemon 2>/dev/null || true
    success "QEMU 权限配置已修复（user=root, group=root）"
}

# configure_libvirt_nonroot 为非 root 用户配置 libvirt 访问权限
# openEuler 文档要求：用户加入 libvirt 组 + 设置 LIBVIRT_DEFAULT_URI 环境变量
# 注意：此函数在 write_env 之前调用；首次安装时 .env 不存在，直接跳过（默认 root）
#       更新时读取已有 .env 中的用户，符合预期（为当前运行用户配置）
configure_libvirt_nonroot() {
    [ "$PKG_MGR" = "apt" ] && return 0
    info "配置非 root 用户 libvirt 访问权限..."
    # 获取面板运行用户（从 .env 或默认 root）
    local panel_user="root"
    if [ -f "$ENV_FILE" ]; then
        local env_user
        env_user=$(grep -E '^KVM_USER=' "$ENV_FILE" 2>/dev/null | cut -d= -f2 | tr -d '"' || true)
        [ -n "$env_user" ] && panel_user="$env_user"
    fi
    if [ "$panel_user" = "root" ]; then
        info "面板以 root 运行，跳过非 root 用户配置"
        return 0
    fi
    # 将用户加入 libvirt 组
    if ! id -nG "$panel_user" 2>/dev/null | grep -qw libvirt; then
        usermod -a -G libvirt "$panel_user" 2>/dev/null && \
            info "用户 $panel_user 已加入 libvirt 组" || \
            warn "无法将用户 $panel_user 加入 libvirt 组"
    fi
    # 设置 LIBVIRT_DEFAULT_URI 环境变量
    local bashrc="/home/$panel_user/.bashrc"
    if [ -f "$bashrc" ] && ! grep -q 'LIBVIRT_DEFAULT_URI' "$bashrc"; then
        echo 'export LIBVIRT_DEFAULT_URI="qemu:///system"' >> "$bashrc"
        info "已为 $panel_user 设置 LIBVIRT_DEFAULT_URI 环境变量"
    fi
    success "libvirt 非 root 用户配置完成"
}

detect_storage_backing_size() {
    local backing_dir="$1"
    # 优先使用更稳健的 df --output 方式获取选中磁盘的文件系统大小
    local filesystem_size_gb
    filesystem_size_gb=$(df --output=size -BG "$backing_dir" 2>/dev/null | awk 'NR==2{gsub(/[^0-9]/,"",$1); print $1}')
    if [ -n "$filesystem_size_gb" ] && [ "$filesystem_size_gb" -gt 0 ] 2>/dev/null; then
        echo "${filesystem_size_gb}G"
        return
    fi
    # 回退方案：解析 df -k 输出
    local filesystem_size_kb
    filesystem_size_kb=$(df -k "$backing_dir" 2>/dev/null | awk 'NR==2{print $2}')
    if [ -n "$filesystem_size_kb" ] && [ "$filesystem_size_kb" -gt 0 ] 2>/dev/null; then
        echo "$((filesystem_size_kb / 1024 / 1024))G"
        return
    fi
    echo "100G"
}

load_existing_storage_image() {
    local configured_image
    local mounted_source

    # 优先从 fstab 读取已持久化的镜像位置，兼容此前的默认路径。
    configured_image=$(awk -v mount_point="$STORAGE_MOUNT" '$1 !~ /^#/ && $2 == mount_point { print $1; exit }' /etc/fstab 2>/dev/null || true)
    if [[ "$configured_image" = /* ]] && [ -f "$configured_image" ]; then
        STORAGE_IMG="$configured_image"
        return 0
    fi

    # fstab 尚未写入时，尝试从已挂载的 loop 设备恢复镜像路径。
    if mountpoint -q "$STORAGE_MOUNT"; then
        mounted_source=$(findmnt -rn -o SOURCE -T "$STORAGE_MOUNT" 2>/dev/null || true)
        if [[ "$mounted_source" == /dev/loop* ]]; then
            configured_image=$(losetup -n -O BACK-FILE "$mounted_source" 2>/dev/null | head -n 1 || true)
            if [[ "$configured_image" = /* ]] && [ -f "$configured_image" ]; then
                STORAGE_IMG="$configured_image"
                return 0
            fi
        fi
    fi

    [ -f "$STORAGE_IMG" ]
}

choose_storage_image_location() {
    local root_target
    local source
    local target
    local filesystem
    local available
    local total
    local choice
    local index
    declare -a storage_dirs
    declare -a storage_labels

    root_target=$(findmnt -rn -o TARGET -T "$STORAGE_DEFAULT_BACKING_DIR" 2>/dev/null || true)
    storage_dirs=("$STORAGE_DEFAULT_BACKING_DIR")
    storage_labels=("根目录（默认） 目录: ${STORAGE_DEFAULT_BACKING_DIR}")

    # 仅提供已挂载、可写的本地文件系统；未挂载磁盘不能安全地直接用于创建镜像。
    while read -r source target filesystem; do
        [ -n "$source" ] && [ -n "$target" ] || continue
        [ "$target" = "$root_target" ] && continue
        [ "$target" = "$STORAGE_MOUNT" ] && continue
        [[ "$source" == /dev/* ]] || continue
        if findmnt -rn -o OPTIONS -T "$target" 2>/dev/null | grep -qw ro; then
            continue
        fi
        available=$(df -hP "$target" 2>/dev/null | awk 'NR==2 {print $4}')
        total=$(df -hP "$target" 2>/dev/null | awk 'NR==2 {print $2}')
        storage_dirs+=("$target")
        storage_labels+=("设备: ${source}  挂载点: ${target}  文件系统: ${filesystem}  可用: ${available:-未知}/${total:-未知}")
    done < <(findmnt -rn -t ext4,xfs,btrfs -o SOURCE,TARGET,FSTYPE 2>/dev/null || true)

    echo ""
    info "请选择用户存储镜像所在磁盘（默认使用根目录）"
    for index in "${!storage_dirs[@]}"; do
        printf '  %s) %s\n' "$index" "${storage_labels[$index]}"
    done
    echo "  提示：未挂载或只读磁盘不会显示，请先在系统或存储池页面完成挂载。"

    while true; do
        read -rp "请选择存储磁盘 [默认 0]: " choice
        choice=${choice:-0}
        if [[ "$choice" =~ ^[0-9]+$ ]] && [ "$choice" -lt "${#storage_dirs[@]}" ]; then
            STORAGE_IMG="${storage_dirs[$choice]%/}/${STORAGE_IMG_FILENAME}"
            success "用户存储镜像将创建在: $STORAGE_IMG"
            return
        fi
        warn "无效的选择，请输入 0 到 $((${#storage_dirs[@]} - 1)) 之间的数字"
    done
}

ensure_storage_fstab() {
    local expected_entry="${STORAGE_IMG} ${STORAGE_MOUNT} ext4 loop,prjquota 0 0"
    local temporary_fstab

    touch /etc/fstab
    if grep -Fxq "$expected_entry" /etc/fstab 2>/dev/null; then
        return
    fi

    # 挂载点由面板专用，替换旧条目可避免升级或重新选择磁盘后产生重复挂载。
    temporary_fstab=$(mktemp /etc/fstab.kvm-console.XXXXXX)
    awk -v mount_point="$STORAGE_MOUNT" '$2 != mount_point { print }' /etc/fstab > "$temporary_fstab"
    printf '%s\n' "$expected_entry" >> "$temporary_fstab"
    chmod --reference=/etc/fstab "$temporary_fstab" 2>/dev/null || chmod 644 "$temporary_fstab"
    mv "$temporary_fstab" /etc/fstab
    success "已更新用户存储挂载配置到 /etc/fstab"
}

setup_quota() {
    info "检查用户存储 Project Quota 文件系统..."
    mkdir -p "$STORAGE_MOUNT"
    touch /etc/projects /etc/projid

    if load_existing_storage_image; then
        info "检测到已有用户存储镜像: $STORAGE_IMG"
    else
        choose_storage_image_location
    fi

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
    default_size=$(detect_storage_backing_size "$(dirname "$STORAGE_IMG")")
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

ipv4_to_int() {
    local ip="$1"
    local a b c d octet
    IFS=. read -r a b c d <<< "$ip"
    for octet in "$a" "$b" "$c" "$d"; do
        if ! [[ "$octet" =~ ^[0-9]{1,3}$ ]] || [ "$((10#$octet))" -gt 255 ]; then
            return 1
        fi
    done
    printf '%u\n' "$(( (10#$a << 24) + (10#$b << 16) + (10#$c << 8) + 10#$d ))"
}

cidr_to_range() {
    local cidr="$1"
    local ip="${cidr%/*}"
    local prefix_len="${cidr#*/}"
    local ip_int mask network broadcast
    if [ "$ip" = "$cidr" ] || ! [[ "$prefix_len" =~ ^[0-9]{1,2}$ ]] || [ "$prefix_len" -gt 32 ]; then
        return 1
    fi
    ip_int=$(ipv4_to_int "$ip") || return 1
    if [ "$prefix_len" -eq 0 ]; then
        mask=0
    else
        mask=$(( (0xFFFFFFFF << (32 - prefix_len)) & 0xFFFFFFFF ))
    fi
    network=$(( ip_int & mask ))
    broadcast=$(( network | (0xFFFFFFFF ^ mask) ))
    printf '%u %u\n' "$network" "$broadcast"
}

cidr_overlaps() {
    local left="$1"
    local right="$2"
    local left_range right_range left_start left_end right_start right_end
    left_range=$(cidr_to_range "$left") || return 1
    right_range=$(cidr_to_range "$right") || return 1
    read -r left_start left_end <<< "$left_range"
    read -r right_start right_end <<< "$right_range"
    [ "$left_start" -le "$right_end" ] && [ "$right_start" -le "$left_end" ]
}

host_ipv4_cidrs() {
    { ip -o -4 addr show 2>/dev/null || true; } | awk '
        $2 != "lo" {
            for (i = 1; i <= NF; i++) {
                if ($i == "inet") {
                    print $2, $(i + 1)
                    break
                }
            }
        }
    '
}

first_ovs_subnet_conflict() {
    local subnet_prefix="$1"
    local candidate_cidr="${subnet_prefix}.0/24"
    local iface iface_cidr
    while read -r iface iface_cidr; do
        [ -n "${iface:-}" ] && [ -n "${iface_cidr:-}" ] || continue
        if cidr_overlaps "$candidate_cidr" "$iface_cidr"; then
            printf '%s %s' "$iface" "$iface_cidr"
            return 0
        fi
    done < <(host_ipv4_cidrs)
    return 1
}

generate_ovs_subnet_candidates() {
    local third second
    printf '%s\n' "$DEFAULT_OVS_SUBNET_PREFIX"
    for third in $(seq 123 254) $(seq 2 121); do
        printf '192.168.%s\n' "$third"
    done
    for second in 250 251 252 253 254 240 241 242 243 244 245 246 247 248 249; do
        for third in $(seq 0 254); do
            printf '10.%s.%s\n' "$second" "$third"
        done
    done
    for second in 31 30 29 28 27 26 25 24 23 22 21 20; do
        for third in $(seq 0 254); do
            printf '172.%s.%s\n' "$second" "$third"
        done
    done
}

select_available_ovs_subnet_prefix() {
    local candidate
    while read -r candidate; do
        [ -n "$candidate" ] || continue
        if ! first_ovs_subnet_conflict "$candidate" >/dev/null; then
            printf '%s' "$candidate"
            return 0
        fi
    done < <(generate_ovs_subnet_candidates)
    printf '%s' "$DEFAULT_OVS_SUBNET_PREFIX"
    return 0
}

write_default_ovs_subnet_prefix() {
    local existing_value
    existing_value=$(env_get "KVM_SUBNET_PREFIX")
    if [ -n "$existing_value" ] || grep -q "^KVM_SUBNET_PREFIX=" "$ENV_FILE" 2>/dev/null; then
        return
    fi

    local selected_prefix default_conflict
    selected_prefix=$(select_available_ovs_subnet_prefix)
    default_conflict=$(first_ovs_subnet_conflict "$DEFAULT_OVS_SUBNET_PREFIX" || true)
    env_set "KVM_SUBNET_PREFIX" "$selected_prefix"

    if [ "$selected_prefix" != "$DEFAULT_OVS_SUBNET_PREFIX" ]; then
        if [ -n "$default_conflict" ]; then
            warn "检测到默认基础网络 ${DEFAULT_OVS_SUBNET_PREFIX}.0/24 与宿主机网卡 ${default_conflict} 冲突，已自动改用 ${selected_prefix}.0/24"
        else
            warn "默认基础网络 ${DEFAULT_OVS_SUBNET_PREFIX}.0/24 不可用，已自动改用 ${selected_prefix}.0/24"
        fi
    else
        success "基础网络默认网段: ${selected_prefix}.0/24"
    fi
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
    env_set "KVM_USER_STORAGE_IMAGE" "$STORAGE_IMG"

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
        env_default "KVM_ELASTIC_CLOUD_UPLINK" ""
        env_default "KVM_OVS_DHCP_START" ""
        env_default "KVM_OVS_DHCP_END" ""
        write_default_ovs_subnet_prefix
        env_default "KVM_AUTO_PORT_START" "10000"
        env_default "KVM_AUTO_PORT_END" "20000"
        env_default "KVM_HOST_IP" ""
        env_default "KVM_EXTERNAL_NIC" ""
        env_default "KVM_MAX_BURST_INBOUND" "0"
        env_default "KVM_MAX_BURST_OUTBOUND" "0"
        env_default "KVM_PORT_SECURITY_ENABLED" "false"
        env_default "KVM_PORT_SECURITY_TOTAL_KPPS" "50"
        env_default "KVM_PORT_SECURITY_TOTAL_BURST_KPACKETS" "40"
        env_default "KVM_PORT_SECURITY_NEIGHBOR_PPS" "200"
        env_default "KVM_PORT_SECURITY_NEIGHBOR_BURST_PACKETS" "400"
        env_default "KVM_PORT_SECURITY_BROADCAST_PPS" "1000"
        env_default "KVM_PORT_SECURITY_BROADCAST_BURST_PACKETS" "2000"
        env_default "KVM_PORT_SECURITY_RECONCILE_INTERVAL_SECONDS" "60"
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

    # ARM 架构部署旧版 AAVMF 兼容固件（解决统信 UOS 等 OS 的 UEFI 引导兼容性问题）
    if [ "$ARCH" = "aarch64" ]; then
        local firmware_dir="${INSTALL_DIR}/firmware"
        mkdir -p "$firmware_dir"
        if [ ! -f "$firmware_dir/AAVMF_CODE_legacy.fd" ]; then
            info "部署 ARM UEFI 兼容固件..."
            # 优先从 Ubuntu 24.04 仓库下载旧版 EDK2
            local efi_deb_url="http://ports.ubuntu.com/pool/main/e/edk2/qemu-efi-aarch64_2024.02-2_all.deb"
            local efi_deb_file="/tmp/qemu-efi-legacy.deb"
            if wget -q "$efi_deb_url" -O "$efi_deb_file" 2>/dev/null || \
               wget -q "http://mirrors.aliyun.com/ubuntu-ports/pool/main/e/edk2/qemu-efi-aarch64_2024.02-2_all.deb" -O "$efi_deb_file" 2>/dev/null; then
                local efi_extract="/tmp/efi-legacy-extract"
                rm -rf "$efi_extract"
                mkdir -p "$efi_extract"
                dpkg-deb -x "$efi_deb_file" "$efi_extract" 2>/dev/null
                if [ -f "$efi_extract/usr/share/AAVMF/AAVMF_CODE.no-secboot.fd" ]; then
                    cp -f "$efi_extract/usr/share/AAVMF/AAVMF_CODE.no-secboot.fd" "$firmware_dir/AAVMF_CODE_legacy.fd"
                    cp -f "$efi_extract/usr/share/AAVMF/AAVMF_VARS.fd" "$firmware_dir/AAVMF_VARS_legacy.fd"
                    success "ARM UEFI 兼容固件部署完成"
                else
                    warn "旧版固件提取失败，跳过兼容固件部署"
                fi
                rm -rf "$efi_extract" "$efi_deb_file"
            else
                warn "下载旧版 AAVMF 固件失败，跳过兼容固件部署（可手动放置到 $firmware_dir）"
            fi
        else
            success "ARM UEFI 兼容固件已存在"
        fi
    fi

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
    ip -4 route get 223.5.5.5 2>/dev/null | awk '{for (i=1; i<=NF; i++) if ($i == "dev") {print $(i+1); exit}}'
}

resolve_nat_uplink() {
    local configured="$1"
    local bridge=""

    if [ -z "$configured" ]; then
        configured=$(detect_default_uplink)
    fi
    if [ -z "$configured" ]; then
        return 0
    fi

    # 物理口加入 OVS 网桥且默认路由已迁移后，Netfilter 的实际出口是网桥。
    if command -v ovs-vsctl >/dev/null 2>&1; then
        bridge=$(ovs-vsctl --timeout=5 port-to-br "$configured" 2>/dev/null || true)
        if [ -n "$bridge" ] && [ "$bridge" != "$configured" ] && \
            ip -4 route show default dev "$bridge" 2>/dev/null | grep -q '^default '; then
            printf '%s\n' "$bridge"
            return 0
        fi
    fi
    printf '%s\n' "$configured"
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
            iptables -I INPUT 1 -i "$iface" -p "$proto" --dport "$port" -j ACCEPT 2>/dev/null || true
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
    print_ovs_dep_info

    # 内部子函数，任何失败只警告不中断安装
    _setup_ovs_inner || warn "OVS 网络地基配置部分失败，可在面板 OVS 诊断中执行修复"
    success "OVS 网络地基已准备"
}

check_ovs_port_security_prerequisites() {
    local bridge="$1"
    info "检查端口安全所需的 OVS 基础能力（总开关保持关闭）..."
    if ! ovs-ofctl -O OpenFlow13 dump-flows "$bridge" >/dev/null 2>&1; then
        warn "当前网桥未通过 OpenFlow13 协商，端口安全预检将阻止开启"
        return 0
    fi
    local columns
    columns=$(ovsdb-client list-columns Open_vSwitch Interface 2>/dev/null || true)
    if [[ "$columns" != *"ingress_policing_kpkts_rate"* ]] || [[ "$columns" != *"ingress_policing_kpkts_burst"* ]]; then
        warn "当前 OVS 缺少包速率 policing 字段，端口安全预检将阻止开启"
        return 0
    fi
    local meter_features
    meter_features=$(ovs-ofctl -O OpenFlow13 meter-features "$bridge" 2>/dev/null || true)
    if [[ "${meter_features,,}" != *"pktps"* ]] || [[ "${meter_features,,}" != *"burst"* ]]; then
        warn "当前 OVS 缺少 pktps/burst meter 能力，端口安全预检将阻止开启"
        return 0
    fi
    success "端口安全 OVS 基础能力可用；实际规则落地与清理由兼容性实机测试验证"
}

_setup_ovs_inner() {
    load_env_file
    local bridge="${KVM_OVS_BRIDGE:-br-ovs}"
    local subnet="${KVM_SUBNET_PREFIX:-192.168.122}"
    local gateway="${subnet}.1"
    local dhcp_start="${KVM_OVS_DHCP_START:-${subnet}.2}"
    local dhcp_end="${KVM_OVS_DHCP_END:-${subnet}.254}"
    local uplink=""

    systemctl enable --now openvswitch-switch 2>/dev/null || \
        systemctl enable --now openvswitch 2>/dev/null || true
    # 等待 OVS 数据库就绪，避免 ovs-vsctl 挂起
    for _i in 1 2 3 4 5 6 7 8 9 10; do
        ovs-vsctl --no-wait show 2>/dev/null && break
        sleep 1
    done
    uplink=$(resolve_nat_uplink "${KVM_OVS_UPLINK:-}")
    if [ -z "$uplink" ]; then
        warn "未检测到默认出口网卡，OVS NAT 将在面板网络修复时再次尝试。也可在 $ENV_FILE 配置 KVM_OVS_UPLINK"
    fi
    if ! ovs-vsctl --timeout=5 --may-exist add-br "$bridge" 2>/dev/null; then
        warn "创建 OVS 网桥失败，跳过 OVS 网络配置"
        return 0
    fi
    if ! ip link set "$bridge" up 2>/dev/null; then
        warn "启动 OVS 网桥失败，跳过 OVS 网络配置"
        return 0
    fi
    if ! ip -4 addr show dev "$bridge" | grep -q "${gateway}/24"; then
        ip addr flush dev "$bridge" 2>/dev/null || true
        ip addr add "${gateway}/24" dev "$bridge" 2>/dev/null || true
    fi
    ensure_local_dnsmasq_input_rules "$bridge" || true
    ensure_existing_vpc_dnsmasq_input_rules || true

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
ovs-vsctl --may-exist add-br "\$BRIDGE" 2>/dev/null || true
ip link set "\$BRIDGE" up 2>/dev/null || true
if ! ip -4 addr show dev "\$BRIDGE" | grep -q "\$GATEWAY"; then
  ip addr flush dev "\$BRIDGE" 2>/dev/null || true
  ip addr add "\$GATEWAY" dev "\$BRIDGE" 2>/dev/null || true
fi
# 释放端口，确保 libvirt dnsmasq 完全停止
for i in 1 2 3 4 5; do
  if ss -tlnp | grep -q "${gateway}:53"; then
    sleep 1
  else
    break
  fi
done
pkill -f "dnsmasq.*${subnet}" 2>/dev/null || true
sleep 0.5
for rule in "udp 67" "udp 53" "tcp 53"; do
  proto="\${rule%% *}"
  port="\${rule##* }"
  iptables -C INPUT -i "\$BRIDGE" -p "\$proto" --dport "\$port" -j ACCEPT 2>/dev/null || \\
    iptables -I INPUT 1 -i "\$BRIDGE" -p "\$proto" --dport "\$port" -j ACCEPT 2>/dev/null || true
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
ExecStart=/usr/sbin/dnsmasq --conf-file=${OVS_CONFIG_DIR}/dnsmasq.conf
ExecReload=/bin/kill -HUP \$MAINPID
Restart=on-failure
RestartSec=5
# 网桥和端口释放由主服务 EnsureOVSNetworkReady() 统一处理

[Install]
WantedBy=multi-user.target
EOF

    systemctl daemon-reload
    systemctl enable "$OVS_DNSMASQ_UNIT" >/dev/null 2>&1 || true
    restart_ovs_dnsmasq_service

    if [ -n "$uplink" ]; then
        iptables -t nat -C POSTROUTING -s "${subnet}.0/24" -o "$uplink" -j MASQUERADE 2>/dev/null || \
            iptables -t nat -A POSTROUTING -s "${subnet}.0/24" -o "$uplink" -j MASQUERADE 2>/dev/null || true
        iptables -C FORWARD -i "$bridge" -o "$uplink" -j ACCEPT 2>/dev/null || \
            iptables -A FORWARD -i "$bridge" -o "$uplink" -j ACCEPT 2>/dev/null || true
        iptables -C FORWARD -i "$uplink" -o "$bridge" -m conntrack --ctstate RELATED,ESTABLISHED -j ACCEPT 2>/dev/null || \
            iptables -A FORWARD -i "$uplink" -o "$bridge" -m conntrack --ctstate RELATED,ESTABLISHED -j ACCEPT 2>/dev/null || true
    fi

    if virsh net-info default >/dev/null 2>&1; then
        virsh net-destroy default >/dev/null 2>&1 || true
        virsh net-autostart default --disable >/dev/null 2>&1 || true
    fi
    check_ovs_port_security_prerequisites "$bridge"
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
    found_bin=$(find "$TMP_RELEASE_DIR" -maxdepth 3 -name "kvm-console" -type f -perm /111 2>/dev/null | sed -n '1p') || true
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

    # 按架构确定安装包名称与下载链接
    local local_tarball_name download_url
    if [ "$ARCH" = "x86_64" ]; then
        local_tarball_name="kvm-console-linux-amd64.tar.gz"
        download_url="$DOWNLOAD_URL_AMD64"
    elif [ "$ARCH" = "aarch64" ]; then
        local_tarball_name="kvm-console-linux-arm64.tar.gz"
        download_url="$DOWNLOAD_URL_ARM64"
    fi

    # 优先使用当前目录已有的本地发行包
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

    # 未检测到本地安装包，从官方下载源自动下载
    info "未检测到本地安装包，从官方下载源获取 ${local_tarball_name} (${ARCH})..."
    TMP_RELEASE_DIR=$(mktemp -d)
    local tarball_path="${TMP_RELEASE_DIR}/${local_tarball_name}"
    if command -v wget >/dev/null 2>&1; then
        if ! wget -O "$tarball_path" "$download_url"; then
            error "下载安装包失败，请检查网络或手动下载后放置于当前目录: ${local_tarball_name}"
            exit 1
        fi
    else
        if ! curl -fL --progress-bar -o "$tarball_path" "$download_url"; then
            error "下载安装包失败，请检查网络或手动下载后放置于当前目录: ${local_tarball_name}"
            exit 1
        fi
    fi
    extract_tarball "$tarball_path"
}

install_files() {
    if [ "$MODE" = "update" ]; then
        info "停止 ${APP_NAME} 服务..."
        systemctl stop "$SERVICE_NAME" 2>/dev/null || true
        # 更新前先删除旧版前端所有资源，避免残留旧版静态文件
        info "清理旧版前端资源..."
        rm -rf "${INSTALL_DIR}/web-dist"
    fi

    mkdir -p "$INSTALL_DIR/data"
    info "安装后端程序..."
    cp -f "${RELEASE_SOURCE_DIR}/kvm-console" "${INSTALL_DIR}/kvm-console"
    chmod +x "${INSTALL_DIR}/kvm-console"

    # 如果发行包包含宿主机原生版二进制，一并部署并自动选择
    if [ -f "${RELEASE_SOURCE_DIR}/kvm-console-native" ]; then
        cp -f "${RELEASE_SOURCE_DIR}/kvm-console-native" "${INSTALL_DIR}/kvm-console-native"
        chmod +x "${INSTALL_DIR}/kvm-console-native"
        info "已部署宿主机原生版二进制 kvm-console-native"

        # 检测宿主机 glibc 版本，自动选择最合适的二进制作为主程序
        local glibc_ver
        glibc_ver=$(ldd --version 2>&1 | sed -n '1 s/.* //p') || true
        if [ -z "$glibc_ver" ] || ! echo "$glibc_ver" | grep -qE '^[0-9]+\.[0-9]+$'; then
            glibc_ver=$(getconf GNU_LIBC_VERSION 2>/dev/null | awk '{print $2}' || echo "0")
        fi
        info "检测到宿主机 GLIBC 版本: ${glibc_ver}"

        # 版本比较：若 glibc >= 2.34 则切换为原生版
        local need_compat=true
        IFS=. read -r major minor <<< "$glibc_ver"
        minor=${minor:-0}
        if [ "$major" -gt 2 ] || { [ "$major" -eq 2 ] && [ "$minor" -ge 34 ]; }; then
            need_compat=false
        fi

        if [ "$need_compat" = false ]; then
            # 额外检测 CPU 是否支持 AVX2+FMA（native 版可能使用这些指令）
            # Ivy Bridge 等 CPU 仅支持 AVX1，运行含 FMA/AVX2 的二进制会 SIGILL 崩溃
            if [ "$ARCH" = "x86_64" ] && ! grep -q 'avx2' /proc/cpuinfo 2>/dev/null; then
                warn "CPU 不支持 AVX2/FMA 指令集，保留 zig 兼容版作为主程序（原生版可能崩溃）"
                info "原生版保留为 kvm-console-native，可手动测试切换"
            else
                info "GLIBC ≥ 2.34 且 CPU 支持 AVX2，切换宿主机原生版为主程序（兼容版保留为 kvm-console-compat）"
                mv -f "${INSTALL_DIR}/kvm-console" "${INSTALL_DIR}/kvm-console-compat"
                mv -f "${INSTALL_DIR}/kvm-console-native" "${INSTALL_DIR}/kvm-console"
                success "已切换为宿主机原生版"
            fi
        else
            info "GLIBC < 2.34，继续使用 zig 兼容版作为主程序（原生版保留为 kvm-console-native）"
        fi
    fi

    info "安装前端静态文件..."
    rm -rf "${INSTALL_DIR}/web-dist"
    cp -r "${RELEASE_SOURCE_DIR}/web-dist" "${INSTALL_DIR}/web-dist"

    # 发行包内脚本始终部署，首次安装选择执行时仍按“当前目录 → 下载”流程取得待运行版本。
    if [ -f "${RELEASE_SOURCE_DIR}/${COMPATIBILITY_CHECK_SCRIPT}" ]; then
        mkdir -p "${INSTALL_DIR}/scripts"
        install -m 700 "${RELEASE_SOURCE_DIR}/${COMPATIBILITY_CHECK_SCRIPT}" \
            "${INSTALL_DIR}/scripts/${COMPATIBILITY_CHECK_SCRIPT}"
    fi
    success "程序文件已安装"
}

validate_compatibility_script() {
    local script_path="$1"
    [ -f "$script_path" ] || return 1
    [ -s "$script_path" ] || return 1
    [ -r "$script_path" ] || return 1
    bash -n "$script_path" >/dev/null 2>&1
}

download_compatibility_script() {
    local target_path="${INSTALL_LAUNCH_DIR}/${COMPATIBILITY_CHECK_SCRIPT}"
    local downloaded=0

    if [ -z "$COMPATIBILITY_CHECK_URL" ]; then
        COMPATIBILITY_FAILURE_STAGE="兼容性测试脚本获取"
        warn "兼容性测试脚本下载地址尚未配置: COMPATIBILITY_CHECK_URL"
        return 1
    fi
    if [ -d "$target_path" ]; then
        COMPATIBILITY_FAILURE_STAGE="兼容性测试脚本下载"
        warn "目标路径是目录，不能原子替换为兼容性测试脚本: $target_path"
        return 1
    fi

    COMPATIBILITY_DOWNLOAD_TMP="${target_path}.download.$$"
    rm -f "$COMPATIBILITY_DOWNLOAD_TMP"
    info "正在下载兼容性测试脚本到当前目录..."

    if command -v curl >/dev/null 2>&1; then
        if curl -fL --progress-bar -o "$COMPATIBILITY_DOWNLOAD_TMP" "$COMPATIBILITY_CHECK_URL"; then
            downloaded=1
        else
            warn "curl 下载兼容性测试脚本失败，尝试使用 wget"
        fi
    fi
    if [ "$downloaded" -eq 0 ] && command -v wget >/dev/null 2>&1; then
        if wget -O "$COMPATIBILITY_DOWNLOAD_TMP" "$COMPATIBILITY_CHECK_URL"; then
            downloaded=1
        fi
    fi
    if [ "$downloaded" -eq 0 ]; then
        rm -f "$COMPATIBILITY_DOWNLOAD_TMP"
        COMPATIBILITY_DOWNLOAD_TMP=""
        COMPATIBILITY_FAILURE_STAGE="兼容性测试脚本下载"
        warn "下载兼容性测试脚本失败"
        return 1
    fi

    if ! validate_compatibility_script "$COMPATIBILITY_DOWNLOAD_TMP"; then
        rm -f "$COMPATIBILITY_DOWNLOAD_TMP"
        COMPATIBILITY_DOWNLOAD_TMP=""
        COMPATIBILITY_FAILURE_STAGE="兼容性测试脚本校验"
        warn "下载的兼容性测试脚本为空、不可读或语法校验失败"
        return 1
    fi

    chmod 700 "$COMPATIBILITY_DOWNLOAD_TMP"
    mv -f "$COMPATIBILITY_DOWNLOAD_TMP" "$target_path"
    COMPATIBILITY_DOWNLOAD_TMP=""
    chmod 700 "$target_path"
    COMPATIBILITY_SCRIPT_PATH="$target_path"
    success "兼容性测试脚本下载并校验完成"
}

obtain_compatibility_script() {
    local local_script="${INSTALL_LAUNCH_DIR}/${COMPATIBILITY_CHECK_SCRIPT}"
    COMPATIBILITY_SCRIPT_PATH=""

    if [ -e "$local_script" ]; then
        info "检测当前目录中的兼容性测试脚本: $local_script"
        if validate_compatibility_script "$local_script"; then
            COMPATIBILITY_SCRIPT_PATH="$local_script"
            success "本地兼容性测试脚本校验通过"
            return 0
        fi
        warn "本地兼容性测试脚本校验失败，将通过网络重新下载"
    else
        info "当前目录未找到 ${COMPATIBILITY_CHECK_SCRIPT}，将通过网络获取"
    fi

    download_compatibility_script
}

deploy_compatibility_script() {
    local source_path="$1"
    mkdir -p "${INSTALL_DIR}/scripts"
    install -m 700 "$source_path" "${INSTALL_DIR}/scripts/${COMPATIBILITY_CHECK_SCRIPT}"
    success "兼容性测试脚本已部署到 ${INSTALL_DIR}/scripts/${COMPATIBILITY_CHECK_SCRIPT}"
}

execute_compatibility_check() {
    local script_path="$1"
    COMPATIBILITY_CHECK_STATUS=0
    COMPATIBILITY_INTERRUPTED=0

    info "开始创建 1 vCPU / 1GB 内存 / 1GB 磁盘的临时测试虚拟机..."
    trap 'COMPATIBILITY_INTERRUPTED=1; warn "收到中断信号，正在等待兼容性测试清理临时资源"' INT TERM
    set +e
    bash "$script_path" \
        --binary "${INSTALL_DIR}/kvm-console" \
        --report-dir "$COMPATIBILITY_REPORT_DIR" \
        --vcpu 1 \
        --ram-gb 1 \
        --disk-gb 1
    COMPATIBILITY_CHECK_STATUS=$?
    set -e
    trap - INT TERM

    if [ "$COMPATIBILITY_INTERRUPTED" -eq 1 ]; then
        COMPATIBILITY_CHECK_STATUS=130
    fi
}

rollback_first_install_program_files() {
    warn "正在撤回本次复制的程序文件；依赖、网络地基、配置和诊断报告将保留"
    rm -f \
        "${INSTALL_DIR}/kvm-console" \
        "${INSTALL_DIR}/kvm-console-native" \
        "${INSTALL_DIR}/kvm-console-compat" \
        "${INSTALL_DIR}/scripts/${COMPATIBILITY_CHECK_SCRIPT}"
    rm -rf "${INSTALL_DIR}/web-dist"
    rmdir "${INSTALL_DIR}/scripts" 2>/dev/null || true
    success "程序文件已撤回；重新运行安装脚本仍会进入首次安装"
}

confirm_continue_after_compatibility_failure() {
    local continue_install
    echo ""
    warn "兼容性测试未通过"
    warn "失败阶段: ${COMPATIBILITY_FAILURE_STAGE:-兼容性测试执行}"
    warn "报告目录: ${COMPATIBILITY_REPORT_DIR}"
    read -rp "兼容性测试未通过，是否仍继续安装面板？[y/N]: " continue_install
    continue_install=${continue_install:-N}
    if [[ "$continue_install" =~ ^[Yy]$ ]]; then
        COMPATIBILITY_WARNING=1
        warn "用户选择继续安装，安装完成信息将保留兼容性警告"
        return 0
    fi

    rollback_first_install_program_files
    exit 1
}

run_first_install_compatibility_check() {
    [ "$MODE" = "install" ] || return 0

    local run_check
    local check_status
    echo ""
    read -rp "是否运行系统兼容性测试？首次安装强烈推荐 [Y/n]: " run_check
    run_check=${run_check:-Y}
    if [[ ! "$run_check" =~ ^[Yy]$ ]]; then
        COMPATIBILITY_SKIPPED=1
        warn "已跳过兼容性测试，宿主机尚未完成实际虚拟机创建与 OVS 接入验证"
        return 0
    fi

    if ! obtain_compatibility_script; then
        warn "兼容性测试未执行：未取得有效的测试脚本"
        confirm_continue_after_compatibility_failure
        return 0
    fi

    deploy_compatibility_script "$COMPATIBILITY_SCRIPT_PATH"
    execute_compatibility_check "${INSTALL_DIR}/scripts/${COMPATIBILITY_CHECK_SCRIPT}"
    check_status=$COMPATIBILITY_CHECK_STATUS

    if [ "$check_status" -eq 0 ]; then
        success "宿主机虚拟机创建与基础 OVS 网络兼容性测试通过"
        return 0
    fi

    if [ "$check_status" -eq 130 ]; then
        COMPATIBILITY_FAILURE_STAGE="用户中断"
    else
        COMPATIBILITY_FAILURE_STAGE="一个或多个兼容性测试阶段"
    fi
    confirm_continue_after_compatibility_failure
}

run_update_compatibility_check() {
    [ "$MODE" = "update" ] || return 0

    local run_check
    local release_script="${RELEASE_SOURCE_DIR}/${COMPATIBILITY_CHECK_SCRIPT}"
    local installed_script="${INSTALL_DIR}/scripts/${COMPATIBILITY_CHECK_SCRIPT}"
    echo ""
    read -rp "是否使用本次更新的新版本代码重新运行系统兼容性测试？[y/N]: " run_check
    run_check=${run_check:-N}
    if [[ ! "$run_check" =~ ^[Yy]$ ]]; then
        info "已跳过更新后的系统兼容性测试"
        return 0
    fi

    if ! validate_compatibility_script "$release_script"; then
        COMPATIBILITY_WARNING=1
        COMPATIBILITY_FAILURE_STAGE="新版本兼容性测试脚本校验"
        warn "本次发行包中的兼容性测试脚本缺失或校验失败，未使用已安装的旧脚本替代"
        return 0
    fi
    if [ ! -x "${INSTALL_DIR}/kvm-console" ]; then
        COMPATIBILITY_WARNING=1
        COMPATIBILITY_FAILURE_STAGE="新版本后端程序校验"
        warn "本次更新后的后端程序不存在或不可执行"
        return 0
    fi

    deploy_compatibility_script "$release_script"
    if ! validate_compatibility_script "$installed_script" || ! cmp -s "$release_script" "$installed_script"; then
        COMPATIBILITY_WARNING=1
        COMPATIBILITY_FAILURE_STAGE="新版本兼容性测试脚本部署"
        warn "新版本兼容性测试脚本部署校验失败"
        return 0
    fi

    info "将使用本次发行包部署的兼容性脚本和更新后的后端程序执行测试"
    execute_compatibility_check "$installed_script"
    if [ "$COMPATIBILITY_CHECK_STATUS" -eq 0 ]; then
        success "更新后的系统兼容性测试通过"
        return 0
    fi

    COMPATIBILITY_WARNING=1
    if [ "$COMPATIBILITY_CHECK_STATUS" -eq 130 ]; then
        COMPATIBILITY_FAILURE_STAGE="用户中断"
        warn "更新后的系统兼容性测试已中断，更新流程将继续启动面板"
    else
        COMPATIBILITY_FAILURE_STAGE="一个或多个兼容性测试阶段"
        warn "更新后的系统兼容性测试未通过，更新流程将继续启动面板"
    fi
    warn "诊断报告目录: ${COMPATIBILITY_REPORT_DIR}"
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
        rm -f "${INSTALL_DIR}/kvm-console-native" "${INSTALL_DIR}/kvm-console-compat"
        rm -f "${INSTALL_DIR}/scripts/${COMPATIBILITY_CHECK_SCRIPT}"
        rmdir "${INSTALL_DIR}/scripts" 2>/dev/null || true
        rm -rf "${INSTALL_DIR}/web-dist"
        warn "已保留 ${INSTALL_DIR}/data 与 ${ENV_FILE}"
    fi

    success "${APP_NAME} 已卸载"
}

BOX_INNER_WIDTH=64

# 测算可视化宽度，剔除所有ANSI转义序列
get_visual_width() {
    local txt="$1"
    local stripped=$(sed -E $'s/\x1b\[[0-9;]*[mKHF]//g' <<<"$txt")
    echo -n "$stripped" | wc -L
}

# 纯文本补齐空格，右填充到BOX_INNER_WIDTH
pad_plain() {
    local raw="$1"
    local w=$(get_visual_width "$raw")
    local pad=$(( BOX_INNER_WIDTH - w ))
    (( pad < 0 )) && pad=0
    local space_str
    space_str=$(printf "%${pad}s" "")
    printf '%s%s' "$raw" "$space_str"
}

# 新增：文本居中函数，左右自动分配空格
center_text() {
    local raw="$1"
    local w=$(get_visual_width "$raw")
    local total_pad=$(( BOX_INNER_WIDTH - w ))
    (( total_pad < 0 )) && total_pad=0
    local left_pad=$(( total_pad / 2 ))
    local right_pad=$(( total_pad - left_pad ))
    # 左侧空格 + 文字 + 右侧空格
    printf "%${left_pad}s%s%${right_pad}s" "" "$raw" ""
}

show_info() {
    local host_ip
    host_ip=$(hostname -I 2>/dev/null | awk '{print $1}')
    host_ip=${host_ip:-localhost}

    # 拼接固定边框字符串，边框统一使用青色
    top_border="${CYAN}╔$(printf '═%.0s' $(seq 1 $BOX_INNER_WIDTH))╗${NC}"
    mid_border="${CYAN}╠$(printf '═%.0s' $(seq 1 $BOX_INNER_WIDTH))╣${NC}"
    bot_border="${CYAN}╚$(printf '═%.0s' $(seq 1 $BOX_INNER_WIDTH))╝${NC}"

    echo ""
    echo -e "$top_border"

    # 标题居中，标题文字保持青色
    if [ "$MODE" = "install" ]; then
        title_raw="${APP_NAME} 安装完成！"
    else
        title_raw="${APP_NAME} 更新完成！"
    fi
    title_filled=$(center_text "$title_raw")
    title_line="${CYAN}║${title_filled}║${NC}"
    echo -e "$title_line"

    echo -e "$mid_border"

    # ========== 信息区块：标签普通白色，后面路径/地址部分单独绿色 ==========
    # 访问地址行
    label1="  访问地址:"
    val1=" http://${host_ip}:${KVM_PORT}"
    plain1="${label1}${val1}"
    pad1=$(pad_plain "$plain1")
    # 截取填充后的空白后缀
    suffix1="${pad1#"$plain1"}"
    line_info1="${CYAN}║${NC}${label1}${GREEN}${val1}${NC}${suffix1}${CYAN}║${NC}"

    # 安装目录行
    label2="  安装目录:"
    val2=" ${INSTALL_DIR}"
    plain2="${label2}${val2}"
    pad2=$(pad_plain "$plain2")
    suffix2="${pad2#"$plain2"}"
    line_info2="${CYAN}║${NC}${label2}${GREEN}${val2}${NC}${suffix2}${CYAN}║${NC}"

    # 配置文件行
    label3="  配置文件:"
    val3=" ${ENV_FILE}"
    plain3="${label3}${val3}"
    pad3=$(pad_plain "$plain3")
    suffix3="${pad3#"$plain3"}"
    line_info3="${CYAN}║${NC}${label3}${GREEN}${val3}${NC}${suffix3}${CYAN}║${NC}"

    echo -e "$line_info1"
    echo -e "$line_info2"
    echo -e "$line_info3"

    # 安装模式额外输出默认账号
    if [ "$MODE" = "install" ]; then
        label4="  默认账号:"
        val4=" admin / admin123"
        plain4="${label4}${val4}"
        pad4=$(pad_plain "$plain4")
        suffix4="${pad4#"$plain4"}"
        line_info4="${CYAN}║${NC}${label4}${GREEN}${val4}${NC}${suffix4}${CYAN}║${NC}"
        echo -e "$line_info4"
    fi

    echo -e "$mid_border"

    # ========== 命令区块：整行普通白色原色，不施加绿色 ==========
    c_raw1="  查看状态: systemctl status $SERVICE_NAME"
    c_fill1=$(pad_plain "$c_raw1")
    cmd_line1="${CYAN}║${NC}${c_fill1}${CYAN}║${NC}"

    c_raw2="  查看日志: journalctl -u $SERVICE_NAME -f"
    c_fill2=$(pad_plain "$c_raw2")
    cmd_line2="${CYAN}║${NC}${c_fill2}${CYAN}║${NC}"

    c_raw3="  重启服务: systemctl restart $SERVICE_NAME"
    c_fill3=$(pad_plain "$c_raw3")
    cmd_line3="${CYAN}║${NC}${c_fill3}${CYAN}║${NC}"

    echo -e "$cmd_line1"
    echo -e "$cmd_line2"
    echo -e "$cmd_line3"

    echo -e "$bot_border"
    echo ""

    if [ "$COMPATIBILITY_WARNING" -eq 1 ]; then
        warn "兼容性警告：实机测试未通过，面板虽已启动，但虚拟机创建或 OVS 网络可能异常"
        warn "诊断报告目录: ${COMPATIBILITY_REPORT_DIR}"
    elif [ "$COMPATIBILITY_SKIPPED" -eq 1 ]; then
        warn "兼容性警告：已跳过实机测试，宿主机尚未验证虚拟机创建与 OVS 网络"
        info "安装后可手动执行: sudo ${INSTALL_DIR}/scripts/${COMPATIBILITY_CHECK_SCRIPT}"
    fi
}

run_install_or_update() {
    check_kvm_hardware
    check_and_install_deps
    configure_qemu_for_rpm
    configure_libvirt_nonroot
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
    run_first_install_compatibility_check
    run_update_compatibility_check
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
    echo -e "${CYAN}║                     （开源版）                    ║${NC}"
    echo -e "${CYAN}╚══════════════════════════════════════════════════╝${NC}"
    echo ""

    check_root
    check_os
    check_arch
    check_locale
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
