/**
 * 路由守卫
 * - RequireAuth：未登录跳转登录页
 * - 轻量云非管理员用户：仅允许访问白名单路由，其余重定向到虚拟机列表
 */
import type { ReactNode } from 'react'
import { Navigate, useLocation } from 'react-router'
import { useUserStore } from '@/stores/user'
import { CLOUD_TYPES, ROLES } from '@/config/constants'

/** 轻量云普通用户可访问的路径白名单（精确匹配或其子路径）；轻量云无网络/存储模块 */
const LIGHTWEIGHT_ALLOWED_PATHS = ['/dashboard', '/vm', '/task', '/api-docs', '/about']

/** 轻量云用户默认首页 */
const LIGHTWEIGHT_HOME = '/dashboard'

export function RequireAuth({ children }: { children: ReactNode }) {
  const location = useLocation()
  const token = useUserStore((s) => s.token)
  const role = useUserStore((s) => s.role)
  const cloudType = useUserStore((s) => s.cloudType)

  if (!token) {
    const redirect = encodeURIComponent(location.pathname + location.search)
    return <Navigate to={`/login?redirect=${redirect}`} replace />
  }

  // 轻量云非管理员用户的路由限制
  if (role !== ROLES.admin && cloudType === CLOUD_TYPES.lightweight) {
    const path = location.pathname
    const allowed =
      LIGHTWEIGHT_ALLOWED_PATHS.some((p) => path === p || path.startsWith(`${p}/`)) ||
      path.endsWith('/vnc-window')
    if (path === '/') {
      return <Navigate to={LIGHTWEIGHT_HOME} replace />
    }
    if (!allowed) {
      return <Navigate to={LIGHTWEIGHT_HOME} replace />
    }
  }

  return <>{children}</>
}
