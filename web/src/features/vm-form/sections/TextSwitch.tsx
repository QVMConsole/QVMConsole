/**
 * 带文字说明的开关（Switch + 右侧状态文字）
 * Semi 官方推荐文本放在 Switch 外部（内嵌文字过窄会竖排），
 * 表单内所有开关统一使用该组件。
 */
import { Switch } from '@douyinfe/semi-ui'

interface TextSwitchProps {
  checked: boolean
  onChange: (checked: boolean) => void
  /** 开启状态文字 */
  checkedText?: string
  /** 关闭状态文字 */
  uncheckedText?: string
  disabled?: boolean
  size?: 'default' | 'small' | 'large'
}

export default function TextSwitch({
  checked,
  onChange,
  checkedText = '启用',
  uncheckedText = '关闭',
  disabled,
  size,
}: TextSwitchProps) {
  return (
    <span className="qvm-vf-text-switch">
      <Switch checked={checked} onChange={onChange} disabled={disabled} size={size} />
      <span className={`qvm-vf-text-switch-label${checked ? ' on' : ''}`}>
        {checked ? checkedText : uncheckedText}
      </span>
    </span>
  )
}
