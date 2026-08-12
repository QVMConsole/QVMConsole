/**
 * 用户仪表盘：配额详情（3 个折叠分类）
 * - 计算与实例配额（默认展开）
 * - 存储配额
 * - 网络资源配额（按用户云类型展示弹性云 / 轻量云口径）
 */
import { useState, type CSSProperties, type ReactNode } from 'react'
import { IconChevronDown } from '@douyinfe/semi-icons'
import type { QuotaUsage } from '@/api/vm'
import { useUserStore } from '@/stores/user'
import { CLOUD_TYPES } from '@/config/constants'
import {
  CpuIcon,
  MemIcon,
  VmIcon,
  ClockIcon,
  DiskIcon,
  SnapshotIcon,
  FolderIcon,
  DownloadIcon,
  UploadIcon,
  ChartIcon,
  GlobeIcon,
  LinkIcon,
  NetworkIcon,
} from './icons'

interface UserQuotaDetailsProps {
  quota: QuotaUsage | null
}

interface QuotaRow {
  label: string
  desc?: string
  icon: ReactNode
  color: string
  bg: string
  /** 当前值 / 最大值展示文本 */
  display: string
  /** 0-100，null 表示不显示进度条 */
  percent: number | null
}

interface QuotaCategory {
  key: string
  title: string
  summary: string
  icon: ReactNode
  color: string
  bg: string
  border: string
  rows: QuotaRow[]
  /** 底部来源说明（网络类用） */
  source?: string
}

function percent(used: number, max: number): number | null {
  if (!max || max <= 0) return null
  return Math.min(Math.round((used / max) * 100), 100)
}

function barColor(p: number | null, fallback: string): string {
  if (p === null) return fallback
  if (p >= 90) return 'linear-gradient(90deg,#FB7185,#F43F5E)'
  if (p >= 70) return 'linear-gradient(90deg,#FBBF24,#F59E0B)'
  return fallback
}

