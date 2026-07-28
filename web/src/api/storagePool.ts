/**
 * 存储池管理 API（仅管理员）
 * 对应后端 /api/storage-pool 路由组：
 * - 列表 / 配置 / 默认存储位置
 * - 格式化挂载、分区创建、磁盘清除、LVM 存储卷创建/删除
 *   （均提交任务队列异步执行，428 高风险二次验证由请求层自动处理）
 */
import service from './client'
import type { ApiResponse } from '@/types/api'

/**
 * 宿主机块设备/存储池节点（树形结构，children 为分区或 LVM 子节点）
 * 与后端 service/storage/pool.HostStoragePoolInfo 对齐
 */
export interface HostStoragePoolInfo {
  id: string
  name: string
  display_name: string
  device_path: string
  kname?: string
  /** disk / part / lvm / vg / lv / pv / loop / rom */
  type: string
  size: number // Bytes
  fstype?: string
  fsver?: string
  label?: string
  uuid?: string
  mountpoints?: string[]
  mount_path?: string
  vm_dir?: string
  model?: string
  serial?: string
  rota?: boolean
  removable?: boolean
  readonly?: boolean
  tran?: string
  pkname?: string
  used?: number // Bytes
  available?: number // Bytes
  use_percent?: number
  enabled?: boolean
  is_default?: boolean
  configured?: boolean
  can_format?: boolean
  can_use_for_vm?: boolean
  system_disk?: boolean
  has_existing_data?: boolean
  existing_data_warning?: string
  status_reason?: string
  children?: HostStoragePoolInfo[]
  // ===== LVM 扩展字段 =====
  vg_name?: string // 所属卷组名（LV/PV 节点）
  lv_type?: string // LV 类型：linear/striped/mirrored
  pv_count?: number // VG 节点的 PV 数量
  lv_count?: number // VG 节点的 LV 数量
  is_lvm_vg?: boolean // 标记为 LVM VG 合成节点
  // ===== VM 磁盘占用统计扩展字段 =====
  vm_usage_list?: VMDiskUsageInfo[] // 该分区/存储池上的虚拟机列表
  vm_total_virtual?: number // 所有 VM 虚拟配置总大小 (Bytes)
  vm_total_actual?: number // 所有 VM 实际占用总大小 (Bytes)
}

/** 虚拟机磁盘使用信息 */
export interface VMDiskUsageInfo {
  name: string // 虚拟机名称
  disk_path: string // 磁盘完整路径
  virtual_size: number // 虚拟配置大小（字节）
  actual_size: number // 实际占用大小（字节）
  mount_path: string // 所在挂载点路径
}

/** 获取宿主机存储池列表（树形） */
export function getStoragePoolList() {
  return service.get<unknown, ApiResponse<HostStoragePoolInfo[]>>('/storage-pool/list', {
    silent: true,
  })
}

/** 更新存储池配置（显示名称 / 启用状态） */
export function updateStoragePoolConfig(
  id: string,
  data: { display_name: string; enabled: boolean },
) {
  return service.put<unknown, ApiResponse>(
    `/storage-pool/${encodeURIComponent(id)}/config`,
    data,
  )
}

/** 设为默认虚拟机存储位置 */
export function setDefaultStoragePool(id: string) {
  return service.post<unknown, ApiResponse>(`/storage-pool/${encodeURIComponent(id)}/default`)
}

/** 格式化并挂载为虚拟机存储池（任务队列，高风险） */
export function formatMountStoragePool(id: string, fstype: string) {
  return service.post<unknown, ApiResponse<{ task_id?: string }>>(
    `/storage-pool/${encodeURIComponent(id)}/format-mount`,
    { fstype },
  )
}

/** 创建磁盘分区（任务队列，高风险），size_gb=0 表示使用全部剩余空间 */
export function createStoragePartition(id: string, data: { size_gb: number }) {
  return service.post<unknown, ApiResponse<{ task_id?: string }>>(
    `/storage-pool/${encodeURIComponent(id)}/create-partition`,
    data,
  )
}

/** 清除磁盘（卸载并删除所有分区/擦除签名，任务队列，高风险） */
export function deleteStoragePartitions(id: string) {
  return service.post<unknown, ApiResponse<{ task_id?: string }>>(
    `/storage-pool/${encodeURIComponent(id)}/delete-partitions`,
  )
}

/** 获取可供 LVM 使用的磁盘列表（PV 目标） */
export function getAvailablePVTargets() {
  return service.get<unknown, ApiResponse<HostStoragePoolInfo[]>>('/storage-pool/pv-targets')
}

/** LVM 存储卷创建参数（与后端 pool.LVMVolumeRequest 对齐） */
export interface LVMVolumePayload {
  device_ids: string[] // 选中的 PV 设备 ID 列表
  vg_name: string // 卷组名称
  pe_size: string // PE 大小，默认 4M
  lv_name: string // 逻辑卷名称
  lv_size: string // LV 大小，如 "10G" / "50%VG" / "100%FREE"
  lv_type: string // linear / striped / mirrored
  stripes: number // 条带数（striped 模式）
  mirrors: number // 镜像数（mirrored 模式）
  fs_type: string // ext4 / xfs / btrfs / none
  mount_path: string // 挂载路径，留空则自动生成
  add_fstab: boolean // 是否写入 fstab
}

/** 创建 LVM 存储卷（任务队列，高风险） */
export function createLVMVolume(data: LVMVolumePayload) {
  return service.post<unknown, ApiResponse<{ task_id?: string }>>(
    '/storage-pool/create-volume',
    data,
  )
}

/** 删除 LVM 存储卷（任务队列，高风险） */
export function deleteLVMVolume(vgName: string) {
  return service.post<unknown, ApiResponse<{ task_id?: string }>>(
    '/storage-pool/delete-volume',
    { vg_name: vgName },
  )
}
