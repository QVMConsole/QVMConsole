/**
 * 节点管理 API（仅管理员）
 * 对应后端 /api/nodes 路由（列表 / 创建 / 更新 / 删除 / 探测）
 * 注：跨节点迁移相关接口（migration-options 等）在 api/migration.ts 中维护
 */
import service from './client'
import type { ApiResponse } from '@/types/api'

/** 节点完整视图（对应后端 HostNodeView） */
export interface HostNodeItem {
  id: number
  name: string
  api_base_url: string
  api_key_id: string
  ssh_host: string
  ssh_port: number
  ssh_user: string
  /** true 时使用 SSH 密钥免密认证（面板不保存密码，仅检测连通性） */
  ssh_key_auth: boolean
  /** 可选本机私钥路径，留空使用默认迁移密钥 */
  ssh_key_path: string
  enabled: boolean
  status: 'online' | 'error' | 'unknown' | string
  last_probe_message: string
  capabilities?: Record<string, unknown>
  last_probed_at?: string | null
  created_at: string
  updated_at: string
}

/** 创建/更新节点请求（编辑时 api_key / ssh_password 留空表示不修改） */
export interface HostNodePayload {
  name: string
  api_base_url: string
  api_key_id: string
  api_key?: string
  ssh_host: string
  ssh_port: number
  ssh_user: string
  ssh_password?: string
  /** true 时使用 SSH 密钥免密认证，无需密码 */
  ssh_key_auth: boolean
  /** 可选本机私钥路径，留空使用默认迁移密钥 */
  ssh_key_path?: string
  enabled: boolean
}

/** 获取节点列表 */
export function listHostNodes() {
  return service.get<unknown, ApiResponse<HostNodeItem[]>>('/nodes')
}

/** 创建节点（后端先探测连接，探测通过才创建；探测耗时较长需加大超时） */
export function createHostNode(data: HostNodePayload) {
  return service.post<unknown, ApiResponse<HostNodeItem>>('/nodes', data, { timeout: 300000 })
}

/** 更新节点（后端先探测连接，探测通过才更新；探测耗时较长需加大超时） */
export function updateHostNode(id: number, data: HostNodePayload) {
  return service.put<unknown, ApiResponse<HostNodeItem>>(`/nodes/${id}`, data, { timeout: 300000 })
}

/** 删除节点 */
export function deleteHostNode(id: number) {
  return service.delete<unknown, ApiResponse<void>>(`/nodes/${id}`)
}

/** 探测节点（探测含 API + SSH 双通道校验，耗时较长） */
export function probeHostNode(id: number) {
  return service.post<unknown, ApiResponse<HostNodeItem>>(`/nodes/${id}/probe`, {}, { timeout: 120000 })
}
