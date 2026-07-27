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

// ==================== 我的存储文件（ISO / 虚拟磁盘） ====================

/** 用户存储中的文件项 */
export interface StorageFileItem {
  name: string
  path: string
  size_text?: string
}

/** 获取我的存储中指定分类的文件列表（disk / iso ...） */
export function getStorageFiles(category: string) {
  return service.get<unknown, ApiResponse<StorageFileItem[]>>(
    `/self/storage/files/${encodeURIComponent(category)}`,
    { silent: true },
  )
}

/** 用户存储中的 ISO 项 */
export interface UserIsoItem {
  name: string
  path: string
  size?: string
  pool?: string
  os_type?: string
  os_variant?: string
  min_disk?: number
}

/** 获取当前用户可用的 ISO 列表 */
export function getUserISOs() {
  return service.get<unknown, ApiResponse<UserIsoItem[]>>('/self/storage/isos', { silent: true })
}

// ==================== 用户自助创建 / 导入虚拟机 ====================

/** 用户自助：创建虚拟机（ISO 安装） */
export function selfCreateVm(data: import('./vm').CreateVmPayload) {
  return service.post<unknown, ApiResponse<{ task_id?: string }>>('/self/vm/create', data)
}

/** 用户自助：从我的存储导入磁盘创建虚拟机 */
export function importVM(data: import('./vm').ImportVmPayload) {
  return service.post<unknown, ApiResponse<{ task_id?: string }>>('/self/vm/import', data)
}
