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
