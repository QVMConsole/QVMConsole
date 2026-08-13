/**
 * OVS 网络诊断 API（仅管理员）
 * 对应后端 /api/ovs 路由组
 */
import service from './client'
import type { ApiResponse } from '@/types/api'

/** systemd 服务状态 */
export interface OvsServiceStatus {
  name: string
  active: boolean
  state: string
  error?: string
}

/** iptables 规则状态 */
export interface OvsRuleStatus {
  name: string
  command: string
  exists: boolean
  error?: string
}

/** OVS 网络整体状态 */
export interface OvsStatus {
  bridge: string
  gateway_ip: string
  subnet_cidr: string
  uplink: string
  bridge_exists: boolean
  bridge_has_gateway: boolean
  openvswitch_service: OvsServiceStatus
  dnsmasq_service: OvsServiceStatus
  ip_forward_enabled: boolean
  nat_rule: OvsRuleStatus
  forward_out_rule: OvsRuleStatus
  forward_return_rule: OvsRuleStatus
  healthy: boolean
  issues?: string[]
  repair_suggestions?: string[]
}

/** 单个 OVS 端口 */
export interface OvsPort {
  name: string
  ofport: string
  type: string
  vm_name?: string
  mac?: string
  ip?: string
  ip_source?: string
  issues?: string[]
}

/** OVS 端口列表 */
export interface OvsPortList {
  bridge: string
  ports: OvsPort[]
  issues?: string[]
}

/** OVS 网络检测结果（/ovs/check 响应 data） */
export interface OvsCheckResult {
  status?: OvsStatus
  ports?: OvsPortList
  healthy?: boolean
  repair_suggestions?: string[]
}

export interface PortSecurityIssue {
  code: string
  message: string
  bridge?: string
  port?: string
  vm_name?: string
  interface_order?: number
  blocking: boolean
}

export interface PortSecurityCapability {
  bridge: string
  exists: boolean
  openflow13: boolean
  openflow14_bundle: boolean
  packet_meters: boolean
  packet_policing: boolean
  max_meters: number
  existing_meters: number
  required_meters: number
  sequential_apply_guard: boolean
}

export interface PortSecurityPort {
  bridge: string
  port: string
  ofport: string
  vm_name: string
  interface_order: number
  mac: string
  switch_id: number
  switch_name: string
  direct_bridge: boolean
  mode: 'strict' | 'compatible' | 'quarantined' | 'disabled'
  ipv6_enabled: boolean
  allowed_ipv4_addresses: string[]
  allowed_ipv6_addresses: string[]
  trusted_ipv6_prefixes: string[]
  neighbor_meter_id?: number
  broadcast_meter_id?: number
  policing_kpps: number
  policing_burst_kpackets: number
  isolated: boolean
  applied: boolean
  drop_packets: number
  neighbor_drop_packets: number
  broadcast_drop_packets: number
  last_error?: string
}

export interface PortSecurityStatus {
  enabled: boolean
  healthy: boolean
  applied_ports: number
  compatible_ports: number
  isolated_ports: number
  ports: PortSecurityPort[]
  issues: PortSecurityIssue[]
  last_reconciled?: string
}

export interface PortSecurityPreflight {
  ready: boolean
  enabled: boolean
  capabilities: PortSecurityCapability[]
  ports: PortSecurityPort[]
  issues: PortSecurityIssue[]
  checked_at: string
}

export interface PortSecurityTaskResult {
  task_id: number
  status: string
}

export type PortMirrorDirection = 'ingress' | 'egress' | 'both'

export interface PortMirrorSourceOption {
  name: string
  kind: 'physical' | 'ovs_bridge' | 'interface'
  state: string
  addresses: string[]
  default_route: boolean
  capture_stage: 'pre_nat' | 'post_nat' | 'interface'
  risk?: string
}

export interface PortMirrorTargetOption {
  switch_id: number
  switch_name: string
  bridge: string
  vm_count: number
}

export interface PortMirrorOptions {
  sources: PortMirrorSourceOption[]
  targets: PortMirrorTargetOption[]
}

export interface PortMirrorDirectionStats {
  enabled: boolean
  packets: number
  bytes: number
  dropped: number
}

export interface PortMirrorTargetConfig {
  switch_id: number
  switch_name: string
  bridge: string
}

export interface PortMirrorSourceStatus {
  source_interface: string
  ingress: PortMirrorDirectionStats
  egress: PortMirrorDirectionStats
}

export interface PortMirrorTargetStatus {
  switch_id: number
  switch_name: string
  bridge: string
  connections: number
  ovs_packets: number
  ovs_bytes: number
}

export interface PortMirrorStatus {
  enabled: boolean
  healthy: boolean
  source_interfaces: string[]
  targets: PortMirrorTargetConfig[]
  direction?: PortMirrorDirection
  sources: PortMirrorSourceStatus[]
  target_stats: PortMirrorTargetStatus[]
  ingress: PortMirrorDirectionStats
  egress: PortMirrorDirectionStats
  ovs_packets: number
  ovs_bytes: number
  issues: string[]
  updated_at?: string
}

/** 检测 OVS 网络（聚合状态 + 端口） */
export function checkOVSNetwork() {
  return service.post<unknown, ApiResponse<OvsCheckResult>>('/ovs/check')
}

/** 修复 OVS 网络（提交异步任务） */
export function repairOVSNetwork() {
  return service.post<unknown, ApiResponse<unknown>>('/ovs/repair')
}

export function getPortSecurityStatus() {
  return service.get<unknown, ApiResponse<PortSecurityStatus>>('/ovs/port-security/status', {
    silent: true,
  })
}

export function preflightPortSecurity() {
  return service.post<unknown, ApiResponse<PortSecurityPreflight>>('/ovs/port-security/preflight')
}

export function enablePortSecurity() {
  return service.post<unknown, ApiResponse<PortSecurityTaskResult>>('/ovs/port-security/enable')
}

export function disablePortSecurity() {
  return service.post<unknown, ApiResponse<PortSecurityTaskResult>>('/ovs/port-security/disable')
}

export function reconcilePortSecurity() {
  return service.post<unknown, ApiResponse<PortSecurityTaskResult>>('/ovs/port-security/reconcile')
}

export function isolatePortSecurityPort(port: string) {
  return service.post<unknown, ApiResponse<PortSecurityTaskResult>>(
    `/ovs/port-security/ports/${encodeURIComponent(port)}/isolate`,
  )
}

export function releasePortSecurityPort(port: string) {
  return service.post<unknown, ApiResponse<PortSecurityTaskResult>>(
    `/ovs/port-security/ports/${encodeURIComponent(port)}/release`,
  )
}

export function getPortMirrorOptions() {
  return service.get<unknown, ApiResponse<PortMirrorOptions>>('/ovs/port-mirror/options', {
    silent: true,
  })
}

export function getPortMirrorStatus() {
  return service.get<unknown, ApiResponse<PortMirrorStatus>>('/ovs/port-mirror/status', {
    silent: true,
  })
}

export function enablePortMirror(data: {
  source_interfaces: string[]
  target_switch_ids: number[]
  direction: PortMirrorDirection
}) {
  return service.post<unknown, ApiResponse<PortSecurityTaskResult>>('/ovs/port-mirror/enable', data)
}

export function disablePortMirror() {
  return service.post<unknown, ApiResponse<PortSecurityTaskResult>>('/ovs/port-mirror/disable')
}
