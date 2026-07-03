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
BUILD_VARIANT=""  # 构建变体：空=全部, compat=zig兼容版, native=宿主机原生版

usage() {
    echo "用法: $0 [选项]"
    echo ""
    echo "选项:"
    echo "  -v, --version VERSION    指定版本号 (例如: 1.0.0)"
    echo "  --target-arch ARCH       目标架构: amd64 或 arm64 (默认: ${HOST_ARCH})"
    echo "  --variant VARIANT        构建变体: compat(兼容版) / native(原生版) (默认: 全部)"
    echo "  --skip-frontend          跳过前端构建"
    echo "  --skip-backend           跳过后端构建"
    echo "  -h, --help               显示帮助信息"
    echo ""
    echo "示例:"
    echo "  $0                       构建全部，版本号为 dev"
    echo "  $0 -v 1.0.0             指定版本号构建全部"
    echo "  $0 --variant compat      仅构建 zig 兼容版（最低 GLIBC 2.2.5）"
    echo "  $0 --variant native      仅构建宿主机原生版"
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
        --variant)
            BUILD_VARIANT="$2"
            if [[ "$BUILD_VARIANT" != "compat" && "$BUILD_VARIANT" != "native" ]]; then
                error "不支持的构建变体: ${BUILD_VARIANT}，仅支持 compat / native"
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

    cd "$SERVER_DIR"

    # 确定需要构建哪些变体
    BUILD_COMPAT=false
    BUILD_NATIVE=false
    case "${BUILD_VARIANT}" in
        "")     BUILD_COMPAT=true; BUILD_NATIVE=true ;;
        compat)  BUILD_COMPAT=true ;;
        native)  BUILD_NATIVE=true ;;
    esac

    # ========== 构建 zig 兼容版（最低 GLIBC 2.2.5） ==========
    if [ "$BUILD_COMPAT" = true ]; then
        info "构建 zig 兼容版..."

        # CGO 编译检测：优先使用 zig，回退到 gcc 交叉编译器
        if [ "${CGO_ENABLED:-1}" = "1" ]; then
            if command -v zig &>/dev/null; then
                if [ "$IS_CROSS_COMPILE" = true ]; then
                    if [ "$TARGET_ARCH" = "amd64" ]; then
                        export CC="zig cc -target x86_64-linux-gnu"
                        export CXX="zig cxx -target x86_64-linux-gnu"
                    elif [ "$TARGET_ARCH" = "arm64" ]; then
                        export CC="zig cc -target aarch64-linux-gnu"
                        export CXX="zig cxx -target aarch64-linux-gnu"
                    fi
                else
                    export CC="zig cc"
                    export CXX="zig cxx"
                fi
                info "使用 zig 作为 C 编译器: ${CC}"
            elif [ "$IS_CROSS_COMPILE" = true ]; then
                cross_cc=$(GOOS=linux GOARCH="$GOARCH_VALUE" go env CC 2>/dev/null || true)
                if [ -z "$cross_cc" ] || ! command -v "$cross_cc" >/dev/null 2>&1; then
                    warn "CGO 交叉编译需要安装交叉编译器或 zig"
                    if [ "$TARGET_ARCH" = "amd64" ]; then
                        warn "  方案1: apt-get install gcc-x86-64-linux-gnu"
                        warn "  方案2: 安装 zig (推荐，兼容旧版 glibc)"
                    elif [ "$TARGET_ARCH" = "arm64" ]; then
                        warn "  方案1: apt-get install gcc-aarch64-linux-gnu"
                        warn "  方案2: 安装 zig (推荐，兼容旧版 glibc)"
                    fi
                    error "缺少交叉编译器，请安装 gcc 交叉编译器或 zig"
                fi
                info "检测到交叉编译器: ${cross_cc}"
            fi
        fi

        CGO_ENABLED=${CGO_ENABLED:-1} GOOS=linux GOARCH="$GOARCH_VALUE" \
            go build \
            -ldflags="-s -w \
                -X main.Version=${BUILD_VERSION} \
                -X kvm_console/handler.Version=${BUILD_VERSION} \
                -X kvm_console/handler.BuildTime=${BUILD_TIME}" \
            -o "$RELEASE_DIR/${OUTPUT_NAME}/kvm-console" \
            .

        if [ ! -f "$RELEASE_DIR/${OUTPUT_NAME}/kvm-console" ]; then
            error "zig 兼容版构建失败，未生成二进制文件"
        fi
        success "zig 兼容版构建完成（最低 GLIBC 2.2.5）"
    fi

    # ========== 构建宿主机原生版 ==========
    if [ "$BUILD_NATIVE" = true ]; then
        info "构建宿主机原生版..."

        # 清除 zig 编译器环境，使用系统默认编译器
        saved_cc="${CC:-}"
        saved_cxx="${CXX:-}"
        unset CC CXX

        native_output="kvm-console"
        if [ "$BUILD_COMPAT" = true ]; then
            native_output="kvm-console-native"  # 双构建时加后缀区分
        fi

        # 交叉编译且无 zig 时，检测 gcc 交叉编译器
        if [ "${CGO_ENABLED:-1}" = "1" ] && [ "$IS_CROSS_COMPILE" = true ]; then
            cross_cc=$(GOOS=linux GOARCH="$GOARCH_VALUE" go env CC 2>/dev/null || true)
            if [ -z "$cross_cc" ] || ! command -v "$cross_cc" >/dev/null 2>&1; then
                warn "CGO 交叉编译需要安装交叉编译器"
                if [ "$TARGET_ARCH" = "amd64" ]; then
                    warn "  请执行: apt-get install gcc-x86-64-linux-gnu"
                elif [ "$TARGET_ARCH" = "arm64" ]; then
                    warn "  请执行: apt-get install gcc-aarch64-linux-gnu"
                fi
                error "缺少交叉编译器 ${cross_cc:-gcc-${TARGET_ARCH}-linux-gnu}，无法完成 CGO 交叉编译"
            fi
            info "检测到交叉编译器: ${cross_cc}"
        fi

        CGO_ENABLED=${CGO_ENABLED:-1} GOOS=linux GOARCH="$GOARCH_VALUE" \
            go build \
            -ldflags="-s -w \
                -X main.Version=${BUILD_VERSION} \
                -X kvm_console/handler.Version=${BUILD_VERSION} \
                -X kvm_console/handler.BuildTime=${BUILD_TIME}" \
            -o "$RELEASE_DIR/${OUTPUT_NAME}/${native_output}" \
            .

        export CC="$saved_cc"
        export CXX="$saved_cxx"

        if [ ! -f "$RELEASE_DIR/${OUTPUT_NAME}/${native_output}" ]; then
            error "宿主机原生版构建失败，未生成二进制文件"
        fi
        success "宿主机原生版构建完成"
    fi
