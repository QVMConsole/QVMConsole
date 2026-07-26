/**
 * 系统设置相关 API（地基阶段仅公开设置，完整设置模块后续迭代）
 */
import service from './client'
import type { ApiResponse } from '@/types/api'

/** 公开设置（无需登录即可获取） */
export interface PublicSettings {
  site_title?: string
  password_breach_check_enabled?: boolean
  spice_enabled_by_default?: boolean
  [key: string]: unknown
}

/** 获取公开设置 */
export function getPublicSettings() {
  return service.get<unknown, ApiResponse<PublicSettings>>('/public/settings', { silent: true })
}
