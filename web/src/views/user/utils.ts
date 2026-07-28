/**
 * 用户管理页工具函数
 * 状态/角色/云类型标签映射、配额百分比计算、轻量云单 VM 配额默认值等
 */
import type { LightweightVmQuotaPayload, UserListItem } from '@/api/user'

/** 用户状态 → 中文标签 */
export function userStatusLabel(status?: string): string {
  const map: Record<string, string> = {
    active: '正常',
    pending_invite: '待激活',
    disabled: '已封禁',
  }
  return map[status || ''] || status || '-'
}

/** 用户状态 → Tag 颜色 */
export function userStatusTagColor(status?: string): 'green' | 'orange' | 'red' | 'grey' {
  if (status === 'active') return 'green'
  if (status === 'pending_invite') return 'orange'
  if (status === 'disabled') return 'red'
  return 'grey'
}

/** 云类型 → 中文标签 */
export function cloudTypeLabel(cloudType?: string): string {
  return cloudType === 'lightweight' ? '轻量云' : '弹性云'
}

/** 是否为轻量云普通用户 */
export function isLightweightUser(row: UserListItem): boolean {
  return row.role !== 'admin' && row.cloud_type === 'lightweight'
}

/** 注册状态 → 中文标签 */
export function registrationStatusLabel(status?: string): string {
  const map: Record<string, string> = {
    pending: '待确认',
    provisioning: '开通中',
    active: '已开通',
    failed: '失败',
    draft: '未保存',
  }
  return map[status || ''] || status || '待确认'
}

/** 注册状态 → Tag 颜色 */
export function registrationStatusTagColor(
  status?: string,
): 'orange' | 'blue' | 'green' | 'red' | 'grey' {
  const map: Record<string, 'orange' | 'blue' | 'green' | 'red' | 'grey'> = {
    pending: 'orange',
    provisioning: 'blue',
    active: 'green',
    failed: 'red',
    draft: 'grey',
  }
  return map[status || ''] || 'orange'
}

/** 使用量百分比（max<=0 视为不限制，返回 0） */
export function quotaPercent(max?: number, used?: number): number {
  if (!max || max <= 0) return 0
  return Math.min(Math.round(((used || 0) / max) * 100), 100)
}

/** 流量上限展示（<1GB 换算为 MB） */
export function trafficLimitLabel(maxGB?: number): string {
  if (!maxGB || maxGB <= 0) return '不限'
  if (maxGB < 1) return `${(maxGB * 1024).toFixed(0)} MB`
  return `${maxGB} GB`
}

/** 轻量云单 VM 配额默认行 */
export function defaultLightweightQuotaRow(vmName: string): LightweightVmQuotaPayload {
  return {
    vm_name: vmName,
    traffic_down_gb: 0,
    traffic_up_gb: 0,
    bandwidth_down_mbps: 0,
    bandwidth_up_mbps: 0,
    max_port_forwards: 10,
    max_snapshots: 2,
    max_runtime_hours: 0,
  }
}

/** 归一化轻量云单 VM 配额提交载荷 */
export function buildLightweightQuotaPayload(
  row: Partial<LightweightVmQuotaPayload> & { vm_name: string },
): LightweightVmQuotaPayload {
  return {
    vm_name: row.vm_name,
    traffic_down_gb: Number(row.traffic_down_gb || 0),
    traffic_up_gb: Number(row.traffic_up_gb || 0),
    bandwidth_down_mbps: Number(row.bandwidth_down_mbps || 0),
    bandwidth_up_mbps: Number(row.bandwidth_up_mbps || 0),
    max_port_forwards: Number(row.max_port_forwards ?? 10),
    max_snapshots: Number(row.max_snapshots ?? 2),
    max_runtime_hours: Number(row.max_runtime_hours || 0),
  }
}

/** 注册项网络配额摘要文本 */
export function formatRegistrationQuota(row: {
  traffic_down_gb?: number
  traffic_up_gb?: number
  bandwidth_down_mbps?: number
  bandwidth_up_mbps?: number
  max_port_forwards?: number
  max_snapshots?: number
  max_runtime_hours?: number
}): string {
  const traffic = `流量 ${row.traffic_down_gb || 0}/${row.traffic_up_gb || 0}GB`
  const bandwidth = `带宽 ${row.bandwidth_down_mbps || 0}/${row.bandwidth_up_mbps || 0}Mbps`
  const ports = `端口 ${row.max_port_forwards ?? 10}`
  const snapshots = `快照 ${row.max_snapshots ?? 2}`
  const runtime = `运行 ${row.max_runtime_hours ? `${row.max_runtime_hours}小时` : '不限'}`
  return `${traffic}，${bandwidth}，${ports}，${snapshots}，${runtime}`
}

/** 轻量云用户配额占位提示 */
export function quotaPlaceholder(
  row: UserListItem,
  type: 'compute' | 'port_forward' | 'snapshot' | 'traffic' | 'disabled',
): string {
  if (!isLightweightUser(row)) return '-'
  const map: Record<string, string> = {
    compute: '管理员分配',
    port_forward: '单 VM 配额',
    snapshot: '单 VM 配额',
    traffic: '单 VM 配额',
    disabled: '不适用',
  }
  return map[type] || '不适用'
}
