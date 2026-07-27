/**
 * 跨节点迁移相关 API（节点列表 / 迁移选项 / 预检 / 提交 / 硬盘迁移）
 * 对应后端 /api/nodes、/api/vm/:name/migration*、/api/vm/:name/disk-migration/* 路由
 */
import service from './client'
import type { ApiResponse } from '@/types/api'

/** 宿主机节点 */
export interface HostNode {
  id: number
  name: string
  ssh_host: string
  enabled: boolean
}

/** 目标存储候选 */
export interface MigrationStorageTarget {
  id: string
  display_name?: string
  available: number // Bytes
  enabled: boolean
  is_default?: boolean
}

/** 目标交换机候选 */
export interface MigrationSwitchTarget {
  id: number
  username?: string
  name: string
  cidr?: string
  bridge_mode?: string
}

/** 目标安全组候选 */
export interface MigrationSecurityGroup {
  id: number
  username?: string
  name: string
}

/** 节点迁移选项（GET /nodes/:id/migration-options） */
export interface NodeMigrationOptions {
  mode?: 'live' | 'cold'
  source_state?: string
  is_lightweight?: boolean
  target_user_exists?: boolean
  will_create_target_user?: boolean
  target_storage_targets?: MigrationStorageTarget[]
  target_switches?: MigrationSwitchTarget[]
  target_security_groups?: MigrationSecurityGroup[]
  target_switch_id?: number
  target_security_group_id?: number
}

/** 每块硬盘的目标存储映射 */
export interface DiskStorageTarget {
  target: string
  device: string
  target_storage_pool_id: string
}

/** 迁移预检/提交请求参数 */
export interface VmMigrationPayload {
  node_id: number
  mode: string
  preview_id?: string
  skip_precheck: boolean
  target_storage_pool_id: string
  disk_storage_targets: DiskStorageTarget[]
  target_switch_id: number
  target_security_group_id: number
  enable_cpu_throttle: boolean
  cpu_throttle_percent: number
}

/** 迁移预检结果（字段较多，按需声明） */
export interface VmMigrationPreview {
  allowed: boolean
  preview_id?: string
  source_state?: string
  owner?: string
  cloud_type?: string
  will_create_target_user?: boolean
  target_storage_dir?: string
  required_storage_bytes?: number
  credential?: boolean
  mode?: string
  node?: HostNode
  target_storage_pool_id?: string
  target_switch_id?: number
  target_security_group_id?: number
  live_assessment?: {
    average_bandwidth_mib?: number
    dirty_rate_mib?: number
    dirty_rate_ratio_percent?: number
    cpu_throttle_enabled?: boolean
    cpu_throttle_percent?: number
    kvm_stat_available?: boolean
    kvm_page_fault_rate?: number
    allowed?: boolean
  }
  disks?: Array<{
    target: string
    target_storage_pool_id: string
    source_path: string
    target_path: string
    backing_path?: string
  }>
  backing_checks?: Array<{ ok: boolean; path: string; message?: string }>
  port_forwards?: Array<{
    protocol: string
    source_host_port: number
    target_host_port?: number
    vm_port: number
    dest_ip?: string
  }>
  warnings?: string[]
  blockers?: string[]
}

/** 硬盘迁移选项（GET /vm/:name/disk-migration/options） */
export interface DiskMigrationOptions {
  mode?: 'live' | 'cold'
  source_state?: string
  target_storage_targets?: MigrationStorageTarget[]
  disks?: Array<{
    device: string
    capacity_gb?: number | string
    format?: string
    bus?: string
    path?: string
    backing_path?: string
    can_migrate: boolean
    block_reason?: string
  }>
  warnings?: string[]
}

/** 获取节点列表 */
export function listNodes() {
  return service.get<unknown, ApiResponse<HostNode[]>>('/nodes')
}

/** 获取目标节点的迁移选项 */
export function getNodeMigrationOptions(id: number, params: { vm_name: string }) {
  return service.get<unknown, ApiResponse<NodeMigrationOptions>>(
    `/nodes/${id}/migration-options`,
    { params },
  )
}

/** 预览跨节点迁移（耗时操作，5 分钟超时） */
export function previewVmMigration(name: string, data: VmMigrationPayload) {
  return service.post<unknown, ApiResponse<VmMigrationPreview>>(
    `/vm/${encodeURIComponent(name)}/migration/preview`,
    data,
    { timeout: 300000 },
  )
}

/** 提交跨节点迁移 */
export function migrateVm(name: string, data: VmMigrationPayload) {
  return service.post<unknown, ApiResponse<{ task_id?: string }>>(
    `/vm/${encodeURIComponent(name)}/migrate`,
    data,
    { timeout: 60000 },
  )
}

/** 获取本机硬盘迁移选项 */
export function getDiskMigrationOptions(name: string) {
  return service.get<unknown, ApiResponse<DiskMigrationOptions>>(
    `/vm/${encodeURIComponent(name)}/disk-migration/options`,
  )
}

/** 提交本机硬盘迁移 */
export function migrateDisk(name: string, dev: string, data: { target_storage_pool_id: string }) {
  return service.post<unknown, ApiResponse<{ task_id?: string }>>(
    `/vm/${encodeURIComponent(name)}/disk/${encodeURIComponent(dev)}/migrate`,
    data,
    { timeout: 60000 },
  )
}
