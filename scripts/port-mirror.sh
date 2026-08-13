#!/usr/bin/env bash
set -euo pipefail

# 端口镜像运维脚本：支持多来源到多目标矩阵，供脱离面板诊断或紧急回滚。

STATE_DEFAULT="/run/kvm-console/port-mirror-script.env"
INGRESS_PREF=49152
EGRESS_PREF=49153
COOKIE_PREFIX="51564d4d"

die() {
  echo "错误：$*" >&2
  exit 1
}

require_root() {
  [[ "${EUID}" -eq 0 ]] || die "需要 root 权限"
}

valid_interface() {
  [[ "$1" =~ ^[a-zA-Z0-9_.:-]{1,15}$ ]]
}

split_csv() {
  local value="$1"
  local -n output="$2"
  IFS=',' read -r -a output <<<"$value"
  ((${#output[@]} > 0)) || die "列表至少需要一个值"
}

validate_unique_interfaces() {
  local label="$1"
  shift
  local item
  declare -A seen=()
  for item in "$@"; do
    valid_interface "$item" || die "${label}名称无效：$item"
    [[ -z "${seen[$item]:-}" ]] || die "${label}存在重复项：$item"
    seen[$item]=1
  done
}

runtime_names() {
  local source_iface="$1"
  local target_bridge="$2"
  local hash
  hash="$(printf '%s\0%s' "$source_iface" "$target_bridge" | sha256sum | cut -c1-8)"
  RUNTIME_VETH="qpm${hash}"
  RUNTIME_PORT="qpo${hash}"
  RUNTIME_COOKIE="0x${COOKIE_PREFIX}${hash}"
}

load_state() {
  local state_file="$1"
  [[ -r "$state_file" ]] || die "状态文件不存在：$state_file"
  # 状态文件仅由本脚本以 root 0600 权限创建，字段值已限制为接口名列表或枚举。
  # shellcheck disable=SC1090
  source "$state_file"
  : "${SOURCE_IFACES:?状态文件缺少来源列表}"
  : "${TARGET_BRIDGES:?状态文件缺少目标列表}"
  : "${DIRECTION:?状态文件缺少方向}"
  : "${CLSACT_CREATED_CSV:=}"
  split_csv "$SOURCE_IFACES" STATE_SOURCES
  split_csv "$TARGET_BRIDGES" STATE_TARGETS
  validate_unique_interfaces "源接口" "${STATE_SOURCES[@]}"
  validate_unique_interfaces "目标网桥" "${STATE_TARGETS[@]}"
  [[ "$DIRECTION" =~ ^(ingress|egress|both)$ ]] || die "状态文件中的方向无效"
  STATE_CLSACT=()
  if [[ -n "$CLSACT_CREATED_CSV" ]]; then
    split_csv "$CLSACT_CREATED_CSV" STATE_CLSACT
    validate_unique_interfaces "clsact 来源" "${STATE_CLSACT[@]}"
  fi
}

source_outputs() {
  local source_iface="$1"
  local target_bridge
  SOURCE_OUTPUTS=()
  for target_bridge in "${STATE_TARGETS[@]}"; do
    runtime_names "$source_iface" "$target_bridge"
    SOURCE_OUTPUTS+=("$RUNTIME_VETH")
  done
}

filter_owned() {
  local source_iface="$1"
  local direction="$2"
  local preference="$3"
  local filter_text expected_output expected_count actual_count
  filter_text="$(tc filter show dev "$source_iface" "$direction" pref "$preference" 2>/dev/null || true)"
  source_outputs "$source_iface"
  expected_count="${#SOURCE_OUTPUTS[@]}"
  actual_count="$(grep -c 'Mirror to device' <<<"$filter_text" || true)"
  [[ "$actual_count" -eq "$expected_count" ]] || return 1
  for expected_output in "${SOURCE_OUTPUTS[@]}"; do
    grep -Fq "Mirror to device $expected_output" <<<"$filter_text" || return 1
  done
}

rollback() {
  local state_file="${1:-$STATE_DEFAULT}"
  [[ -r "$state_file" ]] || {
    echo "端口镜像状态不存在，无需回滚"
    return 0
  }
  load_state "$state_file"
  local source_iface target_bridge created
  for source_iface in "${STATE_SOURCES[@]}"; do
    if ip link show dev "$source_iface" >/dev/null 2>&1; then
      if [[ "$DIRECTION" != "egress" ]] && filter_owned "$source_iface" ingress "$INGRESS_PREF"; then
        tc filter del dev "$source_iface" ingress pref "$INGRESS_PREF" 2>/dev/null || true
      fi
      if [[ "$DIRECTION" != "ingress" ]] && filter_owned "$source_iface" egress "$EGRESS_PREF"; then
        tc filter del dev "$source_iface" egress pref "$EGRESS_PREF" 2>/dev/null || true
      fi
    fi
  done
  for source_iface in "${STATE_SOURCES[@]}"; do
    for target_bridge in "${STATE_TARGETS[@]}"; do
      runtime_names "$source_iface" "$target_bridge"
      if ovs-vsctl br-exists "$target_bridge" >/dev/null 2>&1; then
        ovs-ofctl -O OpenFlow13 del-flows "$target_bridge" "cookie=$RUNTIME_COOKIE/-1" 2>/dev/null || true
        ovs-vsctl --if-exists del-port "$target_bridge" "$RUNTIME_PORT" 2>/dev/null || true
      fi
      ip link del "$RUNTIME_VETH" 2>/dev/null || true
    done
  done
  for created in "${STATE_CLSACT[@]}"; do
    if ip link show dev "$created" >/dev/null 2>&1; then
      local ingress_left egress_left
      ingress_left="$(tc filter show dev "$created" ingress 2>/dev/null || true)"
      egress_left="$(tc filter show dev "$created" egress 2>/dev/null || true)"
      if [[ -z "$ingress_left" && -z "$egress_left" ]]; then
        tc qdisc del dev "$created" clsact 2>/dev/null || true
      fi
    fi
  done
  rm -f "$state_file"
  echo "端口镜像回滚完成：${#STATE_SOURCES[@]} 个来源 × ${#STATE_TARGETS[@]} 个目标"
}

append_clsact_source() {
  local state_file="$1"
  local source_iface="$2"
  local current="${CLSACT_CREATED_CSV:-}"
  if [[ -n "$current" ]]; then
    current+=","
  fi
  current+="$source_iface"
  CLSACT_CREATED_CSV="$current"
  sed -i "s/^CLSACT_CREATED_CSV=.*/CLSACT_CREATED_CSV=$CLSACT_CREATED_CSV/" "$state_file"
}

add_source_filter() {
  local source_iface="$1"
  local direction="$2"
  local preference="$3"
  local output
  source_outputs "$source_iface"
  local args=(filter add dev "$source_iface" "$direction" pref "$preference" protocol all matchall)
  for output in "${SOURCE_OUTPUTS[@]}"; do
    args+=(action mirred egress mirror dev "$output")
  done
  tc "${args[@]}"
}

apply() {
  local sources_csv="${1:-}"
  local targets_csv="${2:-}"
  local direction="${3:-both}"
  local state_file="${4:-$STATE_DEFAULT}"
  split_csv "$sources_csv" STATE_SOURCES
  split_csv "$targets_csv" STATE_TARGETS
  validate_unique_interfaces "源接口" "${STATE_SOURCES[@]}"
  validate_unique_interfaces "目标网桥" "${STATE_TARGETS[@]}"
  [[ "$direction" =~ ^(ingress|egress|both)$ ]] || die "方向必须为 ingress、egress 或 both"

  local source_iface target_bridge
  for source_iface in "${STATE_SOURCES[@]}"; do
    ip link show dev "$source_iface" >/dev/null 2>&1 || die "源接口不存在：$source_iface"
    for target_bridge in "${STATE_TARGETS[@]}"; do
      [[ "$source_iface" != "$target_bridge" ]] || die "源接口不能与目标网桥相同：$source_iface"
    done
  done
  for target_bridge in "${STATE_TARGETS[@]}"; do
    ovs-vsctl br-exists "$target_bridge" >/dev/null 2>&1 || die "目标 OVS 网桥不存在：$target_bridge"
  done
  [[ ! -e "$state_file" ]] || die "状态文件已存在，请先执行 rollback"
  for source_iface in "${STATE_SOURCES[@]}"; do
    for target_bridge in "${STATE_TARGETS[@]}"; do
      runtime_names "$source_iface" "$target_bridge"
      ! ip link show dev "$RUNTIME_VETH" >/dev/null 2>&1 || die "镜像 veth 已存在：$RUNTIME_VETH"
      ! ip link show dev "$RUNTIME_PORT" >/dev/null 2>&1 || die "镜像 OVS 端口已存在：$RUNTIME_PORT"
    done
  done

  mkdir -p "$(dirname "$state_file")"
  umask 077
  cat >"$state_file" <<EOF
SOURCE_IFACES=$sources_csv
TARGET_BRIDGES=$targets_csv
DIRECTION=$direction
CLSACT_CREATED_CSV=
EOF
  local unit="qvm-port-mirror-script-$(date +%s)-$$"
  trap 'rollback "$state_file"' EXIT
  systemd-run --quiet --unit="$unit" --on-active=120s bash "$(realpath "$0")" rollback "$state_file"
  [[ "$(systemctl is-active "$unit.timer")" == "active" ]] || die "自动回滚看门狗启动失败"

  for source_iface in "${STATE_SOURCES[@]}"; do
    for target_bridge in "${STATE_TARGETS[@]}"; do
      runtime_names "$source_iface" "$target_bridge"
      ip link add "$RUNTIME_VETH" type veth peer name "$RUNTIME_PORT"
      ip link set dev "$RUNTIME_VETH" mtu 1500 up
      ip link set dev "$RUNTIME_PORT" mtu 1500 up
      ovs-vsctl add-port "$target_bridge" "$RUNTIME_PORT" -- set Interface "$RUNTIME_PORT" external_ids:qvm-purpose=port-mirror external_ids:qvm-source="$source_iface"
      local ofport
      ofport="$(ovs-vsctl get Interface "$RUNTIME_PORT" ofport | tr -d '"')"
      [[ "$ofport" =~ ^[1-9][0-9]*$ ]] || die "OVS ofport 无效：$ofport"
      ovs-ofctl -O OpenFlow13 add-flow "$target_bridge" "cookie=$RUNTIME_COOKIE,priority=200,in_port=$ofport,actions=FLOOD"
    done
  done
  for source_iface in "${STATE_SOURCES[@]}"; do
    if ! tc qdisc show dev "$source_iface" | grep -qw clsact; then
      tc qdisc add dev "$source_iface" clsact
      append_clsact_source "$state_file" "$source_iface"
    fi
    if [[ "$direction" != "egress" ]]; then
      add_source_filter "$source_iface" ingress "$INGRESS_PREF"
    fi
    if [[ "$direction" != "ingress" ]]; then
      add_source_filter "$source_iface" egress "$EGRESS_PREF"
    fi
  done
  for source_iface in "${STATE_SOURCES[@]}"; do
    if [[ "$direction" != "egress" ]]; then
      filter_owned "$source_iface" ingress "$INGRESS_PREF"
    fi
    if [[ "$direction" != "ingress" ]]; then
      filter_owned "$source_iface" egress "$EGRESS_PREF"
    fi
    for target_bridge in "${STATE_TARGETS[@]}"; do
      runtime_names "$source_iface" "$target_bridge"
      ovs-vsctl port-to-br "$RUNTIME_PORT" | grep -Fxq "$target_bridge"
      ovs-ofctl -O OpenFlow13 dump-flows "$target_bridge" "cookie=$RUNTIME_COOKIE/-1" | grep -Fqi "in_port="
    done
  done
  systemctl stop "$unit.timer"
  trap - EXIT
  echo "端口镜像已启用：${#STATE_SOURCES[@]} 个来源 × ${#STATE_TARGETS[@]} 个目标，方向：$direction"
  echo "状态文件：$state_file"
}

status() {
  local state_file="${1:-$STATE_DEFAULT}"
  load_state "$state_file"
  local source_iface target_bridge
  for source_iface in "${STATE_SOURCES[@]}"; do
    echo "=== $source_iface ingress ==="
    tc -s filter show dev "$source_iface" ingress pref "$INGRESS_PREF" 2>/dev/null || true
    echo "=== $source_iface egress ==="
    tc -s filter show dev "$source_iface" egress pref "$EGRESS_PREF" 2>/dev/null || true
    for target_bridge in "${STATE_TARGETS[@]}"; do
      runtime_names "$source_iface" "$target_bridge"
      echo "=== $source_iface -> $target_bridge ==="
      ovs-ofctl -O OpenFlow13 dump-flows "$target_bridge" "cookie=$RUNTIME_COOKIE/-1" 2>/dev/null || true
    done
  done
}

require_root
case "${1:-}" in
  apply)
    shift
    apply "$@"
    ;;
  rollback)
    shift
    rollback "$@"
    ;;
  status)
    shift
    status "$@"
    ;;
  *)
    echo "用法：$0 apply SOURCE1,SOURCE2 TARGET1,TARGET2 [ingress|egress|both] [STATE_FILE]"
    echo "      $0 status [STATE_FILE]"
    echo "      $0 rollback [STATE_FILE]"
    exit 2
    ;;
esac
