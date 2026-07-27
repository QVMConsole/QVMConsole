/**
 * 用户仪表盘：我的虚拟机 · 资源追踪
 * - 折叠面板展示每台虚拟机概况
 * - 展开时按需加载 24h 监控历史，渲染 CPU/内存/网络/磁盘 IO 迷你图
 */
import { useState, type CSSProperties } from 'react'
import { Toast } from '@douyinfe/semi-ui'
import { IconChevronDown, IconArrowRight } from '@douyinfe/semi-icons'
import type { VmListItem, VmStatsRecord } from '@/api/vm'
import { getVmStatsHistory } from '@/api/vm'
import { formatRuntime } from '@/utils/format'
import { StatusPill, Sparkline } from './widgets'
import { vmStatusKind } from './vmStatus'
import { VmIcon } from './icons'

interface UserVmTrackerProps {
  vms: VmListItem[]
}

/** 计算 24h 查询参数 */
function historyParam(): { start: string; end: string } {
  const pad = (n: number) => String(n).padStart(2, '0')
  const fmt = (d: Date) =>
    `${d.getFullYear()}-${pad(d.getMonth() + 1)}-${pad(d.getDate())}T${pad(d.getHours())}:${pad(d.getMinutes())}:${pad(d.getSeconds())}`
  const end = new Date()
  return { start: fmt(new Date(end.getTime() - 24 * 3600 * 1000)), end: fmt(end) }
}

/** 由累计字节数差分计算速率序列（B/s） */
function diffRate(records: VmStatsRecord[], pick: (r: VmStatsRecord) => number): number[] {
  const rates: number[] = []
  for (let i = 1; i < records.length; i++) {
    const dt = (new Date(records[i].recorded_at).getTime() - new Date(records[i - 1].recorded_at).getTime()) / 1000
    if (dt <= 0) continue
    const dv = pick(records[i]) - pick(records[i - 1])
    rates.push(Math.max(dv / dt, 0))
  }
  return rates
}

/** 速率文本（B/s 自动换算） */
function rateText(bytesPerSec: number): string {
  if (bytesPerSec >= 1024 * 1024) return `${(bytesPerSec / 1024 / 1024).toFixed(1)} MB/s`
  if (bytesPerSec >= 1024) return `${(bytesPerSec / 1024).toFixed(1)} KB/s`
  return `${bytesPerSec.toFixed(0)} B/s`
}

