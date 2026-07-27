/**
 * 虚拟机模块自定义图标（Semi 图标集中没有电源图标，按设计稿补齐）
 */
import { Icon } from '@douyinfe/semi-ui'

interface CustomIconProps {
  size?: 'extra-small' | 'small' | 'default' | 'large' | 'extra-large' | 'inherit'
  spin?: boolean
  className?: string
  style?: React.CSSProperties
}

function PowerSvg() {
  return (
    <svg
      width="1em"
      height="1em"
      viewBox="0 0 24 24"
      fill="none"
      stroke="currentColor"
      strokeWidth="2"
      strokeLinecap="round"
      strokeLinejoin="round"
    >
      <path d="M18.36 6.64a9 9 0 1 1-12.73 0" />
      <line x1="12" y1="2" x2="12" y2="12" />
    </svg>
  )
}

/** 电源图标（关机 / 强制断电 / 批量电源） */
export function PowerIcon(props: CustomIconProps) {
  return <Icon svg={<PowerSvg />} {...props} />
}
