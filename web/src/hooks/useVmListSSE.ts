/**
 * 虚拟机列表实时推送 Hook
 * - 默认常驻连接（列表实时刷新，无需手动开关）
 * - 缓存优先：进入页面先渲染 store 缓存，SSE/请求静默更新
 * - 断线 5s 自动重连
 */
import { useCallback, useEffect, useRef, useState } from 'react'
import { createVmListSSE, getSelfVMs, getVmList, type VmListItem } from '@/api/vm'
import { useUserStore } from '@/stores/user'
import { useVmStore } from '@/stores/vm'
import type { SseStatus } from './useHostStatsSSE'

interface UseVmListSSEOptions {
  isAdmin: boolean
}

export function useVmListSSE({ isAdmin }: UseVmListSSEOptions) {
  const token = useUserStore((s) => s.token)
  const vmList = useVmStore((s) => s.vmList)
  const lastFetchTime = useVmStore((s) => s.lastFetchTime)
  const setVmList = useVmStore((s) => s.setVmList)
  const [sseStatus, setSseStatus] = useState<SseStatus>('connecting')
  /** 首次数据是否已到达（用于首屏骨架屏） */
  const [loaded, setLoaded] = useState(lastFetchTime > 0)
  const esRef = useRef<EventSource | null>(null)

  /** 静默拉取一次列表（HTTP），不触发加载态 */
  const reload = useCallback(async (): Promise<VmListItem[] | null> => {
    try {
      const res = isAdmin
        ? await getVmList({ include_resource_usage: true, include_ip: true })
        : await getSelfVMs({ include_resource_usage: true, include_ip: true })
      if (Array.isArray(res.data)) {
        setVmList(res.data)
        setLoaded(true)
        return res.data
      }
    } catch (err) {
      console.error('刷新虚拟机列表失败', err)
    }
    return null
  }, [isAdmin, setVmList])

  useEffect(() => {
    if (!token) return
    let reconnectTimer: ReturnType<typeof setTimeout> | null = null
    let disposed = false

    // 无缓存时先 HTTP 拉一次，避免等待首个 SSE 事件
    if (useVmStore.getState().lastFetchTime <= 0) {
      void reload()
    }

    const connect = () => {
      if (disposed) return
      setSseStatus('connecting')
      const es = createVmListSSE(isAdmin, token)
      esRef.current = es

      es.onopen = () => setSseStatus('connected')
      es.addEventListener('vm_list', (event) => {
        try {
          const data = JSON.parse((event as MessageEvent).data)
          if (Array.isArray(data)) {
            setVmList(data as VmListItem[])
            setLoaded(true)
            setSseStatus('connected')
          }
        } catch (err) {
          console.error('解析虚拟机列表 SSE 事件失败', err)
        }
      })
      es.onerror = () => {
        setSseStatus('disconnected')
        es.close()
        if (esRef.current === es) esRef.current = null
        if (!disposed) {
          reconnectTimer = setTimeout(connect, 5000)
        }
      }
    }

    connect()

    return () => {
      disposed = true
      if (reconnectTimer) clearTimeout(reconnectTimer)
      esRef.current?.close()
      esRef.current = null
    }
  }, [token, isAdmin, setVmList, reload])

  return { list: vmList, sseStatus, loaded, reload }
}
