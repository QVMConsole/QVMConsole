/**
 * 仪表盘顶部问候行
 * - 问候语 + 状态摘要 + 全局搜索（管理员）
 * - 主题切换已上移至顶部导航栏（layout/components/TopBar.tsx）
 */
import type { ReactNode } from 'react'
import { Toast } from '@douyinfe/semi-ui'
import { IconSearch } from '@douyinfe/semi-icons'
import { useUserStore } from '@/stores/user'
import { greetingByHour } from '@/utils/format'

interface TopLineProps {
  /** 问候语右侧的副标题（状态摘要） */
  subtitle: ReactNode
  /** 用户名右侧的云类型标签（普通用户用） */
  cloudTag?: string
  /** 是否展示全局搜索框（管理员端，搜索功能后续迭代接入） */
  showSearch?: boolean
}

export default function TopLine({ subtitle, cloudTag, showSearch }: TopLineProps) {
  const username = useUserStore((s) => s.username)

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
      {showSearch && (
        <div className="qvm-top-tools">
          <div
            className="qvm-search-box"
            onClick={() => Toast.info({ content: '全局搜索将在后续迭代提供', duration: 2 })}
          >
            <IconSearch size="small" />
            搜索虚拟机、模板
            <span className="kbd">⌘K</span>
          </div>
        </div>
      )}
    </div>
  )
}