else
    warn "跳过后端构建"
fi

# ==================== 打包发行文件 ====================
info "打包发行文件..."

# 安全校验：后端二进制必须至少存在一个
if [ ! -f "$RELEASE_DIR/${OUTPUT_NAME}/kvm-console" ] && [ ! -f "$RELEASE_DIR/${OUTPUT_NAME}/kvm-console-native" ]; then
    error "后端二进制不存在于 ${RELEASE_DIR}/${OUTPUT_NAME}/。\n  若使用 --skip-backend，请确保之前已成功构建且未清空 release 目录。\n  建议：不带 --skip-backend 重新构建。"
fi

# 复制前端静态文件
cp -r "$WEB_DIR/dist" "$RELEASE_DIR/${OUTPUT_NAME}/web-dist"

# 复制安装脚本
cp "$SCRIPT_DIR/install.sh" "$RELEASE_DIR/${OUTPUT_NAME}/"
chmod +x "$RELEASE_DIR/${OUTPUT_NAME}/install.sh"

# 设置后端二进制可执行权限
if [ -f "$RELEASE_DIR/${OUTPUT_NAME}/kvm-console" ]; then
    chmod +x "$RELEASE_DIR/${OUTPUT_NAME}/kvm-console"
fi
if [ -f "$RELEASE_DIR/${OUTPUT_NAME}/kvm-console-native" ]; then
    chmod +x "$RELEASE_DIR/${OUTPUT_NAME}/kvm-console-native"
fi

# ==================== 下载捆绑的 RPM 包（用于 Kylin/openEuler 等缺少的包）====================
info "下载捆绑的 RPM 包..."
mkdir -p "$RELEASE_DIR/${OUTPUT_NAME}/bundled"

