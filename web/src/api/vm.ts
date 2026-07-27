/**
 * 虚拟机相关 API（本轮仅覆盖仪表盘所需接口）
 * 对应后端 /api/vm、/api/self 路由组
 */
import service from './client'
import type { ApiResponse } from '@/types/api'

/** 虚拟机列表项（管理员 /vm/list 与用户 /self/vms 结构一致） */
export interface VmListItem {
  name: string
  remark: string
  group: string
  status: string // running / shut off / paused ...
  vcpu: number
  memory: number // MB
  max_memory: number // MB
  ip: string
  disk_size: string // 如 "20 GB"
  template: string
  network: string
  autostart: boolean
  cpu_percent: number
  mem_percent: number
  locked: boolean
  in_rescue: boolean
  is_linked_clone: boolean
  continuous_runtime_seconds: number
  continuous_running_since: string
  created_at: string
}

/** 虚拟机历史监控记录 */
export interface VmStatsRecord {
  cpu_percent: number
  mem_used: number // KB
  mem_total: number // KB
  net_rx_bytes: number
  net_tx_bytes: number
  disk_rd_bytes: number
  disk_wr_bytes: number
  disk_rd_ops: number
  disk_wr_ops: number
  recorded_at: string
}

/** 用户配额使用情况（/self/quota） */
export interface QuotaUsage {
  used_cpu: number
  used_memory: number // GB
  used_disk: number // GB
  used_vm: number
  used_storage: number // Bytes
  used_storage_gb: string
  used_runtime_seconds: number
  used_runtime_display: string
  used_port_forwards: number
  used_snapshots: number
  enable_port_forward: boolean
  max_cpu: number
  max_memory: number // GB
  max_disk: number // GB
  max_vm: number
  max_storage: number // GB
  max_runtime_hours: number
  max_port_forwards: number
  max_snapshots: number
  max_bandwidth_up: number // Mbps
  max_bandwidth_down: number // Mbps
  max_traffic_down: number // GB
  max_traffic_up: number // GB
  max_public_ips: number
  used_public_ips: number
  used_traffic_down: number // Bytes
  used_traffic_up: number // Bytes
  used_traffic_down_gb: string
  used_traffic_up_gb: string
  is_limited_down: boolean
  is_limited_up: boolean
  remaining_runtime_seconds: number
  remaining_runtime_display: string
  runtime_quota_reached: boolean
}

/** 管理员：获取全部虚拟机列表 */
export function getVmList() {
  return service.get<unknown, ApiResponse<VmListItem[]>>('/vm/list', { silent: true })
}

/** 用户：获取自己的虚拟机列表 */
export function getSelfVMs() {
  return service.get<unknown, ApiResponse<VmListItem[]>>('/self/vms', { silent: true })
}

/** 用户：获取自己的配额使用情况 */
export function getSelfQuota() {
  return service.get<unknown, ApiResponse<QuotaUsage>>('/self/quota', { silent: true })
}

/** 获取虚拟机历史监控数据 */
export function getVmStatsHistory(name: string, params: { start: string; end: string }) {
  return service.get<unknown, ApiResponse<VmStatsRecord[]>>(
    `/vm/${encodeURIComponent(name)}/stats/history`,
    { params, silent: true },
  )
}
