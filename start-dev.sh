#!/bin/bash
# ============================================================
# QVMConsole 一键启动脚本
# 同时启动后端（air 热重载）和前端（vite dev）
# ============================================================

set -e

# 确保 GOPATH/bin 在 PATH 中
export PATH="$PATH:$(go env GOPATH)/bin:/usr/local/go/bin"

# 颜色定义
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
CYAN='\033[0;36m'
RED='\033[0;31m'
NC='\033[0m'

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
SERVER_DIR="$SCRIPT_DIR/server"
WEB_DIR="$SCRIPT_DIR/web"

info()  { echo -e "${GREEN}[INFO]${NC} $1"; }
warn()  { echo -e "${YELLOW}[WARN]${NC} $1"; }
error() { echo -e "${RED}[ERROR]${NC} $1"; }

# 清理函数：退出时杀掉所有子进程
cleanup() {
    echo ""
    info "正在停止所有服务..."
    kill $BACKEND_PID $FRONTEND_PID 2>/dev/null
    wait $BACKEND_PID $FRONTEND_PID 2>/dev/null
    info "服务已停止"
}
trap cleanup EXIT INT TERM

# ==================== 检查 CGO 环境 ====================
# go-sqlite3 是 CGO 库，CGO_ENABLED=0 时会编译成空壳（stub），
# 运行时报 "Binary was compiled with 'CGO_ENABLED=0'..." 错误。
# 这里确保 C 编译器可用并持久化启用 CGO。
ensure_cgo() {
    # 持久化启用 CGO（写入 go env 配置，一次设置永久生效）
    if [ "$(go env CGO_ENABLED)" != "1" ]; then
        info "持久化启用 CGO_ENABLED=1 ..."
        go env -w CGO_ENABLED=1
    fi
    # 当前 shell 显式覆盖（防止环境变量层把 CGO 关掉）
    export CGO_ENABLED=1

    # 检查 C 编译器
    if command -v gcc &>/dev/null; then
        info "C 编译器: gcc $(gcc -dumpversion)"
        return 0
    fi

    # 非 Linux 平台提示手动安装
    if [ "$(uname -s)" != "Linux" ]; then
        warn "未检测到 gcc，请手动安装 C 编译器后再运行（macOS: xcode-select --install；Windows: mingw-w64）"
        return 0
    fi

    # Linux 下自动安装 build-essential / gcc-c++ make
    local pkg_mgr=""
    if command -v apt-get &>/dev/null; then
        pkg_mgr="apt-get"
    elif command -v dnf &>/dev/null; then
        pkg_mgr="dnf"
    elif command -v yum &>/dev/null; then
        pkg_mgr="yum"
    else
        error "未检测到包管理器，请手动安装 C 编译器（Debian/Ubuntu: build-essential；RHEL 系: gcc-c++ make）"
        exit 1
    fi

    local need_sudo=""
    if [ "$(id -u)" -ne 0 ]; then
        need_sudo="sudo"
    fi

    local pkg_name="build-essential"
    if [ "$pkg_mgr" = "dnf" ] || [ "$pkg_mgr" = "yum" ]; then
        pkg_name="gcc-c++ make"
    fi

    warn "未检测到 gcc，go-sqlite3 依赖 CGO 需要 C 编译器，正在安装 $pkg_name（首次安装可能需要几分钟）..."
    if ! $need_sudo $pkg_mgr install -y $pkg_name; then
        error "安装 C 编译器失败，请手动安装: $pkg_name"
        exit 1
    fi
    info "C 编译器安装完成: gcc $(gcc -dumpversion)"
}

# ==================== 检查依赖 ====================
info "检查依赖..."
ensure_cgo

# 检查 air（兼容 Go 1.22~1.24，使用 v1.61.7）
if ! command -v air &>/dev/null; then
    # 也检查 GOPATH/bin 下是否已有
    if [ -x "$(go env GOPATH)/bin/air" ]; then
        info "air 已存在于 $(go env GOPATH)/bin/air"
    else
        warn "air 未安装，正在安装 v1.61.7 ..."
        go install github.com/air-verse/air@v1.61.7
        if [ ! -x "$(go env GOPATH)/bin/air" ]; then
            error "air 安装失败，请手动安装: go install github.com/air-verse/air@v1.61.7"
            exit 1
        fi
    fi
fi

# 检查 node/npm
if ! command -v npm &>/dev/null; then
    error "npm 未安装，请先安装 Node.js"
    exit 1
fi

# ==================== 安装前端依赖 ====================
if [ ! -d "$WEB_DIR/node_modules" ]; then
    info "安装前端依赖..."
    cd "$WEB_DIR" && npm install
fi

# ==================== 启动后端 ====================
echo ""
echo -e "${CYAN}╔══════════════════════════════════════╗${NC}"
echo -e "${CYAN}║     QVMConsole 开发环境启动中       ║${NC}"
echo -e "${CYAN}╚══════════════════════════════════════╝${NC}"
echo ""

# ==================== 日志配置 ====================
# KVM_LOG_DIR          日志存储目录（默认 ./log）
# KVM_LOG_LEVEL        文件日志最低级别: debug/info/warn/error（默认 info）
# KVM_LOG_MAX_DAYS     日志最大保留天数（默认 7）
# KVM_LOG_COMPRESS     是否压缩归档日志: true/false（默认 true）
# KVM_LOG_MAX_SIZE_MB  单个日志文件最大MB（默认 100）
# KVM_LOG_CONSOLE      是否输出到终端: true/false（默认 true）
# KVM_LOG_CONSOLE_TYPES 终端显示的日志类型，逗号分隔（默认 app,cmd,libvirt）
#                       可选值: app, request, cmd, libvirt, all
#                       示例: 只看libvirt → "libvirt"
#                             看全部 → "all"
#                             看app和cmd → "app,cmd"
# KVM_LOG_CONSOLE_LEVEL 终端独立日志级别（留空跟随 KVM_LOG_LEVEL）
#                       示例: 终端只看warn以上 → "warn"
export KVM_LOG_CONSOLE_TYPES=""
export KVM_LOG_CONSOLE_LEVEL="warn"
# KVM_TMPDIR           临时文件目录（multipart 上传暂存），留空则自动使用服务器 tmp 目录
#                       如果 /tmp 是 tmpfs 建议设置此变量到磁盘目录，避免大文件上传时空间不足
#                       示例: export KVM_TMPDIR=/opt/project/QVMConsole/server/tmp
#export KVM_TMPDIR="${KVM_TMPDIR:-}"
# ====================================================

info "启动后端 (air 热重载)..."
cd "$SERVER_DIR" && KVM_DEVELOPMENT_MODE=true air &
BACKEND_PID=$!

# ==================== 启动前端 ====================
info "启动前端 (vite dev, 0.0.0.0)..."
cd "$WEB_DIR" && npx vite --host 0.0.0.0 &
FRONTEND_PID=$!

echo ""
info "========================================="
info "  后端: http://localhost:8080"
info "  前端: http://0.0.0.0:5173"
info "  按 Ctrl+C 停止所有服务"
info "========================================="
echo ""

# 等待任一进程退出
wait -n $BACKEND_PID $FRONTEND_PID 2>/dev/null