# arp-scan: 在 Kylin/openEuler 默认源中不存在，从 EPEL 获取
ARP_SCAN_RPM_URL=""
if [ "$TARGET_ARCH" = "amd64" ]; then
    ARP_SCAN_RPM_URL="https://dl.fedoraproject.org/pub/epel/8/Everything/x86_64/Packages/a/arp-scan-1.10.0-1.el8.x86_64.rpm"
elif [ "$TARGET_ARCH" = "arm64" ]; then
    ARP_SCAN_RPM_URL="https://dl.fedoraproject.org/pub/epel/8/Everything/aarch64/Packages/a/arp-scan-1.10.0-1.el8.aarch64.rpm"
fi

if [ -n "$ARP_SCAN_RPM_URL" ]; then
    if curl -fL --connect-timeout 10 "$ARP_SCAN_RPM_URL" -o "$RELEASE_DIR/${OUTPUT_NAME}/bundled/arp-scan.rpm" 2>/dev/null; then
        success "arp-scan RPM 下载完成"
    else
        warn "arp-scan RPM 下载失败，该功能将在系统中不可用时跳过（不影响核心功能）"
    fi
fi

# libguestfs-tools-c: 包含 virt-filesystems、virt-customize 等 C 工具
# 在 Kylin 上 guestfs-tools 包可能因依赖不足安装失败，从 AlmaLinux 8 AppStream 预取
LIBGUESTFS_TOOLS_C_URL=""
if [ "$TARGET_ARCH" = "amd64" ]; then
    LIBGUESTFS_TOOLS_C_URL="https://repo.almalinux.org/almalinux/8/AppStream/x86_64/os/Packages/libguestfs-tools-c-1.44.0-9.module_el8.7.0+3493+5ed0bd1c.alma.x86_64.rpm"
elif [ "$TARGET_ARCH" = "arm64" ]; then
    LIBGUESTFS_TOOLS_C_URL="https://repo.almalinux.org/almalinux/8/AppStream/aarch64/os/Packages/libguestfs-tools-c-1.44.0-9.module_el8.7.0+3493+5ed0bd1c.alma.aarch64.rpm"
fi

if [ -n "$LIBGUESTFS_TOOLS_C_URL" ]; then
    if curl -fL --connect-timeout 10 "$LIBGUESTFS_TOOLS_C_URL" -o "$RELEASE_DIR/${OUTPUT_NAME}/bundled/libguestfs-tools-c.rpm" 2>/dev/null; then
        success "libguestfs-tools-c RPM 下载完成"
    else
        warn "libguestfs-tools-c RPM 下载失败，virt-filesystems/virt-customize 将尝试通过系统源安装"
    fi
fi

# libguestfs-tools (noarch): 包含 virt-win-reg 等 Perl 脚本
# 在 openEuler 上可能为独立子包，安装失败时从捆绑包提取
LIBGUESTFS_TOOLS_NOARCH_URL="https://repo.almalinux.org/almalinux/8/AppStream/x86_64/os/Packages/libguestfs-tools-1.44.0-9.module_el8.7.0+3493+5ed0bd1c.alma.noarch.rpm"
if curl -fL --connect-timeout 10 "$LIBGUESTFS_TOOLS_NOARCH_URL" -o "$RELEASE_DIR/${OUTPUT_NAME}/bundled/libguestfs-tools.rpm" 2>/dev/null; then
    success "libguestfs-tools (noarch) RPM 下载完成"
else
    warn "libguestfs-tools (noarch) RPM 下载失败，virt-win-reg 将尝试通过系统源安装"
fi

# ==================== 生成 tar.gz ====================
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
if [ -f "$RELEASE_DIR/${OUTPUT_NAME}/kvm-console" ]; then
    echo -e "${CYAN}║${NC}    - kvm-console        后端二进制（zig 兼容版，最低 GLIBC 2.2.5）"
fi
if [ -f "$RELEASE_DIR/${OUTPUT_NAME}/kvm-console-native" ]; then
    echo -e "${CYAN}║${NC}    - kvm-console-native  后端二进制（宿主机原生版）"
fi
echo -e "${CYAN}║${NC}    - web-dist/          前端静态文件"
echo -e "${CYAN}║${NC}    - install.sh         安装脚本"
echo -e "${CYAN}║${NC}    - bundled/           捆绑的 RPM 包（用于缺失的系统包）"
echo -e "${CYAN}╚══════════════════════════════════════════════════╝${NC}"
echo ""
