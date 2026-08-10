#!/usr/bin/env bash
# QVMConsole 安装环境实机兼容性测试入口。

set -Eeuo pipefail

RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
CYAN='\033[0;36m'
NC='\033[0m'

info() { echo -e "${CYAN}[INFO]${NC} $1"; }
warn() { echo -e "${YELLOW}[WARN]${NC} $1"; }
error() { echo -e "${RED}[ERROR]${NC} $1" >&2; }
success() { echo -e "${GREEN}[✓]${NC} $1"; }

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
BINARY_PATH=""
REPORT_DIR=""
VCPU=1
RAM_GB=1
DISK_GB=1

usage() {
    cat <<'EOF'
用法: check-system-compatibility.sh [选项]

选项:
  --binary PATH       指定 kvm-console 可执行文件
  --report-dir PATH   指定兼容性报告目录
  --vcpu N            测试虚拟机 vCPU 数量，默认 1
  --ram-gb N          测试虚拟机内存（GB），默认 1
  --disk-gb N         测试虚拟机磁盘（GB），默认 1
  -h, --help          显示帮助
EOF
}

require_positive_integer() {
    local label="$1"
    local value="$2"
    if ! [[ "$value" =~ ^[1-9][0-9]*$ ]]; then
        error "${label} 必须是大于 0 的整数"
        exit 2
    fi
}

resolve_binary() {
    if [ -n "$BINARY_PATH" ]; then
        BINARY_PATH="$(readlink -f "$BINARY_PATH" 2>/dev/null || printf '%s' "$BINARY_PATH")"
        return
    fi

    local candidate
    for candidate in \
        "${SCRIPT_DIR}/kvm-console" \
        "${SCRIPT_DIR}/../kvm-console" \
        "/opt/kvm-console/kvm-console"; do
        if [ -x "$candidate" ]; then
            BINARY_PATH="$(readlink -f "$candidate" 2>/dev/null || printf '%s' "$candidate")"
            return
        fi
    done
}

while [ "$#" -gt 0 ]; do
    case "$1" in
        --binary)
            [ "$#" -ge 2 ] || { error "--binary 缺少路径"; exit 2; }
            BINARY_PATH="$2"
            shift 2
            ;;
        --report-dir)
            [ "$#" -ge 2 ] || { error "--report-dir 缺少路径"; exit 2; }
            REPORT_DIR="$2"
            shift 2
            ;;
        --vcpu)
            [ "$#" -ge 2 ] || { error "--vcpu 缺少数值"; exit 2; }
            VCPU="$2"
            shift 2
            ;;
        --ram-gb)
            [ "$#" -ge 2 ] || { error "--ram-gb 缺少数值"; exit 2; }
            RAM_GB="$2"
            shift 2
            ;;
        --disk-gb)
            [ "$#" -ge 2 ] || { error "--disk-gb 缺少数值"; exit 2; }
            DISK_GB="$2"
            shift 2
            ;;
        -h|--help)
            usage
            exit 0
            ;;
        *)
            error "未知参数: $1"
            usage >&2
            exit 2
            ;;
    esac
done

if [ "$(id -u)" -ne 0 ]; then
    error "请使用 root 用户或 sudo 执行兼容性测试"
    exit 1
fi

require_positive_integer "vCPU" "$VCPU"
require_positive_integer "内存" "$RAM_GB"
require_positive_integer "磁盘" "$DISK_GB"
resolve_binary

for command_name in ovs-vsctl ovs-ofctl ovsdb-client; do
    if ! command -v "$command_name" >/dev/null 2>&1; then
        error "未找到端口安全兼容性测试所需命令: ${command_name}"
        exit 1
    fi
done
info "将验证 OVS packet meter、包速率 policing、OpenFlow bundle/兼容流程及资源清理"

if [ -z "$BINARY_PATH" ] || [ ! -x "$BINARY_PATH" ]; then
    error "未找到可执行的 kvm-console，请通过 --binary 指定路径"
    exit 1
fi

BINARY_DIR="$(cd "$(dirname "$BINARY_PATH")" && pwd)"
if [ -z "$REPORT_DIR" ]; then
    REPORT_DIR="${BINARY_DIR}/logs/compatibility"
fi
mkdir -p "$REPORT_DIR"
chmod 700 "$REPORT_DIR"
REPORT_DIR="$(cd "$REPORT_DIR" && pwd)"
RUN_LOG="${REPORT_DIR}/compatibility-run-$(date -u +%Y%m%d-%H%M%S)-$$.log"

info "后端程序: ${BINARY_PATH}"
info "报告目录: ${REPORT_DIR}"
info "测试规格: ${VCPU} vCPU / ${RAM_GB}GB 内存 / ${DISK_GB}GB 磁盘"
warn "测试会创建并启动一台临时虚拟机，完成后自动清理"

trap 'warn "收到中断信号，正在等待后端清理临时资源"' INT TERM
set +e
(
    cd "$BINARY_DIR"
    "$BINARY_PATH" system-compatibility-check \
        --vcpu "$VCPU" \
        --ram-gb "$RAM_GB" \
        --disk-gb "$DISK_GB" \
        --report-dir "$REPORT_DIR"
) 2>&1 | tee "$RUN_LOG"
status=${PIPESTATUS[0]}
set -e
trap - INT TERM
chmod 600 "$RUN_LOG"

if [ "$status" -eq 0 ]; then
    success "系统兼容性测试通过"
    info "运行日志: ${RUN_LOG}"
    exit 0
fi

error "系统兼容性测试未通过，请查看报告和运行日志"
info "运行日志: ${RUN_LOG}"
exit "$status"
