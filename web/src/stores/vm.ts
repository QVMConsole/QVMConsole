/**
 * 虚拟机列表缓存 Store（zustand）
 * 页面间导航（如从其他页返回列表页）时先渲染缓存，再由 SSE/请求静默更新
 */
import { create } from 'zustand'
import type { VmListItem } from '@/api/vm'

interface VmState {
  vmList: VmListItem[]
  lastFetchTime: number
  setVmList: (data: VmListItem[]) => void
  hasCachedData: () => boolean
  clearCache: () => void
}

export const useVmStore = create<VmState>()((set, get) => ({
  vmList: [],
  lastFetchTime: 0,

  setVmList: (data) => {
    if (Array.isArray(data)) {
      set({ vmList: data, lastFetchTime: Date.now() })
    }
  },

  hasCachedData: () => get().vmList.length > 0 && get().lastFetchTime > 0,

  clearCache: () => set({ vmList: [], lastFetchTime: 0 }),
}))
