/**
 * 存储池相关 API（本轮仅覆盖仪表盘所需接口）
 * 对应后端 /api/storage-pool 路由组
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
