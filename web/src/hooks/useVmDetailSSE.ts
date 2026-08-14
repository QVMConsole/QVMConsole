/**
 * 虚拟机详情 SSE 通道
 * - 监听 vm_detail 事件推送全量详情
 * - 网络/磁盘累计字节在前端增量计算速率（rx/tx 速率、读写速率、IOPS）
 * - 断线 5s 自动重连
 * - 标签页或浏览器失焦 10s 后自动停止刷新；重新聚焦时立即重连（后端首推一次）
 *   随后按原频率持续推送
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
    /** 失焦 10s 后停止刷新的计时器 */
    let idleStopTimer: number | null = null
    let closed = false
    /** 是否因失焦主动停止（用于抑制断线自动重连） */
    let stoppedByIdle = false
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
        // 因失焦主动停止时不自动重连，等待重新聚焦后由 focus 逻辑重连
        if (!closed && !stoppedByIdle) {
          reconnectTimer = window.setTimeout(connect, 5000)
        }
      }
    }

    // 窗口焦点状态：document.hasFocus() 不反映浏览器窗口的操作系统级焦点，
    // 切到其他应用时仍可能返回 true，因此必须用 window focus/blur 事件维护状态
    let windowFocused = document.hasFocus()
    let tabVisible = document.visibilityState === 'visible'

    // 判断页面是否处于活跃状态：标签页可见且窗口聚焦
    const isPageActive = () => windowFocused && tabVisible

    // 失焦持续 10s 后主动关闭 SSE，停止刷新
    const scheduleIdleStop = () => {
      if (idleStopTimer !== null) return
      idleStopTimer = window.setTimeout(() => {
        idleStopTimer = null
        // 再次确认仍未活跃（可能在计时期间已重新聚焦）
        if (isPageActive()) return
        stoppedByIdle = true
        if (reconnectTimer !== null) {
          window.clearTimeout(reconnectTimer)
          reconnectTimer = null
        }
        if (es) {
          es.close()
          es = null
        }
        setSseStatus('disconnected')
      }, 10000)
    }

    const cancelIdleStop = () => {
      if (idleStopTimer !== null) {
        window.clearTimeout(idleStopTimer)
        idleStopTimer = null
      }
    }

    // 活跃状态变化处理：重新聚焦则立即重连，失焦则启动停止计时
    const handleActivityChange = () => {
      if (isPageActive()) {
        // 重新聚焦：取消停止计时
        cancelIdleStop()
        // 若此前因失焦停止，立即重连（后端连接后立即推送一次 = 立即刷新）
        if (stoppedByIdle) {
          stoppedByIdle = false
          // 清空速率基准，避免跨失焦期间的累计值污染瞬时速率
          prev = null
          prevTime = 0
          connect()
        }
      } else {
        // 失焦：启动 10s 停止计时
        scheduleIdleStop()
      }
    }

    // 标签页可见性变化（切到其他标签页 / 最小化）
    const handleVisibility = () => {
      tabVisible = document.visibilityState === 'visible'
      handleActivityChange()
    }

    // 窗口获得焦点（从其他应用/窗口切回浏览器）
    const handleWindowFocus = () => {
      windowFocused = true
      handleActivityChange()
    }

    // 窗口失去焦点（切到其他应用/窗口；焦点移至地址栏/DevTools 也会触发，
    // 因有 10s 缓冲，短暂操作不会误停）
    const handleWindowBlur = () => {
      windowFocused = false
      handleActivityChange()
    }

    connect()
    // 初始挂载时若已处于失焦状态，同样启动停止计时
    if (!isPageActive()) scheduleIdleStop()

    document.addEventListener('visibilitychange', handleVisibility)
    window.addEventListener('focus', handleWindowFocus)
    window.addEventListener('blur', handleWindowBlur)

    return () => {
      closed = true
      es?.close()
      if (reconnectTimer) window.clearTimeout(reconnectTimer)
      if (idleStopTimer) window.clearTimeout(idleStopTimer)
      document.removeEventListener('visibilitychange', handleVisibility)
      window.removeEventListener('focus', handleWindowFocus)
      window.removeEventListener('blur', handleWindowBlur)
    }
  }, [vmName, token])

  return { vmData, sseStatus, statusTick, liveTick }
}
