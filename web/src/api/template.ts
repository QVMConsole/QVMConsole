/**
 * 模板相关 API（本轮覆盖列表查询与制作模板）
 * 对应后端 /api/template 路由组
 */
import service from './client'
import type { ApiResponse } from '@/types/api'

/** 模板默认配置（克隆时带出推荐值） */
export interface TemplateDefaultConfig {
  vcpu?: number
  ram?: number // GB
  disk_size?: number // GB
  disk_bus?: string
  nic_model?: string
  video_model?: string
  cpu_topology_mode?: string
  first_boot_reboot_mode?: string
}

/** 模板列表项 */
export interface TemplateItem {
  name: string
  display_name?: string
  admin_name?: string
  type?: string // linux / windows / fnos / openwrt
  category?: string
  virtual_size?: string // 如 "20 GiB"
  template_user?: string
  boot_type?: string // bios / uefi
  cloud_init_mode?: string // none 表示不初始化
  default_config?: TemplateDefaultConfig
  disabled?: boolean
  clone_visible?: boolean
  level?: number // 派生层级（管理员显示缩进）
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
