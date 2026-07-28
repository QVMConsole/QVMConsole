/**
 * 防火墙页工具函数与常量
 * - 默认策略/规则工厂
 * - 端口区间格式化、规则请求归一化
 * - 区域选项与 VM 覆盖策略归一化
 */
import type {
  FirewallPolicy,
  FirewallRegion,
  FirewallStatus,
  FirewallVmOverride,
  HostFirewallRule,
  HostFirewallRulePayload,
} from '@/api/firewall'

/** KVM 网络防火墙默认策略（与旧版 createDefaultPolicy 对齐） */
export function createDefaultPolicy(): FirewallPolicy {
  return {
    bridge: 'br-ovs',
    vm_subnet: '192.168.122.0/24',
    outbound_enabled: false,
    outbound_allowed_regions: [],
    inbound_enabled: false,
    inbound_allowed_regions: [],
    disable_vm_ipv6: true,
    block_action: 'reject',
    whitelist_cidrs: [],
    geoip_base_url: 'https://www.ipdeny.com/ipblocks/data/aggregated',
    regions: [],
    vm_overrides: {},
  }
}

/** 宿主机规则表单默认值 */
export function createDefaultRule(): HostFirewallRulePayload {
  return {
    action: 'allow',
    protocol: 'tcp',
    port_start: 1,
    port_end: 65535,
    source_cidr: '',
    comment: '',
  }
}

/** 格式化规则端口区间（如 5900-5999 / 22 / any） */
export function formatRulePort(row: Partial<HostFirewallRule>): string {
  if (!row) return '-'
  if (row.port_start && row.port_end && row.port_start !== row.port_end) {
    return `${row.port_start}-${row.port_end}`
  }
  if (row.port_start) return String(row.port_start)
  if (row.port_end) return String(row.port_end)
  return 'any'
}

/** 归一化宿主机规则请求体（空端口转 null，空来源表示 any） */
export function normalizeRulePayload(
  rule: Partial<HostFirewallRule> | HostFirewallRulePayload,
): HostFirewallRulePayload {
  return {
    action: rule.action || 'allow',
    protocol: rule.protocol || 'tcp',
    port_start: rule.port_start || null,
    port_end: rule.port_end || null,
    source_cidr: rule.source_cidr || '',
    comment: rule.comment || '',
  }
}

/** 区域下拉选项（名称 + 代码） */
export interface RegionOption {
  label: string
  value: string
}

export function buildRegionOptions(regions: FirewallRegion[] | undefined): RegionOption[] {
  return (regions || []).map((item) => ({
    label: `${item.name || item.code} (${item.code})`,
    value: item.code,
  }))
}

/** 从状态中提取 VM 名称列表（后端为 string[]，兼容旧版对象形态） */
export function extractVmNames(status: FirewallStatus | null): string[] {
  return (status?.vms || [])
    .map((vm) => (typeof vm === 'string' ? vm : vm?.name || ''))
    .filter(Boolean)
}

/** 默认 VM 覆盖策略 */
export function createDefaultVmOverride(): FirewallVmOverride {
  return { mode: 'inherit', regions: [] }
}

/**
 * 归一化 VM 覆盖策略表：确保每台 VM 都有条目，
 * 并移除已不存在 VM 的残留条目（避免脏数据提交）
 */
export function normalizeVmOverrides(
  overrides: FirewallPolicy['vm_overrides'] | undefined,
  vmNames: string[],
): Record<string, FirewallVmOverride> {
  const next: Record<string, FirewallVmOverride> = {}
  vmNames.forEach((name) => {
    const exist = overrides?.[name]
    next[name] = exist
      ? { mode: exist.mode || 'inherit', regions: exist.regions || [] }
      : createDefaultVmOverride()
  })
  return next
}

/** VM 管控模式选项 */
export const VM_OVERRIDE_MODE_OPTIONS = [
  { label: '继承全局', value: 'inherit' },
  { label: '关闭管控', value: 'disabled' },
  { label: '仅允许入站', value: 'inbound_only' },
  { label: '仅允许区域', value: 'allow' },
  { label: '阻断区域', value: 'block' },
] as const

/** 拦截动作选项 */
export const BLOCK_ACTION_OPTIONS = [
  { label: 'reject', value: 'reject' },
  { label: 'drop', value: 'drop' },
] as const
