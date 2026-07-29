import service from './client'
import type { ApiResponse } from '@/types/api'
import type { TaskItem } from './task'

export interface PasswordBreachAffectedAccount {
  username: string
  role: string
  breach_count: number
  detected_at?: string | null
  totp_enabled: boolean
  action: string
}

export interface PasswordBreachStatus {
  scheduler_enabled: boolean
  last_checked_at?: string | null
  breached_total: number
  breached_admins: number
  breached_users: number
  affected_accounts: PasswordBreachAffectedAccount[]
}

export interface PasswordBreachStatusResponse {
  status: PasswordBreachStatus
  active_task?: TaskItem
}

export interface PasswordBreachScanSubmitResponse {
  task: TaskItem
  reused: boolean
}

export function getPasswordBreachStatus() {
  return service.get<unknown, ApiResponse<PasswordBreachStatusResponse>>(
    '/security/password-breach/status',
    { silent: true },
  )
}

export function startPasswordBreachScan() {
  return service.post<unknown, ApiResponse<PasswordBreachScanSubmitResponse>>(
    '/security/password-breach/scan',
    {},
  )
}
