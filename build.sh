#!/bin/bash
# ============================================================
# QVMConsole 本地打包脚本
# 构建前端 + 后端，自动检测宿主机架构，支持原生/交叉编译
# 产物: kvm-console-linux-{amd64|arm64}.tar.gz
# ============================================================

set -Eeuo pipefail

# 颜色定义
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
CYAN='\033[0;36m'
NC='\033[0m'

info()  { echo -e "${GREEN}[INFO]${NC} $1"; }
warn()  { echo -e "${YELLOW}[WARN]${NC} $1"; }
error() { echo -e "${RED}[ERROR]${NC} $1"; exit 1; }
success() { echo -e "${GREEN}[✓]${NC} $1"; }

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
SERVER_DIR="$SCRIPT_DIR/server"
WEB_DIR="$SCRIPT_DIR/web"
RELEASE_DIR="$SCRIPT_DIR/release"

# 自动检测宿主机架构
HOST_ARCH=$(uname -m)
case "$HOST_ARCH" in
    x86_64|amd64)   HOST_ARCH="amd64" ;;
    aarch64|arm64)  HOST_ARCH="arm64" ;;
    *)              HOST_ARCH="amd64" ;;  # 未知架构默认 amd64
esac

# 目标架构：默认与宿主机一致（原生编译）
TARGET_ARCH="$HOST_ARCH"

# ==================== 参数解析 ====================
VERSION=""
SKIP_FRONTEND=false
SKIP_BACKEND=false

usage() {
    echo "用法: $0 [选项]"
    echo ""
    echo "选项:"
    echo "  -v, --version VERSION    指定版本号 (例如: 1.0.0)"
    echo "  --target-arch ARCH       目标架构: amd64 或 arm64 (默认: ${HOST_ARCH})"
    echo "  --skip-frontend          跳过前端构建"
    echo "  --skip-backend           跳过后端构建"
    echo "  -h, --help               显示帮助信息"
    echo ""
    echo "示例:"
    echo "  $0                       原生构建，版本号为 dev"
    echo "  $0 -v 1.0.0             指定版本号原生构建"
    echo "  $0 --target-arch arm64   交叉编译 ARM64 版本"
    echo "  $0 --target-arch amd64   交叉编译 AMD64 版本"
}

while [[ $# -gt 0 ]]; do
    case "$1" in
        -v|--version)
            VERSION="$2"
            shift 2
            ;;
        --target-arch)
            TARGET_ARCH="$2"
            if [[ "$TARGET_ARCH" != "amd64" && "$TARGET_ARCH" != "arm64" ]]; then
                error "不支持的架构: ${TARGET_ARCH}，仅支持 amd64 / arm64"
            fi
            shift 2
            ;;
        --skip-frontend)
            SKIP_FRONTEND=true
            shift
            ;;
        --skip-backend)
            SKIP_BACKEND=true
            shift
            ;;
        -h|--help)
            usage
            exit 0
            ;;
        *)
            error "未知参数: $1，使用 -h 查看帮助"
            ;;
    esac
done

# 版本号处理：去除可能的 v 前缀，构建时统一加 v
if [ -n "$VERSION" ]; then
    VERSION="${VERSION#v}"
else
    VERSION="dev"
fi

BUILD_VERSION="v${VERSION}"
BUILD_TIME=$(date -u +"%Y-%m-%dT%H:%M:%SZ")

# 根据目标架构确定输出名和 Go 编译参数
OUTPUT_NAME="kvm-console-linux-${TARGET_ARCH}"
GOARCH_VALUE="$TARGET_ARCH"  # Go GOARCH 与我们的命名一致（amd64/arm64）
IS_CROSS_COMPILE=false
if [ "$TARGET_ARCH" != "$HOST_ARCH" ]; then
    IS_CROSS_COMPILE=true
fi

echo ""
echo -e "${CYAN}╔══════════════════════════════════════════════════╗${NC}"
echo -e "${CYAN}║         QVMConsole 构建打包脚本                  ║${NC}"
echo -e "${CYAN}╠══════════════════════════════════════════════════╣${NC}"
echo -e "${CYAN}║${NC}  版本:   ${GREEN}${BUILD_VERSION}${NC}"
echo -e "${CYAN}║${NC}  时间:   ${GREEN}${BUILD_TIME}${NC}"
echo -e "${CYAN}║${NC}  宿主机: ${GREEN}${HOST_ARCH}${NC}"
echo -e "${CYAN}║${NC}  目标:   ${GREEN}${TARGET_ARCH}${NC}"
if [ "$IS_CROSS_COMPILE" = true ]; then
    echo -e "${CYAN}║${NC}  模式:   ${YELLOW}交叉编译${NC}"
else
    echo -e "${CYAN}║${NC}  模式:   ${GREEN}原生编译${NC}"
fi
echo -e "${CYAN}╚══════════════════════════════════════════════════╝${NC}"
echo ""

# ==================== 清理旧产物 ====================
info "清理旧构建产物..."
rm -rf "$RELEASE_DIR"
mkdir -p "$RELEASE_DIR/${OUTPUT_NAME}"

