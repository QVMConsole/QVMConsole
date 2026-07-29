/**
 * 管理员仪表盘：统计卡片区（4 张）
 * - 虚拟机总数 / CPU 使用率 / 内存使用率 / 存储使用
 * - CPU / 内存卡片的「理论最大量」仅统计运行中虚拟机；存储仍统计全部虚拟机配置容量
 * - 系统设置开启 KSM / zRAM 时，在内存使用率卡片统一体现节省与压缩情况
 * - CPU / 内存卡片带「硬件详情」折叠区（默认收起）：CPU 型号与每核使用率色块 / 内存条信息
 */
import { useMemo, useState } from 'react'
import type { HostStats } from '@/api/host'
import type { VmListItem } from '@/api/vm'
import { formatKB, formatMB, parseSizeToGB } from '@/utils/format'
import { useHostMemOptimize } from '@/hooks/useHostMemOptimize'
import DualUsageBar from './DualUsageBar'
import CpuDetailPanel from './CpuDetailPanel'
import MemModulesPanel from './MemModulesPanel'
import StatExpandToggle from './StatExpandToggle'
import { CpuIcon, MemIcon, DiskIcon, VmIcon } from './icons'

interface AdminStatsProps {
  stats: HostStats | null
  vms: VmListItem[]
}

