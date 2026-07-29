import { Switch } from '@douyinfe/semi-ui'

interface TextSwitchProps {
  checked: boolean
  onChange: (checked: boolean) => void
  checkedText?: string
  uncheckedText?: string
  disabled?: boolean
  size?: 'default' | 'small' | 'large'
}

export default function TextSwitch({
  checked,
  onChange,
  checkedText = '开',
  uncheckedText = '关',
  disabled,
  size,
}: TextSwitchProps) {
  const checkedLabel = Array.from(checkedText.trim())[0] || '开'
  const uncheckedLabel = Array.from(uncheckedText.trim())[0] || '关'

  return (
    <Switch
      checked={checked}
      onChange={onChange}
      disabled={disabled}
      size={size}
      checkedText={checkedLabel}
      uncheckedText={uncheckedLabel}
    />
  )
}
