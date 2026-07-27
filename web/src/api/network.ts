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

/** 批量删除端口转发 */
export function batchDeletePortForward(data: { ids: number[] }) {
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

// ==================== 宿主机网桥管理（仅管理员） ====================

/** 宿主机网桥信息 */
export interface NetworkBridge {
  id: number
  name: string
  mode: string // nat / bridge
  uplink_if: string
  migrate_host_ip: boolean
  is_default: boolean
  exists: boolean
  active: boolean
  switch_count: number
  host_addrs?: string
  host_gateway?: string
  host_dns?: string
}

/** 获取宿主机网桥列表 */
export function getNetworkBridges() {
  return service.get<unknown, ApiResponse<NetworkBridge[]>>('/network/bridges', { silent: true })
}

/** 创建桥接网桥 */
export function createNetworkBridge(data: {
  name: string
  mode: string
  uplink_if: string
  migrate_host_ip: boolean
}) {
  return service.post<unknown, ApiResponse<unknown>>('/network/bridges', data)
}

/** 删除网桥（id 为数据库主键，name 用于辅助确认） */
export function deleteNetworkBridge(id: number, name = '') {
  const params = name ? { name } : undefined
  return service.delete<unknown, ApiResponse<unknown>>(`/network/bridges/${id}`, { params })
}

// ==================== 宿主机物理网卡（仅管理员） ====================

/** 宿主机网卡信息 */
export interface HostInterface {
  name: string
  mac: string
  state: string
  mtu: number
  addresses?: string[]
  default_route?: boolean
  ovs_bridge?: string
  ovs_port?: boolean
  physical?: boolean
  managed_bridge?: string
  risk?: string
}

/** 获取宿主机网卡列表 */
export function getHostInterfaces() {
  return service.get<unknown, ApiResponse<HostInterface[]>>('/network/host/interfaces', {
    silent: true,
  })
}

// ==================== 接口 IP/DNS 配置（仅管理员） ====================

/** 接口当前 IP/DNS 配置 */
export interface InterfaceConfig {
  name: string
  type: string // bridge / nic
  bridge_name?: string
  addrs?: string[]
  gateway?: string
  metric?: string
  dns?: string[]
  configurable: boolean
  reason?: string
  managed_bridge?: boolean
  migrate_host_ip?: boolean
}

/** 获取接口 IP/DNS 配置 */
export function getInterfaceConfig(name: string) {
  return service.get<unknown, ApiResponse<InterfaceConfig>>(
    `/network/interfaces/${encodeURIComponent(name)}/config`,
  )
}

/** 设置接口 IP/DNS 配置（clear=true 时清除全部静态配置） */
export function setInterfaceConfig(
  name: string,
  data: { addrs?: string; gateway?: string; dns?: string; clear?: boolean },
) {
  return service.put<unknown, ApiResponse<unknown>>(
    `/network/interfaces/${encodeURIComponent(name)}/config`,
    data,
  )
}
