/**
 * 防火墙相关 API（仅管理员）
 * 对应后端 /api/firewall 路由组：
 * - KVM 网络防火墙：策略保存/预览/应用/禁用/回滚、GeoIP 区域导入与在线更新
 * - 宿主机防火墙（UFW）：状态、启用预览/启用/关闭、规则 CRUD、VNC 默认规则
 * - 连接管理：已建立 TCP 连接预览与关闭
 * 应用/禁用/回滚/启用/规则变更/关闭连接均为高风险操作
 * （428 二次验证由请求层自动处理），部分走任务队列异步执行
 */
import service from './client'
import type { ApiResponse } from '@/types/api'

// ==================== 类型定义 ====================

/** GeoIP 区域（一个区域的 IPv4 CIDR 集合） */
export interface FirewallRegion {
  code: string
  name?: string
  cidrs?: string[]
  source?: string
  updated_at?: string
}

/** 单台 VM 的覆盖策略 */
export interface FirewallVmOverride {
  /** inherit=继承全局 disabled=关闭管控 inbound_only=仅允许入站 allow=仅允许区域 block=阻断区域 */
  mode: 'inherit' | 'disabled' | 'inbound_only' | 'allow' | 'block' | string
  /** allow/block 模式下使用的区域代码列表 */
  regions: string[]
}

/** KVM 网络防火墙策略 */
export interface FirewallPolicy {
  enabled?: boolean
  bridge: string
  vm_subnet: string
  outbound_enabled: boolean
  inbound_enabled: boolean
  disable_vm_ipv6: boolean
  /** reject / drop */
  block_action: string
  outbound_allowed_regions: string[]
  inbound_allowed_regions: string[]
  whitelist_cidrs: string[]
  regions: FirewallRegion[]
  vm_overrides: Record<string, FirewallVmOverride>
  port_forward_exemptions?: Record<string, boolean>
  geoip_base_url: string
  updated_at?: string
  applied_at?: string
}

/** KVM 网络防火墙运行状态（含当前策略） */
export interface FirewallStatus {
  policy?: FirewallPolicy
  active: boolean
  last_error?: string
  rule_file?: string
  policy_file?: string
  table_name?: string
  nft_available?: boolean
  /** 被管控的 VM 名称列表（后端定义为 string[]，旧版兼容对象形态） */
  vms?: Array<string | { name?: string }>
  ipv6_note?: string
  geoip_copyright?: string
}

/** 宿主机防火墙（UFW）单条规则 */
export interface HostFirewallRule {
  id: string
  action: string
  protocol: string
  port_start?: number
  port_end?: number
  source_cidr?: string
  comment?: string
  protected?: boolean
  protected_reason?: string
  managed_by_panel?: boolean
  raw?: string
}

/** 宿主机防火墙运行状态 */
export interface HostFirewallStatus {
  active: boolean
  ufw_available?: boolean
  default_incoming?: string
  default_outgoing?: string
  default_routed?: string
  rules?: HostFirewallRule[]
  protected_rules?: HostFirewallRule[]
  recommended_rules?: HostFirewallRule[]
  ssh_ports?: number[]
  panel_ports?: number[]
  docker_compatible?: boolean
  docker_compatibility?: string
  last_error?: string
}

/** 宿主机规则创建/更新请求 */
export interface HostFirewallRulePayload {
  action: string
  protocol: string
  port_start?: number | null
  port_end?: number | null
  source_cidr?: string
  comment?: string
}

/** 已建立连接条目 */
export interface HostFirewallConnection {
  protocol: string
  local_ip: string
  local_port: number
  peer_ip: string
  peer_port: number
  allowed_port: boolean
}

/** 连接预览结果 */
export interface HostFirewallConnectionPreview {
  mode: string
  connections?: HostFirewallConnection[]
  count?: number
  warning?: string
}

/** 区域 CIDR 导入请求 */
export interface FirewallRegionImportPayload {
  code: string
  name: string
  source: string
  /** 每行一个 IPv4 CIDR 的原始文本 */
  cidrs: string
}

/** 任务提交响应 */
export interface FirewallTaskResult {
  id?: string
  task_id?: string
  status?: string
}

// ==================== KVM 网络防火墙 ====================

/** 获取 KVM 网络防火墙状态（含当前策略与被管控 VM 列表） */
export function getFirewallStatus() {
  return service.get<unknown, ApiResponse<FirewallStatus>>('/firewall/status', { silent: true })
}

