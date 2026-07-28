/**
 * 通用展示小组件：虚拟机状态 pill / SVG 迷你折线
 */
import { useMemo } from 'react'
import { vmStatusKind, vmStatusText } from './vmStatus'

// ==================== 虚拟机状态 Pill ====================

export function StatusPill({ status }: { status: string }) {
  const kind = vmStatusKind(status)
  return (
    <span className={`qvm-st-pill ${kind}`}>
      <i />
      {vmStatusText(status)}
    </span>
  )
}

// ==================== SVG 迷你折线（sparkline） ====================

interface SparklineProps {
  /** 数据点（0-max 的数值序列） */
  data: number[]
  color: string
  /** 数据最大值（归一化用），默认取数据内最大值 */
  max?: number
  height?: number
}

export function Sparkline({ data, color, max, height = 72 }: SparklineProps) {
  const width = 320
  const gradId = useMemo(() => `sp-${Math.random().toString(36).slice(2, 9)}`, [])

  if (data.length < 2) {
    return (
      <div style={{ height, display: 'flex', alignItems: 'center', justifyContent: 'center' }}>
        <span style={{ fontSize: 11, color: 'var(--qvm-text-2)' }}>暂无监控数据</span>
      </div>
    )
  }

  const peak = max ?? Math.max(...data, 1)
  const stepX = width / (data.length - 1)
  const points = data.map((v, i) => {
    const x = i * stepX
    const y = height - 6 - (Math.min(v / peak, 1) * (height - 14))
    return [x, y] as const
  })
  const linePath = points.map(([x, y], i) => `${i === 0 ? 'M' : 'L'}${x.toFixed(1)},${y.toFixed(1)}`).join(' ')
  const areaPath = `${linePath} L${width},${height} L0,${height} Z`

  return (
    <svg width="100%" height={height} viewBox={`0 0 ${width} ${height}`} preserveAspectRatio="none">
      <defs>
        <linearGradient id={gradId} x1="0" y1="0" x2="0" y2="1">
          <stop offset="0" stopColor={color} stopOpacity="0.25" />
          <stop offset="1" stopColor={color} stopOpacity="0" />
        </linearGradient>
      </defs>
      <path d={areaPath} fill={`url(#${gradId})`} />
      <path
        d={linePath}
        fill="none"
        stroke={color}
        strokeWidth="2"
        strokeLinecap="round"
        style={{ filter: `drop-shadow(0 0 4px ${color}66)` }}
      />
    </svg>
  )
}
