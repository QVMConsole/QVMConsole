/**
 * 主布局（极简占位）
 * 注意：具体布局设计待确认后再实现，此处仅提供路由出口与页面标题同步。
 */
import { useEffect } from 'react'
import { Outlet, useMatches } from 'react-router-dom'
import { applyDocumentTitle } from '@/config/site'

export default function MainLayout() {
  const matches = useMatches()
  const currentTitle =
    (matches[matches.length - 1]?.handle as { title?: string } | undefined)?.title || ''

  // 路由变化时同步浏览器标题
  useEffect(() => {
    applyDocumentTitle(currentTitle)
  }, [currentTitle])

  return <Outlet />
}
