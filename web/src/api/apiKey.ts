/**
 * 用户 API 凭证接口
 * 对应后端 /api/auth/api-key；生成与撤销由请求层自动处理高风险二次验证。
 */
import service from './client'
import type { ApiResponse } from '@/types/api'

/** API Key 脱敏元信息。 */
export interface UserAPIKeyInfo {
  api_key_id: string
  key_prefix: string
  created_at: string
  last_used_at: string | null
  enabled: boolean
}

/** 新生成的 API Key，仅在生成响应中返回一次明文。 */
export interface GeneratedAPIKey extends UserAPIKeyInfo {
  api_key: string
}

/** 读取当前用户 API Key 状态。 */
export function getAPIKeyInfo() {
  return service.get<unknown, ApiResponse<UserAPIKeyInfo | null>>('/auth/api-key')
}

/** 生成或重新生成当前用户 API Key。 */
export function rotateAPIKey() {
  return service.post<unknown, ApiResponse<GeneratedAPIKey>>('/auth/api-key')
}

/** 撤销当前用户 API Key。 */
export function revokeAPIKey() {
  return service.delete<unknown, ApiResponse<null>>('/auth/api-key')
}
