/**
 * 侧边栏导航配置（按角色区分）
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
  IconFolder,
  IconUserGroup,
  IconClockStroked,
  IconCheckList,
  IconSetting,
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
  /** 本轮未迁移，点击提示 */
  coming?: boolean
}

export interface NavGroup {
  group: string
  items: NavItem[]
}

/** 管理员导航 */
export const ADMIN_NAV: NavGroup[] = [
  {
    group: '概览',
    items: [{ key: 'dashboard', title: '工作台', icon: <IconGridRectangle />, path: '/dashboard' }],
  },
  {
    group: '计算',
    items: [
      { key: 'vm', title: '虚拟机', icon: <IconDesktop />, path: '/vm', badge: 'vm' },
      { key: 'template', title: '模板管理', icon: <IconLayers />, path: '/template' },
    ],
  },
  {
    group: '网络',
    items: [
      { key: 'network', title: '网络中心', icon: <IconBranch />, path: '/network' },
      { key: 'public-ip', title: '公网 IP', icon: <IconGlobeStroke />, path: '/public-ip' },
      { key: 'firewall', title: '防火墙', icon: <IconShield />, path: '/firewall' },
    ],
  },
  {
    group: '存储',
    items: [
      { key: 'storage-pool', title: '存储池', icon: <IconServer />, path: '/storage-pool' },
      { key: 'my-storage', title: '我的存储', icon: <IconFolder />, path: '/my-storage' },
    ],
  },
  {
    group: '系统',
    items: [
      { key: 'user', title: '用户管理', icon: <IconUserGroup />, path: '/user' },
      { key: 'scheduler', title: '调度事件', icon: <IconClockStroked />, path: '/scheduler', coming: true },
      { key: 'task', title: '任务中心', icon: <IconCheckList />, path: '/task', badge: 'task', coming: true },
      { key: 'settings', title: '系统设置', icon: <IconSetting />, path: '/settings', coming: true },
    ],
  },
  {
    group: '支持',
    items: [
      { key: 'api-docs', title: 'API 文档', icon: <IconCodeStroked />, path: '/api-docs', coming: true },
      { key: 'about', title: '关于项目', icon: <IconInfoCircle />, path: '/about', coming: true },
    ],
  },
]

/** 普通用户导航（弹性云；轻量云在弹性云基础上精简网络菜单） */
export const USER_NAV: NavGroup[] = [
  {
    group: '概览',
    items: [{ key: 'dashboard', title: '工作台', icon: <IconGridRectangle />, path: '/dashboard' }],
  },
  {
    group: '计算',
    items: [
      { key: 'vm', title: '我的虚拟机', icon: <IconDesktop />, path: '/vm', badge: 'vm' },
    ],
  },
  {
    group: '网络',
    items: [
      { key: 'vpc', title: 'VPC 网络', icon: <IconBranch />, path: '/network' },
    ],
  },
  {
    group: '存储',
    items: [
      { key: 'my-storage', title: '我的存储', icon: <IconFolder />, path: '/my-storage' },
    ],
  },
  {
    group: '系统',
    items: [
      { key: 'task', title: '任务中心', icon: <IconCheckList />, path: '/task', badge: 'task', coming: true },
    ],
  },
  {
    group: '支持',
    items: [
      { key: 'api-docs', title: 'API 文档', icon: <IconCodeStroked />, path: '/api-docs', coming: true },
      { key: 'about', title: '关于项目', icon: <IconInfoCircle />, path: '/about', coming: true },
    ],
  },
]
