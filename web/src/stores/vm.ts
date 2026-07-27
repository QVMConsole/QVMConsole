/**
 * 虚拟机列表缓存 Store（zustand）
 * 页面间导航（如从其他页返回列表页）时先渲染缓存，再由 SSE/请求静默更新
 * 同时维护“最近访问的虚拟机”记录，用于侧边栏快速切换
 */
import { create } from 'zustand'
import type { VmListItem } from '@/api/vm'

/** 最近访问的虚拟机记录 */
export interface VisitedVm {
  id: string
  name: string
}

const VISITED_VMS_KEY = 'visitedVms'
const VISITED_VMS_LIMIT = 8

function readVisitedVms(): VisitedVm[] {
  try {
    const raw = localStorage.getItem(VISITED_VMS_KEY)
    const list = raw ? (JSON.parse(raw) as VisitedVm[]) : []
    return Array.isArray(list) ? list : []
  } catch {
    return []
  }
}

interface VmState {
  vmList: VmListItem[]
  lastFetchTime: number
  visitedVms: VisitedVm[]
  setVmList: (data: VmListItem[]) => void
  hasCachedData: () => boolean
  clearCache: () => void
  /** 记录一次访问（同名置顶，最多保留 VISITED_VMS_LIMIT 条） */
  addVisitedVm: (vm: VisitedVm) => void
  removeVisitedVm: (id: string) => void
}

export const useVmStore = create<VmState>()((set, get) => ({
  vmList: [],
  lastFetchTime: 0,
  visitedVms: readVisitedVms(),

  setVmList: (data) => {
    if (Array.isArray(data)) {
      set({ vmList: data, lastFetchTime: Date.now() })
    }
  },

  hasCachedData: () => get().vmList.length > 0 && get().lastFetchTime > 0,

  clearCache: () => set({ vmList: [], lastFetchTime: 0 }),

  addVisitedVm: (vm) => {
    if (!vm?.id) return
    set((state) => {
      const rest = state.visitedVms.filter((v) => v.id !== vm.id)
      const visitedVms = [vm, ...rest].slice(0, VISITED_VMS_LIMIT)
      localStorage.setItem(VISITED_VMS_KEY, JSON.stringify(visitedVms))
      return { visitedVms }
    })
  },

  removeVisitedVm: (id) => {
    set((state) => {
      const visitedVms = state.visitedVms.filter((v) => v.id !== id)
      localStorage.setItem(VISITED_VMS_KEY, JSON.stringify(visitedVms))
      return { visitedVms }
    })
  },
}))
