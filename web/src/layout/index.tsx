/**
 * 主布局（深空极光版）
 * - 悬浮式侧边栏 + 顶部标签页栏 + 底部任务栏
 * - 登录后启动任务 SSE；路由变化时同步页面标签与浏览器标题
 */
import { useEffect, useState } from 'react'
import { Outlet, useLocation, useMatches } from 'react-router-dom'
import { applyDocumentTitle } from '@/config/site'
import { useUserStore } from '@/stores/user'
import { useTaskStore } from '@/stores/task'
import { usePageTabsStore } from '@/stores/pageTabs'
import Sidebar from './components/Sidebar'
import PageTabsBar from './components/PageTabsBar'
import TaskBar from './components/TaskBar'
import { IconMenu } from '@douyinfe/semi-icons'
import './layout.css'

export default function MainLayout() {
  const location = useLocation()
  const matches = useMatches()
  const token = useUserStore((s) => s.token)
  const startSSE = useTaskStore((s) => s.startSSE)
  const stopSSE = useTaskStore((s) => s.stopSSE)
  const openTab = usePageTabsStore((s) => s.openTab)
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
    <div className="qvm-layout">
      {/* 极光氛围背景 */}
      <div className="qvm-aurora" />
      <div className="qvm-grid-tex" />

      <Sidebar mobileOpen={mobileOpen} onCloseMobile={() => setMobileOpen(false)} />
      {mobileOpen && <div className="qvm-sidebar-mask" onClick={() => setMobileOpen(false)} />}

      <main className="qvm-main">
        {/* 移动端顶栏：菜单按钮（≤820px 显示） */}
        <div className="qvm-mobile-bar">
          <div className="qvm-tool-ic qvm-side-toggle" onClick={() => setMobileOpen(true)}>
            <IconMenu />
          </div>
        </div>
        <PageTabsBar />
        <Outlet />
      </main>

      <TaskBar />
    </div>
  )
}
