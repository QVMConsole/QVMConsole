/**
 * 用户仪表盘：资源总览 5 张配额卡
 * - CPU / 内存 / 虚拟机数量 / 磁盘 / 本月运行时长
 * - 配额为 0 表示不限
 */
import type { CSSProperties, ReactNode } from 'react'
import type { QuotaUsage } from '@/api/vm'
import { CpuIcon, MemIcon, VmIcon, DiskIcon, ClockIcon } from './icons'

interface QuotaCardsProps {
  quota: QuotaUsage | null
}

interface CardDef {
  label: string
  color: string
  bg: string
  border: string
  icon: ReactNode
  usedText: string
  maxText: string
  /** 0-100，null 表示不限（不渲染进度条） */
  percent: number | null
  foot: string
  footDanger?: boolean
  delay: number
}

function quotaPercent(used: number, max: number): number | null {
  if (!max || max <= 0) return null
  return Math.min(Math.round((used / max) * 100), 100)
}

export default function UserQuotaCards({ quota }: QuotaCardsProps) {
  const q = quota
  const runtimeHours = q ? q.used_runtime_seconds / 3600 : 0

  const cards: CardDef[] = [
    {
      label: 'CPU 配额',
      color: '#2DD4BF',
      bg: 'rgba(45,212,191,.1)',
      border: 'rgba(45,212,191,.2)',
      icon: <CpuIcon size={14} />,
      usedText: `${q?.used_cpu ?? 0}`,
      maxText: q?.max_cpu ? `/ ${q.max_cpu} 核` : '不限',
      percent: q ? quotaPercent(q.used_cpu, q.max_cpu) : 0,
      foot: q?.max_cpu ? `使用率 ${Math.round((q.used_cpu / q.max_cpu) * 100)}%` : '配额不限',
      delay: 0,
    },
    {
      label: '内存配额',
      color: '#8B5CF6',
      bg: 'rgba(139,92,246,.1)',
      border: 'rgba(139,92,246,.2)',
      icon: <MemIcon size={14} />,
      usedText: `${q?.used_memory ?? 0}`,
      maxText: q?.max_memory ? `/ ${q.max_memory} GB` : '不限',
      percent: q ? quotaPercent(q.used_memory, q.max_memory) : 0,
      foot: q?.max_memory ? `使用率 ${Math.round((q.used_memory / q.max_memory) * 100)}%` : '配额不限',
      delay: 60,
    },
    {
      label: '虚拟机数量',
      color: '#38BDF8',
      bg: 'rgba(56,189,248,.1)',
      border: 'rgba(56,189,248,.2)',
      icon: <VmIcon size={14} />,
      usedText: `${q?.used_vm ?? 0}`,
      maxText: q?.max_vm ? `/ ${q.max_vm} 台` : '不限',
      percent: q ? quotaPercent(q.used_vm, q.max_vm) : 0,
      foot: q?.max_vm ? `还可创建 ${Math.max(q.max_vm - q.used_vm, 0)} 台` : '配额不限',
      delay: 120,
    },
    {
      label: '磁盘配额',
      color: '#FB7185',
      bg: 'rgba(251,113,133,.09)',
      border: 'rgba(251,113,133,.25)',
      icon: <DiskIcon size={14} />,
      usedText: `${q?.used_disk ?? 0}`,
      maxText: q?.max_disk ? `/ ${q.max_disk} GB` : '不限',
      percent: q ? quotaPercent(q.used_disk, q.max_disk) : 0,
      foot: q?.max_disk
        ? `使用率 ${Math.round((q.used_disk / q.max_disk) * 100)}%${q.used_disk / q.max_disk >= 0.9 ? ' · 即将耗尽' : ''}`
        : '配额不限',
      footDanger: !!q?.max_disk && q.used_disk / q.max_disk >= 0.9,
      delay: 180,
    },
    {
      label: '本月运行时长',
      color: '#FBBF24',
      bg: 'rgba(251,191,36,.09)',
      border: 'rgba(251,191,36,.2)',
      icon: <ClockIcon size={14} />,
      usedText: `${Math.round(runtimeHours)}`,
      maxText: q?.max_runtime_hours ? `/ ${q.max_runtime_hours} 小时` : '不限',
      percent: q ? quotaPercent(q.used_runtime_seconds, q.max_runtime_hours * 3600) : 0,
      foot: q?.runtime_quota_reached
        ? '时长配额已耗尽'
        : q?.max_runtime_hours
          ? `剩余 ${q.remaining_runtime_display || '-'}`
          : '配额不限',
      footDanger: !!q?.runtime_quota_reached,
      delay: 240,
    },
  ]

  return (
    <section className="qvm-quota-cards">
      {cards.map((card) => (
        <div
          className="qvm-q-card qvm-g-border qvm-fade-up"
          key={card.label}
          style={{ '--qvm-delay': `${card.delay}ms` } as CSSProperties}
        >
          <div className="qvm-q-top">
            <div
              className="qvm-q-ic"
              style={{ background: card.bg, border: `1px solid ${card.border}`, color: card.color }}
            >
              {card.icon}
            </div>
            <span className="qvm-q-label">{card.label}</span>
          </div>
          <div className="qvm-q-val">
            {card.usedText}
            <small>{card.maxText}</small>
          </div>
          {card.percent !== null && (
            <div className="qvm-q-track">
              <div
                className="qvm-q-fill"
                style={{ width: `${card.percent}%`, background: card.footDanger ? 'linear-gradient(90deg,#FB7185,#F43F5E)' : `linear-gradient(90deg,${card.color},${card.color}CC)` }}
              />
            </div>
          )}
          <div className={`qvm-q-foot ${card.footDanger ? 'danger' : ''}`}>{card.foot}</div>
        </div>
      ))}
    </section>
  )
}
