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

// ==================== 安全中心（个人安全设置） ====================

/** 修改密码请求 */
export interface ChangePasswordRequest {
  old_password: string
  new_password: string
}

/** 修改用户名请求 */
export interface ChangeUsernameRequest {
  new_username: string
  password: string
}

/** 修改用户名响应（后端重新签发访问令牌） */
export interface ChangeUsernameResult {
  token: string
  username: string
}

/** 发送邮箱绑定验证码响应 */
export interface EmailCodeSendResult {
  challenge_id: number
  masked_email: string
  expires_in: number
}

/** 绑定/换绑邮箱请求 */
export interface EmailBindRequest {
  email: string
  code: string
  challenge_id: number
}

/** 2FA 配置（POST /auth/2fa/setup 响应） */
export interface TotpSetupInfo {
  secret: string
  otpauth_url: string
}

/** 恢复码（启用 2FA / 重新生成时返回，仅展示一次） */
export interface RecoverySetup {
  recovery_codes: string[]
}

/** 携带恢复码的响应（recovery 与 data 平级，仅 enable2FA / regenRecoveryCodes） */
export type ApiResponseWithRecovery<T> = ApiResponse<T> & { recovery?: RecoverySetup }

/** 修改当前用户密码（高风险操作，428 二次验证由请求层自动处理） */
export function changePassword(data: ChangePasswordRequest) {
  return service.put<unknown, ApiResponse<null>>('/auth/password', data)
}

/** 修改当前用户用户名 */
export function changeUsername(data: ChangeUsernameRequest) {
  return service.put<unknown, ApiResponse<ChangeUsernameResult>>('/auth/username', data)
}

/** 发送邮箱绑定验证码（未传 email 时发送到当前已绑定邮箱） */
export function sendEmailCode(data: { email: string }) {
  return service.post<unknown, ApiResponse<EmailCodeSendResult>>('/auth/email/code/send', data)
}

/** 绑定或更新邮箱（安全中心场景均为 access 令牌，返回最新安全状态） */
export function bindEmail(data: EmailBindRequest) {
  return service.post<unknown, ApiResponse<{ security: SecurityState }>>('/auth/email/bind', data)
}

/** 生成 2FA 配置（密钥 + otpauth 链接，前端据此渲染二维码） */
export function setup2FA() {
  return service.post<unknown, ApiResponse<TotpSetupInfo>>('/auth/2fa/setup')
}

/** 启用 2FA（成功后返回一次性恢复码） */
export function enable2FA(data: { secret: string; code: string }) {
  return service.post<unknown, ApiResponseWithRecovery<{ security: SecurityState }>>(
    '/auth/2fa/enable',
    data,
  )
}

/** 关闭 2FA（需当前密码 + 2FA 验证码） */
export function disable2FA(data: { password: string; code: string }) {
  return service.post<unknown, ApiResponse<{ security: SecurityState }>>('/auth/2fa/disable', data)
}

/** 重新生成恢复码（旧码立即失效，需当前密码 + 2FA 验证码） */
export function regenRecoveryCodes(data: { password: string; code: string }) {
  return service.post<unknown, ApiResponseWithRecovery<null>>('/auth/2fa/recovery/regen', data)
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
