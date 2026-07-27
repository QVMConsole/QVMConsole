/**
 * VPC 相关 API（交换机 / 安全组 / VM 绑定）
 * 对应后端 /api/vpc、/api/vm/:name/vpc 路由组
 */
import service from './client'
import type { ApiResponse } from '@/types/api'

// ==================== 类型定义 ====================

/** VPC 交换机 */
export interface VpcSwitch {
  id: number
  username: string
  is_system: boolean
  name: string
  bridge_name: string
  bridge_mode: string // nat / bridge
  bridge_vlan_id: number
  allow_promiscuous?: boolean
  allow_mac_change?: boolean
  allow_forged_transmits?: boolean
  vlan_id: number
  cidr: string
  gateway_ip: string
  dhcp_start: string
  dhcp_end: string
  traffic_down_gb: number
  traffic_up_gb: number
  bandwidth_mbps?: number // 兼容旧版单总带宽字段
  bandwidth_down_mbps: number
  bandwidth_up_mbps: number
  used_traffic_down?: number // 字节
  used_traffic_up?: number // 字节
  used_traffic_down_gb?: string
  used_traffic_up_gb?: string
  is_limited_down?: boolean
  is_limited_up?: boolean
  effective_bandwidth_down_mbps?: number
  effective_bandwidth_up_mbps?: number
}

/** 交换机创建/编辑请求 */
export interface VpcSwitchPayload {
  username?: string
  name: string
  bridge_name?: string
  bridge_vlan_id?: number
  allow_promiscuous?: boolean
  allow_mac_change?: boolean
  allow_forged_transmits?: boolean
  cidr?: string
  gateway_ip?: string
  dhcp_start?: string
  dhcp_end?: string
  traffic_down_gb?: number
  traffic_up_gb?: number
  bandwidth_mbps?: number
  bandwidth_down_mbps?: number
  bandwidth_up_mbps?: number
}

/** VPC 流量/带宽配额 */
export interface VpcQuota {
  username: string
  max_traffic_down: number
  max_traffic_up: number
  allocated_traffic_down: number
  allocated_traffic_up: number
  remaining_traffic_down: number
  remaining_traffic_up: number
  max_bandwidth_down: number
  max_bandwidth_up: number
  allocated_bandwidth_down: number
  allocated_bandwidth_up: number
  remaining_bandwidth_down: number
  remaining_bandwidth_up: number
}

/** 交换机下绑定的虚拟机 */
export interface VpcSwitchVM {
  vm_name: string
  username: string
  interface_order: number
}

/** 安全组规则 */
export interface VpcSecurityGroupRule {
  id: number
  security_group_id: number
  direction: string // ingress / egress
  protocol: string // tcp / udp / icmp / all
  port_start: number
  port_end: number
  target_type: string
  target_value: string
  remark: string
}

/** 安全组 */
export interface VpcSecurityGroup {
  id: number
  username: string
  vm_name?: string
  name: string
  is_default: boolean
  is_vm_scoped?: boolean
  remark?: string
  rules?: VpcSecurityGroupRule[]
}

/** VM 与交换机/安全组的绑定 */
export interface VpcVMBinding {
  id: number
  vm_name: string
  username: string
  switch_id: number
  security_group_id: number
  interface_order: number
  nic_model: string
  bandwidth_inbound_avg: number
  bandwidth_outbound_avg: number
}

/** 轻量云 VM 配额（来自绑定信息） */
export interface LightweightVMQuota {
  traffic_down_gb: number
  traffic_up_gb: number
  bandwidth_down_mbps: number
  bandwidth_up_mbps: number
  max_port_forwards: number
  max_snapshots: number
  max_runtime_hours: number
  used_runtime_seconds: number
  used_traffic_down: number
  used_traffic_up: number
  used_traffic_down_gb: string
  used_traffic_up_gb: string
  used_runtime_display: string
  remaining_runtime_seconds: number
  remaining_runtime_display: string
  runtime_quota_reached: boolean
  current_net_rx_rate: string
  current_net_tx_rate: string
  is_limited_down: boolean
  is_limited_up: boolean
  used_port_forwards: number
  used_snapshots: number
}

/** VM VPC 绑定信息（/vm/:name/vpc 响应） */
export interface VpcBindingInfo {
  binding?: VpcVMBinding | null
  bindings?: VpcVMBinding[]
  switch?: VpcSwitch | null
  security_group?: VpcSecurityGroup | null
  groups?: VpcSecurityGroup[]
  switches?: VpcSwitch[]
  lightweight_quota?: LightweightVMQuota | null
}

// ==================== 接口 ====================

/** 获取 VPC 交换机列表 */
export function getVPCSwitches(params?: { username?: string; keyword?: string }) {
  return service.get<unknown, ApiResponse<VpcSwitch[]>>('/vpc/switches', { params, silent: true })
}

/** 获取 VPC 配额（剩余/已分配流量与带宽） */
export function getVPCQuota(params?: { username?: string }) {
  return service.get<unknown, ApiResponse<VpcQuota>>('/vpc/quota', { params, silent: true })
}

/** 创建交换机 */
export function createVPCSwitch(data: VpcSwitchPayload) {
  return service.post<unknown, ApiResponse<unknown>>('/vpc/switches', data)
}

