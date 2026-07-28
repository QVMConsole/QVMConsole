/**
 * 用户自助相关 API（轻量云待开通服务器）
 * 对应后端 /api/self 路由组
 */
import service from './client'
import type { ApiResponse } from '@/types/api'

/** 轻量云待开通服务器登记项 */
export interface LightweightRegistration {
  id: number
  vm_name: string
  template?: string
  template_type?: string // windows / linux ...
  vcpu: number
  ram: number // GB
  disk_size: number // GB
  status: string // pending / provisioning / failed
  error_message?: string
  traffic_down_gb?: number
  traffic_up_gb?: number
  bandwidth_down_mbps?: number
  bandwidth_up_mbps?: number
  max_port_forwards?: number
}

/** 获取当前用户待确认的轻量云服务器 */
export function getSelfLightweightVmRegistrations() {
  return service.get<unknown, ApiResponse<LightweightRegistration[]>>(
    '/self/lightweight-registrations',
    { silent: true },
  )
}

/** 确认并开通轻量云服务器 */
export function confirmSelfLightweightVmRegistration(
  id: number,
  data: { username: string; password: string },
) {
  return service.post<unknown, ApiResponse<{ task_id?: string }>>(
    `/self/lightweight-registrations/${id}/confirm`,
    data,
  )
}

/** 用户自助：模板克隆虚拟机 */
export function selfCloneVm(data: import('./vm').CloneVmPayload) {
  return service.post<unknown, ApiResponse<{ task_id?: string }>>('/self/vm/clone', data)
}

// ==================== 用户管理（仅管理员） ====================

/** 用户配额使用情况（对应后端 QuotaUsage） */
export interface UserQuotaUsage {
  used_cpu: number
  used_memory: number
  used_disk: number
  used_vm: number
  used_storage: number
  used_storage_gb: string
  used_runtime_seconds: number
  used_runtime_display: string
  used_port_forwards: number
  used_snapshots: number
  used_public_ips: number
  used_traffic_down: number
  used_traffic_up: number
  used_traffic_down_gb: string
  used_traffic_up_gb: string
  is_limited_down: boolean
  is_limited_up: boolean
  remaining_runtime_display: string
  runtime_quota_reached: boolean
}

/** 轻量云单 VM 配额记录（对应后端 model.LightweightVMQuota） */
export interface LightweightVmQuotaItem {
  id?: number
  username?: string
  vm_name: string
  traffic_down_gb: number
  traffic_up_gb: number
  bandwidth_down_mbps: number
  bandwidth_up_mbps: number
  max_port_forwards: number
  max_snapshots: number
  max_runtime_hours: number
  used_runtime_seconds?: number
  used_traffic_down_gb?: string
  used_traffic_up_gb?: string
  used_runtime_display?: string
  runtime_quota_reached?: boolean
  is_limited_down?: boolean
  is_limited_up?: boolean
  used_port_forwards?: number
  used_snapshots?: number
}

/** 轻量云待开通 VM 注册记录（管理员视角） */
export interface LightweightVmRegistrationItem {
  id: number
  username?: string
  vm_name: string
  template?: string
  template_type?: string
  vcpu: number
  ram: number
  disk_size: number
  status?: string // pending / provisioning / active / failed
  switch_id?: number
  switch_name?: string
  traffic_down_gb?: number
  traffic_up_gb?: number
  bandwidth_down_mbps?: number
  bandwidth_up_mbps?: number
  max_port_forwards?: number
  max_snapshots?: number
  max_runtime_hours?: number
}

/** 用户列表项（对应后端 VMUserInfo） */
export interface UserListItem {
  id?: number
  username: string
  email?: string
  role?: string
  cloud_type?: string // elastic / lightweight
  dedicated_vpc_switch_id?: number
  status?: string // active / pending_invite / disabled
  max_cpu?: number
  max_memory?: number
  max_disk?: number
  max_vm?: number
  max_storage?: number
  max_runtime_hours?: number
  enable_port_forward?: boolean
  max_port_forwards?: number
  max_snapshots?: number
  max_bandwidth_up?: number
  max_bandwidth_down?: number
  max_traffic_down?: number
  max_traffic_up?: number
  max_public_ips?: number
  ssh_enabled?: boolean
  /** 用户名下已分配的虚拟机名称列表（管理员接口返回） */
  vms?: string[]
  quota?: UserQuotaUsage | null
  lightweight_quotas?: LightweightVmQuotaItem[]
  lightweight_vm_registrations?: LightweightVmRegistrationItem[]
}

/** 获取用户列表（管理员，用于「所属用户」下拉选项） */
export function getUserList() {
  return service.get<unknown, ApiResponse<UserListItem[]>>('/user/list', { silent: true })
}

