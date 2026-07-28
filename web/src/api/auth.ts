/**
 * 认证相关 API
 * 对应后端 /api/auth 路由组
 */
import service from './client'
import type { ApiResponse, SecurityState, UserInfo } from '@/types/api'
import type { LoginStage } from '@/config/constants'

// ==================== 类型定义 ====================

/** 登录请求 */
export interface LoginRequest {
  username: string
  password: string
}

/** 多阶段登录响应 */
export interface LoginStageResponse {
  stage: LoginStage
  token?: string
  username: string
  role: string
  cloud_type: string
  security: SecurityState
  allowed_methods?: string[]
  force_password_change?: boolean
}

/** 泄露密码检测响应 */
export interface PasswordBreachResult {
  enabled: boolean
  breached: boolean
  warning?: string
}

/** 登录中间态验证请求 */
export interface LoginVerifyRequest {
  method: string
  code: string
}

/** 轻量云待确认开通服务器（邀请详情内嵌） */
export interface InviteLightweightRegistration {
  id: number
  vm_name: string
  template: string
  vcpu: number
  ram: number
  disk_size: number
  traffic_down_gb: number
  traffic_up_gb: number
  bandwidth_down_mbps: number
  bandwidth_up_mbps: number
  max_port_forwards: number
  max_runtime_hours: number
  status: string
}

/** 邀请注册详情（GET /auth/invite） */
export interface InviteDetail {
  username: string
  email: string
  role: string
  cloud_type: string
  dedicated_vpc_switch_id: number
  status: string
  expires_at: string
  max_cpu: number
  max_memory: number
  max_disk: number
  max_vm: number
  max_storage: number
  max_runtime_hours: number
  enable_port_forward: boolean
  max_port_forwards: number
  max_snapshots: number
  max_bandwidth_up: number
  max_bandwidth_down: number
  max_traffic_down: number
  max_traffic_up: number
  max_public_ips: number
  lightweight_vm_registrations?: InviteLightweightRegistration[]
}

/** 完成邀请注册请求 */
export interface InviteCompleteRequest {
  token: string
  password: string
  confirm_password: string
}

/** 找回密码：可选择重置的候选账号 */
export interface ForgotPasswordAccount {
  username: string
  role: string
}

/** 找回密码：发送验证码响应 */
export interface ForgotPasswordCodeResult {
  challenge_id: number
  masked_email: string
  expires_in: number
}

/** 找回密码：验证码校验响应 */
export interface ForgotPasswordVerifyResult {
  selection_token: string
  accounts: ForgotPasswordAccount[]
  email: string
  masked_email: string
}

/** 找回密码：账号确认响应 */
export interface ForgotPasswordSelectResult {
  reset_token: string
  username: string
}

// ==================== 接口 ====================

/** 用户登录（可能返回多阶段状态：success / bootstrap_security / login_verify） */
export function login(data: LoginRequest) {
  return service.post<unknown, ApiResponse<LoginStageResponse>>('/auth/login', data)
}

/** 获取当前登录用户信息 */
export function getUserInfo() {
  return service.get<unknown, ApiResponse<UserInfo>>('/auth/info')
}

/** 泄露密码检测（后端 HIBP k-匿名检测 + 本地弱密码库） */
export function checkPasswordBreach(password: string) {
  return service.post<unknown, ApiResponse<PasswordBreachResult>>(
    '/auth/check-password',
    { password },
    { silent: true },
  )
}

/** 登录中间态：发送邮箱验证码 */
export function sendLoginEmailCode(token: string) {
  return service.post<unknown, ApiResponse<{ challenge_id?: number; masked_email?: string }>>(
    '/auth/login/email/send',
    {},
    { headers: { Authorization: `Bearer ${token}` } },
  )
}

/** 登录中间态：提交验证（TOTP / 邮箱验证码 / 恢复码） */
export function verifyLoginStage(data: LoginVerifyRequest, token: string) {
  return service.post<unknown, ApiResponse<LoginStageResponse>>(
    '/auth/login/verify',
    data,
    { headers: { Authorization: `Bearer ${token}` } },
  )
}

/** 读取邀请注册信息（公开接口，凭邀请令牌访问） */
export function getInviteInfo(token: string) {
  return service.get<unknown, ApiResponse<InviteDetail>>('/auth/invite', { params: { token } })
}

/** 完成邀请注册（成功后返回登录态） */
export function completeInvite(data: InviteCompleteRequest) {
  return service.post<unknown, ApiResponse<LoginStageResponse>>('/auth/invite/complete', data)
}

/** 找回密码：向绑定邮箱发送验证码 */
export function sendForgotPasswordCode(email: string) {
  return service.post<unknown, ApiResponse<ForgotPasswordCodeResult>>(
    '/auth/password/forgot/send-code',
    { email },
  )
}

/** 找回密码：校验验证码并返回邮箱下可重置的账号列表 */
export function verifyForgotPasswordCode(data: {
  email: string
  code: string
  challenge_id: number
}) {
  return service.post<unknown, ApiResponse<ForgotPasswordVerifyResult>>(
    '/auth/password/forgot/verify-code',
    data,
  )
}

/** 找回密码：选择要重置的账号，获取重置令牌 */
export function selectForgotPasswordAccount(data: {
  selection_token: string
  username: string
}) {
  return service.post<unknown, ApiResponse<ForgotPasswordSelectResult>>(
    '/auth/password/forgot/select-account',
    data,
  )
}

/** 凭重置令牌设置新密码 */
export function resetPasswordByEmail(data: {
  token: string
  password: string
  confirm_password: string
}) {
  return service.post<unknown, ApiResponse<null>>('/auth/password/reset', data)
}
