/**
 * 页面标签页状态管理（顶部多对象快速切换栏）
 * - 工作台为固定标签（pin），不可关闭
 * - 其余标签随路由访问自动注册，可关闭
 * - 后续虚拟机详情等页面接入时，按 path 自动复用
 */
import { create } from 'zustand'

export interface PageTab {
  /** 唯一键（一般为路由 path） */
  key: string
  /** 标签标题 */
  title: string
  /** 是否固定（固定标签不可关闭，排在最前） */
  pinned?: boolean
  /** 状态点（虚拟机标签用）：run / warn / off，无则不显示 */
  dot?: 'run' | 'warn' | 'off'
}

interface PageTabsState {
  tabs: PageTab[]
  /** 注册（或更新）一个标签 */
  openTab: (tab: PageTab) => void
  /** 关闭标签，返回关闭后应跳转的 path（若关闭的是当前页） */
  closeTab: (key: string, currentPath: string) => string | null
  /** 清空（退出登录时调用） */
  reset: () => void
}

/** 固定标签：工作台 */
export const PINNED_TAB: PageTab = { key: '/dashboard', title: '工作台', pinned: true }

export const usePageTabsStore = create<PageTabsState>()((set, get) => ({
  tabs: [PINNED_TAB],

  openTab: (tab) => {
    set((state) => {
      const idx = state.tabs.findIndex((t) => t.key === tab.key)
      if (idx === -1) {
        return { tabs: [...state.tabs, tab] }
      }
      // 已存在则更新标题/状态点
      const next = state.tabs.slice()
      next[idx] = { ...next[idx], ...tab }
      return { tabs: next }
    })
  },

  closeTab: (key, currentPath) => {
    const state = get()
    const tab = state.tabs.find((t) => t.key === key)
    if (!tab || tab.pinned) return null
    const idx = state.tabs.findIndex((t) => t.key === key)
    const next = state.tabs.filter((t) => t.key !== key)
    set({ tabs: next })
    if (currentPath === key) {
      // 关闭当前页：优先回退到前一个标签，否则回工作台
      const fallback = next[Math.min(idx, next.length - 1)] || PINNED_TAB
      return fallback.key
    }
    return null
  },

  reset: () => set({ tabs: [PINNED_TAB] }),
}))
