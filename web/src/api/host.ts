/**
 * 宿主机监控相关 API
 * 对应后端 /api/host 路由组
 */
import service from './client'
import type { ApiResponse } from '@/types/api'
import { API_BASE_URL } from '@/config/constants'

/** 宿主机实时状态（/host/stats 与 SSE 推送结构一致） */
export interface HostStats {
  cpu_count: number
  cpu_percent: number
  mem_total: number // KB
  mem_free: number // KB
  mem_available: number // KB
  mem_used: number // KB
  swap_total: number // KB
  swap_free: number // KB
  swap_used: number // KB
  disk_total: number // KB
  disk_used: number // KB
  disk_free: number // KB
  vm_disk_actual: number // 所有虚拟机实际磁盘占用总和（KB）
  vm_memory_actual: number // 运行中虚拟机当前分配内存总和（KB）
  vm_memory_known: boolean // 运行中虚拟机当前分配内存是否已全部采集
  net_rx_bytes: number
  net_tx_bytes: number
  disk_rd_bytes: number
  disk_wr_bytes: number
  hostname: string
  uptime: string
  arch: string
  vm_running: number
  vm_total: number
  ksm_pages_shared: number
  ksm_pages_sharing: number
  disk_io_latency_ms: number
}

/** 宿主机历史监控记录 */
export interface HostStatsRecord {
  cpu_percent: number
  mem_used: number
  mem_total: number
  net_rx_bytes: number
  net_tx_bytes: number
  disk_rd_bytes: number
  disk_wr_bytes: number
  recorded_at: string
}

/** 宿主机磁盘挂载信息 */
export interface HostDisk {
  mount_point: string
  device: string
  fs_type: string
  total_kb: number
  used_kb: number
  free_kb: number
  use_percent: string
  read_only: boolean
}

/** 宿主机 CPU 硬件信息与每核实时使用率 */
export interface HostCpuHardware {
  model: string
  sockets: number
  cores: number
  threads: number
  per_core_usage: number[]
}

/** 单根内存条（DIMM）信息 */
export interface HostMemoryModule {
  slot: string
  size_mb: number
  type: string
  speed: string
  configured_speed: string
  manufacturer: string
  part_number: string
}

/** 宿主机内存条汇总信息 */
export interface HostMemoryModulesInfo {
  total_slots: number
  installed: number
  modules: HostMemoryModule[]
  message: string
}

/** 获取宿主机实时状态（单次） */
export function getHostStats() {
  return service.get<unknown, ApiResponse<HostStats>>('/host/stats', { silent: true })
}

/** 获取宿主机历史监控数据 */
export function getHostStatsHistory(params: { start: string; end: string }) {
  return service.get<unknown, ApiResponse<HostStatsRecord[]>>('/host/stats/history', {
    params,
    silent: true,
  })
}

/** 获取宿主机磁盘挂载列表 */
export function getHostDisks() {
  return service.get<unknown, ApiResponse<HostDisk[]>>('/host/disks', { silent: true })
}

/** 获取宿主机 CPU 硬件信息与每核使用率（管理员，概览页展开区轮询） */
export function getHostCpuHardware() {
  return service.get<unknown, ApiResponse<HostCpuHardware>>('/host/cpu/hardware', { silent: true })
}

/** 获取宿主机内存条信息（管理员，静态硬件信息） */
export function getHostMemoryModules() {
  return service.get<unknown, ApiResponse<HostMemoryModulesInfo>>('/host/memory/modules', { silent: true })
}

/** 创建宿主机状态 SSE 连接（5s 推送一次） */
export function createHostStatsSSE(token: string): EventSource {
  return new EventSource(`${API_BASE_URL}/host/stats/sse?token=${encodeURIComponent(token)}`)
}
