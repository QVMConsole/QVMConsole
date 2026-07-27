/**
 * 公网 IP 相关 API（仅管理员）
 * 对应后端 /api/network/public-ips 路由组
 * 绑定/解绑/迁移/重载为高风险操作（428 二次验证由请求层自动处理），
 * 且通过任务队列异步应用规则
 */
import service from './client'
import type { ApiResponse } from '@/types/api'

/** 公网 IP 绑定模式 */
export type PublicIpMode = 'nat' | 'classic_route' | 'classic_bridge'

/** 公网 IP 绑定信息 */
export interface PublicIpBinding {
  id: number
  public_ip_id: number
  public_ip: string
  username: string
  vm_name: string
  vm_private_ip?: string
  mode: PublicIpMode
  runtime_status?: string
  config_hint?: string
  last_applied_at?: string
}

/** 公网 IP 列表项 */
export interface PublicIpItem {
  id: number
  ip: string
  cidr?: string
  gateway?: string
  uplink_if?: string
  supported_modes?: string
  status?: string // free / bound / reserved
  remark?: string
  modes?: PublicIpMode[]
  mode_labels?: string[]
  binding?: PublicIpBinding
  runtime_rules?: string[]
  issues?: string[]
}

/** 公网 IP 创建/编辑请求 */
export interface PublicIpPayload {
  ip: string
  cidr?: string
  gateway?: string
  uplink_if?: string
  /** 支持模式，逗号分隔，如 "nat,classic_route" */
  supported_modes?: string
  status?: string // free / reserved
  remark?: string
}

/** 绑定/迁移请求 */
export interface PublicIpBindPayload {
  username?: string
  vm_name?: string
  vm_private_ip?: string
  mode?: PublicIpMode
}

/** 规则预览结果 */
export interface PublicIpPreview {
  public_ip?: PublicIpItem
  binding?: PublicIpBindPayload
  commands?: string[]
  config_hint?: string
  warnings?: string[]
}

/** 任务提交响应 */
export interface PublicIpTaskResult {
  task_id?: string
  status?: string
}

/** 获取公网 IP 列表 */
export function getPublicIPs() {
  return service.get<unknown, ApiResponse<PublicIpItem[]>>('/network/public-ips', {
    silent: true,
  })
}

/** 新增公网 IP */
export function createPublicIP(data: PublicIpPayload) {
  return service.post<unknown, ApiResponse<PublicIpItem>>('/network/public-ips', data)
}

/** 更新公网 IP */
export function updatePublicIP(id: number, data: PublicIpPayload) {
  return service.put<unknown, ApiResponse<PublicIpItem>>(`/network/public-ips/${id}`, data)
}

/** 删除公网 IP（高风险） */
export function deletePublicIP(id: number) {
  return service.delete<unknown, ApiResponse<unknown>>(`/network/public-ips/${id}`)
}

/** 预览绑定规则（试算） */
export function previewPublicIP(id: number, data: PublicIpBindPayload) {
  return service.post<unknown, ApiResponse<PublicIpPreview>>(
    `/network/public-ips/${id}/preview`,
    data,
  )
}

/** 绑定公网 IP（高风险，任务队列） */
export function bindPublicIP(id: number, data: PublicIpBindPayload) {
  return service.post<unknown, ApiResponse<PublicIpTaskResult>>(
    `/network/public-ips/${id}/bind`,
    data,
  )
}

/** 解绑公网 IP（高风险，任务队列） */
export function unbindPublicIP(id: number) {
  return service.post<unknown, ApiResponse<PublicIpTaskResult>>(
    `/network/public-ips/${id}/unbind`,
  )
}

/** 迁移公网 IP（高风险，任务队列） */
export function migratePublicIP(id: number, data: PublicIpBindPayload) {
  return service.post<unknown, ApiResponse<PublicIpTaskResult>>(
    `/network/public-ips/${id}/migrate`,
    data,
  )
}

/** 重载全部公网 IP 规则（高风险，任务队列） */
export function applyPublicIPRules() {
  return service.post<unknown, ApiResponse<PublicIpTaskResult>>('/network/public-ips/apply')
}