/** 更新交换机 */
export function updateVPCSwitch(id: number, data: VpcSwitchPayload) {
  return service.put<unknown, ApiResponse<unknown>>(`/vpc/switches/${id}`, data)
}

/** 删除交换机（force=true 时强制移除绑定 VM 的网卡） */
export function deleteVPCSwitch(id: number, force = false) {
  return service.delete<unknown, ApiResponse<unknown>>(`/vpc/switches/${id}`, {
    params: { force: force ? 'true' : '' },
  })
}

/** 获取交换机下绑定的虚拟机 */
export function getVPCSwitchVMs(id: number) {
  return service.get<unknown, ApiResponse<VpcSwitchVM[]>>(`/vpc/switches/${id}/vms`)
}

/** 重置交换机本月流量计数 */
export function resetVPCSwitchTraffic(id: number) {
  return service.post<unknown, ApiResponse<unknown>>(`/vpc/switches/${id}/traffic/reset`)
}

/** 获取安全组列表 */
export function getVPCSecurityGroups(params?: { username?: string; keyword?: string }) {
  return service.get<unknown, ApiResponse<VpcSecurityGroup[]>>('/vpc/security-groups', {
    params,
    silent: true,
  })
}

/** 创建安全组 */
export function createVPCSecurityGroup(data: { username?: string; name: string; remark?: string }) {
  return service.post<unknown, ApiResponse<unknown>>('/vpc/security-groups', data)
}

/** 更新安全组 */
export function updateVPCSecurityGroup(
  id: number,
  data: { username?: string; name: string; remark?: string },
) {
  return service.put<unknown, ApiResponse<unknown>>(`/vpc/security-groups/${id}`, data)
}

/** 删除安全组 */
export function deleteVPCSecurityGroup(id: number) {
  return service.delete<unknown, ApiResponse<unknown>>(`/vpc/security-groups/${id}`)
}

/** 预览 VPC ACL（nftables 规则文本） */
export function previewVPCACL() {
  return service.get<unknown, ApiResponse<string>>('/vpc/acl/preview')
}

/** 应用 VPC ACL（重建防火墙规则，高风险） */
export function applyVPCACL() {
  return service.post<unknown, ApiResponse<unknown>>('/vpc/acl/apply')
}

/** 添加安全组规则 */
export function addVPCSecurityGroupRule(
  id: number,
  data: {
    direction: string
    protocol: string
    port_start: number
    port_end: number
    target_type: string
    target_value: string
    remark: string
  },
) {
  return service.post<unknown, ApiResponse<unknown>>(`/vpc/security-groups/${id}/rules`, data)
}

/** 删除安全组规则 */
export function deleteVPCSecurityGroupRule(id: number) {
  return service.delete<unknown, ApiResponse<unknown>>(`/vpc/security-groups/rules/${id}`)
}

/** 获取 VM 的 VPC 绑定信息 */
export function getVMVPCBinding(name: string) {
  return service.get<unknown, ApiResponse<VpcBindingInfo>>(
    `/vm/${encodeURIComponent(name)}/vpc`,
    { silent: true },
  )
}

/** 更新 VM 的 VPC 绑定（切换交换机/安全组） */
export function bindVMVPC(name: string, data: { switch_id: number; security_group_id: number }) {
  return service.put<unknown, ApiResponse<unknown>>(`/vm/${encodeURIComponent(name)}/vpc`, data)
}

/** 切换 VM 安全组 */
export function switchVMSecurityGroup(name: string, securityGroupID: number) {
  return service.put<unknown, ApiResponse<unknown>>(
    `/vm/${encodeURIComponent(name)}/security-group`,
    { security_group_id: securityGroupID },
  )
}

// ==================== 多网口管理（仅管理员） ====================

/** 网口信息（绑定 + 交换机 + 安全组） */
export interface VMInterfaceInfo {
  binding: VpcVMBinding
  switch?: VpcSwitch | null
  security_group?: VpcSecurityGroup | null
}

/** 网口新增/更新请求 */
export interface VMInterfacePayload {
  switch_id: number
  security_group_id: number
  nic_model: string
  bandwidth_inbound_avg: number
  bandwidth_outbound_avg: number
}

/** 获取虚拟机网口列表（仅管理员） */
export function listVMInterfaces(name: string) {
  return service.get<unknown, ApiResponse<VMInterfaceInfo[]>>(
    `/vm/${encodeURIComponent(name)}/interfaces`,
    { silent: true },
  )
}

/** 新增网口（仅管理员） */
export function addVMInterface(name: string, data: VMInterfacePayload) {
  return service.post<unknown, ApiResponse<VMInterfaceInfo>>(
    `/vm/${encodeURIComponent(name)}/interfaces`,
    data,
  )
}

/** 更新网口（仅管理员） */
export function updateVMInterface(name: string, order: number, data: VMInterfacePayload) {
  return service.put<unknown, ApiResponse<null>>(
    `/vm/${encodeURIComponent(name)}/interfaces/${order}`,
    data,
  )
}

/** 删除网口（仅管理员） */
export function removeVMInterface(name: string, order: number) {
  return service.delete<unknown, ApiResponse<null>>(
    `/vm/${encodeURIComponent(name)}/interfaces/${order}`,
  )
}
