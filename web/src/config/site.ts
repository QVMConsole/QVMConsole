/**
 * 站点信息管理：站点标题、公开设置同步
 * 对应旧版 utils/site.js
 */
import { useAppStore, buildDocumentTitle } from '@/stores/app'
import { getPublicSettings } from '@/api/settings'

/** 更新浏览器标签页标题 */
export function applyDocumentTitle(pageTitle = '') {
  const { siteTitle } = useAppStore.getState()
  document.title = buildDocumentTitle(pageTitle, siteTitle)
}

/** 从后端同步公开设置（站点标题、泄露密码检测开关、SPICE 默认值等） */
export async function syncPublicSiteInfo(): Promise<string> {
  try {
    const res = await getPublicSettings()
    const data = res.data || {}
    const appStore = useAppStore.getState()
    if (data.site_title !== undefined) {
      appStore.setSiteTitle(data.site_title || '')
    }
    appStore.setPublicFlags({
      password_breach_check_enabled: data.password_breach_check_enabled,
      spice_enabled_by_default: data.spice_enabled_by_default,
    })
    return useAppStore.getState().siteTitle
  } catch {
    return useAppStore.getState().siteTitle
  }
}
