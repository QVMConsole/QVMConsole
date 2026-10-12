#!/bin/bash
# ============================================================
# QVMConsole 一键启动脚本
# 同时启动后端（air 热重载）和前端（vite dev）
# 启动前检测前后端端口，被占用时直接结束占用进程
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

# 子进程 PID 先置空，退出清理时避免误杀
BACKEND_PID=""
FRONTEND_PID=""

info()  { echo -e "${GREEN}[INFO]${NC} $1"; }
warn()  { echo -e "${YELLOW}[WARN]${NC} $1"; }
error() { echo -e "${RED}[ERROR]${NC} $1"; }

# ==================== 端口配置 ====================
# 端口不写死，均可用环境变量覆盖：
#   KVM_PORT            后端端口（与后端配置项 KVM_PORT 保持一致，默认 8080）
#   KVM_WEB_PORT        前端 dev 端口（默认 5173）
#   KVM_PORT_KILL       置为 0 时只检测不结束占用进程（默认 1，直接结束）
#   KVM_PORT_TERM_WAIT  占用进程的优雅退出等待秒数（默认 10，超时后 SIGKILL）
BACKEND_HOST="0.0.0.0"
BACKEND_PORT="${KVM_PORT:-8080}"
FRONTEND_HOST="0.0.0.0"
FRONTEND_PORT="${KVM_WEB_PORT:-5173}"
PORT_TERM_WAIT="${KVM_PORT_TERM_WAIT:-10}"
export KVM_PORT="$BACKEND_PORT"

# 可连带结束的父进程特征：air/vite 等检测到子进程退出会自动重新拉起，需一并结束
DEV_PROC_PATTERN='(^|/)(air|vite|npm|npx|pnpm|yarn|node|go|kvm-console|esbuild|rollup|start-dev\.sh)( |$)'

# 输出监听指定端口的原始信息（权限不足时可能不含 pid）
port_raw() {
    local port="$1"
    if command -v ss &>/dev/null; then
        { ss -lptnH "sport = :$port" 2>/dev/null || ss -lptn "sport = :$port" 2>/dev/null; } | grep -v '^State'
    elif command -v lsof &>/dev/null; then
        lsof -nP -iTCP:"$port" -sTCP:LISTEN 2>/dev/null | tail -n +2
    elif command -v fuser &>/dev/null; then
        fuser -n tcp "$port" 2>/dev/null | tr -s ' ' '\n' | grep -E '^[0-9]+$' | while read -r pid; do
            ps -o pid=,args= -p "$pid" 2>/dev/null
        done
    fi
}

# 监听指定端口的进程 PID（去重）
port_pids() {
    local port="$1"
    if command -v ss &>/dev/null; then
        port_raw "$port" | grep -o 'pid=[0-9]\+' | cut -d= -f2 | sort -u
    elif command -v lsof &>/dev/null; then
        lsof -tiTCP:"$port" -sTCP:LISTEN 2>/dev/null | sort -u
    elif command -v fuser &>/dev/null; then
        fuser -n tcp "$port" 2>/dev/null | tr -s ' ' '\n' | grep -E '^[0-9]+$' | sort -u
    fi
}

# 端口是否处于监听状态
port_busy() {
    [ -n "$(port_raw "$1")" ]
}

# 进程命令行
proc_args() {
    local pid="$1"
    if [ -r "/proc/$pid/cmdline" ]; then
        tr '\0' ' ' < "/proc/$pid/cmdline"
    else
        ps -o args= -p "$pid" 2>/dev/null
    fi
}

# 父进程 PID
proc_ppid() {
    local pid="$1" ppid=""
    if [ -r "/proc/$pid/status" ]; then
        ppid="$(awk '/^PPid:/{print $2}' "/proc/$pid/status" 2>/dev/null)"
    fi
    if [ -z "$ppid" ]; then
        ppid="$(ps -o ppid= -p "$pid" 2>/dev/null | tr -d ' ')"
    fi
    echo "$ppid"
}

# 收集需要结束的进程：占用端口的进程 + 由开发工具拉起的父进程链（子进程在前）
collect_kill_pids() {
    local port="$1" pid ppid depth
    for pid in $(port_pids "$port"); do
        depth=0
        while [ -n "$pid" ] && [ "$pid" != "1" ] && [ "$pid" != "$$" ] && [ "$pid" != "$PPID" ] && [ "$depth" -lt 4 ]; do
            echo "$pid"
            ppid="$(proc_ppid "$pid")"
            if [ -z "$ppid" ]; then
                break
            fi
            # 只有父进程也是开发工具时才继续向上，避免误杀终端会话
            if echo "$(proc_args "$ppid")" | grep -qE "$DEV_PROC_PATTERN"; then
                pid="$ppid"
                depth=$((depth + 1))
            else
                break
            fi
        done
    done | awk '!seen[$0]++'
}