export default function AdminStats({ stats, vms }: AdminStatsProps) {
  const memOptimize = useHostMemOptimize()
  // 硬件详情折叠状态（默认收起；收起即卸载面板，停止每核使用率轮询）
  const [cpuDetailOpen, setCpuDetailOpen] = useState(false)
  const [memDetailOpen, setMemDetailOpen] = useState(false)

  // CPU / 内存理论最大量：仅运行中虚拟机同时 100% 满载时的占用合计。
  // 磁盘容量与运行状态无关，仍统计全部虚拟机。
  const theory = useMemo(() => {
    let vcpuSum = 0
    let memSumMB = 0
    let diskSumGB = 0
    for (const vm of vms) {
      if (vm.status === 'running') {
        vcpuSum += vm.vcpu || 0
        memSumMB += vm.max_memory || vm.memory || 0
      }
      diskSumGB += parseSizeToGB(vm.disk_size)
    }
    return { vcpuSum, memSumMB, diskSumGB }
  }, [vms])

  const runningCount = stats?.vm_running ?? vms.filter((v) => v.status === 'running').length
  const totalCount = stats?.vm_total ?? vms.length

  // CPU：当前 = 宿主机实时使用率；理论 = Σ运行中虚拟机 vCPU / 宿主机核数
  const cpuCount = stats?.cpu_count || 0
  const cpuCurrent = (stats?.cpu_percent || 0) / 100
  const cpuTheory = cpuCount > 0 ? theory.vcpuSum / cpuCount : 0

  // 内存：当前 = 已用 / 总量；理论 = 系统占用 + Σ运行中虚拟机最大内存。
  // 采集器尚未就绪时，使用运行中虚拟机配置内存估算当前分配量，避免首次加载时重复计入虚拟机内存。
  const memTotalMB = (stats?.mem_total || 0) / 1024
  const memUsedMB = (stats?.mem_used || 0) / 1024
  const memCurrent = memTotalMB > 0 ? memUsedMB / memTotalMB : 0
  const vmMemoryActualMB = stats?.vm_memory_known ? (stats.vm_memory_actual || 0) / 1024 : theory.memSumMB
  const systemMemUsedMB = Math.max(memUsedMB - vmMemoryActualMB, 0)
  const memTheoryUsedMB = systemMemUsedMB + theory.memSumMB
  const memTheory = memTotalMB > 0 ? memTheoryUsedMB / memTotalMB : 0

  // KSM 节省内存（KB）：被共享页 × 4KB（与旧前端口径一致），由 SSE 实时刷新
  const ksmSavedKB = (stats?.ksm_pages_sharing || 0) * 4
  const showKsm = memOptimize.ksmEnabled && ksmSavedKB > 0
  const showZram = memOptimize.zramEnabled && memOptimize.zramSizeMB > 0
  // 进度条区段占比（相对宿主机内存总量）
  const memTotalKB = stats?.mem_total || 0
  const zramRatio = showZram && memTotalKB > 0 ? (memOptimize.zramSizeMB * 1024) / memTotalKB : 0
  // 进度条悬停提示扩展行
  const memExtraTips: string[] = []
  if (showKsm) memExtraTips.push(`KSM 去重节省：${formatMB(ksmSavedKB / 1024)}`)
  if (showZram) {
    memExtraTips.push(
      `zRAM 压缩交换：已用 ${formatMB(memOptimize.zramUsedMB)} / ${formatMB(memOptimize.zramSizeMB)}${memOptimize.zramAlgorithm ? `（${memOptimize.zramAlgorithm}）` : ''}`,
    )
  }

  // 磁盘：当前 = 已用 / 总量；理论 = (系统占用 + 虚拟机磁盘配置总和) / 总量
  // 系统占用 = 根分区已用 - 虚拟机实际磁盘占用（vm_disk_actual 由后端缓存提供）
  const diskTotalGB = (stats?.disk_total || 0) / 1024 / 1024
  const diskUsedGB = (stats?.disk_used || 0) / 1024 / 1024
  const vmDiskActualGB = (stats?.vm_disk_actual || 0) / 1024 / 1024
  const systemUsageGB = Math.max(diskUsedGB - vmDiskActualGB, 0)
  const diskCurrent = diskTotalGB > 0 ? diskUsedGB / diskTotalGB : 0
  const diskTheory = diskTotalGB > 0 ? (theory.diskSumGB + systemUsageGB) / diskTotalGB : 0

  return (
    <section className="qvm-stats">
      {/* 虚拟机总数 */}
      <div className="qvm-stat-card qvm-g-border qvm-fade-up">
        <div className="qvm-stat-top">
          <span className="qvm-stat-label">虚拟机总数</span>
          <div
            className="qvm-stat-ic"
            style={{ background: 'rgba(45,212,191,.1)', border: '1px solid rgba(45,212,191,.2)' }}
          >
            <VmIcon color="#2DD4BF" size={16} />
          </div>
        </div>
        <div className="qvm-stat-val">
          {totalCount}
          <small>台</small>
        </div>
        <div className="qvm-stat-foot">
          <span className="qvm-trend-up">运行中 {runningCount}</span> · 已停止 {Math.max(totalCount - runningCount, 0)}
        </div>
      </div>

      {/* CPU 使用率 */}
      <div className="qvm-stat-card qvm-g-border qvm-fade-up" style={{ '--qvm-delay': '60ms' } as React.CSSProperties}>
        <div className="qvm-stat-top">
          <span className="qvm-stat-label">CPU 使用率</span>
          <div
            className="qvm-stat-ic"
            style={{ background: 'rgba(56,189,248,.1)', border: '1px solid rgba(56,189,248,.2)' }}
          >
            <CpuIcon color="#38BDF8" size={16} />
          </div>
        </div>
        <div className="qvm-stat-val">
          {(cpuCurrent * 100).toFixed(0)}
          <small>%</small>
        </div>
        <DualUsageBar
          currentRatio={cpuCurrent}
          theoryRatio={cpuTheory}
          currentText={`${(cpuCurrent * 100).toFixed(1)}%（约 ${(cpuCurrent * cpuCount).toFixed(1)} / ${cpuCount} 核）`}
          theoryText={`${(cpuTheory * 100).toFixed(1)}%（${theory.vcpuSum} / ${cpuCount} 核）`}
          theoryNote="理论最大 = 运行中虚拟机同时满载时的占用"
          color="#38BDF8"
          colorEnd="#2DD4BF"
        />
        {/* 硬件详情：CPU 型号 / 核心数 / 每核使用率色块（展开期间 3s 轮询） */}
        <StatExpandToggle open={cpuDetailOpen} onToggle={() => setCpuDetailOpen((v) => !v)} />
        {cpuDetailOpen && <CpuDetailPanel />}
      </div>

      {/* 内存使用率 */}
      <div className="qvm-stat-card qvm-g-border qvm-fade-up" style={{ '--qvm-delay': '120ms' } as React.CSSProperties}>
        <div className="qvm-stat-top">
          <span className="qvm-stat-label">内存使用率</span>
          <div
            className="qvm-stat-ic"
            style={{ background: 'rgba(139,92,246,.1)', border: '1px solid rgba(139,92,246,.2)' }}
          >
            <MemIcon color="#8B5CF6" size={16} />
          </div>
        </div>
        <div className="qvm-stat-val">
          {(memCurrent * 100).toFixed(0)}
          <small>%</small>
        </div>
        <DualUsageBar
          currentRatio={memCurrent}
          theoryRatio={memTheory}
          currentText={`${(memCurrent * 100).toFixed(1)}%（${formatKB(stats?.mem_used || 0)} / ${formatKB(stats?.mem_total || 0)}）`}
          theoryText={`${(memTheory * 100).toFixed(1)}%（${formatKB(memTheoryUsedMB * 1024)} / ${formatKB(stats?.mem_total || 0)}）`}
          theoryNote={`理论最大 = 系统占用 ${formatKB(systemMemUsedMB * 1024)} + 运行中虚拟机满载 ${formatKB(theory.memSumMB * 1024)}`}
          extraTipLines={memExtraTips}
          zramRatio={zramRatio}
          color="#8B5CF6"
          colorEnd="#C084FC"
          theoryColor="#38BDF8"
        />
        {/* KSM 开启时以文本体现节省量（跟随系统设置） */}
        {showKsm && (
          <div className="qvm-stat-foot qvm-mem-opt">
            <span className="qvm-trend-up">KSM 节省 {formatMB(ksmSavedKB / 1024)}</span>
          </div>
        )}
        {/* 硬件详情：内存条（DIMM）信息（静态，展开时加载一次） */}
        <StatExpandToggle open={memDetailOpen} onToggle={() => setMemDetailOpen((v) => !v)} />
        {memDetailOpen && <MemModulesPanel />}
      </div>

      {/* 存储使用 */}
      <div className="qvm-stat-card qvm-g-border qvm-fade-up" style={{ '--qvm-delay': '180ms' } as React.CSSProperties}>
        <div className="qvm-stat-top">
          <span className="qvm-stat-label">存储使用</span>
          <div
            className="qvm-stat-ic"
            style={{ background: 'rgba(251,191,36,.09)', border: '1px solid rgba(251,191,36,.2)' }}
          >
            <DiskIcon color="#FBBF24" size={16} />
          </div>
        </div>
        <div className="qvm-stat-val">
          {(diskCurrent * 100).toFixed(0)}
          <small>%</small>
        </div>
        <DualUsageBar
          currentRatio={diskCurrent}
          theoryRatio={diskTheory}
          currentText={`${(diskCurrent * 100).toFixed(1)}%（${formatKB(stats?.disk_used || 0)} / ${formatKB(stats?.disk_total || 0)}）`}
          theoryText={`${(diskTheory * 100).toFixed(1)}%（${(theory.diskSumGB + systemUsageGB).toFixed(0)} GB / ${formatKB(stats?.disk_total || 0)}）`}
          theoryNote={`理论最大 = 系统占用 ${systemUsageGB.toFixed(0)} GB + 虚拟机满载 ${theory.diskSumGB.toFixed(0)} GB`}
          color="#FBBF24"
          colorEnd="#F59E0B"
        />
      </div>
    </section>
  )
}
