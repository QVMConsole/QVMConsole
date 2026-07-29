/**
 * 概览页内存卡片展开区：内存条（DIMM）信息列表
 * - 静态硬件信息，展开时加载一次即可（后端亦有进程内缓存）
 * - dmidecode 不可用 / SMBIOS 无数据时展示后端返回的中文说明
 */
import { useEffect, useState } from 'react'
import { getHostMemoryModules, type HostMemoryModulesInfo } from '@/api/host'
import { formatMB } from '@/utils/format'

export default function MemModulesPanel() {
  const [info, setInfo] = useState<HostMemoryModulesInfo | null>(null)
  const [loading, setLoading] = useState(true)

  useEffect(() => {
    let mounted = true
    void getHostMemoryModules()
      .then((res) => {
        if (mounted) {
          setInfo(res.data || null)
          setLoading(false)
        }
      })
      .catch(() => {
        if (mounted) setLoading(false)
      })
    return () => {
      mounted = false
    }
  }, [])

  if (loading && !info) {
    return <div className="qvm-stat-expand qvm-hw-loading">正在读取内存条信息…</div>
  }
  if (!info || info.modules.length === 0) {
    return (
      <div className="qvm-stat-expand qvm-hw-loading">
        {info?.message || '内存条信息暂不可用'}
      </div>
    )
  }

  return (
    <div className="qvm-stat-expand">
      <div className="qvm-hw-meta">
        已插 {info.installed} / {info.total_slots} 插槽
      </div>
      <div className="qvm-dimm-list">
        {info.modules.map((m, i) => (
          <div key={i} className="qvm-dimm-row">
            <span className="qvm-dimm-slot" title={m.slot}>
              {m.slot || `插槽 ${i + 1}`}
            </span>
            <span className="qvm-dimm-size">{formatMB(m.size_mb)}</span>
            <span className="qvm-dimm-meta" title={[m.manufacturer, m.part_number].filter(Boolean).join(' ')}>
              {[m.type, m.configured_speed || m.speed, m.manufacturer].filter(Boolean).join(' · ')}
            </span>
          </div>
        ))}
      </div>
    </div>
  )
}
