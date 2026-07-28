/**
 * 设置页通用小组件：分区标题 / 表单行（label + 控件 + 提示）
 * 所有 Tab 共用，保证布局一致
 */
import type { ReactNode } from 'react'
import { IconInfoCircle } from '@douyinfe/semi-icons'

/** 分区标题（带图标的分隔条） */
export function SectionHead({ icon, title }: { icon?: ReactNode; title: string }) {
  return (
    <div className="stg-section-head">
      {icon && <span className="stg-section-icon">{icon}</span>}
      <span>{title}</span>
      <span className="stg-section-line" />
    </div>
  )
}

interface SettingRowProps {
  label: string
  /** 提示文字（一般包含环境变量说明） */
  tip?: ReactNode
  children: ReactNode
}

/** 表单行：左侧固定宽 label，右侧控件 + 提示 */
export function SettingRow({ label, tip, children }: SettingRowProps) {
  return (
    <div className="stg-row">
      <div className="stg-row-label">{label}</div>
      <div className="stg-row-main">
        <div className="stg-row-control">{children}</div>
        {tip && (
          <div className="stg-row-tip">
            <IconInfoCircle size="small" />
            <span>{tip}</span>
          </div>
        )}
      </div>
    </div>
  )
}
