/**
 * 虚拟机相关 API
 * 对应后端 /api/vm、/api/self 路由组
 */
import service from './client'
import { API_BASE_URL } from '@/config/constants'
import type { ApiResponse } from '@/types/api'

/** 虚拟机列表项（管理员 /vm/list 与用户 /self/vms 结构一致） */
export interface VmListItem {
  name: string
  remark: string
  group: string
  status: string // running / shut off / paused / migrating ...
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

/** 虚拟机磁盘信息（/vm/:name/disks） */
export interface VmDiskItem {
  device: string
  device_type?: string // disk / cdrom
  path?: string
  capacity_gb?: number | string
  used_gb?: number | string
  format?: string
  bus?: string
  backing_path?: string
}

/** 删除确认用 qcow2 磁盘（/vm/:name/qcow2-disks） */
export interface VmQcow2Disk {
  device: string
  path: string
  format: string
  capacity_gb: number | string
  is_system: boolean
}

/** 虚拟机 IP 信息（/vm/:name/ip） */
export interface VmIPInfo {
  ip: string
  ip_status?: string // vlan_bridge / shut_off ...
}

/** 电源操作类型 */
export type VmPowerAction = 'start' | 'shutdown' | 'reboot' | 'destroy' | 'reset'

/** 虚拟机列表查询参数 */
export interface VmListQuery {
  include_resource_usage?: boolean
  include_ip?: boolean
}

/** 管理员：获取全部虚拟机列表 */
export function getVmList(params?: VmListQuery) {
  return service.get<unknown, ApiResponse<VmListItem[]>>('/vm/list', { params, silent: true })
}

/** 用户：获取自己的虚拟机列表 */
export function getSelfVMs(params?: VmListQuery) {
  return service.get<unknown, ApiResponse<VmListItem[]>>('/self/vms', { params, silent: true })
}

/**
 * 创建虚拟机列表 SSE 连接（实时推送）
 * 管理员使用 /vm/sse，普通用户使用 /self/vms/sse
 */
export function createVmListSSE(isAdmin: boolean, token: string): EventSource {
  const query = new URLSearchParams({
    token,
    include_resource_usage: '1',
    include_ip: '1',
  })
  const path = isAdmin ? '/vm/sse' : '/self/vms/sse'
  return new EventSource(`${API_BASE_URL}${path}?${query.toString()}`)
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

/** 按需加载虚拟机 IP */
export function getVmIP(name: string) {
  return service.get<unknown, ApiResponse<VmIPInfo>>(`/vm/${encodeURIComponent(name)}/ip`, {
    silent: true,
  })
}

/** 获取虚拟机磁盘列表 */
export function getDiskList(name: string) {
  return service.get<unknown, ApiResponse<VmDiskItem[]>>(`/vm/${encodeURIComponent(name)}/disks`)
}

/** 获取虚拟机 qcow2 磁盘列表（删除确认用） */
export function getVmQcow2Disks(name: string) {
  return service.get<unknown, ApiResponse<VmQcow2Disk[]>>(
    `/vm/${encodeURIComponent(name)}/qcow2-disks`,
  )
}

/** 用户自助：获取 qcow2 磁盘列表（删除确认用） */
export function selfGetVmQcow2Disks(name: string) {
  return service.get<unknown, ApiResponse<VmQcow2Disk[]>>(
    `/self/vm/${encodeURIComponent(name)}/qcow2-disks`,
  )
}

/** 电源操作（start/shutdown/reboot/destroy/reset） */
export function operateVm(name: string, action: VmPowerAction) {
  return service.post<unknown, ApiResponse<{ task_id?: string }>>(
    `/vm/${encodeURIComponent(name)}/operate`,
    { action },
  )
}

/** 编辑虚拟机（备注 / 分组等局部字段） */
export function updateVm(name: string, data: { remark?: string; group?: string }) {
  return service.put<unknown, ApiResponse<null>>(`/vm/${encodeURIComponent(name)}`, data)
}

/** 删除虚拟机（管理员） */
export function deleteVm(name: string, data: { delete_disks?: string[]; transfer_disks?: string[] } = {}) {
  return service.delete<unknown, ApiResponse<null>>(`/vm/${encodeURIComponent(name)}`, { data })
}

/** 用户自助删除虚拟机 */
export function selfDeleteVm(
  name: string,
  data: { delete_disks?: string[]; transfer_disks?: string[] } = {},
) {
  return service.delete<unknown, ApiResponse<null>>(`/self/vm/${encodeURIComponent(name)}`, { data })
}

/** 锁定虚拟机 */
export function lockVm(name: string) {
  return service.post<unknown, ApiResponse<null>>(`/vm/${encodeURIComponent(name)}/lock`)
}

/** 解锁虚拟机（高风险操作，428 二次验证由请求层自动处理） */
export function unlockVm(name: string) {
  return service.post<unknown, ApiResponse<null>>(`/vm/${encodeURIComponent(name)}/unlock`)
}

/** 启动/关闭救援系统 */
export function rescueVm(name: string, action: 'start' | 'stop') {
  return service.post<unknown, ApiResponse<{ task_id?: string }>>(
    `/vm/${encodeURIComponent(name)}/rescue`,
    { action },
  )
}

/** 转为独立虚拟机（脱离链式克隆 backing chain，仅管理员） */
export function makeVMIndependent(name: string) {
  return service.post<unknown, ApiResponse<{ task_id?: string }>>(
    `/vm/${encodeURIComponent(name)}/make-independent`,
  )
}

/** 重装系统请求参数 */
export interface ReinstallVmPayload {
  template: string
  disk_size: number
  hostname: string
  user: string
  password: string
  preserve_fnos_device_id?: boolean
  fnos_device_id?: string
}

/** 重装系统 */
export function reinstallVm(name: string, data: ReinstallVmPayload) {
  return service.post<unknown, ApiResponse<{ task_id?: string }>>(
    `/vm/${encodeURIComponent(name)}/reinstall`,
    data,
  )
}
