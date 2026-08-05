#!/usr/bin/env bash
set -euo pipefail

GRUB_DROP_IN="/etc/default/grub.d/99-qvmconsole-vfio-primary-gpu.cfg"
MODPROBE_CONF="/etc/modprobe.d/qvmconsole-vfio-primary-gpu.conf"
INITRAMFS_MODULES="/etc/initramfs-tools/modules"
STATE_DIR="/var/lib/qvmconsole/vfio-primary-gpu"
MODULES_BEGIN="# qvmconsole-vfio-primary-gpu-begin"
MODULES_END="# qvmconsole-vfio-primary-gpu-end"

MODE="check"
BDF=""
CONFIRM_HOST_CONSOLE_LOSS=false

usage() {
  cat <<'EOF'
用法：
  bash scripts/prepare-vfio-primary-gpu.sh --device 0000:00:02.0 --check
  sudo bash scripts/prepare-vfio-primary-gpu.sh --device 0000:00:02.0 --apply --confirm-host-console-loss
  sudo bash scripts/prepare-vfio-primary-gpu.sh --revert

选项：
  --device BDF                    PCI 地址，格式为 0000:00:02.0
  --check                         仅检查直通前置条件，默认模式
  --apply                         写入 GRUB、initramfs 和 vfio-pci 配置，不自动重启
  --confirm-host-console-loss     明确确认宿主机本地显示将不可用
  --revert                        删除本脚本写入的配置，不自动重启
  --help                          显示此帮助
EOF
}

die() {
  printf '错误：%s\n' "$*" >&2
  exit 1
}

info() {
  printf '%s\n' "$*"
}

warn() {
  printf '警告：%s\n' "$*" >&2
}

require_root() {
  [[ "$(id -u)" -eq 0 ]] || die "此操作需要 root 权限，请使用 sudo 执行"
}

require_command() {
  command -v "$1" >/dev/null 2>&1 || die "缺少命令：$1"
}

parse_args() {
  while [[ $# -gt 0 ]]; do
    case "$1" in
      --device)
        [[ $# -ge 2 ]] || die "--device 缺少 PCI 地址"
        BDF="$2"
        shift 2
        ;;
      --check)
        MODE="check"
        shift
        ;;
      --apply)
        MODE="apply"
        shift
        ;;
      --revert)
        MODE="revert"
        shift
        ;;
      --confirm-host-console-loss)
        CONFIRM_HOST_CONSOLE_LOSS=true
        shift
        ;;
      --help|-h)
        usage
        exit 0
        ;;
      *)
        die "未知参数：$1"
        ;;
    esac
  done

  if [[ "$MODE" != "revert" && -z "$BDF" ]]; then
    die "必须通过 --device 指定 PCI 地址"
  fi
}

validate_bdf() {
  [[ "$BDF" =~ ^[0-9a-fA-F]{4}:[0-9a-fA-F]{2}:[0-9a-fA-F]{2}\.[0-7]$ ]] || die "PCI 地址格式无效：$BDF"
}

remove_module_block() {
  [[ -f "$INITRAMFS_MODULES" ]] || return 0

  local tmp_file
  tmp_file="$(mktemp)"
  awk -v begin="$MODULES_BEGIN" -v end="$MODULES_END" '
    $0 == begin { skip = 1; next }
    $0 == end { skip = 0; next }
    !skip { print }
  ' "$INITRAMFS_MODULES" > "$tmp_file"
  install -m 0644 "$tmp_file" "$INITRAMFS_MODULES"
  rm -f "$tmp_file"
}

append_module_block() {
  touch "$INITRAMFS_MODULES"
  remove_module_block
  {
    printf '%s\n' "$MODULES_BEGIN"
    printf '%s\n' "vfio"
    printf '%s\n' "vfio_pci"
    printf '%s\n' "vfio_iommu_type1"
    printf '%s\n' "$MODULES_END"
  } >> "$INITRAMFS_MODULES"
}

backup_existing_configuration() {
  local backup_dir
  backup_dir="$STATE_DIR/backup-$(date +%Y%m%d%H%M%S)"
  install -d -m 0700 "$backup_dir"

  for file in "$GRUB_DROP_IN" "$MODPROBE_CONF" "$INITRAMFS_MODULES"; do
    if [[ -e "$file" ]]; then
      cp -a "$file" "$backup_dir/$(basename "$file")"
    fi
  done

  info "已备份现有配置：$backup_dir"
}

detect_iommu_parameter() {
  local cpu_vendor
  cpu_vendor="$(awk -F ': ' '/^vendor_id/{print $2; exit}' /proc/cpuinfo)"
  case "$cpu_vendor" in
    GenuineIntel)
      IOMMU_PARAMETER="intel_iommu=on"
      ;;
    AuthenticAMD)
      IOMMU_PARAMETER="amd_iommu=on"
      ;;
    *)
      die "无法确定 CPU 厂商，不能安全写入 IOMMU 启动参数"
      ;;
  esac
}

