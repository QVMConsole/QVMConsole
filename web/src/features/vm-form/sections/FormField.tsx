/**
 * 表单字段行（label + 控件 + 提示/错误）
 * 所有 Section 统一使用，保证表单视觉一致。
 */
import type { ReactNode } from 'react'
import { Tooltip } from '@douyinfe/semi-ui'
import { IconHelpCircle } from '@douyinfe/semi-icons'

interface FormFieldProps {
  label?: ReactNode
  required?: boolean
  /** 字段提示（显示在控件下方） */
  tip?: ReactNode
  /** 提示级别（warn 显示为警告色） */
  tipType?: 'info' | 'warn'
  /** 校验错误消息（优先于 tip 显示） */
  error?: string
  /** label 旁的帮助 Tooltip */
  help?: ReactNode
  children: ReactNode
  style?: React.CSSProperties
}

export default function FormField({
  label,
  required,
  tip,
  tipType = 'info',
  error,
  help,
  children,
  style,
}: FormFieldProps) {
  return (
    <div className="qvm-vf-field" style={style}>
      {label && (
        <div className={`qvm-vf-label${required ? ' required' : ''}`}>
          <span>{label}</span>
          {help && (
            <Tooltip content={help} position="top">
              <IconHelpCircle className="qvm-vf-label-help" size="small" />
            </Tooltip>
          )}
        </div>
      )}
      {children}
      {error ? (
        <div className="qvm-vf-tip error">{error}</div>
      ) : (
        tip && <div className={`qvm-vf-tip${tipType === 'warn' ? ' warn' : ''}`}>{tip}</div>
      )}
    </div>
  )
}
