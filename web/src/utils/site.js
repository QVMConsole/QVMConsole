import { ref } from 'vue'
import { getPublicSettings } from '@/api/settings'

export const DEFAULT_SITE_TITLE = 'QVMConsole'
const SITE_TITLE_STORAGE_KEY = 'site_title'

// 泄露密码检测开关（默认开启）
export const passwordBreachCheckEnabled = ref(true)

// 创建虚拟机时 SPICE 显示协议开关的默认初始值（默认关闭）
export const spiceEnabledByDefault = ref(false)

function normalizeSiteTitle(value) {
  const normalized = String(value || '').trim()
  return normalized || DEFAULT_SITE_TITLE
}

function readCachedSiteTitle() {
  if (typeof window === 'undefined') {
    return DEFAULT_SITE_TITLE
  }
  try {
    return normalizeSiteTitle(localStorage.getItem(SITE_TITLE_STORAGE_KEY) || '')
  } catch {
    return DEFAULT_SITE_TITLE
  }
}

export const siteTitle = ref(readCachedSiteTitle())

export function getSiteTitle() {
  return normalizeSiteTitle(siteTitle.value)
}

export function setSiteTitle(value) {
  const normalized = normalizeSiteTitle(value)
  siteTitle.value = normalized
  if (typeof window !== 'undefined') {
    localStorage.setItem(SITE_TITLE_STORAGE_KEY, normalized)
  }
  return normalized
}

export function buildDocumentTitle(pageTitle = '') {
  const normalizedPageTitle = String(pageTitle || '').trim()
  const currentSiteTitle = getSiteTitle()
  return normalizedPageTitle ? `${normalizedPageTitle} - ${currentSiteTitle}` : currentSiteTitle
}

export function applyDocumentTitle(pageTitle = '') {
  if (typeof document !== 'undefined') {
    document.title = buildDocumentTitle(pageTitle)
  }
}

export async function syncPublicSiteTitle() {
  try {
    const res = await getPublicSettings()
    setSiteTitle(res.data?.site_title)
    // 同步泄露密码检测开关
    if (res.data?.password_breach_check_enabled !== undefined) {
      passwordBreachCheckEnabled.value = res.data.password_breach_check_enabled !== false
    }
    // 同步 SPICE 开关默认初始值
    if (res.data?.spice_enabled_by_default !== undefined) {
      spiceEnabledByDefault.value = res.data.spice_enabled_by_default === true
    }
    return getSiteTitle()
  } catch {
    return getSiteTitle()
  }
}
