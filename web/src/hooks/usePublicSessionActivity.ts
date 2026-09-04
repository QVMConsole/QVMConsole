import { useEffect } from 'react'
import { Toast } from '@douyinfe/semi-ui'
import { reportSessionActivity } from '@/api/auth'
import { useUserStore } from '@/stores/user'

const REPORT_INTERVAL_MS = 60_000

/** 仅由真实浏览器交互维持公网登录会话。 */
export function usePublicSessionActivity(token: string) {
  useEffect(() => {
    if (!token) return

    let disposed = false
    let publicSession: boolean | null = null
    let lastReportAt = 0
    let idleDeadline = 0
    let expiryTimer: number | null = null

    const clearExpiryTimer = () => {
      if (expiryTimer !== null) window.clearTimeout(expiryTimer)
      expiryTimer = null
    }

    const expireSession = () => {
      if (disposed || publicSession !== true) return
      useUserStore.getState().logout()
      Toast.warning({ content: '登录会话因长时间未操作已失效，请重新登录', duration: 5 })
      const redirect = encodeURIComponent(window.location.pathname + window.location.search)
      window.location.href = `/login?redirect=${redirect}`
    }

    const armExpiryTimer = () => {
      clearExpiryTimer()
      if (!idleDeadline || publicSession !== true) return
      const remaining = idleDeadline - Date.now()
      if (remaining <= 0) {
        expireSession()
        return
      }
      expiryTimer = window.setTimeout(expireSession, remaining + 250)
    }

    const report = async (force = false) => {
      if (disposed || publicSession === false) return
      const now = Date.now()
      if (!force && now-lastReportAt < REPORT_INTERVAL_MS) return
      lastReportAt = now
      try {
        const res = await reportSessionActivity()
        if (disposed) return
        publicSession = !!res.data?.public_session
        if (!publicSession) {
          clearExpiryTimer()
          return
        }
        idleDeadline = res.data?.idle_expires_at
          ? new Date(res.data.idle_expires_at).getTime()
          : Date.now() + (res.data?.idle_timeout_seconds || 1800) * 1000
        armExpiryTimer()
      } catch {
        // 401 由请求层统一清理登录态；暂时性网络错误不伪造活动时间。
      }
    }

    const onActivity = () => void report()
    const onVisibilityChange = () => {
      if (document.visibilityState !== 'visible' || publicSession !== true) return
      if (idleDeadline > 0 && Date.now() >= idleDeadline) {
        expireSession()
      }
    }

    const events: Array<keyof WindowEventMap> = ['pointerdown', 'keydown', 'touchstart', 'wheel', 'scroll']
    events.forEach((event) => window.addEventListener(event, onActivity, { passive: true }))
    window.addEventListener('qvm-user-activity', onActivity)
    document.addEventListener('visibilitychange', onVisibilityChange)

    return () => {
      disposed = true
      clearExpiryTimer()
      events.forEach((event) => window.removeEventListener(event, onActivity))
      window.removeEventListener('qvm-user-activity', onActivity)
      document.removeEventListener('visibilitychange', onVisibilityChange)
    }
  }, [token])
}

/** 统一处理 SSE/长连接收到的会话失效事件。 */
export function redirectAfterPublicSessionExpired() {
  useUserStore.getState().logout()
  Toast.warning({ content: '登录会话因长时间未操作已失效，请重新登录', duration: 5 })
  const redirect = encodeURIComponent(window.location.pathname + window.location.search)
  window.location.href = `/login?redirect=${redirect}`
}

declare global {
  interface WindowEventMap {
    'qvm-user-activity': Event
  }
}