# 结束单个进程：自身为进程组首进程时整组结束，避免子进程残留继续抢占端口
kill_one() {
    local sig="$1" pid="$2" pgid
    pgid="$(ps -o pgid= -p "$pid" 2>/dev/null | tr -d ' ')"
    if [ -n "$pgid" ] && [ "$pgid" = "$pid" ] && [ "$pgid" != "$$" ]; then
        kill -"$sig" -- "-$pgid" 2>/dev/null && return 0
    fi
    kill -"$sig" "$pid" 2>/dev/null || true
}

# 检测端口占用，占用则直接结束占用进程
kill_port() {
    local port="$1" name="$2" pid targets waited
    if ! port_busy "$port"; then
        info "端口 $port（$name）空闲"
        return 0
    fi

    if [ -z "$(port_pids "$port")" ]; then
        error "端口 $port 已被占用，但当前用户无权查看占用进程，请使用 root 或 sudo 重新运行。"
        exit 1
    fi

    # 允许通过 KVM_PORT_KILL=0 关闭自动结束，仅做检测
    if [ "${KVM_PORT_KILL:-1}" != "1" ]; then
        error "端口 $port（$name）已被占用，且已设置 KVM_PORT_KILL=0，不自动结束，请手动处理后重试。"
        exit 1
    fi

    warn "端口 $port（$name）已被占用，占用进程如下，即将直接结束："
    for pid in $(collect_kill_pids "$port"); do
        echo -e "    ${YELLOW}PID $pid${NC}  $(proc_args "$pid")"
    done

    targets="$(collect_kill_pids "$port")"
    info "正在结束占用进程..."
    for pid in $targets; do
        kill_one TERM "$pid"
    done

    # 等待优雅退出，超时则强制结束
    waited=0
    while [ "$waited" -lt "$PORT_TERM_WAIT" ] && port_busy "$port"; do
        sleep 1
        waited=$((waited + 1))
    done

    if port_busy "$port"; then
        warn "进程未在 ${PORT_TERM_WAIT}s 内退出，强制结束（SIGKILL）..."
        for pid in $targets; do
            kill_one KILL "$pid"
        done
        sleep 1
    fi

    if port_busy "$port"; then
        error "端口 $port 仍被占用，请手动处理后重试，例如：ss -lptn 'sport = :$port'"
        exit 1
    fi
    info "端口 $port 已释放"
}

# 启动前统一检查前后端端口
ensure_ports_free() {
    info "检查端口占用..."
    kill_port "$BACKEND_PORT" "后端"
    kill_port "$FRONTEND_PORT" "前端"
    if [ "$BACKEND_PORT" != "8080" ]; then
        warn "后端端口已改为 $BACKEND_PORT，请同步修改 web/vite.config.ts 中的代理地址"
    fi
}

# 清理函数：退出时杀掉所有子进程
cleanup() {
    echo ""
    info "正在停止所有服务..."
    if [ -n "$BACKEND_PID" ]; then
        kill "$BACKEND_PID" 2>/dev/null || true
    fi
    if [ -n "$FRONTEND_PID" ]; then
        kill "$FRONTEND_PID" 2>/dev/null || true
    fi
    wait 2>/dev/null || true
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

# ==================== 端口占用检查 ====================
# 端口被占用时直接结束占用进程（含 air/vite 等会自动拉起子进程的父进程）
ensure_ports_free

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

info "启动后端 (air 热重载, $BACKEND_HOST:$BACKEND_PORT)..."
cd "$SERVER_DIR" && KVM_DEVELOPMENT_MODE=true air &
BACKEND_PID=$!

# ==================== 启动前端 ====================
info "启动前端 (vite dev, $FRONTEND_HOST:$FRONTEND_PORT)..."
cd "$WEB_DIR" && npx vite --host "$FRONTEND_HOST" --port "$FRONTEND_PORT" --strictPort &
FRONTEND_PID=$!

echo ""
info "========================================="
info "  后端: http://localhost:$BACKEND_PORT"
info "  前端: http://$FRONTEND_HOST:$FRONTEND_PORT"
info "  按 Ctrl+C 停止所有服务"
info "========================================="
echo ""

# 等待任一进程退出
wait -n $BACKEND_PID $FRONTEND_PID 2>/dev/null
