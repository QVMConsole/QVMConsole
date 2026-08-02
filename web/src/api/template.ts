/**
 * 模板相关 API
 * 对应后端 /api/template 路由组（弹性云；部分管理操作仅管理员）
 */
import service from './client'
import { API_BASE_URL } from '@/config/constants'
import { useUserStore } from '@/stores/user'
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

/** 模板列表项（模板树节点） */
export interface TemplateItem {
  name: string
  display_name?: string
  admin_name?: string
  type?: string // linux / windows / fnos / openwrt
  category?: string
  virtual_size?: string // 如 "20 GiB"
  actual_size?: string // 实际磁盘占用
  template_user?: string
  boot_type?: string // bios / uefi
  cloud_init_mode?: string // none 表示不初始化
  default_config?: TemplateDefaultConfig
  disabled?: boolean
  clone_visible?: boolean
  level?: number // 派生层级（管理员显示缩进）
  // ---- 模板树结构字段 ----
  template_uid?: string // 模板族标识
  node_id?: string
  parent_node_id?: string
  root_node_id?: string
  is_root?: boolean
  has_children?: boolean
  children_count?: number
  direct_vm_count?: number // 直接关联 VM 数
  tree_vm_count?: number // 子树关联 VM 数
  // ---- 导出与校验状态 ----
  exported?: boolean // 是否存在导出文件
  export_path?: string // 导出文件下载路径
  hash_status?: string // ok / missing / size_mismatch
  linux_init_status?: string // ready / failed / unknown
  md5?: string
  sha256?: string
  is_default?: boolean
  has_meta?: boolean
  created_from_vm?: string
  created_at?: string
  post_boot_command?: string
  post_boot_blocking?: boolean
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

/** Linux 模板离线预处理（仅管理员，补齐 cloud-init 与磁盘扩容依赖） */
export function prepareImportedLinuxTemplate(name: string) {
  return service.post<unknown, ApiResponse<{ task_id?: string }>>(
    `/template/${encodeURIComponent(name)}/prepare-linux`,
  )
}

/** Linux 模板离线预处理链式依赖检查结果 */
export interface LinuxTemplatePrepareCheck {
  template_name: string
  linked_vms: TemplateRelatedVM[]
  can_prepare: boolean
}

/** 检查 Linux 模板预处理是否存在链式克隆依赖 */
export function getLinuxTemplatePrepareCheck(name: string) {
  return service.get<unknown, ApiResponse<LinuxTemplatePrepareCheck>>(
    `/template/${encodeURIComponent(name)}/prepare-linux/check`,
  )
}

// ==================== 模板包分片上传 ====================

/** 初始化/恢复模板包上传会话（含秒传判断） */
export function templateUploadInit(data: {
  file_name: string
  total_size: number
  file_hash: string
}) {
  return service.post<unknown, ApiResponse<{
    session_key: string
    total_chunks?: number
    chunk_size?: number
    received?: number[]
    uploaded_bytes?: number
    instant?: boolean
    completed?: boolean
  }>>('/template/upload/init', data)
}

/** 上传单个分片（multipart: file, session_key, index） */
export function templateUploadChunk(formData: FormData) {
  return service.post<unknown, ApiResponse>('/template/upload/chunk', formData, {
    timeout: 0, // 大分片上传不超时
  })
}

/** 全部分片到齐后完成校验，返回临时路径供 preview/confirm 使用 */
export function templateUploadComplete(data: { session_key: string; file_hash: string }) {
  return service.post<unknown, ApiResponse<{
    completed?: boolean
    missing?: number[]
    session_key?: string
  }>>('/template/upload/complete', data)
}

/** 清理已上传的模板包临时文件（预览后未导入而关闭对话框时调用） */
export function templateUploadCancel(path: string) {
  return service.delete<unknown, ApiResponse>('/template/upload', {
    params: { path },
  })
}

// ==================== 模板包导入 ====================

/** 导入预览节点 */
export interface ImportPreviewNode {
  name: string
  admin_name?: string
  display_name?: string
  category?: string
  template_uid?: string
  node_id?: string
  parent_node_id?: string
  root_node_id?: string
  type?: string
  clone_visible?: boolean
  disabled?: boolean
  file_size?: number
  md5?: string
  sha256?: string
  exists?: boolean
  will_import?: boolean
  conflict_reason?: string
}

/** 导入预览结果 */
export interface ImportTemplatePreview {
  token: string
  mode?: string // create=新模板树 / update=增量更新
  template_uid?: string
  root_node_id?: string
  nodes?: ImportPreviewNode[]
  can_import?: boolean
  message?: string
}

/** 解析预览模板包（source_path 为分片上传产出的临时路径或主机绝对路径） */
export function previewImportTemplate(formData: FormData) {
  return service.post<unknown, ApiResponse<{ preview?: ImportTemplatePreview; file?: string }>>(
    '/template/import/preview',
    formData,
    { timeout: 0 }, // 大文件解析不超时
  )
}

/** 确认模板包导入（提交异步任务） */
export function confirmImportTemplate(token: string) {
  return service.post<unknown, ApiResponse<{ task_id?: string }>>('/template/import/confirm', {
    token,
  })
}

// ==================== 模板导出 ====================

/** 导出模板（scope=node 导出节点 / scope=root 导出整树，提交异步任务） */
export function exportTemplate(name: string, scope: 'node' | 'root' = 'node') {
  return service.post<unknown, ApiResponse<{ task_id?: string }>>(
    `/template/${encodeURIComponent(name)}/export`,
    undefined,
    { params: { scope } },
  )
}

/** 删除模板导出文件 */
export function deleteTemplateExport(name: string) {
  return service.delete<unknown, ApiResponse>(
    `/template/${encodeURIComponent(name)}/export`,
  )
}

/** 获取模板导出文件下载地址（附带 token 供浏览器直接打开） */
export function getTemplateExportDownloadUrl(path: string): string {
  const { token } = useUserStore.getState()
  if (!path) return ''
  const normalized = path.startsWith(API_BASE_URL) ? path.slice(API_BASE_URL.length) : path
  return `${API_BASE_URL}${normalized}?token=${encodeURIComponent(token)}`
}

// ==================== 模板删除 ====================

/** 模板关联虚拟机 */
export interface TemplateRelatedVM {
  name: string
  status?: string
  ip?: string
  template?: string
  node_id?: string
  clone_mode?: string
}

/** 删除模板预览 */
export interface DeleteTemplatePreview {
  template_name?: string
  templates?: TemplateItem[]
  related_vms?: TemplateRelatedVM[]
  parent_template?: TemplateItem
  promoted_templates?: TemplateItem[]
  rebased_vms?: TemplateRelatedVM[]
  can_promote?: boolean
  promote_blockers?: string[]
  can_promote_hot?: boolean
  promote_hot_blockers?: string[]
}

/** 获取模板删除预览 */
export function getTemplateDeletePreview(name: string) {
  return service.get<unknown, ApiResponse<DeleteTemplatePreview>>(
    `/template/${encodeURIComponent(name)}/delete-preview`,
  )
}

export type TemplateDeleteMode = 'cascade' | 'promote_children' | 'promote_children_hot'

/** 删除模板（高风险操作，428 二次验证由请求层自动处理） */
export function deleteTemplate(
  name: string,
  data: {
    delete_mode?: TemplateDeleteMode
    delete_vms?: boolean
    expected_vms?: string[]
  } = {},
) {
  return service.delete<unknown, ApiResponse<{ task_id?: string }>>(
    `/template/${encodeURIComponent(name)}`,
    { data },
  )
}

// ==================== 发布设置 ====================

/** 更新模板发布展示配置参数 */
export interface UpdateTemplatePublishPayload {
  admin_name?: string
  display_name?: string
  clone_visible?: boolean
  disabled?: boolean
  category?: string
  vcpu?: number
  ram?: number
  disk_size?: number
  disk_bus?: string
  nic_model?: string
  video_model?: string
  cpu_topology_mode?: string
  first_boot_reboot_mode?: string
  post_boot_command?: string
  post_boot_blocking?: boolean
}

/** 更新模板发布展示配置 */
export function updateTemplatePublish(name: string, data: UpdateTemplatePublishPayload) {
  return service.put<unknown, ApiResponse>(
    `/template/${encodeURIComponent(name)}/publish`,
    data,
  )
}