# ==================== 构建前端 ====================
if [ "$SKIP_FRONTEND" = false ]; then
    info "检查前端环境..."
    if ! command -v npm &>/dev/null; then
        error "npm 未安装，请先安装 Node.js (推荐 v20+)"
    fi

    info "安装前端依赖..."
    cd "$WEB_DIR"
    npm ci

    info "构建前端..."
    npm run build

    if [ ! -d "$WEB_DIR/dist" ]; then
        error "前端构建失败，未生成 dist 目录"
    fi
    success "前端构建完成"
else
    warn "跳过前端构建"
    if [ ! -d "$WEB_DIR/dist" ]; then
        error "前端 dist 目录不存在，无法跳过构建"
    fi
fi

# ==================== 构建后端 ====================
if [ "$SKIP_BACKEND" = false ]; then
    info "检查后端环境..."
    if ! command -v go &>/dev/null; then
        error "Go 未安装，请先安装 Go (参考 server/go.mod 中的版本要求)"
    fi

    info "构建后端二进制..."
    cd "$SERVER_DIR"

    # CGO 交叉编译检测
    if [ "$IS_CROSS_COMPILE" = true ]; then
        cross_cc=$(GOOS=linux GOARCH="$GOARCH_VALUE" go env CC 2>/dev/null || true)
        if [ -z "$cross_cc" ] || ! command -v "$cross_cc" >/dev/null 2>&1; then
            warn "CGO 交叉编译需要安装对应交叉编译器"
            if [ "$TARGET_ARCH" = "amd64" ]; then
                warn "  请执行: apt-get install gcc-x86-64-linux-gnu"
            elif [ "$TARGET_ARCH" = "arm64" ]; then
                warn "  请执行: apt-get install gcc-aarch64-linux-gnu"
            fi
            error "缺少交叉编译器 ${cross_cc:-gcc-${TARGET_ARCH}-linux-gnu}，无法完成 CGO 交叉编译"
        fi
        info "检测到交叉编译器: ${cross_cc}"
    fi

    CGO_ENABLED=1 GOOS=linux GOARCH="$GOARCH_VALUE" \
        go build \
        -ldflags="-s -w \
            -X main.Version=${BUILD_VERSION} \
            -X kvm_console/handler.Version=${BUILD_VERSION} \
            -X kvm_console/handler.BuildTime=${BUILD_TIME}" \
        -o "$RELEASE_DIR/${OUTPUT_NAME}/kvm-console" \
        .

    if [ ! -f "$RELEASE_DIR/${OUTPUT_NAME}/kvm-console" ]; then
        error "后端构建失败，未生成二进制文件"
    fi
    success "后端构建完成"
else
    warn "跳过后端构建"
fi

# ==================== 打包发行文件 ====================
info "打包发行文件..."

# 复制前端静态文件
cp -r "$WEB_DIR/dist" "$RELEASE_DIR/${OUTPUT_NAME}/web-dist"

# 复制安装脚本
cp "$SCRIPT_DIR/install.sh" "$RELEASE_DIR/${OUTPUT_NAME}/"
chmod +x "$RELEASE_DIR/${OUTPUT_NAME}/install.sh"

# 设置后端二进制可执行权限
if [ -f "$RELEASE_DIR/${OUTPUT_NAME}/kvm-console" ]; then
    chmod +x "$RELEASE_DIR/${OUTPUT_NAME}/kvm-console"
fi

# 生成 tar.gz
cd "$RELEASE_DIR"
tar -czf "${OUTPUT_NAME}.tar.gz" "${OUTPUT_NAME}/"

PACKAGE_SIZE=$(du -sh "$RELEASE_DIR/${OUTPUT_NAME}.tar.gz" | cut -f1)

echo ""
echo -e "${CYAN}╔══════════════════════════════════════════════════╗${NC}"
echo -e "${CYAN}║         构建完成！                               ║${NC}"
echo -e "${CYAN}╠══════════════════════════════════════════════════╣${NC}"
echo -e "${CYAN}║${NC}  产物:   ${GREEN}release/${OUTPUT_NAME}.tar.gz${NC}"
echo -e "${CYAN}║${NC}  大小:   ${GREEN}${PACKAGE_SIZE}${NC}"
echo -e "${CYAN}║${NC}  版本:   ${GREEN}${BUILD_VERSION}${NC}"
echo -e "${CYAN}║${NC}  架构:   ${GREEN}${TARGET_ARCH}${NC}"
echo -e "${CYAN}╠══════════════════════════════════════════════════╣${NC}"
echo -e "${CYAN}║${NC}  内容:"
echo -e "${CYAN}║${NC}    - kvm-console        后端二进制 (${TARGET_ARCH})"
echo -e "${CYAN}║${NC}    - web-dist/          前端静态文件"
echo -e "${CYAN}║${NC}    - install.sh         安装脚本"
echo -e "${CYAN}╚══════════════════════════════════════════════════╝${NC}"
echo ""
