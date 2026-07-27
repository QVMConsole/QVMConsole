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

/** 检测 OVS 网络（聚合状态 + 端口） */
export function checkOVSNetwork() {
  return service.post<unknown, ApiResponse<OvsCheckResult>>('/ovs/check')
}

/** 修复 OVS 网络（提交异步任务） */
export function repairOVSNetwork() {
  return service.post<unknown, ApiResponse<unknown>>('/ovs/repair')
}
