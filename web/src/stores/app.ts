/**
 * 应用全局状态管理（主题、侧边栏、站点信息）
 */
import { create } from 'zustand'
import { STORAGE_KEYS, THEME_MODES, type ThemeMode } from '@/config/constants'

export const DEFAULT_SITE_TITLE = 'QVMConsole'

interface AppState {
  /** 主题模式：浅色 / 深色 / 跟随系统 */
  themeMode: ThemeMode
  /** 侧边栏是否折叠 */
  sidebarCollapsed: boolean
  /** 站点标题（来自公开设置） */
  siteTitle: string
  /** 泄露密码检测开关（公开设置同步，默认开启） */
  passwordBreachCheckEnabled: boolean
  /** 创建虚拟机时 SPICE 默认开关初始值（公开设置同步，默认关闭） */
  spiceEnabledByDefault: boolean
  setThemeMode: (mode: ThemeMode) => void
  toggleSidebar: () => void
  setSidebarCollapsed: (collapsed: boolean) => void
  setSiteTitle: (title: string) => void
  setPublicFlags: (flags: {
    password_breach_check_enabled?: boolean
    spice_enabled_by_default?: boolean
  }) => void
}

function normalizeTheme(value: string | null): ThemeMode {
  if (value === THEME_MODES.dark || value === THEME_MODES.system) {
    return value
  }
  return THEME_MODES.light
}

function normalizeSiteTitle(value: string | null | undefined): string {
  const normalized = String(value || '').trim()
  return normalized || DEFAULT_SITE_TITLE
}

/** 读取侧边栏折叠偏好：未记录时窄屏默认折叠 */
function normalizeSidebarCollapsed(value: string | null): boolean {
  if (value === '1') return true
  if (value === '0') return false
  return typeof window !== 'undefined' && window.innerWidth <= 1180
}

/** 将主题应用到 DOM（Semi Design 官方方案：body[theme-mode="dark"]） */
export function applyThemeToDOM(mode: ThemeMode) {
  const prefersDark = window.matchMedia('(prefers-color-scheme: dark)').matches
  const isDark = mode === THEME_MODES.dark || (mode === THEME_MODES.system && prefersDark)
  const body = document.body
  if (isDark) {
    body.setAttribute('theme-mode', 'dark')
  } else {
    body.removeAttribute('theme-mode')
  }
}

export const useAppStore = create<AppState>()((set) => ({
  themeMode: normalizeTheme(localStorage.getItem(STORAGE_KEYS.theme)),
  sidebarCollapsed: normalizeSidebarCollapsed(localStorage.getItem(STORAGE_KEYS.sidebarCollapsed)),
  siteTitle: normalizeSiteTitle(localStorage.getItem(STORAGE_KEYS.siteTitle)),
  passwordBreachCheckEnabled: true,
  spiceEnabledByDefault: false,

  setThemeMode: (mode) => {
    localStorage.setItem(STORAGE_KEYS.theme, mode)
    applyThemeToDOM(mode)
    set({ themeMode: mode })
  },

  toggleSidebar: () =>
    set((state) => {
      const next = !state.sidebarCollapsed
      localStorage.setItem(STORAGE_KEYS.sidebarCollapsed, next ? '1' : '0')
      return { sidebarCollapsed: next }
    }),

  setSidebarCollapsed: (collapsed) => {
    localStorage.setItem(STORAGE_KEYS.sidebarCollapsed, collapsed ? '1' : '0')
    set({ sidebarCollapsed: collapsed })
  },

  setSiteTitle: (title) => {
    const normalized = normalizeSiteTitle(title)
    localStorage.setItem(STORAGE_KEYS.siteTitle, normalized)
    set({ siteTitle: normalized })
  },

  setPublicFlags: (flags) => {
    set((state) => ({
      passwordBreachCheckEnabled:
        flags.password_breach_check_enabled !== undefined
          ? flags.password_breach_check_enabled !== false
          : state.passwordBreachCheckEnabled,
      spiceEnabledByDefault:
        flags.spice_enabled_by_default !== undefined
          ? flags.spice_enabled_by_default === true
          : state.spiceEnabledByDefault,
    }))
  },
}))

/** 拼接页面标题：页面名 - 站点名 */
export function buildDocumentTitle(pageTitle: string, siteTitle: string): string {
  const normalized = pageTitle.trim()
  return normalized ? `${normalized} - ${siteTitle}` : siteTitle
}
