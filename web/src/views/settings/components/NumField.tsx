/**
 * 数字参数字段卡片（label + InputNumber + 提示）
 * 用于动态内存调度 / 带宽限制 / 默认 IOPS 等多数字段的网格布局，
 * 配合 .stg-field-grid 使用，保证多列排版整齐一致
 */
import { InputNumber } from '@douyinfe/semi-ui'

interface NumFieldProps {
  label: string
  value: number
  onChange: (value: number) => void
  min?: number
  max?: number
  step?: number
  /** 单位后缀（秒 / MB / % 等） */
  suffix?: string
  /** 简短提示 */
  tip?: string
}

export default function NumField({
  label,
  value,
  onChange,
  min,
  max,
  step,
  suffix,
  tip,
}: NumFieldProps) {
  return (
    <div className="stg-field">
      <div className="stg-field-label">
        {label}
        {suffix && <span className="stg-field-unit">{suffix}</span>}
      </div>
      <InputNumber
        value={value}
        onNumberChange={onChange}
        min={min}
        max={max}
        step={step}
        style={{ width: '100%' }}
      />
      {tip && <div className="stg-field-tip">{tip}</div>}
    </div>
  )
}
