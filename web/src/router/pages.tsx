/**
 * 懒加载页面声明
 * 独立文件以满足 Fast Refresh 的 only-export-components 约束：
 * 路由配置文件仅导出 router 对象，页面组件统一在此导出。
 * 新增业务模块时照此模式追加。
 */
import { lazy } from 'react'

export const LoginPage = lazy(() => import('@/views/login'))
export const InviteRegisterPage = lazy(() => import('@/views/invite'))
export const ResetPasswordPage = lazy(() => import('@/views/reset-password'))
export const DashboardPage = lazy(() => import('@/views/dashboard'))
export const VmListPage = lazy(() => import('@/views/vm'))
export const VmDetailPage = lazy(() => import('@/views/vm/detail'))
export const VncWindowPage = lazy(() => import('@/views/vm/vnc-window'))
export const TemplateListPage = lazy(() => import('@/views/template'))
export const NetworkPage = lazy(() => import('@/views/network'))
export const PublicIpPage = lazy(() => import('@/views/public-ip'))
export const FirewallPage = lazy(() => import('@/views/firewall'))
export const StoragePoolPage = lazy(() => import('@/views/storage-pool'))
export const MyStoragePage = lazy(() => import('@/views/my-storage'))
export const UserPage = lazy(() => import('@/views/user'))
export const NodePage = lazy(() => import('@/views/node'))
export const SchedulerPage = lazy(() => import('@/views/scheduler'))
export const TaskCenterPage = lazy(() => import('@/views/task'))
export const SettingsPage = lazy(() => import('@/views/settings'))
export const SecurityPage = lazy(() => import('@/views/security'))
export const ApiDocsPage = lazy(() => import('@/views/api-docs'))
export const AboutPage = lazy(() => import('@/views/about'))
