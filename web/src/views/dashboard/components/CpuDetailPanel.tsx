/**
 * 概览页 CPU 卡片展开区：CPU 型号 / 插槽 / 物理核心 / 线程 + 每核使用率色块网格
 * - 每核一个小色块按使用率着色，悬停 Tooltip 显示「核心 N：xx%」
 * - 展开挂载期间每 3 秒轮询一次接口实时刷新，收起（卸载）后自动停止
 */
import { useEffect, useState } from 'react'
import { Tooltip } from '@douyinfe/semi-ui'
import { getHostCpuHardware, type HostCpuHardware } from '@/api/host'

/** 展开期间的轮询间隔（ms） */
const REFRESH_INTERVAL_MS = 3000

/** 按使用率区间取色块颜色（低青 / 中蓝 / 高琥珀 / 危红） */
function coreColor(usage: number): string {
  if (usage >= 90) return 'rgba(248, 113, 113, .88)'
  if (usage >= 70) return 'rgba(251, 191, 36, .85)'
  if (usage >= 40) return 'rgba(56, 189, 248, .8)'
  if (usage >= 10) return 'rgba(45, 212, 191, .7)'
  return 'rgba(45, 212, 191, .28)'
}

export default function CpuDetailPanel() {
  const [hw, setHw] = useState<HostCpuHardware | null>(null)
  const [loading, setLoading] = useState(true)

  useEffect(() => {
    let mounted = true

    const refresh = () => {
      void getHostCpuHardware()
        .then((res) => {
          if (mounted) {
            setHw(res.data || null)
            setLoading(false)
          }
        })
        .catch(() => {
          if (mounted) setLoading(false)
        })
    }

    refresh()
    const timer = setInterval(refresh, REFRESH_INTERVAL_MS)
    return () => {
      mounted = false
      clearInterval(timer)
    }
  }, [])

  if (loading && !hw) {
    return <div className="qvm-stat-expand qvm-hw-loading">正在读取 CPU 信息…</div>
  }
  if (!hw) {
    return <div className="qvm-stat-expand qvm-hw-loading">CPU 信息暂不可用</div>
  }

  const cores = hw.per_core_usage || []

  return (
    <div className="qvm-stat-expand">
      <div className="qvm-hw-model" title={hw.model}>
        {hw.model}
      </div>
      <div className="qvm-hw-meta">
        插槽 {hw.sockets} · 物理核心 {hw.cores} · 线程 {hw.threads}
      </div>
      {cores.length > 0 && (
        <div className="qvm-core-grid">
          {cores.map((usage, i) => (
            <Tooltip key={i} content={`核心 ${i}：${usage.toFixed(1)}%`} position="top">
              <span className="qvm-core-cell" style={{ background: coreColor(usage) }} />
            </Tooltip>
          ))}
        </div>
      )}
    </div>
  )
}
