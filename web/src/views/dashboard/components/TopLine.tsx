/**
 * 仪表盘顶部问候行
 * - 问候语 + 状态摘要 + 全局工具（主题切换 / 通知 / 任务 / 主操作按钮）
 */
import type { ReactNode } from 'react'
import { Button, Toast } from '@douyinfe/semi-ui'
import { IconBell, IconSun, IconMoon, IconPlus, IconCheckList, IconSearch } from '@douyinfe/semi-icons'
import { useUserStore } from '@/stores/user'
import { useTheme } from '@/hooks/useTheme'
import { useTaskStore } from '@/stores/task'
import { greetingByHour } from '@/utils/format'
import { THEME_MODES } from '@/config/constants'

interface TopLineProps {
  /** 问候语右侧的副标题（状态摘要） */
  subtitle: ReactNode
  /** 用户名右侧的云类型标签（普通用户用） */
  cloudTag?: string
  /** 主操作按钮文案（如 新建虚拟机 / 从模板创建） */
  actionText: string
  /** 是否展示全局搜索框（管理员端，搜索功能后续迭代接入） */
  showSearch?: boolean
}

export default function TopLine({ subtitle, cloudTag, actionText, showSearch }: TopLineProps) {
  const username = useUserStore((s) => s.username)
  const { isDark, setThemeMode } = useTheme()
  const tasks = useTaskStore((s) => s.tasks)
  // 活动任务数（选择器不返回新引用，避免无限渲染）
  const activeCount = tasks.filter((t) => t.status === 'pending' || t.status === 'running').length

  return (
    <div className="qvm-topline">
      <div>
        <div className="qvm-hello">
          {greetingByHour()}，<em>{username || '用户'}</em>
          {cloudTag && <span className="qvm-cloud-tag">{cloudTag}</span>}
        </div>
        <div className="qvm-hello-sub">
          <span className="qvm-pulse" />
          {subtitle}
        </div>
      </div>
      <div className="qvm-top-tools">
        {showSearch && (
          <div
            className="qvm-search-box"
            onClick={() => Toast.info({ content: '全局搜索将在后续迭代提供', duration: 2 })}
          >
            <IconSearch size="small" />
            搜索虚拟机、模板
            <span className="kbd">⌘K</span>
          </div>
        )}
        <div
          className="qvm-tool-ic"
          title="任务中心"
          onClick={() => Toast.info({ content: '展开底部任务栏可查看实时任务进度', duration: 2 })}
        >
          <IconCheckList />
          {activeCount > 0 && <span className="qvm-tool-dot" />}
        </div>
        <div
          className="qvm-tool-ic"
          title="通知"
          onClick={() => Toast.info({ content: '通知中心将在后续迭代提供', duration: 2 })}
        >
          <IconBell />
        </div>
        <div
          className="qvm-tool-ic"
          title={isDark ? '切换为浅色' : '切换为深色'}
          onClick={() => setThemeMode(isDark ? THEME_MODES.light : THEME_MODES.dark)}
        >
          {isDark ? <IconSun /> : <IconMoon />}
        </div>
        <Button
          className="qvm-btn-grad qvm-btn-new"
          icon={<IconPlus />}
          onClick={() => Toast.info({ content: '新建虚拟机功能将在后续迭代提供', duration: 2 })}
        >
          <span>{actionText}</span>
        </Button>
      </div>
    </div>
  )
}
