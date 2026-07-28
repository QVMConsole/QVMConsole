/**
 * 存储池概览统计（统一卡片 + 全局空间分配进度条）
 * - 顶部 4 项统计：总容量 / 已用空间 / 可用空间 / 存储卷
 * - 底部统一进度条：已用（按总使用率变色）/ 可用 / 未挂载 / 未分配
 * 统计口径：基于树形结构叶子节点，已用/可用仅统计已挂载节点
 */
import { useMemo } from 'react'
import { Tooltip } from '@douyinfe/semi-ui'
import { IconBox, IconCoinMoneyStroked, IconFolder, IconLayers } from '@douyinfe/semi-icons'
import type { HostStoragePoolInfo } from '@/api/storagePool'
import { formatBytes } from '@/utils/format'
import { computeOverviewStats, type VGBriefStats } from '../utils'

interface OverviewCardsProps {
  pools: HostStoragePoolInfo[]
  vgStats?: VGBriefStats // VG 统计（第 4 项）
}

export default function OverviewCards({ pools, vgStats }: OverviewCardsProps) {
  const stats = useMemo(() => computeOverviewStats(pools), [pools])

  // 总体使用率（用于已用段变色）
  const usedPercent = stats.totalSize > 0 ? Math.round((stats.totalUsed / stats.totalSize) * 100) : 0
  const usedSegCls = usedPercent >= 90 ? 'used-danger' : usedPercent >= 70 ? 'used-warn' : 'used'

  const items = [
    {
      key: 'total',
      icon: <IconBox />,
      label: '总容量',
      value: formatBytes(stats.totalSize),
      sub: `${stats.diskCount} 块物理硬盘`,
      cls: 'teal',
    },
    {
      key: 'used',
      icon: <IconFolder />,
      label: '已用空间',
      value: formatBytes(stats.totalUsed),
      sub: `已挂载 ${stats.mountedCount} 个分区`,
      cls: 'purple',
    },
    {
      key: 'avail',
      icon: <IconCoinMoneyStroked />,
      label: '可用空间',
      value: formatBytes(stats.totalAvail),
      sub: '剩余可用',
      cls: 'blue',
    },
    {
      key: 'vg',
      icon: <IconLayers />,
      label: '存储卷',
      value: String(vgStats?.count ?? 0),
      sub: vgStats && vgStats.vgNames.length > 0 ? `LVM 卷组 ${vgStats.vgNames.join(' / ')}` : '无',
      cls: 'amber',
    },
  ]

  // 统一进度条分段（总和可能略大于物理盘总量，按总和归一化）
  const segments = [
    { key: 'used', cls: usedSegCls, dotCls: usedSegCls, label: '已用', value: stats.totalUsed },
    { key: 'avail', cls: 'avail', dotCls: 'avail', label: '可用', value: stats.totalAvail },
    { key: 'held', cls: 'held', dotCls: 'held', label: '未挂载', value: stats.heldSize },
    { key: 'free', cls: 'free', dotCls: 'free', label: '未分配', value: stats.unallocated },
  ].filter((s) => s.value > 0)
  const segTotal = segments.reduce((sum, s) => sum + s.value, 0) || 1

  return (
    <div className="sp-overview-unified qvm-fade-up">
      {/* ==================== 顶部统计 ==================== */}
      <div className="sp-ov-stats">
        {items.map((item) => (
          <div key={item.key} className="sp-ov-item">
            <span className={`sp-overview-icon ${item.cls}`}>{item.icon}</span>
            <div style={{ minWidth: 0 }}>
              <div className="sp-overview-label">{item.label}</div>
              <div className="sp-overview-value qvm-num">{item.value}</div>
              <Tooltip content={item.sub} position="top">
                <div className="sp-overview-sub">{item.sub}</div>
              </Tooltip>
            </div>
          </div>
        ))}
      </div>

      {/* ==================== 统一空间分配条 ==================== */}
      <div>
        <div className="sp-space-title">
          <span>空间分配</span>
          <span className="total">
            共 {formatBytes(stats.totalSize)} · 已用 {usedPercent}%
          </span>
        </div>
        <div className="sp-spacebar">
          {segments.map((seg) => (
            <Tooltip key={seg.key} content={`${seg.label} · ${formatBytes(seg.value)}`} position="top">
              <div className={`seg ${seg.cls}`} style={{ width: `${(seg.value / segTotal) * 100}%` }} />
            </Tooltip>
          ))}
        </div>
        <div className="sp-space-legend">
          {segments.map((seg) => (
            <span key={seg.key} className="lg-item">
              <i className={`lg-dot ${seg.dotCls}`} />
              {seg.label}{' '}
              <span className="lg-num">
                {formatBytes(seg.value)}
                {seg.key === 'used' && ` · ${usedPercent}%`}
              </span>
            </span>
          ))}
        </div>
      </div>
    </div>
  )
}
