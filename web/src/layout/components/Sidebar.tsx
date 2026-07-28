/**
 * 悬浮式侧边栏
 * - 按角色渲染导航分组（管理员 / 普通用户）
 * - 小屏时转为抽屉模式
 */
import { useEffect, useMemo, useState } from 'react'
import { useLocation, useNavigate } from 'react-router-dom'
import { Toast, Modal } from '@douyinfe/semi-ui'
import { IconExit } from '@douyinfe/semi-icons'
import { ADMIN_NAV, USER_NAV, type NavItem } from '@/config/nav'
import { useUserStore } from '@/stores/user'
import { useAppStore } from '@/stores/app'
import { useTaskStore } from '@/stores/task'
import { usePageTabsStore } from '@/stores/pageTabs'
import { getVmList, getSelfVMs, type VmListItem } from '@/api/vm'
import { CLOUD_TYPES, ROLES } from '@/config/constants'

interface SidebarProps {
  mobileOpen: boolean
  onCloseMobile: () => void
}

export default function Sidebar({ mobileOpen, onCloseMobile }: SidebarProps) {
  const navigate = useNavigate()
  const location = useLocation()
  const username = useUserStore((s) => s.username)
  const role = useUserStore((s) => s.role)
  const cloudType = useUserStore((s) => s.cloudType)
  const logout = useUserStore((s) => s.logout)
  const siteTitle = useAppStore((s) => s.siteTitle)
  const tasks = useTaskStore((s) => s.tasks)
  const resetTabs = usePageTabsStore((s) => s.reset)
  const resetTasks = useTaskStore((s) => s.reset)
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

  const handleLogout = () => {
    Modal.confirm({
      title: '退出登录',
      content: '确定要退出当前账号吗？',
      okText: '退出',
      cancelText: '取消',
      onOk: () => {
        logout()
        resetTabs()
        resetTasks()
        navigate('/login', { replace: true })
      },
    })
  }

  const badgeValue = (item: NavItem): number | null => {
    if (item.badge === 'vm') return vms.length
    if (item.badge === 'task') return taskActiveCount
    return null
  }

  return (
    <aside className={`qvm-sidebar qvm-g-border ${mobileOpen ? 'mobile-open' : ''}`}>
      <div className="qvm-logo-zone">
        <div className="qvm-logo-mark sm">Q</div>
        <div>
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
              return (
                <div key={item.key}>
                  <div
                    className={`qvm-nav-item ${active ? 'on' : ''}`}
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
                </div>
              )
            })}
          </div>
        ))}
      </nav>

      <div className="qvm-side-user">
        <div className="qvm-su-avatar">
          {username ? username.charAt(0).toUpperCase() + username.slice(1, 2) : 'U'}
        </div>
        <div className="qvm-su-info">
          <div className="qvm-su-name">{username || '用户'}</div>
          <div className="qvm-su-role">
            {isAdmin ? '系统管理员' : cloudType === CLOUD_TYPES.lightweight ? '轻量云用户' : '弹性云用户'}
          </div>
        </div>
        <span className="qvm-su-out" onClick={handleLogout} title="退出登录">
          <IconExit />
        </span>
      </div>
    </aside>
  )
}
