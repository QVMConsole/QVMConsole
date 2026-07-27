/**
 * 网络中心共享格式化函数
 * 迁移自旧前端 views/network/index.vue 的辅助方法
 */
import type { VpcSwitch, VpcSecurityGroupRule } from '@/api/vpc'

/** 是/否文案 */
export function yesNo(val?: boolean): string {
  return val ? '是' : '否'
}

/** 字节格式化为可读单位 */
export function formatBytes(bytes?: number): string {
  const value = Number(bytes) || 0
  if (value < 1024) return `${value} B`
  const units = ['KB', 'MB', 'GB', 'TB']
  let size = value / 1024
  let unitIndex = 0
  while (size >= 1024 && unitIndex < units.length - 1) {
    size /= 1024
    unitIndex += 1
  }
  return `${size.toFixed(size >= 10 ? 1 : 2)} ${units[unitIndex]}`
}

/** 交换机月流量配额文案（0 或负数表示不限） */
export function switchQuotaText(val?: number): string {
  return !val || val <= 0 ? '不限' : `${val} GB`
}

/** 交换机月流量使用文案：已用 / 配额 */
export function switchTrafficText(row: VpcSwitch, direction: 'down' | 'up'): string {
  const usedText =
    direction === 'down'
      ? row.used_traffic_down_gb || formatBytes(row.used_traffic_down)
      : row.used_traffic_up_gb || formatBytes(row.used_traffic_up)
  const quota = direction === 'down' ? row.traffic_down_gb : row.traffic_up_gb
  return `${usedText} / ${switchQuotaText(quota)}`
}

/** 交换机月流量使用百分比（用于进度条） */
export function switchTrafficPercent(row: VpcSwitch, direction: 'down' | 'up'): number {
  const quota = Number(direction === 'down' ? row.traffic_down_gb : row.traffic_up_gb) || 0
  if (quota <= 0) return 0
  const usedBytes = Number(direction === 'down' ? row.used_traffic_down : row.used_traffic_up) || 0
  const quotaBytes = quota * 1024 * 1024 * 1024
  return Math.min(Math.round((usedBytes / quotaBytes) * 100), 100)
}

/** 带宽文案（0 或负数表示不限） */
export function switchBandwidthText(val?: number): string {
  return !val || val <= 0 ? '不限' : `${val} Mbps`
}

/** 网桥模式文案 */
export function bridgeModeText(mode?: string): string {
  return mode === 'bridge' ? '桥接直通' : '内网 NAT'
}

/** 规则方向文案 */
export function directionText(direction?: string): string {
  if (direction === 'ingress') return '入站'
  if (direction === 'egress') return '出站'
  return direction || '-'
}

/** 规则端口范围文案 */
export function portText(rule: VpcSecurityGroupRule): string {
  if (!rule.port_start && !rule.port_end) return '全部'
  if (rule.port_start === rule.port_end) return String(rule.port_start || '0')
  return `${rule.port_start || 0}-${rule.port_end || 65535}`
}

/** 规则目标文案（switch/security_group 目标带名称） */
export function targetText(rule: VpcSecurityGroupRule & { target_name?: string }): string {
  if (rule.target_type === 'switch') return `交换机: ${rule.target_name || rule.target_value || '-'}`
  if (rule.target_type === 'security_group')
    return `安全组: ${rule.target_name || rule.target_value || '-'}`
  return rule.target_value || '0.0.0.0/0'
}

/** 配额剩余文案（-1 或 max 为 0 表示不限） */
export function remainingText(remaining: number | undefined, max: number | undefined, unit: string): string {
  if (!max || max === -1) return '不限'
  if (remaining === -1 || remaining === undefined) return '不限'
  return `${remaining} ${unit}`
}
