/**
 * 任务队列相关 API
 * 对应后端 /api/task 路由组
 */
import service from './client'
import type { ApiResponse } from '@/types/api'
import { API_BASE_URL } from '@/config/constants'

/** 任务记录 */
export interface TaskItem {
  id: number
  type: string
  status: string // pending / running / success / failed / canceled
  params: string
  result: string
  progress: number
  message: string
  created_by: string
  created_at: string
  updated_at: string
}

/** 任务列表响应 */
export interface TaskListResponse {
  list: TaskItem[]
  total: number
  page: number
  page_size: number
}

/** SSE 推送的任务进度事件 */
export interface TaskProgressEvent {
  task_id: number
  type: string
  status: string
  progress: number
  message: string
}

/** 获取任务列表 */
export function getTaskList(params: { page?: number; page_size?: number; status?: string; type?: string }) {
  return service.get<unknown, ApiResponse<TaskListResponse>>('/task/list', {
    params,
    silent: true,
  })
}

/** 获取任务详情 */
export function getTaskDetail(id: number) {
  return service.get<unknown, ApiResponse<TaskItem>>(`/task/${id}`, { silent: true })
}

/** 取消任务 */
export function cancelTask(id: number) {
  return service.post<unknown, ApiResponse<unknown>>(`/task/${id}/cancel`)
}

/** 清理已完成任务 */
export function clearFinishedTasks() {
  return service.delete<unknown, ApiResponse<unknown>>('/task/clear')
}

/** 创建任务进度 SSE 连接 */
export function createTaskSSE(token: string): EventSource {
  return new EventSource(`${API_BASE_URL}/task/sse?token=${encodeURIComponent(token)}`)
}
