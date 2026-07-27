/**
 * 分区卡片容器（表单分区的统一外观：图标 + 标题 + 右侧操作区 + 内容）
 */
import type { ReactNode } from 'react'

interface SectionCardProps {
  icon?: ReactNode
  title: ReactNode
  extra?: ReactNode
  children: ReactNode
  style?: React.CSSProperties
}

export default function SectionCard({ icon, title, extra, children, style }: SectionCardProps) {
  return (
    <div className="qvm-vf-section" style={style}>
      <div className="qvm-vf-section-header">
        {icon && <span className="qvm-vf-section-icon">{icon}</span>}
        <span className="qvm-vf-section-title">{title}</span>
        {extra && <span className="qvm-vf-section-extra">{extra}</span>}
      </div>
      <div className="qvm-vf-section-body">{children}</div>
    </div>
  )
}
