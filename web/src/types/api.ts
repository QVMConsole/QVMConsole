/**
 * API 层通用类型定义
 * 与后端 Gin 统一响应结构对齐：{ code, message, data }
 */

/** 后端统一响应结构 */
export interface ApiResponse<T = unknown> {
  code: number
  message: string
  data: T
}

/** 分页请求通用参数 */
export interface PageParams {
  page?: number
  page_size?: number
  keyword?: string
}

/** 分页响应通用结构 */
export interface PageResult<T> {
  list: T[]
  total: number
}

/** 用户安全状态（与后端 service.SecurityState 对齐） */
export interface SecurityState {
  email: string
  masked_email: string
  email_verified: boolean
  totp_enabled: boolean
  must_bind_email: boolean
  must_bind_2fa: boolean
  requires_login_verify: boolean
  smtp_configured: boolean
  development_mode: boolean
  maintenance_mode: boolean
  bootstrap_skipped: boolean
  status: string
  login_verified_until: string | null
  high_risk_method: string
  has_recovery_codes: boolean
  password_breached: boolean
  password_breach_count: number
  password_breach_detected_at: string | null
}

/** 当前登录用户信息（GET /auth/info 响应 data） */
export interface UserInfo {
  id: number
  username: string
  role: string
  cloud_type: string
  security: SecurityState
}
