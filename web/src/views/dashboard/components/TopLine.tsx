/**
 * 仪表盘顶部问候行
 * - 问候语 + 状态摘要
 * - 主题切换已上移至顶部导航栏（layout/components/TopBar.tsx）
 */
import type { ReactNode } from 'react'
import { useUserStore } from '@/stores/user'
import { greetingByHour } from '@/utils/format'

interface TopLineProps {
  /** 问候语右侧的副标题（状态摘要） */
  subtitle: ReactNode
  /** 用户名右侧的云类型标签（普通用户用） */
  cloudTag?: string
}

export default function TopLine({ subtitle, cloudTag }: TopLineProps) {
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
    </div>
  )
}
