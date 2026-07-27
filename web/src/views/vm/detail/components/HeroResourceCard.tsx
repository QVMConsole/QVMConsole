/**
 * Hero 资源卡片（详情页中栏）
 * - 实时资源：CPU / 内存使用率（带渐变进度条）
 * - 网络流量 / 磁盘 IO（IOPS ↔ 吞吐量 双单位切换）
 * - 配置摘要：vCPU / 内存 / 磁盘
 */
import { Tooltip } from '@douyinfe/semi-ui'
import { IconBolt, IconDownload, IconUpload } from '@douyinfe/semi-icons'
import type { VmDetailInfo } from '@/api/vm'
import { formatIOPS, formatTrafficRate, formatMemoryMB } from '../utils'

interface HeroResourceCardProps {
  vm: VmDetailInfo | null
  diskIoMode: 'iops' | 'throughput'
  onToggleDiskIoMode: () => void
}

/** 资源计量条（进度条随数值平滑过渡） */
function Meter({ label, percent, tone }: { label: string; percent: number; tone: string }) {
  const p = Math.max(0, Math.min(100, percent))
  return (
    <div className="qvm-meter">
      <div className="qvm-meter-head">
        <span className="qvm-meter-label">{label}</span>
        <span className="qvm-meter-val qvm-num">{p.toFixed(1)}%</span>
      </div>
      <div className="qvm-meter-track">
        <div className={`qvm-meter-fill ${tone} qvm-bar-anim`} style={{ width: `${p}%` }} />
      </div>
    </div>
  )
}

export default function HeroResourceCard({ vm, diskIoMode, onToggleDiskIoMode }: HeroResourceCardProps) {
  const stats = vm?.stats
  const running = vm?.status === 'running'

  const cpuPercent = running && stats?.cpu_percent != null ? stats.cpu_percent : 0
  const memPercent =
    running && stats?.mem_total ? (stats.mem_used / stats.mem_total) * 100 : 0

  const netRx = running ? formatTrafficRate(stats?.net_rx_rate) : '—'
  const netTx = running ? formatTrafficRate(stats?.net_tx_rate) : '—'
  const diskRd =
    running && diskIoMode === 'iops'
      ? formatIOPS(stats?.disk_rd_iops)
      : running
        ? formatTrafficRate(stats?.disk_rd_rate)
        : '—'
  const diskWr =
    running && diskIoMode === 'iops'
      ? formatIOPS(stats?.disk_wr_iops)
      : running
        ? formatTrafficRate(stats?.disk_wr_rate)
        : '—'

  return (
    <div className="qvm-hero-card qvm-hero-res">
      <div className="qvm-hero-res-meters">
        <Meter label="CPU 使用率" percent={cpuPercent} tone="cyan" />
        <Meter label="内存使用率" percent={memPercent} tone="purple" />
      </div>

      <div className="qvm-hero-res-io">
        <div className="qvm-io-item">
          <span className="qvm-io-label">
            <IconBolt size="small" /> 网络流量
          </span>
          <span className="qvm-io-val qvm-num">
            <IconDownload size="small" className="in" /> {netRx}
            <IconUpload size="small" className="out" /> {netTx}
          </span>
        </div>
        <div className="qvm-io-item">
          <span className="qvm-io-label">
            磁盘 IO
            <Tooltip content={diskIoMode === 'iops' ? '切换为吞吐量' : '切换为 IOPS'} position="top">
              <span className="qvm-io-toggle" onClick={onToggleDiskIoMode}>
                {diskIoMode === 'iops' ? 'IOPS' : 'B/s'}
              </span>
            </Tooltip>
          </span>
          <span className="qvm-io-val qvm-num">
            <IconDownload size="small" className="in" /> {diskRd}
            <IconUpload size="small" className="out" /> {diskWr}
          </span>
        </div>
      </div>

      <div className="qvm-hero-res-config">
        <div className="qvm-config-item">
          <span className="qvm-config-label">CPU 配置</span>
          <span className="qvm-config-val qvm-num">{vm?.vcpu ?? '—'} 核</span>
        </div>
        <div className="qvm-config-item">
          <span className="qvm-config-label">内存配置</span>
          <span className="qvm-config-val qvm-num">{vm ? formatMemoryMB(vm.memory) : '—'}</span>
        </div>
        <div className="qvm-config-item">
          <span className="qvm-config-label">磁盘配置</span>
          <span className="qvm-config-val qvm-num">{vm?.disk_size || '—'}</span>
        </div>
      </div>
    </div>
  )
}
