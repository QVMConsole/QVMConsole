/**
 * 存储相关 API
 * - /api/storage-pool：宿主机存储池（管理接口统一收敛于 ./storagePool，此处 re-export 兼容仪表盘）
 * - /api/self/storage：用户「我的存储」
 * - /api/self/vm/export：虚拟机导出到我的存储
 */
import service from './client'
import { API_BASE_URL } from '@/config/constants'
import { useUserStore } from '@/stores/user'
import type { ApiResponse } from '@/types/api'
import type { ImportVmPayload } from './vm'

// 宿主机存储池列表（完整类型见 ./storagePool）
export { getStoragePoolList } from './storagePool'
export type { HostStoragePoolInfo as StoragePoolInfo } from './storagePool'

// ==================== 我的存储：信息 ====================

/** 用户「我的存储」信息（与后端 service/user/types.go UserStorageInfo 对齐） */
export interface UserStorageInfo {
  initialized: boolean
  used_bytes?: number
  used_display?: string
  max_storage?: number // GB，0 = 不限
  max_bytes?: number
  readonly?: boolean
  iso_dir?: string
  share_dir?: string
  disk_dir?: string
}

/** 获取当前用户存储池信息 */
export function getStorageInfo() {
  return service.get<unknown, ApiResponse<UserStorageInfo>>('/self/storage/info', { silent: true })
}

/** 初始化（开通）用户存储池 */
export function initStorage() {
  return service.post<unknown, ApiResponse>('/self/storage/init')
}

export interface VmExportDiskOption {
  device: string
  capacity_bytes: number
  actual_bytes: number
  format: string
  bus: string
  is_system: boolean
  supported: boolean
  reason?: string
}

export interface VmExportOptions {
  vm_name: string
  status: string
  disks: VmExportDiskOption[]
}

/** 获取虚拟机可导出磁盘及格式边界 */
export function getVMExportOptions(vmName: string) {
  return service.get<unknown, ApiResponse<VmExportOptions>>(
    `/self/vm/${encodeURIComponent(vmName)}/export-options`,
    { silent: true },
  )
}

/** 导出虚拟机磁盘或标准 OVA 到我的存储 */
export function exportVM(data: {
  vm_name: string
  format?: 'qcow2' | 'ova'
  disk_devices?: string[]
}) {
  return service.post<unknown, ApiResponse<{ task_id?: string }>>('/self/vm/export', data)
}

// ==================== 我的存储文件（ISO / 文件共享 / 虚拟磁盘） ====================

/** 存储类别 */
export type StorageCategory = 'iso' | 'share' | 'disk'

/** 用户存储中的文件项（与后端 UserFileInfo 对齐） */
export interface StorageFileItem {
  name: string
  path: string
  size?: number
  size_text?: string
  mod_time?: string
  os_type?: string
  os_variant?: string
}

/** 获取我的存储中指定分类的文件列表（iso / share / disk） */
export function getStorageFiles(category: string) {
  return service.get<unknown, ApiResponse<StorageFileItem[]>>(
    `/self/storage/files/${encodeURIComponent(category)}`,
    { silent: true },
  )
}

/** 删除我的存储中的文件（高风险，请求层自动处理 428） */
export function deleteStorageFile(category: string, filename: string) {
  return service.delete<unknown, ApiResponse>(
    `/self/storage/file/${encodeURIComponent(category)}/${encodeURIComponent(filename)}`,
  )
}