export default function UserQuotaDetails({ quota: q }: UserQuotaDetailsProps) {
  const cloudType = useUserStore((s) => s.cloudType)
  const isLightweight = cloudType === CLOUD_TYPES.lightweight
  const [openMap, setOpenMap] = useState<Record<string, boolean>>({ compute: true })

  if (!q) return null

  const toggle = (key: string) => setOpenMap((m) => ({ ...m, [key]: !m[key] }))

  const networkRows: QuotaRow[] = [
    {
      label: '下行带宽',
      desc: isLightweight ? '宿主机网桥直接限速' : '经由 VPC 交换机限速',
      icon: <DownloadIcon size={14} />,
      color: '#38BDF8',
      bg: 'rgba(56,189,248,.1)',
      display: q.max_bandwidth_down ? `${q.max_bandwidth_down} Mbps` : '不限',
      percent: null,
    },
    {
      label: '上行带宽',
      desc: isLightweight ? '宿主机网桥直接限速' : '经由 VPC 交换机限速',
      icon: <UploadIcon size={14} />,
      color: '#2DD4BF',
      bg: 'rgba(45,212,191,.1)',
      display: q.max_bandwidth_up ? `${q.max_bandwidth_up} Mbps` : '不限',
      percent: null,
    },
    {
      label: '下行流量 (日)',
      desc: isLightweight ? '网桥接口统计' : 'VPC 交换机出口统计',
      icon: <ChartIcon size={14} />,
      color: '#FBBF24',
      bg: 'rgba(251,191,36,.09)',
      display: `${q.used_traffic_down_gb || '0'} / ${q.max_traffic_down || '不限'} GB`,
      percent: percent(q.used_traffic_down / 1073741824, q.max_traffic_down),
    },
    {
      label: '上行流量 (日)',
      desc: isLightweight ? '网桥接口统计' : 'VPC 交换机入口统计',
      icon: <ChartIcon size={14} />,
      color: '#8B5CF6',
      bg: 'rgba(139,92,246,.1)',
      display: `${q.used_traffic_up_gb || '0'} / ${q.max_traffic_up || '不限'} GB`,
      percent: percent(q.used_traffic_up / 1073741824, q.max_traffic_up),
    },
	// 公网 IPv4 / IPv6 共用弹性云公网地址配额
    ...(!isLightweight
      ? [
          {
			label: '公网 IP',
			desc: 'IPv4 / IPv6 公网地址',
            icon: <GlobeIcon size={14} />,
            color: '#38BDF8',
            bg: 'rgba(56,189,248,.1)',
            display: `${q.used_public_ips || 0} / ${q.max_public_ips || '不限'}`,
            percent: percent(q.used_public_ips, q.max_public_ips),
          } satisfies QuotaRow,
        ]
      : []),
    {
      label: '端口转发规则',
      desc: q.enable_port_forward ? (isLightweight ? 'iptables DNAT 规则' : 'VPC NAT 网关规则') : '未启用',
      icon: <LinkIcon size={14} />,
      color: '#2DD4BF',
      bg: 'rgba(45,212,191,.1)',
      display: q.enable_port_forward ? `${q.used_port_forwards || 0} / ${q.max_port_forwards || '不限'}` : '未启用',
      percent: q.enable_port_forward ? percent(q.used_port_forwards, q.max_port_forwards) : null,
    },
  ]

  const categories: QuotaCategory[] = [
    {
      key: 'compute',
      title: '计算与实例配额',
      summary: `核心 ${q.used_cpu}/${q.max_cpu || '不限'} · 内存 ${q.used_memory}/${q.max_memory || '不限'} GB · 实例 ${q.used_vm}/${q.max_vm || '不限'}`,
      icon: <CpuIcon size={16} />,
      color: '#2DD4BF',
      bg: 'rgba(45,212,191,.1)',
      border: 'rgba(45,212,191,.2)',
      rows: [
        {
          label: 'CPU 核心',
          desc: '已分配 / 总额度',
          icon: <CpuIcon size={14} />,
          color: '#2DD4BF',
          bg: 'rgba(45,212,191,.1)',
          display: `${q.used_cpu} / ${q.max_cpu || '不限'} 核`,
          percent: percent(q.used_cpu, q.max_cpu),
        },
        {
          label: '内存',
          desc: '已分配 / 总额度',
          icon: <MemIcon size={14} />,
          color: '#8B5CF6',
          bg: 'rgba(139,92,246,.1)',
          display: `${q.used_memory} / ${q.max_memory || '不限'} GB`,
          percent: percent(q.used_memory, q.max_memory),
        },
        {
          label: '虚拟机数量',
          desc: '含已停止实例',
          icon: <VmIcon size={14} />,
          color: '#38BDF8',
          bg: 'rgba(56,189,248,.1)',
          display: `${q.used_vm} / ${q.max_vm || '不限'} 台`,
          percent: percent(q.used_vm, q.max_vm),
        },
        {
          label: '运行时长',
          desc: '本月累计',
          icon: <ClockIcon size={14} />,
          color: '#FBBF24',
          bg: 'rgba(251,191,36,.09)',
          display: `${q.used_runtime_display || '0秒'} / ${q.max_runtime_hours ? `${q.max_runtime_hours} 小时` : '不限'}`,
          percent: percent(q.used_runtime_seconds, q.max_runtime_hours * 3600),
        },
      ],
    },
    {
      key: 'storage',
      title: '存储配额',
      summary: `磁盘 ${q.used_disk}/${q.max_disk || '不限'} GB · 快照 ${q.used_snapshots}/${q.max_snapshots || '不限'} · 存储空间 ${q.used_storage_gb || '0'}/${q.max_storage || '不限'} GB`,
      icon: <DiskIcon size={16} />,
      color: '#FB7185',
      bg: 'rgba(251,113,133,.09)',
      border: 'rgba(251,113,133,.25)',
      rows: [
        {
          label: '磁盘容量',
          desc: '虚拟机磁盘总和',
          icon: <DiskIcon size={14} />,
          color: '#FB7185',
          bg: 'rgba(251,113,133,.09)',
          display: `${q.used_disk} / ${q.max_disk || '不限'} GB`,
          percent: percent(q.used_disk, q.max_disk),
        },
        {
          label: '快照数量',
          desc: '全部虚拟机合计',
          icon: <SnapshotIcon size={14} />,
          color: '#38BDF8',
          bg: 'rgba(56,189,248,.1)',
          display: `${q.used_snapshots} / ${q.max_snapshots || '不限'}`,
          percent: percent(q.used_snapshots, q.max_snapshots),
        },
        {
          label: '存储空间',
          desc: '我的存储项目配额',
          icon: <FolderIcon size={14} />,
          color: '#8B5CF6',
          bg: 'rgba(139,92,246,.1)',
          display: `${q.used_storage_gb || '0'} / ${q.max_storage || '不限'} GB`,
          percent: percent(parseFloat(q.used_storage_gb) || 0, q.max_storage),
        },
      ],
    },
    {
      key: 'network',
      title: '网络资源配额',
      summary: isLightweight
        ? `带宽 ${q.max_bandwidth_down ? `${q.max_bandwidth_down} Mbps` : '不限'} · 流量 ${q.used_traffic_down_gb || '0'}/${q.max_traffic_down || '不限'} GB`
        : `带宽 ${q.max_bandwidth_down ? `${q.max_bandwidth_down} Mbps` : '不限'} · 公网 IP ${q.used_public_ips}/${q.max_public_ips || '不限'} · 端口转发 ${q.used_port_forwards}/${q.max_port_forwards || '不限'}`,
      icon: <NetworkIcon size={16} />,
      color: '#38BDF8',
      bg: 'rgba(56,189,248,.1)',
      border: 'rgba(56,189,248,.2)',
      rows: networkRows,
      source: isLightweight ? '流量来源：宿主机网桥' : '流量来源：VPC 虚拟交换机',
    },
  ]

  return (
    <>
      <div className="qvm-section-title">配额详情</div>
      {categories.map((cat, idx) => {
        const open = !!openMap[cat.key]
        return (
          <div
            className="qvm-panel-card qvm-qcat qvm-g-border qvm-fade-up"
            key={cat.key}
            style={{ '--qvm-delay': `${300 + idx * 60}ms` } as CSSProperties}
          >
            <div className="qvm-qcat-head" onClick={() => toggle(cat.key)}>
              <div
                className="qvm-qcat-ic"
                style={{ background: cat.bg, border: `1px solid ${cat.border}`, color: cat.color }}
              >
                {cat.icon}
              </div>
              <span className="qvm-qcat-title">{cat.title}</span>
              <span className="qvm-qcat-summary">{cat.summary}</span>
              <span className={`qvm-qcat-chevron ${open ? 'open' : ''}`}>
                <IconChevronDown />
              </span>
            </div>
            <div className={`qvm-qcat-body ${open ? 'open' : ''}`}>
              {cat.rows.map((row) => (
                <div className="qvm-qrow" key={row.label}>
                  <div className="qvm-qrow-ic" style={{ background: row.bg, color: row.color }}>
                    {row.icon}
                  </div>
                  <div className="qvm-qrow-info">
                    <div className="qvm-qrow-label">{row.label}</div>
                    {row.desc && <div className="qvm-qrow-desc">{row.desc}</div>}
                  </div>
                  {row.percent !== null ? (
                    <div className="qvm-qrow-track">
                      <div
                        className="qvm-qrow-fill"
                        style={{ width: `${row.percent}%`, background: barColor(row.percent, `linear-gradient(90deg,${row.color},${row.color}CC)`) }}
                      />
                    </div>
                  ) : (
                    <div className="qvm-qrow-nobar" />
                  )}
                  <div className="qvm-qrow-val">{row.display}</div>
                </div>
              ))}
              {cat.source && (
                <div className="qvm-qrow" style={{ borderBottom: 'none', paddingBottom: 0 }}>
                  <span style={{ fontSize: 11, color: 'var(--qvm-text-2)', display: 'flex', alignItems: 'center', gap: 7 }}>
                    <span className="qvm-dot" style={{ background: '#38BDF8', boxShadow: '0 0 6px rgba(56,189,248,.7)' }} />
                    {cat.source}
                  </span>
                </div>
              )}
            </div>
          </div>
        )
      })}
    </>
  )
}
