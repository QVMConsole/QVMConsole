/**
 * 虚拟机详情 SSE 通道
 * - 监听 vm_detail 事件推送全量详情
 * - 网络/磁盘累计字节在前端增量计算速率（rx/tx 速率、读写速率、IOPS）
 * - 断线 5s 自动重连
 */
import { useEffect, useRef, useState } from 'react'
import { createVmDetailSSE, type VmDetailInfo } from '@/api/vm'
import { useUserStore } from '@/stores/user'

export type SseStatus = 'connecting' | 'connected' | 'disconnected'

interface PrevCounters {
  net_rx_bytes: number
  net_tx_bytes: number
  disk_rd_bytes: number
  disk_wr_bytes: number
  disk_rd_ops: number
  disk_wr_ops: number
}

export function useVmDetailSSE(vmName: string) {
  const token = useUserStore((s) => s.token)
  const [vmData, setVmData] = useState<VmDetailInfo | null>(null)
  const [sseStatus, setSseStatus] = useState<SseStatus>('connecting')
  /** 每次收到有效详情事件时递增，供详情页各配置面板同步附属数据。 */
  const [liveTick, setLiveTick] = useState(0)
  /** 状态变化信号（用于操作按钮 loading 复位） */
  const [statusTick, setStatusTick] = useState(0)
  const prevStatusRef = useRef<string>('')

  useEffect(() => {
    if (!vmName || !token) return

    let es: EventSource | null = null
    let reconnectTimer: number | null = null
    let closed = false
    let prev: PrevCounters | null = null
    let prevTime = 0

    const connect = () => {
      if (closed) return
      setSseStatus('connecting')
      es = createVmDetailSSE(vmName, token)

      es.addEventListener('vm_detail', (event) => {
        try {
          const data = JSON.parse((event as MessageEvent).data) as VmDetailInfo
          if (!data || !data.name) return

          // 速率增量计算（后端返回累计值）
          if (data.stats) {
            const now = Date.now()
            if (prev && prevTime > 0) {
              const dt = (now - prevTime) / 1000
              if (dt > 0) {
                data.stats.net_rx_rate = Math.max(0, (data.stats.net_rx_bytes - prev.net_rx_bytes) / dt)
                data.stats.net_tx_rate = Math.max(0, (data.stats.net_tx_bytes - prev.net_tx_bytes) / dt)
                data.stats.disk_rd_rate = Math.max(0, (data.stats.disk_rd_bytes - prev.disk_rd_bytes) / dt)
                data.stats.disk_wr_rate = Math.max(0, (data.stats.disk_wr_bytes - prev.disk_wr_bytes) / dt)
                data.stats.disk_rd_iops = Math.max(0, (data.stats.disk_rd_ops - prev.disk_rd_ops) / dt)
                data.stats.disk_wr_iops = Math.max(0, (data.stats.disk_wr_ops - prev.disk_wr_ops) / dt)
              }
            }
            prev = {
              net_rx_bytes: data.stats.net_rx_bytes,
              net_tx_bytes: data.stats.net_tx_bytes,
              disk_rd_bytes: data.stats.disk_rd_bytes,
              disk_wr_bytes: data.stats.disk_wr_bytes,
              disk_rd_ops: data.stats.disk_rd_ops,
              disk_wr_ops: data.stats.disk_wr_ops,
            }
            prevTime = now
          }

          if (prevStatusRef.current && prevStatusRef.current !== data.status) {
            setStatusTick((t) => t + 1)
          }
          prevStatusRef.current = data.status

          setVmData(data)
          setLiveTick((tick) => tick + 1)
          setSseStatus('connected')
        } catch (err) {
          console.error('解析 SSE 详情数据失败', err)
        }
      })

      es.onerror = () => {
        setSseStatus('disconnected')
        es?.close()
        es = null
        if (!closed) {
          reconnectTimer = window.setTimeout(connect, 5000)
        }
      }
    }

    connect()

    return () => {
      closed = true
      es?.close()
      if (reconnectTimer) window.clearTimeout(reconnectTimer)
    }
  }, [vmName, token])

  return { vmData, sseStatus, statusTick, liveTick }
}
