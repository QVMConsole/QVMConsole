/**
 * 虚拟机表单自定义图标（Semi 图标集中没有硬盘图标，按设计补齐）
 */
import { Icon } from '@douyinfe/semi-ui'

interface CustomIconProps {
  size?: 'extra-small' | 'small' | 'default' | 'large' | 'extra-large' | 'inherit'
  spin?: boolean
  className?: string
  style?: React.CSSProperties
}

function DiskSvg() {
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
      <line x1="22" y1="12" x2="2" y2="12" />
      <path d="M5.45 5.11 2 12v6a2 2 0 0 0 2 2h16a2 2 0 0 0 2-2v-6l-3.45-6.89A2 2 0 0 0 16.76 4H7.24a2 2 0 0 0-1.79 1.11z" />
      <line x1="6" y1="16" x2="6.01" y2="16" />
      <line x1="10" y1="16" x2="10.01" y2="16" />
    </svg>
  )
}

/** 硬盘图标（系统磁盘 / 存储位置 / 额外数据盘 / 当前磁盘 / 硬盘引导） */
export function DiskIcon(props: CustomIconProps) {
  return <Icon svg={<DiskSvg />} {...props} />
}
