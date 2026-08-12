/**
 * 公网 IP 页工具函数：模式标签 / 状态映射
 */
import type { PublicIpItem, PublicIpMode } from '@/api/publicIp'

/** 绑定模式文案 */
export function publicIpModeLabel(mode?: string): string {
  if (mode === 'nat') return '1:1 NAT'
  if (mode === 'classic_route') return '经典网络-路由'
  if (mode === 'classic_bridge') return '经典网络-桥接'
  return mode || '-'
}

/** 全部可选模式（新增时默认全选） */
export const ALL_PUBLIC_IP_MODES: PublicIpMode[] = ['nat', 'classic_route', 'classic_bridge']

/** 行状态：已绑定 / 保留 / 空闲 */
export type PublicIpRowStatus = 'bound' | 'reserved' | 'free'

export function publicIpRowStatus(row: PublicIpItem): PublicIpRowStatus {
  if (row.binding) return 'bound'
  if (row.status === 'reserved') return 'reserved'
  return 'free'
}

export function publicIpStatusLabel(status: PublicIpRowStatus): string {
  if (status === 'bound') return '已绑定'
  if (status === 'reserved') return '保留'
  return '空闲'
}

export function publicIpStatusTagColor(status: PublicIpRowStatus): 'green' | 'orange' | 'grey' {
  if (status === 'bound') return 'green'
  if (status === 'reserved') return 'orange'
  return 'grey'
}

/** 来宾 IPv6 自动配置状态。 */
export function guestIPv6StatusLabel(status?: string): string {
  if (status === 'applied') return '已自动配置'
  if (status === 'pending') return '等待自动配置'
  if (status === 'failed') return '自动配置失败'
  if (status === 'manual') return '需手动配置'
  return status || '-'
}

/** 任务提交成功提示文案 */
export function publicIpTaskToast(prefix: string, taskId?: string): string {
  return taskId ? `${prefix}（任务ID: ${taskId}）` : prefix
}
