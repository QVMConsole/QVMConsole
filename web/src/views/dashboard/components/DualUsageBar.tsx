/**
 * 理论最大量双进度条（内存卡片可扩展 zRAM 区段）
 * - 同一轨道内：当前使用量进度条在上（遮罩层），理论最大量进度条在下
 * - 可选 zRAM 段：右端对齐物理内存 100% 位置、从后往前显示，表示物理内存使用进入
 *   zRAM 压缩交换的阶段区间
 * - 理论量可能超过 100%（所有虚拟机同时满载），轨道按比例归一化
 * - 鼠标悬停通过 Tooltip 展示具体数值
 */
import { Tooltip } from '@douyinfe/semi-ui'

/** zRAM 段配色（琥珀） */
const ZRAM_COLOR = '#FBBF24'

interface DualUsageBarProps {
  /** 当前使用率（小数，如 0.42；可超过 1） */
  currentRatio: number
  /** 理论最大使用率（小数，如 1.31；可超过 1） */
  theoryRatio: number
  /** 悬停提示：当前使用具体数值，如 "42.1%（6.7 / 16 核）" */
  currentText: string
  /** 悬停提示：理论最大具体数值，如 "131.3%（21 / 16 核）" */
  theoryText: string
  /** 当前使用条渐变色（起） */
  color: string
  /** 当前使用条渐变色（止） */
  colorEnd: string
  /** 理论量条颜色（默认电紫） */
  theoryColor?: string
  /** 悬停提示底部说明文案（默认“理论最大 = 全部虚拟机同时满载时的占用”） */
  theoryNote?: string
  /** 悬停提示扩展行（如 KSM 节省 / zRAM 压缩情况） */
  extraTipLines?: string[]
  /** zRAM 容量占比（小数，相对总量；>0 时从 100% 位置从后往前显示区段） */
  zramRatio?: number
}

export default function DualUsageBar({
  currentRatio,
  theoryRatio,
  currentText,
  theoryText,
  color,
  colorEnd,
  theoryColor = '#8B5CF6',
  theoryNote,
  extraTipLines,
  zramRatio = 0,
}: DualUsageBarProps) {
  // 轨道归一化比例尺：取当前/理论/100% 三者最大值，保证理论超配时能完整展示
  const scale = Math.max(currentRatio, theoryRatio, 1)
  const currentWidth = Math.min((currentRatio / scale) * 100, 100)
  const theoryWidth = Math.min((theoryRatio / scale) * 100, 100)
  const showMark = scale > 1
  // 物理内存 100% 刻度位置（理论超配时轨道右端超过 100%）
  const markLeft = (1 / scale) * 100

  // zRAM 段：右端对齐物理内存 100% 位置，从后往前（进入压缩交换的阶段区间）
  const zramWidth = Math.min((Math.min(zramRatio, 1) / scale) * 100, markLeft)
  const zramLeft = markLeft - zramWidth
  const showZramSeg = zramWidth > 0.1

  const tip = (
    <div style={{ fontSize: 12, lineHeight: 1.8 }}>
      <div>当前使用：{currentText}</div>
      <div>理论最大：{theoryText}</div>
      {extraTipLines?.map((line) => (
        <div key={line}>{line}</div>
      ))}
      <div style={{ opacity: 0.7, marginTop: 2 }}>{theoryNote || '理论最大 = 全部虚拟机同时满载时的占用'}</div>
    </div>
  )

  return (
    <Tooltip content={tip} position="top">
      <div className="qvm-dual">
        <div className="qvm-dual-track">
          {/* 下层：理论最大量 */}
          <div
            className="qvm-dual-theory"
            style={{ width: `${theoryWidth}%`, backgroundColor: theoryColor }}
          />
          {/* zRAM 区段：右端贴物理 100% 位置从后往前，位于理论层之上、当前层之下 */}
          {showZramSeg && (
            <div
              className="qvm-dual-zram"
              style={{ left: `${zramLeft}%`, width: `${zramWidth}%`, backgroundColor: ZRAM_COLOR }}
            />
          )}
          {/* 上层：当前使用（遮罩关系，覆盖在理论层之上） */}
          <div
            className="qvm-dual-current"
            style={{
              width: `${currentWidth}%`,
              background: `linear-gradient(90deg, ${color}, ${colorEnd})`,
              boxShadow: `0 0 8px ${color}66`,
            }}
          />
          {showMark && <div className="qvm-dual-mark" style={{ left: `${markLeft}%` }} />}
        </div>
        <div className="qvm-dual-legend">
          <span>
            <i style={{ background: `linear-gradient(90deg, ${color}, ${colorEnd})` }} />
            当前 {(currentRatio * 100).toFixed(1)}%
          </span>
          <span>
            <i style={{ background: theoryColor, opacity: 0.6 }} />
            理论最大 {(theoryRatio * 100).toFixed(1)}%
          </span>
          {showZramSeg && (
            <span>
              <i style={{ background: ZRAM_COLOR, opacity: 0.75 }} />
              zRAM {(zramRatio * 100).toFixed(1)}%
            </span>
          )}
        </div>
      </div>
    </Tooltip>
  )
}
