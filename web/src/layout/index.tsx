/**
 * 主布局（深空极光版）
 * - 贴边侧边栏（可折叠） + 固定顶部导航栏（历史标签页） + 底部任务栏
 * - 登录后启动任务 SSE；路由变化时同步页面标签与浏览器标题
 */
import { useEffect, useState } from 'react'
import { Outlet, useLocation, useMatches } from 'react-router'
import { applyDocumentTitle } from '@/config/site'
import { useUserStore } from '@/stores/user'
import { useAppStore } from '@/stores/app'
import { useTaskStore } from '@/stores/task'
import { usePageTabsStore } from '@/stores/pageTabs'
import Sidebar from './components/Sidebar'
import TopBar from './components/TopBar'
import TaskBar from './components/TaskBar'
import './layout.css'

export default function MainLayout() {
  const location = useLocation()
  const matches = useMatches()
  const token = useUserStore((s) => s.token)
  const startSSE = useTaskStore((s) => s.startSSE)
  const stopSSE = useTaskStore((s) => s.stopSSE)
  const openTab = usePageTabsStore((s) => s.openTab)
  const sidebarCollapsed = useAppStore((s) => s.sidebarCollapsed)
  const [mobileOpen, setMobileOpen] = useState(false)

  const currentTitle =
    (matches[matches.length - 1]?.handle as { title?: string } | undefined)?.title || ''

  // 路由变化时同步浏览器标题 + 注册页面标签
  useEffect(() => {
    applyDocumentTitle(currentTitle)
    if (currentTitle && location.pathname !== '/dashboard') {
      openTab({ key: location.pathname, title: currentTitle })
    }
  }, [currentTitle, location.pathname, openTab])

  // 登录状态下启动任务 SSE
  useEffect(() => {
    if (token) {
      startSSE()
    }
    return () => stopSSE()
  }, [token, startSSE, stopSSE])

  // 路由变化时关闭移动端抽屉
  useEffect(() => {
    setMobileOpen(false)
  }, [location.pathname])

  return (
    <div className={`qvm-layout ${sidebarCollapsed ? 'side-fold' : ''}`}>
      {/* 极光氛围背景 */}
      <div className="qvm-aurora" />
      <div className="qvm-grid-tex" />

      <Sidebar mobileOpen={mobileOpen} onCloseMobile={() => setMobileOpen(false)} />
      {mobileOpen && <div className="qvm-sidebar-mask" onClick={() => setMobileOpen(false)} />}

      {/* 顶部导航栏：与侧边栏贴边衔接，固定承载历史标签页 */}
      <TopBar onOpenMobile={() => setMobileOpen(true)} />

      <main className="qvm-main">
        <Outlet />
      </main>

      <TaskBar />
    </div>
  )
}
