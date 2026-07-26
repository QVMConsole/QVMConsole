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
