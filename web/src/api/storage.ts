/**
 * 存储相关 API
 * - /api/storage-pool：宿主机存储池（仪表盘）
 * - /api/self/storage：用户「我的存储」
 * - /api/self/vm/export：虚拟机导出到我的存储
 */
import service from './client'
import type { ApiResponse } from '@/types/api'

/** 宿主机存储池信息 */
export interface StoragePoolInfo {
  id: string
  name: string
  display_name: string
  device_path: string
  type: string
  size: number // Bytes
  fstype: string
  mount_path: string
  vm_dir: string
  model: string
  rota: boolean
  readonly: boolean
  used: number // Bytes
  available: number // Bytes
  use_percent: number
  enabled: boolean
}

/** 管理员：获取存储池列表 */
export function getStoragePoolList() {
  return service.get<unknown, ApiResponse<StoragePoolInfo[]>>('/storage-pool/list', {
    silent: true,
  })
}

/** 用户「我的存储」信息 */
export interface UserStorageInfo {
  initialized: boolean
  path?: string
  quota_gb?: number
  used_gb?: number
}

/** 获取当前用户存储池信息 */
export function getStorageInfo() {
  return service.get<unknown, ApiResponse<UserStorageInfo>>('/self/storage/info')
}

/** 导出虚拟机磁盘到我的存储 */
export function exportVM(data: { vm_name: string }) {
  return service.post<unknown, ApiResponse<{ task_id?: string }>>('/self/vm/export', data)
}
