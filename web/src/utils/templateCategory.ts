/**
 * 模板分类常量与归一化工具（迁移自旧前端 utils/templateCategory.js）
 * 供虚拟机表单（创建/编辑/重装）与模板管理页共用
 */

export const DEFAULT_LINUX_TEMPLATE_CATEGORY = 'Ubuntu'
export const DEFAULT_WINDOWS_TEMPLATE_CATEGORY = 'WindowsServer2022'
export const DEFAULT_OPENWRT_TEMPLATE_CATEGORY = 'OpenWrt'

export const LINUX_TEMPLATE_CATEGORY_OPTIONS = ['Ubuntu', 'Debian', 'CentOS']

export const WINDOWS_TEMPLATE_CATEGORY_OPTIONS = [
  'WindowsServer2025',
  'WindowsServer2022',
  'Windows11',
  'Windows10',
  'WindowsServer2012R2',
  '其它',
]

export const OPENWRT_TEMPLATE_CATEGORY_OPTIONS = ['OpenWrt', 'iStoreOS']

/** 模板类型归一化（小写） */
export function normalizeTemplateType(type?: string): string {
  return (type || '').toString().trim().toLowerCase()
}

/** 模板类型展示文案 */
export function templateTypeLabel(type?: string): string {
  const map: Record<string, string> = {
    windows: 'Windows',
    fnos: 'FnOS',
    openwrt: 'OpenWrt',
    other: '其它',
  }
  return map[normalizeTemplateType(type)] || 'Linux'
}

/** 模板分类归一化：不合法时回退默认分类；非 linux/windows/openwrt 返回空 */
export function normalizeTemplateCategory(type: string, category?: string): string {
  const normalizedType = normalizeTemplateType(type)
  if (!['linux', 'windows', 'openwrt'].includes(normalizedType)) {
    return ''
  }
  const normalized = (category || '').toString().trim()
  let options: string[]
  let defaultCategory: string
  if (normalizedType === 'windows') {
    options = WINDOWS_TEMPLATE_CATEGORY_OPTIONS
    defaultCategory = DEFAULT_WINDOWS_TEMPLATE_CATEGORY
  } else if (normalizedType === 'openwrt') {
    options = OPENWRT_TEMPLATE_CATEGORY_OPTIONS
    defaultCategory = DEFAULT_OPENWRT_TEMPLATE_CATEGORY
  } else {
    options = LINUX_TEMPLATE_CATEGORY_OPTIONS
    defaultCategory = DEFAULT_LINUX_TEMPLATE_CATEGORY
  }
  if (!normalized) return defaultCategory
  const matched = options.find((item) => item.toLowerCase() === normalized.toLowerCase())
  return matched || defaultCategory
}

/** 分类选项（按类型） */
export function templateCategoryOptions(type: string): string[] {
  const normalizedType = normalizeTemplateType(type)
  if (normalizedType === 'windows') return WINDOWS_TEMPLATE_CATEGORY_OPTIONS
  if (normalizedType === 'openwrt') return OPENWRT_TEMPLATE_CATEGORY_OPTIONS
  return LINUX_TEMPLATE_CATEGORY_OPTIONS
}

/** 模板分类展示文案（非 linux/windows/openwrt 返回空） */
export function templateCategoryLabel(type: string, category?: string): string {
  const normalizedType = normalizeTemplateType(type)
  if (!['linux', 'windows', 'openwrt'].includes(normalizedType)) return ''
  return normalizeTemplateCategory(normalizedType, category)
}

/** 模板分组展示文案（类型 / 分类） */
export function templateGroupLabel(type: string, category?: string): string {
  const typeLabel = templateTypeLabel(type)
  const categoryLabel = templateCategoryLabel(type, category)
  return categoryLabel ? `${typeLabel} / ${categoryLabel}` : typeLabel
}
