/**
 * 基础设施相关 API（存储池目标 / ISO 聚合）
 * 对应后端 /api/storage-pool 路由组
 */
import service from './client'
import type { ApiResponse } from '@/types/api'

/** 虚拟机磁盘可用的存储位置 */
export interface VmStorageTarget {
  id: string
  name: string
  display_name: string
  is_default?: boolean
  available?: number // Bytes
}

/** 获取虚拟机磁盘可用的存储位置列表 */
export function getVMStorageTargets() {
  return service.get<unknown, ApiResponse<VmStorageTarget[]>>('/storage-pool/vm-targets', {
    silent: true,
  })
}

/** 存储池中的 ISO 项 */
export interface IsoItem {
  name: string
  path: string
  size?: string
  pool?: string
  os_type?: string // windows / linux
  os_variant?: string
  min_disk?: number // GB
}

/** 获取所有存储池中的 ISO（管理员聚合视图） */
export function getAllISOs() {
  return service.get<unknown, ApiResponse<IsoItem[]>>('/storage-pool/all-isos', { silent: true })
}
