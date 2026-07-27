/**
 * 管理员仪表盘
 * - 统计卡片（含理论最大量）+ 资源监控 + 实时资源 + 最近虚拟机 + 存储池用量
 * - 宿主机实时数据来自 SSE 推送
 */
import { useEffect, useState } from 'react'
import { useHostStatsSSE } from '@/hooks/useHostStatsSSE'
import { getVmList, type VmListItem } from '@/api/vm'
import TopLine from './components/TopLine'
import AdminStats from './components/AdminStats'
import AdminCharts from './components/AdminCharts'
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
        actionText="新建虚拟机"
        showSearch
      />
      <AdminStats stats={stats} vms={vms} />
      <AdminCharts stats={stats} />
      <AdminBottom vms={vms} />
    </>
  )
}
