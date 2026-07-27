/**
 * 网络相关 API（静态 IP / 端口转发 / 抓包会话）
 * 对应后端 /api/network、/api/firewall 路由组
 */
import service from './client'
import { API_BASE_URL } from '@/config/constants'
import { useUserStore } from '@/stores/user'
import type { ApiResponse } from '@/types/api'

// ==================== 静态 IP ====================

/** 静态 IP 绑定 */
export interface StaticIPBinding {
  id?: number
  vm_name: string
  ip: string
  mac: string
}

/** DHCP 租约 */
export interface DhcpLease {
  vm_name: string
  hostname: string
  ip: string
  mac: string
  expiry_time: string
}

/** 静态 IP 列表响应数据 */
export interface StaticIPListData {
  static_bindings?: StaticIPBinding[]
  dhcp_leases?: DhcpLease[]
}

/** 获取静态 IP 列表（响应 data 含 static_bindings 与 dhcp_leases） */
export function getStaticIPList() {
  return service.get<unknown, ApiResponse<StaticIPListData>>('/network/static-ip/list')
}

/** 绑定静态 IP */
export function bindStaticIP(data: { vm_name: string; ip: string }) {
  return service.post<unknown, ApiResponse<unknown>>('/network/static-ip/bind', data)
}

/** 解绑静态 IP */
export function unbindStaticIP(data: { vm_name: string; ip: string }) {
  return service.post<unknown, ApiResponse<unknown>>('/network/static-ip/unbind', data)
}

// ==================== 端口转发 ====================

/** 端口转发规则 */
export interface PortForwardRule {
  id: number
  rule_key: string
  vm_name: string
  protocol: string // tcp / udp
  host_port: string
  dest_ip: string
  dest_port: string
  access_ip?: string
  access_address?: string
  firewall_key?: string
  region_filter_enabled?: boolean
  live: boolean
  banned?: boolean
  probe_status?: string
  probe_reason?: string
}

/** 获取端口转发列表 */
export function getPortForwardList() {
  return service.get<unknown, ApiResponse<PortForwardRule[]>>('/network/port-forward/list')
}

/** 添加端口转发（host_port 留空自动分配） */
export function addPortForward(data: {
  vm_name: string
  vm_ip: string
  host_port: string
  vm_port: string
  protocol: string
}) {
  return service.post<unknown, ApiResponse<unknown>>('/network/port-forward/add', data)
}

/** 编辑端口转发 */
export function updatePortForward(
  id: number,
  data: { vm_name: string; vm_ip: string; host_port: string; vm_port: string; protocol: string },
) {
  return service.put<unknown, ApiResponse<unknown>>(`/network/port-forward/${id}`, data)
}

/** 删除端口转发（按 ID） */
export function deletePortForward(id: number) {
  return service.delete<unknown, ApiResponse<unknown>>(`/network/port-forward/${id}`)
}

/** 删除端口转发（按 rule_key，用于非持久化规则） */
export function deletePortForwardByRuleKey(ruleKey: string) {
  return service.delete<unknown, ApiResponse<unknown>>(
    `/network/port-forward/by-key/${encodeURIComponent(ruleKey)}`,
  )
}

/** 批量删除端口转发 */
export function batchDeletePortForward(data: { ids: number[]; rule_keys?: string[] }) {
  return service.post<unknown, ApiResponse<unknown>>('/network/port-forward/batch-delete', data)
}

/** 手动 IP 映射（端口转发目标 IP 候选） */
export interface PortForwardIPMapping {
  id: number
  vm_name: string
  ip: string
}

/** 获取端口转发手动 IP 映射 */
export function getPortForwardIPs(vmName: string) {
  return service.get<unknown, ApiResponse<PortForwardIPMapping[]>>(
    '/network/port-forward/ip-mapping',
    { params: { vm_name: vmName }, silent: true },
  )
}

/** 添加端口转发手动 IP 映射 */
export function addPortForwardIP(data: { vm_name: string; ip: string }) {
  return service.post<unknown, ApiResponse<unknown>>('/network/port-forward/ip-mapping', data)
}

/** 删除端口转发手动 IP 映射 */
export function deletePortForwardIP(id: number) {
  return service.delete<unknown, ApiResponse<unknown>>(`/network/port-forward/ip-mapping/${id}`)
}

/** 设置端口转发是否豁免入站区域限制（key 为规则 firewall_key） */
export function setPortForwardFirewall(data: { key: string; exempt: boolean }) {
  return service.put<unknown, ApiResponse<unknown>>('/firewall/port-forward', data)
}

/** 手动触发端口转发 HTTP 探测（仅管理员） */
export function runPortForwardHTTPProbe(data: { vm_name?: string } = {}) {
  return service.post<unknown, ApiResponse<unknown>>('/network/port-forward/probe/run', data)
}

/** 端口转发白名单摘要 */
export interface PortForwardWhitelistSummary {
  user_whitelisted?: boolean
  vm_whitelisted?: boolean
  effective_whitelisted?: boolean
}

/** 获取端口转发白名单摘要（当前用户 + 指定 VM） */
export function getPortForwardWhitelistSummary(vmName: string) {
  return service.get<unknown, ApiResponse<PortForwardWhitelistSummary>>(
    '/network/port-forward/whitelist/summary',
    { params: { vm_name: vmName }, silent: true },
  )
}

// ==================== 抓包会话 ====================

/** 抓包会话状态 */
export interface NetworkCaptureSession {
  task_id: number
  vm_name: string
  interface_name: string
  bpf: string
  status: string // running / success / failed / canceled
  message: string
  file_name: string
  download_path: string
  file_size: number
  duration_seconds: number
  max_mb: number
  max_packets: number
  summary_lines?: string[]
  started_at?: string
  updated_at?: string
  finished_at?: string
}

/** 获取抓包会话状态 */
export function getNetworkCaptureSession(taskId: number) {
  return service.get<unknown, ApiResponse<NetworkCaptureSession>>(`/network/captures/${taskId}`, {
    silent: true,
  })
}

/** 构造抓包文件下载地址（附带 token 查询参数） */
export function getNetworkCaptureDownloadUrl(taskId: number): string {
  const token = useUserStore.getState().token
  return `${API_BASE_URL}/network/captures/${taskId}/download?token=${encodeURIComponent(token || '')}`
}

/** 删除抓包文件 */
export function deleteNetworkCapture(taskId: number) {
  return service.delete<unknown, ApiResponse<unknown>>(`/network/captures/${taskId}`)
}
