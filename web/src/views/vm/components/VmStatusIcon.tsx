/**
 * 虚拟机状态图标（纯图标展示，悬停 Tooltip 显示状态文案）
 */
import { Tooltip } from '@douyinfe/semi-ui'
import { IconPlayCircle, IconPause, IconStop, IconRefresh } from '@douyinfe/semi-icons'
import { vmStatusKind, vmStatusText } from '../utils'

interface VmStatusIconProps {
  status: string
}

export default function VmStatusIcon({ status }: VmStatusIconProps) {
  const kind = vmStatusKind(status)
  return (
    <Tooltip content={vmStatusText(status)} position="top">
      <span className={`qvm-st-ic ${kind}`} role="img" aria-label={vmStatusText(status)}>
        {kind === 'run' && <IconPlayCircle />}
        {kind === 'stop' && <IconStop />}
        {kind === 'warn' && <IconPause />}
        {kind === 'move' && <IconRefresh spin />}
      </span>
    </Tooltip>
  )
}