preflight() {
  validate_bdf
  require_command readlink
  require_command find

  PCI_PATH="/sys/bus/pci/devices/$BDF"
  [[ -d "$PCI_PATH" ]] || die "PCI 设备不存在：$BDF"

  PCI_CLASS="$(tr -d '\n' < "$PCI_PATH/class")"
  [[ "$PCI_CLASS" == 0x03* ]] || die "设备 $BDF 不是显示控制器，检测到的类别为 $PCI_CLASS"

  VENDOR_ID="$(tr -d '\n' < "$PCI_PATH/vendor")"
  DEVICE_ID="$(tr -d '\n' < "$PCI_PATH/device")"
  VFIO_ID="${VENDOR_ID#0x}:${DEVICE_ID#0x}"

  IOMMU_GROUP_PATH="$(readlink -f "$PCI_PATH/iommu_group" 2>/dev/null || true)"
  [[ -n "$IOMMU_GROUP_PATH" && -d "$IOMMU_GROUP_PATH/devices" ]] || die "设备 $BDF 没有 IOMMU 组，请先在 BIOS 和内核中启用 IOMMU"

  mapfile -t IOMMU_DEVICES < <(find "$IOMMU_GROUP_PATH/devices" -maxdepth 1 -type l -printf '%f\n' | sort)
  if [[ ${#IOMMU_DEVICES[@]} -ne 1 || "${IOMMU_DEVICES[0]}" != "$BDF" ]]; then
    printf 'IOMMU 组成员：%s\n' "${IOMMU_DEVICES[*]}" >&2
    die "目标设备未被单独隔离，脚本不会只绑定 IOMMU 组的一部分设备"
  fi

  DRIVER_PATH="$(readlink -f "$PCI_PATH/driver" 2>/dev/null || true)"
  CURRENT_DRIVER=""
  if [[ -n "$DRIVER_PATH" ]]; then
    CURRENT_DRIVER="$(basename "$DRIVER_PATH")"
  fi

  FRAMEBUFFER_DEVICE="$(readlink -f /sys/class/graphics/fb0/device 2>/dev/null || true)"
  CANONICAL_PCI_PATH="$(readlink -f "$PCI_PATH")"
  IS_ACTIVE_FRAMEBUFFER=false
  if [[ -n "$FRAMEBUFFER_DEVICE" && "$FRAMEBUFFER_DEVICE" == "$CANONICAL_PCI_PATH" ]]; then
    IS_ACTIVE_FRAMEBUFFER=true
  fi

  DISPLAY_DEVICE_COUNT=0
  for class_file in /sys/bus/pci/devices/*/class; do
    class_code="$(tr -d '\n' < "$class_file")"
    if [[ "$class_code" == 0x03* ]]; then
      DISPLAY_DEVICE_COUNT=$((DISPLAY_DEVICE_COUNT + 1))
    fi
  done

  detect_iommu_parameter
}

print_preflight_result() {
  info "PCI 设备：$BDF"
  info "设备 ID：$VFIO_ID"
  info "当前驱动：${CURRENT_DRIVER:-无}"
  info "IOMMU 组：$(basename "$IOMMU_GROUP_PATH")（成员：${IOMMU_DEVICES[*]}）"
  info "显示控制器数量：$DISPLAY_DEVICE_COUNT"

  if [[ "$IS_ACTIVE_FRAMEBUFFER" == true ]]; then
    warn "该设备正在承载宿主机 fb0。应用配置并重启后，本地显示输出将不可用。"
  else
    info "该设备当前未承载宿主机 fb0。"
  fi
}

apply_configuration() {
  require_root
  require_command update-initramfs
  require_command update-grub

  if [[ "$IS_ACTIVE_FRAMEBUFFER" == true && "$CONFIRM_HOST_CONSOLE_LOSS" != true ]]; then
    die "目标设备是宿主机当前显示控制器；请显式传入 --confirm-host-console-loss 后再执行"
  fi

  [[ "$CURRENT_DRIVER" != "vfio-pci" ]] || die "设备已经绑定 vfio-pci，无需再次配置"
  [[ -n "$CURRENT_DRIVER" ]] || die "无法确定当前显卡驱动，不能安全生成黑名单配置"

  backup_existing_configuration
  install -d -m 0755 "$(dirname "$GRUB_DROP_IN")"

  cat > "$GRUB_DROP_IN" <<EOF
GRUB_CMDLINE_LINUX_DEFAULT="\${GRUB_CMDLINE_LINUX_DEFAULT} $IOMMU_PARAMETER iommu=pt vfio-pci.ids=$VFIO_ID modprobe.blacklist=$CURRENT_DRIVER video=efifb:off"
EOF

  cat > "$MODPROBE_CONF" <<EOF
options vfio-pci ids=$VFIO_ID
blacklist $CURRENT_DRIVER
EOF

  append_module_block
  update-initramfs -u -k all
  update-grub

  grep -Fq "vfio-pci.ids=$VFIO_ID" /boot/grub/grub.cfg || die "GRUB 配置校验失败，未检测到 vfio-pci.ids 参数"
  info "配置已写入。请确认 SSH 可重新连接后，手动执行 reboot；脚本不会自动重启宿主机。"
}

revert_configuration() {
  require_root
  require_command update-initramfs
  require_command update-grub

  backup_existing_configuration
  rm -f "$GRUB_DROP_IN" "$MODPROBE_CONF"
  remove_module_block
  update-initramfs -u -k all
  update-grub
  info "本脚本写入的直通启动配置已移除。请重启宿主机以恢复原显示驱动。"
}

main() {
  parse_args "$@"

  if [[ "$MODE" == "revert" ]]; then
    revert_configuration
    exit 0
  fi

  preflight
  print_preflight_result

  if [[ "$MODE" == "apply" ]]; then
    apply_configuration
  fi
}

main "$@"
