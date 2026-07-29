/**
 * 贴边侧边栏
 * - 按角色渲染导航分组（管理员 / 普通用户）
 * - 支持手动折叠（仅图标模式，折叠状态持久化）
 * - 小屏时转为抽屉模式
 */
import { useEffect, useMemo, useState, type CSSProperties } from 'react'
import { useLocation, useNavigate } from 'react-router'
import { Toast, Tooltip } from '@douyinfe/semi-ui'
import { IconChevronLeft } from '@douyinfe/semi-icons'
import { ADMIN_NAV, USER_NAV, type NavItem } from '@/config/nav'
import { useUserStore } from '@/stores/user'
import { useAppStore } from '@/stores/app'
import { useTaskStore } from '@/stores/task'
import { getVmList, getSelfVMs, type VmListItem } from '@/api/vm'
import { CLOUD_TYPES, ROLES } from '@/config/constants'

interface SidebarProps {
  mobileOpen: boolean
  onCloseMobile: () => void
}

export default function Sidebar({ mobileOpen, onCloseMobile }: SidebarProps) {
  const navigate = useNavigate()
  const location = useLocation()
  const role = useUserStore((s) => s.role)
  const cloudType = useUserStore((s) => s.cloudType)
  const siteTitle = useAppStore((s) => s.siteTitle)
  const collapsed = useAppStore((s) => s.sidebarCollapsed)
  const toggleSidebar = useAppStore((s) => s.toggleSidebar)
  const tasks = useTaskStore((s) => s.tasks)
  // 活动任务数（选择器不返回新引用，避免无限渲染）
  const taskActiveCount = tasks.filter((t) => t.status === 'pending' || t.status === 'running').length

  const isAdmin = role === ROLES.admin
  const [vms, setVms] = useState<VmListItem[]>([])

  // 轻量云用户不展示 VPC 菜单（轻量云走宿主机网桥直通）与我的存储菜单
  const navGroups = useMemo(() => {
    const base = isAdmin ? ADMIN_NAV : USER_NAV
    if (!isAdmin && cloudType === CLOUD_TYPES.lightweight) {
      return base.filter((g) => g.group !== '网络' && g.group !== '存储')
    }
    return base
  }, [isAdmin, cloudType])

  // 加载虚拟机列表（用于徽标）
  useEffect(() => {
    let mounted = true
    const load = async () => {
      try {
        const res = isAdmin ? await getVmList() : await getSelfVMs()
        if (mounted) setVms(res.data || [])
      } catch {
        // 列表失败不影响布局展示
      }
    }
    void load()
    return () => {
      mounted = false
    }
  }, [isAdmin])

  const handleNavClick = (item: NavItem) => {
    if (item.coming || !item.path) {
      Toast.info({ content: `「${item.title}」模块将在后续迭代提供`, duration: 2 })
      return
    }
    navigate(item.path)
    onCloseMobile()
  }

  const badgeValue = (item: NavItem): number | null => {
    if (item.badge === 'vm') return vms.length
    if (item.badge === 'task') return taskActiveCount
    return null
  }

  return (
    <aside className={`qvm-sidebar ${mobileOpen ? 'mobile-open' : ''}`}>
      {/* 折叠/展开开关（悬浮于侧边栏右缘，小屏抽屉模式隐藏） */}
      <Tooltip content={collapsed ? '展开侧边栏' : '折叠侧边栏'} position="right">
        <button type="button" className="qvm-side-fold" onClick={toggleSidebar}>
          <IconChevronLeft size="small" />
        </button>
      </Tooltip>

      <div className="qvm-logo-zone">
        <img className="qvm-logo-img" src="/favicon.png" alt="QVMC" />
        <div className="qvm-logo-txt">
          <div className="qvm-logo-name">{siteTitle}</div>
          <div className="qvm-logo-sub">KVM 虚拟化管理平台</div>
        </div>
      </div>

      <nav className="qvm-nav">
        {navGroups.map((group) => (
          <div key={group.group}>
            <div className="qvm-nav-group">{group.group}</div>
            {group.items.map((item) => {
              const active = item.path === '/dashboard' && location.pathname === '/dashboard'
              const badge = badgeValue(item)
              const navNode = (
                <div
                  className={`qvm-nav-item ${active ? 'on' : ''}`}
                  style={{ '--nav-ic': item.color } as CSSProperties}
                  onClick={() => handleNavClick(item)}
                >
                  {item.icon}
                  <span className="qvm-nav-txt">{item.title}</span>
                  {badge !== null && badge > 0 && (
                    <span className={`qvm-nav-bdg ${item.badge === 'task' ? 'purple' : ''}`}>
                      {badge}
                    </span>
                  )}
                </div>
              )
              return (
                <div key={item.key}>
                  {/* 折叠状态下仅剩图标，用 Tooltip 提示菜单名 */}
                  {collapsed && !mobileOpen ? (
                    <Tooltip content={item.title} position="right">
                      {navNode}
                    </Tooltip>
                  ) : (
                    navNode
                  )}
                </div>
              )
            })}
          </div>
        ))}
      </nav>
    </aside>
  )
}
