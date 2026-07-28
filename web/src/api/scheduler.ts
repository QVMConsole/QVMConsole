/**
 * 调度事件中心相关 API
 * 对应后端 /api/scheduler 路由组（管理员专属）
 */
import service from './client'
import type { ApiResponse } from '@/types/api'
import { API_BASE_URL } from '@/config/constants'

/** 已注册调度器概览 */
export interface SchedulerInfo {
  key: string
  name: string
  group: string
  enabled: boolean
  description: string
  last_event_at: string | null
}

/** 调度事件记录 */
export interface SchedulerEventItem {
  id: number
  scheduler_key: string
  scheduler_name: string
  scheduler_group: string
  vm_name: string
  vm_backend: string
  status: string // running / success / failed
  trigger_reason: string
  result_message: string
  error_message: string
  started_at: string
  finished_at: string | null
  created_at: string
  updated_at: string
}

/** 调度事件列表响应 */
export interface SchedulerEventListResponse {
  list: SchedulerEventItem[]
  total: number
  page: number
  page_size: number
}

/** SSE 推送的调度事件消息 */
export interface SchedulerEventMessage {
  action: string
  event: SchedulerEventItem
}

/** 获取调度器概览列表 */
export function getSchedulerList() {
  return service.get<unknown, ApiResponse<SchedulerInfo[]>>('/scheduler/list', { silent: true })
}

/** 获取调度事件列表（支持筛选与分页） */
export function getSchedulerEventList(params: {
  page?: number
  page_size?: number
  scheduler_key?: string
  status?: string
  vm_name?: string
  start?: string
  end?: string
}) {
  return service.get<unknown, ApiResponse<SchedulerEventListResponse>>('/scheduler/events', {
    params,
    silent: true,
  })
}

/** 创建调度事件 SSE 连接 */
export function createSchedulerEventSSE(token: string): EventSource {
  return new EventSource(`${API_BASE_URL}/scheduler/events/sse?token=${encodeURIComponent(token)}`)
}