/** 用户级配额字段（创建 / 编辑共用） */
export interface UserQuotaPayload {
  max_cpu: number
  max_memory: number
  max_disk: number
  max_vm: number
  max_storage: number
  max_runtime_hours: number
  enable_port_forward: boolean
  max_port_forwards: number
  max_snapshots: number
  max_public_ips: number
  max_bandwidth_up: number
  max_bandwidth_down: number
  max_traffic_down: number
  max_traffic_up: number
}

/** 轻量云单 VM 配额提交载荷 */
export interface LightweightVmQuotaPayload {
  vm_name: string
  traffic_down_gb: number
  traffic_up_gb: number
  bandwidth_down_mbps: number
  bandwidth_up_mbps: number
  max_port_forwards: number
  max_snapshots: number
  max_runtime_hours: number
}

/** 创建用户载荷 */
export interface CreateUserPayload extends Partial<UserQuotaPayload> {
  username: string
  email: string
  password?: string
  role: string
  cloud_type?: string
  dedicated_vpc_switch_id?: number | null
  lightweight_vm_registrations?: Record<string, unknown>[]
  lightweight_existing_vms?: string[]
  lightweight_existing_vm_quotas?: LightweightVmQuotaPayload[]
}

/** 创建用户（管理员，敏感操作走 428 二次验证） */
export function createUser(data: CreateUserPayload) {
  return service.post<unknown, ApiResponse<unknown>>('/user', data)
}

/** 删除用户及其所有资产（管理员，任务队列执行） */
export function deleteUser(username: string) {
  return service.delete<unknown, ApiResponse<{ task_id?: string }>>(
    `/user/${encodeURIComponent(username)}`,
  )
}

/** 更新用户配额（管理员） */
export function updateUserQuota(
  username: string,
  data: Partial<UserQuotaPayload> & { cloud_type?: string; dedicated_vpc_switch_id?: number | null },
) {
  return service.put<unknown, ApiResponse<unknown>>(
    `/user/${encodeURIComponent(username)}/quota`,
    data,
  )
}

/** 更新用户状态（封禁 / 解封） */
export function updateUserStatus(username: string, data: { status: string }) {
  return service.put<unknown, ApiResponse<{ task_id?: string }>>(
    `/user/${encodeURIComponent(username)}/status`,
    data,
  )
}

/** 分配虚拟机给用户（管理员，轻量云可附带单 VM 配额） */
export function assignVms(
  username: string,
  data: { vms: string[]; lightweight_quotas?: LightweightVmQuotaPayload[] },
) {
  return service.put<unknown, ApiResponse<unknown>>(
    `/user/${encodeURIComponent(username)}/vms`,
    data,
  )
}

/** 切换用户 SSH 访问权限（管理员） */
export function toggleUserSSH(username: string, enabled: boolean) {
  return service.put<unknown, ApiResponse<unknown>>(`/user/${encodeURIComponent(username)}/ssh`, {
    enabled,
  })
}

/** 重置用户流量配额（管理员） */
export function resetUserTraffic(username: string) {
  return service.post<unknown, ApiResponse<unknown>>(
    `/user/${encodeURIComponent(username)}/traffic/reset`,
  )
}

/** 重发邀请邮件（管理员） */
export function resendInvite(username: string) {
  return service.post<unknown, ApiResponse<unknown>>(
    `/user/${encodeURIComponent(username)}/resend-invite`,
  )
}

/** 登记轻量云待开通 VM（管理员） */
export function createLightweightVmRegistrations(
  username: string,
  data: { registrations: Record<string, unknown>[] },
) {
  return service.post<unknown, ApiResponse<unknown>>(
    `/user/${encodeURIComponent(username)}/lightweight-registrations`,
    data,
  )
}

/** 删除轻量云待开通 VM 注册（管理员） */
export function deleteLightweightVmRegistration(username: string, id: number) {
  return service.delete<unknown, ApiResponse<unknown>>(
    `/user/${encodeURIComponent(username)}/lightweight-registrations/${id}`,
  )
}

/** 移除已开通轻量云 VM 的注册记录与单 VM 配额（不删除虚拟机本体） */
export function removeLightweightRegisteredVm(username: string, vmName: string) {
  return service.delete<unknown, ApiResponse<unknown>>(
    `/user/${encodeURIComponent(username)}/lightweight-vm/${encodeURIComponent(vmName)}`,
  )
}

/** 更新轻量云单 VM 配额（管理员） */
export function updateLightweightVmQuota(username: string, data: LightweightVmQuotaPayload) {
  return service.put<
    unknown,
    ApiResponse<{
      registration?: LightweightVmRegistrationItem
      quota?: LightweightVmQuotaItem
    }>
  >(`/user/${encodeURIComponent(username)}/lightweight-vm-quota`, data)
}