/** 获取已保存的 KVM 网络防火墙策略 */
export function getFirewallPolicy() {
  return service.get<unknown, ApiResponse<FirewallPolicy>>('/firewall/policy', { silent: true })
}

/** 保存 KVM 网络防火墙策略（不应用） */
export function saveFirewallPolicy(data: FirewallPolicy) {
  return service.put<unknown, ApiResponse<unknown>>('/firewall/policy', data)
}

/** 预览策略生成的 nftables 规则文本 */
export function previewFirewallPolicy(data: FirewallPolicy) {
  return service.post<unknown, ApiResponse<{ rules: string }>>('/firewall/preview', data)
}

/** 应用 KVM 网络防火墙策略（高风险，任务队列） */
export function applyFirewallPolicy(policy: FirewallPolicy) {
  return service.post<unknown, ApiResponse<FirewallTaskResult>>('/firewall/apply', { policy })
}

/** 禁用 KVM 网络防火墙并删除独立 nft 表（高风险，任务队列） */
export function disableFirewall() {
  return service.post<unknown, ApiResponse<FirewallTaskResult>>('/firewall/disable')
}

/** 回滚 KVM 网络防火墙到未管控状态（高风险，任务队列） */
export function rollbackFirewall() {
  return service.post<unknown, ApiResponse<FirewallTaskResult>>('/firewall/rollback')
}

/** 本地导入区域 CIDR */
export function importFirewallRegion(data: FirewallRegionImportPayload) {
  return service.post<unknown, ApiResponse<FirewallPolicy>>('/firewall/geoip/import', data)
}

/** 在线更新 GeoIP 区域数据（任务队列） */
export function updateFirewallGeoIP(data: { codes: string[]; base_url: string }) {
  return service.post<unknown, ApiResponse<FirewallTaskResult>>('/firewall/geoip/update', data)
}

// ==================== 宿主机防火墙（UFW） ====================

/** 获取宿主机防火墙状态（含规则列表与保护端口信息） */
export function getHostFirewallStatus() {
  return service.get<unknown, ApiResponse<HostFirewallStatus>>('/firewall/host/status', {
    silent: true,
  })
}

/** 启用前预览：返回推荐规则（SSH/面板保护规则 + 端口转发放通） */
export function previewEnableHostFirewall(data?: { rules?: HostFirewallRulePayload[] }) {
  return service.post<unknown, ApiResponse<HostFirewallStatus>>(
    '/firewall/host/enable/preview',
    data || {},
  )
}

/** 启用宿主机防火墙（高风险，任务队列） */
export function enableHostFirewall(data: { rules: HostFirewallRulePayload[] }) {
  return service.post<unknown, ApiResponse<FirewallTaskResult>>('/firewall/host/enable', data)
}

/** 关闭宿主机防火墙（高风险，任务队列） */
export function disableHostFirewall() {
  return service.post<unknown, ApiResponse<FirewallTaskResult>>('/firewall/host/disable')
}

/** 新增宿主机规则（高风险） */
export function createHostFirewallRule(data: HostFirewallRulePayload) {
  return service.post<unknown, ApiResponse<HostFirewallRule>>('/firewall/host/rules', data)
}

/** 更新宿主机规则（高风险） */
export function updateHostFirewallRule(id: string, data: HostFirewallRulePayload) {
  return service.put<unknown, ApiResponse<HostFirewallRule>>(`/firewall/host/rules/${id}`, data)
}

/** 删除宿主机规则（高风险） */
export function deleteHostFirewallRule(id: string) {
  return service.delete<unknown, ApiResponse<unknown>>(`/firewall/host/rules/${id}`)
}

/** 添加 VNC 默认端口范围 5900-5999/tcp 放通规则（高风险） */
export function addHostFirewallVNCDefaultRule() {
  return service.post<unknown, ApiResponse<HostFirewallRule>>(
    '/firewall/host/rules/vnc-default',
  )
}

// ==================== 连接管理 ====================

/** 预览已建立连接（mode=non_firewall 非防火墙端口 / all 全部） */
export function previewHostFirewallConnections(mode: string) {
  return service.get<unknown, ApiResponse<HostFirewallConnectionPreview>>(
    '/firewall/host/connections/preview',
    { params: { mode }, silent: true },
  )
}

/** 关闭已建立连接（高风险，可能断开当前 SSH/面板会话） */
export function closeHostFirewallConnections(data: { mode: string }) {
  return service.post<unknown, ApiResponse<{ count: number }>>(
    '/firewall/host/connections/close',
    data,
  )
}