/** 单台虚拟机面板 */
function VmPanel({ vm, defaultOpen }: { vm: VmListItem; defaultOpen: boolean }) {
  const [open, setOpen] = useState(defaultOpen)
  const [records, setRecords] = useState<VmStatsRecord[] | null>(null)
  const [loading, setLoading] = useState(false)

  const toggle = async () => {
    const next = !open
    setOpen(next)
    // 展开时按需加载历史（仅一次）
    if (next && records === null && !loading) {
      setLoading(true)
      try {
        const res = await getVmStatsHistory(vm.name, historyParam())
        setRecords(res.data || [])
      } catch {
        setRecords([])
      } finally {
        setLoading(false)
      }
    }
  }

  const running = vmStatusKind(vm.status) === 'run'
  const memGB = Math.round((vm.max_memory || vm.memory || 0) / 1024)

  // 图表数据
  const cpuData = (records || []).map((r) => r.cpu_percent)
  const memData = (records || []).map((r) => (r.mem_total > 0 ? (r.mem_used / r.mem_total) * 100 : 0))
  const netRates = diffRate(records || [], (r) => r.net_rx_bytes + r.net_tx_bytes)
  const ioRates = diffRate(records || [], (r) => r.disk_rd_ops + r.disk_wr_ops)
  const latest = (arr: number[]) => (arr.length > 0 ? arr[arr.length - 1] : 0)

  return (
    <div className="qvm-panel-card qvm-vm-panel qvm-g-border">
      <div className="qvm-vm-head" onClick={toggle}>
        <div className="qvm-vm-name">
          <div className={`qvm-vm-ic ${running ? '' : 'off'}`}>
            <VmIcon size={14} />
          </div>
          {vm.name}
        </div>
        <StatusPill status={vm.status} />
        <div className="qvm-vm-meta">
          <span>{vm.vcpu} 核</span>
          <span>{memGB} GB</span>
          <span>{vm.ip || '未分配 IP'}</span>
          {running && vm.continuous_runtime_seconds > 0 && (
            <span>已运行 {formatRuntime(vm.continuous_runtime_seconds)}</span>
          )}
        </div>
        <span className={`qvm-vm-chevron ${open ? 'open' : ''}`}>
          <IconChevronDown />
        </span>
      </div>
      <div className={`qvm-vm-body ${open ? 'open' : ''}`}>
        {loading ? (
          <div className="qvm-vm-chart-grid">
            {[0, 1, 2, 3].map((i) => (
              <div className="qvm-skel" key={i} style={{ height: 104 }} />
            ))}
          </div>
        ) : !running ? (
          <div className="qvm-empty-text">虚拟机未运行，暂无实时监控数据</div>
        ) : (
          <div className="qvm-vm-chart-grid">
            <div className="qvm-mini-chart">
              <div className="qvm-mc-title">
                CPU 使用率 · 近 24 小时<b style={{ color: '#2DD4BF' }}>{latest(cpuData).toFixed(0)}%</b>
              </div>
              <Sparkline data={cpuData} color="#2DD4BF" max={100} />
            </div>
            <div className="qvm-mini-chart">
              <div className="qvm-mc-title">
                内存使用率 · 近 24 小时<b style={{ color: '#8B5CF6' }}>{latest(memData).toFixed(0)}%</b>
              </div>
              <Sparkline data={memData} color="#8B5CF6" max={100} />
            </div>
            <div className="qvm-mini-chart">
              <div className="qvm-mc-title">
                网络吞吐 · 近 24 小时<b style={{ color: '#38BDF8' }}>{rateText(latest(netRates))}</b>
              </div>
              <Sparkline data={netRates} color="#38BDF8" />
            </div>
            <div className="qvm-mini-chart">
              <div className="qvm-mc-title">
                磁盘 IO · 近 24 小时<b style={{ color: '#FBBF24' }}>{latest(ioRates).toFixed(0)} IOPS</b>
              </div>
              <Sparkline data={ioRates} color="#FBBF24" />
            </div>
          </div>
        )}
        <div className="qvm-vm-foot">
          <span
            className="qvm-lnk-btn"
            onClick={() => Toast.info({ content: '虚拟机详情页将在后续迭代提供', duration: 2 })}
          >
            前往虚拟机详情
            <IconArrowRight size="small" />
          </span>
        </div>
      </div>
    </div>
  )
}

export default function UserVmTracker({ vms }: UserVmTrackerProps) {
  return (
    <>
      <div className="qvm-section-title">
        我的虚拟机 · 资源追踪
        <div className="qvm-section-actions">
          <span
            className="qvm-lnk-btn"
            onClick={() => Toast.info({ content: '虚拟机列表页将在后续迭代提供', duration: 2 })}
          >
            管理我的虚拟机
          </span>
        </div>
      </div>
      {vms.length === 0 ? (
        <div className="qvm-panel-card qvm-g-border">
          <div className="qvm-empty-text">暂无虚拟机，可联系管理员开通或从模板创建</div>
        </div>
      ) : (
        vms.map((vm, idx) => (
          <div
            className="qvm-fade-up"
            key={vm.name}
            style={{ '--qvm-delay': `${idx * 60}ms` } as CSSProperties}
          >
            <VmPanel vm={vm} defaultOpen={idx === 0 && vm.status === 'running'} />
          </div>
        ))
      )}
    </>
  )
}
