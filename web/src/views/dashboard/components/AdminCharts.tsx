/**
 * 管理员仪表盘：图表区（资源监控 24h + 实时资源环）
 */
import { useEffect, useState } from 'react'
import type { HostStats, HostStatsRecord } from '@/api/host'
import { getHostStatsHistory } from '@/api/host'
import MonitorChart from './MonitorChart'
import { RingGauge } from './widgets'
import { IconPulse } from '@douyinfe/semi-icons'

interface AdminChartsProps {
  stats: HostStats | null
}

/** 计算 24h 前的时间字符串（后端支持 2006-01-02T15:04:05） */
function dayAgoParam(): { start: string; end: string } {
  const pad = (n: number) => String(n).padStart(2, '0')
  const fmt = (d: Date) =>
    `${d.getFullYear()}-${pad(d.getMonth() + 1)}-${pad(d.getDate())}T${pad(d.getHours())}:${pad(d.getMinutes())}:${pad(d.getSeconds())}`
  const end = new Date()
  const start = new Date(end.getTime() - 24 * 3600 * 1000)
  return { start: fmt(start), end: fmt(end) }
}

export default function AdminCharts({ stats }: AdminChartsProps) {
  const [records, setRecords] = useState<HostStatsRecord[]>([])

  useEffect(() => {
    let mounted = true
    getHostStatsHistory(dayAgoParam())
      .then((res) => {
        if (mounted) setRecords(res.data || [])
      })
      .catch(() => undefined)
    return () => {
      mounted = false
    }
  }, [])

  const cpuPercent = stats?.cpu_percent || 0
  const memPercent = stats && stats.mem_total > 0 ? (stats.mem_used / stats.mem_total) * 100 : 0
  const diskPercent = stats && stats.disk_total > 0 ? (stats.disk_used / stats.disk_total) * 100 : 0
  const healthy = cpuPercent < 85 && memPercent < 90 && diskPercent < 90

  return (
    <section className="qvm-chart-row">
      <div className="qvm-panel-card qvm-g-border qvm-fade-up" style={{ '--qvm-delay': '240ms' } as React.CSSProperties}>
        <div className="qvm-panel-head">
          <span className="qvm-panel-title">资源监控</span>
          <span className="qvm-panel-sub">近 24 小时</span>
          <div className="qvm-legend" style={{ marginLeft: 14 }}>
            <span>
              <i style={{ background: '#2DD4BF' }} />
              CPU
            </span>
            <span>
              <i style={{ background: '#8B5CF6' }} />
              内存
            </span>
          </div>
        </div>
        <MonitorChart records={records} />
      </div>

      <div className="qvm-panel-card qvm-g-border qvm-fade-up" style={{ '--qvm-delay': '300ms' } as React.CSSProperties}>
        <div className="qvm-panel-head">
          <span className="qvm-panel-title">实时资源</span>
          <span className="qvm-panel-sub">秒级刷新</span>
        </div>
        <div className="qvm-rings">
          <div className="qvm-ring">
            <RingGauge percent={cpuPercent} color="#2DD4BF" />
            <div className="qvm-ring-val" style={{ color: '#2DD4BF' }}>
              {cpuPercent.toFixed(0)}%
            </div>
            <div className="qvm-ring-label">CPU</div>
          </div>
          <div className="qvm-ring">
            <RingGauge percent={memPercent} color="#8B5CF6" />
            <div className="qvm-ring-val" style={{ color: '#A78BFA' }}>
              {memPercent.toFixed(0)}%
            </div>
            <div className="qvm-ring-label">内存</div>
          </div>
          <div className="qvm-ring">
            <RingGauge percent={diskPercent} color="#38BDF8" />
            <div className="qvm-ring-val" style={{ color: '#38BDF8' }}>
              {diskPercent.toFixed(0)}%
            </div>
            <div className="qvm-ring-label">磁盘</div>
          </div>
        </div>
        <div className={`qvm-health ${healthy ? '' : 'warn'}`}>
          <IconPulse />
          {healthy ? '宿主机负载均衡，各项指标处于健康区间' : '部分指标偏高，请关注宿主机负载'}
        </div>
      </div>
    </section>
  )
}
