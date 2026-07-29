/**
 * 顶部导航栏（与侧边栏贴边无缝衔接）
 * - 承载历史页面标签栏（固定顶部）
 * - 左侧为小屏菜单按钮（≤820px 显示）
 * - 右侧为开源版链接 + 赞助入口 + 主题切换按钮 + 预留扩展插槽（后续可放搜索、通知等）
 */
import { useEffect, useState, type ReactNode } from 'react'
import { Modal, Toast, Banner, Button, Tooltip } from '@douyinfe/semi-ui'
import { IconMenu, IconMoon, IconSun, IconAlertTriangle, IconCopy, IconGithubLogo } from '@douyinfe/semi-icons'
import { useTheme } from '@/hooks/useTheme'
import { THEME_MODES, EXTERNAL_LINKS } from '@/config/constants'
import PageTabsBar from './PageTabsBar'
import SponsorWidget from './SponsorWidget'

interface TopBarProps {
  /** 小屏打开侧边栏抽屉 */
  onOpenMobile: () => void
  /** 右侧扩展区内容（可选，保持可拓展性） */
  extra?: ReactNode
}

export default function TopBar({ onOpenMobile, extra }: TopBarProps) {
  const { isDark, setThemeMode } = useTheme()
  const [betaVisible, setBetaVisible] = useState(false)

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
