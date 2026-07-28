/**
 * 管理员仪表盘
 * - 顶部状态横幅（正常 / 警告）+ 统计卡片（含理论最大量）+ 宿主机资源监控四图 + 最近虚拟机
 * - 宿主机实时数据来自 SSE 推送
 */
import { useEffect, useState } from 'react'
import { useHostStatsSSE } from '@/hooks/useHostStatsSSE'
import { getVmList, type VmListItem } from '@/api/vm'
import TopLine from './components/TopLine'
import HostStatusBanner from './components/HostStatusBanner'
import AdminStats from './components/AdminStats'
import HostMonitorCharts from './components/HostMonitorCharts'
import AdminBottom from './components/AdminBottom'

export default function AdminDashboard() {
  const { stats } = useHostStatsSSE()
  const [vms, setVms] = useState<VmListItem[]>([])

  useEffect(() => {
    let mounted = true
    getVmList()
      .then((res) => {
        if (mounted) setVms(res.data || [])
      })
      .catch(() => undefined)
    return () => {
      mounted = false
    }
  }, [])

  const runningCount = stats?.vm_running ?? vms.filter((v) => v.status === 'running').length
  const totalCount = stats?.vm_total ?? vms.length

  return (
    <>
      <TopLine
        subtitle={
          <>
            系统运行正常 · {runningCount} / {totalCount} 台虚拟机在线
            {stats?.hostname ? ` · ${stats.hostname}` : ''}
            {stats?.arch ? `（${stats.arch}）` : ''}
          </>
        }
      />
      <HostStatusBanner stats={stats} />
      <AdminStats stats={stats} vms={vms} />
      <HostMonitorCharts externalStats={stats} />
      <AdminBottom vms={vms} />
    </>
  )
}
