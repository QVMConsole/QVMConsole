/**
 * 顶部导航栏（与侧边栏贴边无缝衔接）
 * - 承载历史页面标签栏（固定顶部）
 * - 左侧为小屏菜单按钮（≤820px 显示）
 * - 右侧为开源版链接 + 赞助入口 + 主题切换按钮 + 预留扩展插槽（后续可放搜索、通知等）
 */
import { useEffect, useState, type ReactNode } from 'react'
import { Modal, Toast, Banner, Button, Tooltip, Dropdown } from '@douyinfe/semi-ui'
import { IconMenu, IconMoon, IconSun, IconAlertTriangle, IconCopy, IconGithubLogo, IconExit, IconSafeStroked } from '@douyinfe/semi-icons'
import { useTheme } from '@/hooks/useTheme'
import { CLOUD_TYPES, ROLES, THEME_MODES, EXTERNAL_LINKS } from '@/config/constants'
import { useUserStore } from '@/stores/user'
import { useTaskStore } from '@/stores/task'
import { usePageTabsStore } from '@/stores/pageTabs'
import { useNavigate } from 'react-router'
import PageTabsBar from './PageTabsBar'
import SponsorWidget from './SponsorWidget'

interface TopBarProps {
  /** 小屏打开侧边栏抽屉 */
  onOpenMobile: () => void
  /** 右侧扩展区内容（可选，保持可拓展性） */
  extra?: ReactNode
}

export default function TopBar({ onOpenMobile, extra }: TopBarProps) {
  const navigate = useNavigate()
  const { isDark, setThemeMode } = useTheme()
  const username = useUserStore((s) => s.username)
  const role = useUserStore((s) => s.role)
  const cloudType = useUserStore((s) => s.cloudType)
  const logout = useUserStore((s) => s.logout)
  const resetTabs = usePageTabsStore((s) => s.reset)
  const resetTasks = useTaskStore((s) => s.reset)
  const [betaVisible, setBetaVisible] = useState(false)

  const userRole = role === ROLES.admin
    ? '系统管理员'
    : cloudType === CLOUD_TYPES.lightweight
      ? '轻量云用户'
      : '弹性云用户'

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

  // 首次访问自动弹出公测须知
  useEffect(() => {
    if (localStorage.getItem('qvm_beta_notice') !== 'confirmed') {
      setBetaVisible(true)
    }
  }, [])

  const confirmBeta = () => {
    localStorage.setItem('qvm_beta_notice', 'confirmed')
    setBetaVisible(false)
  }

  const copyQQ = () => {
    const qq = '654641487'
    if (navigator.clipboard?.writeText) {
      navigator.clipboard.writeText(qq).then(() => {
        Toast.success({ content: 'QQ 群号已复制', duration: 2 })
      }).catch(() => fallbackCopyQQ(qq))
    } else {
      fallbackCopyQQ(qq)
    }
  }

  const fallbackCopyQQ = (text: string) => {
    const ta = document.createElement('textarea')
    ta.value = text
    ta.style.position = 'fixed'
    ta.style.opacity = '0'
    document.body.appendChild(ta)
    ta.select()
    document.execCommand('copy')
    document.body.removeChild(ta)
    Toast.success({ content: 'QQ 群号已复制', duration: 2 })
  }

  return (
    <header className="qvm-topbar">
      {/* 小屏菜单按钮 */}
      <div className="qvm-tool-ic qvm-side-toggle" onClick={onOpenMobile}>
        <IconMenu />
      </div>

      <PageTabsBar />

      {/* 公测提示 */}
      <div className="qvm-beta-notice" onClick={() => setBetaVisible(true)} title="点击查看公测须知">
        <IconAlertTriangle />
        <span>公测期间，建议做好数据备份</span>
      </div>

      <div className="qvm-topbar-extra">
        {extra}
        {/* 开源版链接 */}
        <Tooltip content="前往 GitHub 开源仓库" position="bottom">
          <a className="qvm-oss-link" href={EXTERNAL_LINKS.github} target="_blank" rel="noreferrer">
            <IconGithubLogo />
            <span>开源版</span>
          </a>
        </Tooltip>
        {/* 赞助支持入口（下拉菜单 + 自动弹窗） */}
        <SponsorWidget />
        {/* 主题切换（深色 / 浅色） */}
        <Tooltip content={isDark ? '切换为浅色' : '切换为深色'} position="bottom">
          <div
            className="qvm-tool-ic qvm-theme-toggle"
            onClick={() => setThemeMode(isDark ? THEME_MODES.light : THEME_MODES.dark)}
          >
            {isDark ? <IconSun /> : <IconMoon />}
          </div>
        </Tooltip>
        {/* 用户入口：保留安全中心与退出登录操作，收拢至右上角 */}
        <Dropdown
          trigger="click"
          position="bottomRight"
          clickToHide
          render={
            <Dropdown.Menu>
              <Dropdown.Item icon={<IconSafeStroked />} onClick={() => navigate('/security')}>
                安全中心
              </Dropdown.Item>
              <Dropdown.Divider />
              <Dropdown.Item icon={<IconExit />} type="danger" onClick={handleLogout}>
                退出登录
              </Dropdown.Item>
            </Dropdown.Menu>
          }
        >
          <span className="qvm-top-user-wrap">
            <Tooltip content={`${username || '用户'} · ${userRole}`} position="bottom">
              <button type="button" className="qvm-top-user" aria-label="用户菜单">
                <span className="qvm-top-user-avatar">
                  {username ? username.charAt(0).toUpperCase() + username.slice(1, 2) : 'U'}
                </span>
                <span className="qvm-top-user-info">
                  <span className="qvm-top-user-name">{username || '用户'}</span>
                  <span className="qvm-top-user-role">{userRole}</span>
                </span>
              </button>
            </Tooltip>
          </span>
        </Dropdown>
      </div>

      {/* 公测须知弹窗 */}
      <Modal
        title="公测须知"
        visible={betaVisible}
        onOk={confirmBeta}
        okText="我已知晓，继续使用"
        onCancel={confirmBeta}
        cancelText="稍后提醒"
        closeOnEsc={false}
        maskClosable={false}
        width={520}
      >
        <div className="qvm-beta-content">
          <Banner
            type="warning"
            closeIcon={null}
            title="当前系统处于公测阶段"
          />
          <div className="qvm-beta-body">
            <p>项目已完成内测，所有功能正常使用的情况下一般不会出现问题。但为了安全，还是建议您做好数据备份，避免不合适的操作触发程序 bug 造成数据丢失。</p>
            <div className="qvm-beta-divider" />
            <div className="qvm-beta-join">
              <p>务必加入官方 QQ 群：</p>
              <div className="qvm-beta-qq-group">
                <span className="qvm-beta-qq-number">654641487</span>
                <Button type="primary" theme="light" size="small" icon={<IconCopy />} onClick={copyQQ}>
                  复制群号
                </Button>
              </div>
              <p className="qvm-beta-tip">遇到问题及时反馈，反馈有效问题多的用户可以奖励 Pro 资格！</p>
            </div>
          </div>
        </div>
      </Modal>
    </header>
  )
}
