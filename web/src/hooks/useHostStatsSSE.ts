/**
 * 宿主机实时状态 SSE Hook（管理员仪表盘用）
 * - 后端每 5s 推送一次 HostStats
 * - 断线 5s 后自动重连
 */
import { useEffect, useRef, useState } from 'react'
import { createHostStatsSSE, getHostStats, type HostStats } from '@/api/host'
import { useUserStore } from '@/stores/user'

export type SseStatus = 'connecting' | 'connected' | 'disconnected'

export function useHostStatsSSE() {
  const token = useUserStore((s) => s.token)
  const [stats, setStats] = useState<HostStats | null>(null)
  const [sseStatus, setSseStatus] = useState<SseStatus>('connecting')
  const esRef = useRef<EventSource | null>(null)

  useEffect(() => {
    if (!token) return
    let reconnectTimer: ReturnType<typeof setTimeout> | null = null
    let disposed = false

    // 首屏先拉一次，避免等待首个 SSE 事件
    void getHostStats()
      .then((res) => setStats(res.data || null))
      .catch(() => undefined)

    const connect = () => {
      if (disposed) return
      setSseStatus('connecting')
      const es = createHostStatsSSE(token)
      esRef.current = es

      es.onopen = () => setSseStatus('connected')
      es.onmessage = (e) => {
        try {
          setStats(JSON.parse(e.data) as HostStats)
          setSseStatus('connected')
        } catch (err) {
          console.error('解析宿主机 SSE 事件失败', err)
        }
      }
      es.onerror = () => {
        setSseStatus('disconnected')
        es.close()
        esRef.current = null
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
  }, [token])

  return { stats, sseStatus }
}
