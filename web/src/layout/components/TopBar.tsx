/**
 * 顶部导航栏（与侧边栏贴边无缝衔接）
 * - 承载历史页面标签栏（固定顶部）
 * - 左侧为小屏菜单按钮（≤820px 显示）
 * - 右侧为主题切换按钮 + 预留扩展插槽（后续可放搜索、通知等）
 */
import type { ReactNode } from 'react'
import { IconMenu, IconMoon, IconSun } from '@douyinfe/semi-icons'
import { useTheme } from '@/hooks/useTheme'
import { THEME_MODES } from '@/config/constants'
import PageTabsBar from './PageTabsBar'

interface TopBarProps {
  /** 小屏打开侧边栏抽屉 */
  onOpenMobile: () => void
  /** 右侧扩展区内容（可选，保持可拓展性） */
  extra?: ReactNode
}

export default function TopBar({ onOpenMobile, extra }: TopBarProps) {
  const { isDark, setThemeMode } = useTheme()

  return (
    <header className="qvm-topbar">
      {/* 小屏菜单按钮 */}
      <div className="qvm-tool-ic qvm-side-toggle" onClick={onOpenMobile}>
        <IconMenu />
      </div>

      <PageTabsBar />

      <div className="qvm-topbar-extra">
        {extra}
        {/* 主题切换（深色 / 浅色） */}
        <div
          className="qvm-tool-ic qvm-theme-toggle"
          title={isDark ? '切换为浅色' : '切换为深色'}
          onClick={() => setThemeMode(isDark ? THEME_MODES.light : THEME_MODES.dark)}
        >
          {isDark ? <IconSun /> : <IconMoon />}
        </div>
      </div>
    </header>
  )
}
