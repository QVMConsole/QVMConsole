/**
 * 用户 API 凭证接口
 * 对应后端 /api/auth/api-key；创建、轮换和撤销属于账户安全流程，允许 API Key 的业务接口调用不触发二次验证。
 */
import service from './client'
import type { ApiResponse } from '@/types/api'

/** API Key 脱敏元信息。 */
export interface UserAPIKeyInfo {
  api_key_id: string
  key_prefix: string
  created_at: string
  expires_at: string | null
  trusted_ip: string
  last_used_at: string | null
  enabled: boolean
  public_access_enabled: boolean
  public_usable: boolean
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
export function rotateAPIKey(data?: { trusted_ip?: string }) {
  return service.post<unknown, ApiResponse<GeneratedAPIKey>>('/auth/api-key', data || {})
}

/** 撤销当前用户 API Key。 */
export function revokeAPIKey() {
  return service.delete<unknown, ApiResponse<null>>('/auth/api-key')
}
