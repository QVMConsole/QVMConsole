/**
 * 路由配置
 * 按模块拆分：公共页面（登录等）与主框架页面（Layout 嵌套）。
 * 所有页面组件使用 React.lazy 懒加载，新增业务模块时照此模式追加。
 */
import { lazy, Suspense, type ReactNode } from 'react'
import { createBrowserRouter, Navigate } from 'react-router-dom'
import { Spin } from '@douyinfe/semi-ui'
import Layout from '@/layout'
import { RequireAuth } from './guards'
import NotFound from '@/views/error/NotFound'

// 页面懒加载（后续业务模块迁移后在此追加）
const LoginPage = lazy(() => import('@/views/login'))
const DashboardPage = lazy(() => import('@/views/dashboard'))
const VmListPage = lazy(() => import('@/views/vm'))
const VmDetailPage = lazy(() => import('@/views/vm/detail'))
const VncWindowPage = lazy(() => import('@/views/vm/vnc-window'))
const TemplateListPage = lazy(() => import('@/views/template'))
const NetworkPage = lazy(() => import('@/views/network'))
const PublicIpPage = lazy(() => import('@/views/public-ip'))

/** 懒加载页面统一加载态 */
function lazyPage(node: ReactNode) {
  return (
    <Suspense
      fallback={
        <div style={{ display: 'flex', justifyContent: 'center', paddingTop: 120 }}>
          <Spin size="large" />
        </div>
      }
    >
      {node}
    </Suspense>
  )
}

/**
 * 主框架内嵌页面路由。
 * handle.title 用于浏览器标题与面包屑。
 * 业务模块迁移完成后在此追加（虚拟机/模板/网络/存储/用户/设置等）。
 */
const mainChildren = [
  { index: true, element: <Navigate to="/dashboard" replace /> },
  {
    path: 'dashboard',
    element: lazyPage(<DashboardPage />),
    handle: { title: '首页' },
  },
  {
    path: 'vm',
    element: lazyPage(<VmListPage />),
    handle: { title: '虚拟机列表' },
  },
  {
    path: 'vm/detail/:id',
    element: lazyPage(<VmDetailPage />),
    handle: { title: '虚拟机详情' },
  },
  {
    path: 'template',
    element: lazyPage(<TemplateListPage />),
    handle: { title: '模板管理' },
  },
  {
    path: 'network',
    element: lazyPage(<NetworkPage />),
    handle: { title: '网络中心' },
  },
  {
    path: 'public-ip',
    element: lazyPage(<PublicIpPage />),
    handle: { title: '公网 IP' },
  },
  // TODO(重构迭代): 以下路由随各模块迁移逐步补齐
  // firewall、
  // storage-pool/list、nodes、my-storage、user/list、scheduler/events、
  // settings、api-docs、task/recent、about
  { path: '*', element: <NotFound />, handle: { title: '页面不存在' } },
] as const

export const router = createBrowserRouter([
  {
    path: '/login',
    element: lazyPage(<LoginPage />),
    handle: { title: '登录' },
  },
  {
    path: '/vm/:id/vnc-window',
    element: <VncWindowPage />,
    handle: { title: 'VNC 控制台' },
  },
  // TODO(重构迭代): /invite、/reset-password
  {
    path: '/',
    element: (
      <RequireAuth>
        <Layout />
      </RequireAuth>
    ),
    children: [...mainChildren],
  },
])
