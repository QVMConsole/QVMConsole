/** 侧边栏导航配置（按角色区分）
 * - lightweight：工作台、虚拟机、任务中心、支持
 * - elastic：+ VPC + 存储
 * - admin：全部模块 + 系统管理
 * - path 为后续模块迭代预留；coming=true 表示本轮尚未迁移，点击提示
 */
import type { ReactNode } from 'react'
import {
  IconGridRectangle,
  IconDesktop,
  IconLayers,
  IconBranch,
  IconGlobeStroke,
  IconShield,
  IconServer,
  IconServerStroked,
  IconFolder,
  IconUserGroup,
  IconClockStroked,
  IconCheckList,
  IconSetting,
  IconSafeStroked,
  IconCodeStroked,
  IconInfoCircle,
} from '@douyinfe/semi-icons'

export interface NavItem {
  key: string
  title: string
  icon: ReactNode
  /** 目标路由（后续模块路由） */
  path?: string
  /** 徽标类型：vm=虚拟机数量 task=活动任务数 */
  badge?: 'vm' | 'task'
  /** 悬停时图标彩色（CSS 颜色值） */
  color?: string
  /** 本轮未迁移，点击提示 */
  coming?: boolean
}

export interface NavGroup {
  group: string
  items: NavItem[]
}

/** 导航图标彩色映射（悬停时点亮，两套导航共用同一键位保证一致） */
export const NAV_COLORS: Record<string, string> = {
  dashboard: '#2dd4bf',
  vm: '#38bdf8',
  template: '#8b5cf6',
  network: '#f472b6',
  vpc: '#f472b6',
  'public-ip': '#60a5fa',
  firewall: '#fb7185',
  'storage-pool': '#f59e0b',
  'my-storage': '#fbbf24',
  user: '#34d399',
  nodes: '#fb923c',
  scheduler: '#22d3ee',
  task: '#a78bfa',
  settings: '#818cf8',
  security: '#4ade80',
  'api-docs': '#c084fc',
  about: '#94a3b8',
}

/** 管理员导航 */
export const ADMIN_NAV: NavGroup[] = [
  {
    group: '概览',
    items: [{ key: 'dashboard', title: '工作台', icon: <IconGridRectangle />, path: '/dashboard', color: NAV_COLORS.dashboard }],
  },
  {
    group: '计算',
    items: [
      { key: 'vm', title: '虚拟机', icon: <IconDesktop />, path: '/vm', badge: 'vm', color: NAV_COLORS.vm },
      { key: 'template', title: '模板管理', icon: <IconLayers />, path: '/template', color: NAV_COLORS.template },
    ],
  },
  {
    group: '网络',
    items: [
      { key: 'network', title: '网络中心', icon: <IconBranch />, path: '/network', color: NAV_COLORS.network },
      { key: 'public-ip', title: '公网 IP', icon: <IconGlobeStroke />, path: '/public-ip', color: NAV_COLORS['public-ip'] },
      { key: 'firewall', title: '防火墙', icon: <IconShield />, path: '/firewall', color: NAV_COLORS.firewall },
    ],
  },
  {
    group: '存储',
    items: [
      { key: 'storage-pool', title: '存储池', icon: <IconServer />, path: '/storage-pool', color: NAV_COLORS['storage-pool'] },
      { key: 'my-storage', title: '我的存储', icon: <IconFolder />, path: '/my-storage', color: NAV_COLORS['my-storage'] },
    ],
  },
  {
    group: '系统',
    items: [
      { key: 'user', title: '用户管理', icon: <IconUserGroup />, path: '/user', color: NAV_COLORS.user },
      { key: 'nodes', title: '节点管理', icon: <IconServerStroked />, path: '/nodes', color: NAV_COLORS.nodes },
      { key: 'scheduler', title: '调度事件', icon: <IconClockStroked />, path: '/scheduler', color: NAV_COLORS.scheduler },
      { key: 'task', title: '任务中心', icon: <IconCheckList />, path: '/task', badge: 'task', color: NAV_COLORS.task },
      { key: 'settings', title: '系统设置', icon: <IconSetting />, path: '/settings', color: NAV_COLORS.settings },
      { key: 'security', title: '安全中心', icon: <IconSafeStroked />, path: '/security', color: NAV_COLORS.security },
    ],
  },
  {
    group: '支持',
    items: [
      { key: 'api-docs', title: 'API 文档', icon: <IconCodeStroked />, path: '/api-docs', color: NAV_COLORS['api-docs'] },
      { key: 'about', title: '关于项目', icon: <IconInfoCircle />, path: '/about', color: NAV_COLORS.about },
    ],
  },
]

/** 普通用户导航（弹性云：工作台、虚拟机、VPC、存储、任务中心；轻量云在基础上精简网络/存储） */
export const USER_NAV: NavGroup[] = [
  {
    group: '概览',
    items: [{ key: 'dashboard', title: '工作台', icon: <IconGridRectangle />, path: '/dashboard', color: NAV_COLORS.dashboard }],
  },
  {
    group: '计算',
    items: [
      { key: 'vm', title: '我的虚拟机', icon: <IconDesktop />, path: '/vm', badge: 'vm', color: NAV_COLORS.vm },
    ],
  },
  {
    group: '网络',
    items: [
      { key: 'vpc', title: 'VPC 网络', icon: <IconBranch />, path: '/network', color: NAV_COLORS.vpc },
    ],
  },
  {
    group: '存储',
    items: [
      { key: 'my-storage', title: '我的存储', icon: <IconFolder />, path: '/my-storage', color: NAV_COLORS['my-storage'] },
    ],
  },
  {
    group: '系统',
    items: [
      { key: 'task', title: '任务中心', icon: <IconCheckList />, path: '/task', badge: 'task', color: NAV_COLORS.task },
      { key: 'security', title: '安全中心', icon: <IconSafeStroked />, path: '/security', color: NAV_COLORS.security },
    ],
  },
  {
    group: '支持',
    items: [
      { key: 'api-docs', title: 'API 文档', icon: <IconCodeStroked />, path: '/api-docs', color: NAV_COLORS['api-docs'] },
      { key: 'about', title: '关于项目', icon: <IconInfoCircle />, path: '/about', color: NAV_COLORS.about },
    ],
  },
]