/** 构建我的存储文件下载 URL（附带 token 参数） */
export function getStorageDownloadUrl(category: string, filename: string): string {
  const { token } = useUserStore.getState()
  return `${API_BASE_URL}/self/storage/download/${encodeURIComponent(category)}/${encodeURIComponent(filename)}?token=${encodeURIComponent(token)}`
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

// ==================== 分片上传（断点续传 + 秒传） ====================

/** 初始化/恢复上传会话（含秒传判断） */
export function storageUploadInit(data: {
  category: string
  file_name: string
  total_size: number
  file_hash: string
}) {
  return service.post<unknown, ApiResponse<{
    session_key: string
    total_chunks?: number
    chunk_size?: number
    received?: number[]
    uploaded_bytes?: number
    instant?: boolean
    completed?: boolean
  }>>('/self/storage/upload/init', data)
}

/** 上传单个分片（multipart: file, session_key, index） */
export function storageUploadChunk(formData: FormData) {
  return service.post<unknown, ApiResponse>('/self/storage/upload/chunk', formData, {
    timeout: 0, // 单分片不超时
  })
}

/** 全部分片到齐后完成校验 */
export function storageUploadComplete(data: { session_key: string; file_hash: string }) {
  return service.post<unknown, ApiResponse<{
    completed?: boolean
    missing?: number[]
    session_key?: string
  }>>('/self/storage/upload/complete', data)
}

/** 取消上传（删除未完成文件与会话） */
export function storageUploadCancel(path: string) {
  return service.delete<unknown, ApiResponse>('/self/storage/upload', {
    params: { path },
  })
}

/** 未完成的上传会话 */
export interface PendingUploadItem {
  session_key: string
  category: string
  file_name: string
  total_size: number
  uploaded_bytes: number
  total_chunks: number
  progress: number // 0-100
  file_hash: string
}

/** 列出未完成的上传会话（主动恢复） */
export function getPendingUploads() {
  return service.get<unknown, ApiResponse<PendingUploadItem[]>>(
    '/self/storage/upload/pending',
    { silent: true },
  )
}

// ==================== 挂载管理 ====================

/** 虚拟机挂载信息 */
export interface VmMountItem {
  vm_name: string
  tag: string
  source: string
  access_mode: string // readonly / rw
}

/** 获取用户所有 VM 的挂载列表 */
export function getUserMounts() {
  return service.get<unknown, ApiResponse<VmMountItem[]>>('/self/storage/mounts', {
    silent: true,
  })
}

/** 挂载存储池到 VM（9p VirtFS） */
export function mountStorage(data: { vm_name: string; category: string; readonly: boolean }) {
  return service.post<unknown, ApiResponse>('/self/storage/mount', data)
}

/** 从 VM 卸载存储池 */
export function unmountStorage(vmName: string, tag: string) {
  return service.delete<unknown, ApiResponse>(
    `/self/storage/mount/${encodeURIComponent(vmName)}/${encodeURIComponent(tag)}`,
  )
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

export interface ApplianceDiskMetadata {
  id: string
  file_ref: string
  capacity_bytes: number
  format: string
  bus: string
  is_system: boolean
}

export interface ApplianceNetworkMetadata {
  name: string
  model: string
}

export interface ApplianceMetadata {
  source_format: string
  name: string
  architecture: string
  vcpu: number
  ram: number
  boot_type: string
  machine_type: string
  os_type: string
  disks: ApplianceDiskMetadata[]
  networks: ApplianceNetworkMetadata[]
  warnings: string[]
}

export interface InspectAppliancePayload {
  appliance_file?: string
  appliance_path?: string
  source_type: 'storage' | 'path'
}

export interface ImportAppliancePayload extends ImportVmPayload {
  appliance_file?: string
  appliance_path?: string
  source_type: 'storage' | 'path'
  config_mode: 'ovf' | 'custom'
  copy_source: boolean
}

/** 检查 OVF/OVA 虚拟机包 */
export function inspectAppliance(data: InspectAppliancePayload, isAdmin: boolean) {
  return service.post<unknown, ApiResponse<ApplianceMetadata>>(
    isAdmin ? '/vm/import-appliance/inspect' : '/self/vm/import-appliance/inspect',
    data,
    { timeout: 0 },
  )
}

/** 提交 OVF/OVA 虚拟机包导入任务 */
export function importAppliance(data: ImportAppliancePayload, isAdmin: boolean) {
  return service.post<unknown, ApiResponse<{ task_id?: string }>>(
    isAdmin ? '/vm/import-appliance' : '/self/vm/import-appliance',
    data,
  )
}
