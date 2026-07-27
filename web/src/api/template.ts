/**
 * 模板相关 API（本轮覆盖列表查询与制作模板）
 * 对应后端 /api/template 路由组
 */
import service from './client'
import type { ApiResponse } from '@/types/api'

/** 模板列表项 */
export interface TemplateItem {
  name: string
  display_name?: string
  admin_name?: string
  type?: string // linux / windows / fnos / openwrt
  category?: string
  virtual_size?: string // 如 "20 GiB"
  template_user?: string
}

/** 获取模板列表 */
export function getTemplateList() {
  return service.get<unknown, ApiResponse<TemplateItem[]>>('/template/list')
}

/** 制作模板请求参数 */
export interface PrepareTemplatePayload {
  vm_name: string
  template_name: string
  display_name: string
  type: string
  category?: string
  cloud_init_mode?: string
  template_user?: string
  post_boot_command?: string
  post_boot_blocking?: boolean
}

/** 制作模板（从虚拟机） */
export function prepareTemplate(data: PrepareTemplatePayload) {
  return service.post<unknown, ApiResponse<{ task_id?: string }>>('/template/prepare', data)
}
