/**
 * 管理员仪表盘
 * - 顶部状态横幅（正常 / 警告）+ 统计卡片（含理论最大量）+ 宿主机资源监控四图 + 最近虚拟机
 * - 宿主机实时数据来自 SSE 推送
 */
import { useEffect, useState } from 'react'
import { useHostStatsSSE } from '@/hooks/useHostStatsSSE'
import { getVmList, type VmListItem } from '@/api/vm'
import { getUserInfo } from '@/api/auth'
import { useUserStore } from '@/stores/user'
import TopLine from './components/TopLine'
import HostStatusBanner from './components/HostStatusBanner'
import AdminStats from './components/AdminStats'
import HostMonitorCharts from './components/HostMonitorCharts'
import AdminBottom from './components/AdminBottom'

export default function AdminDashboard() {
  const { stats } = useHostStatsSSE()
  const [vms, setVms] = useState<VmListItem[]>([])
  const security = useUserStore((s) => s.security)
  const setSecurity = useUserStore((s) => s.setSecurity)

  useEffect(() => {
    let mounted = true
    getVmList()
      .then((res) => {
        if (mounted) setVms(res.data || [])
      })
      .catch(() => undefined)
    // 刷新安全状态（含 SMTP 配置情况），驱动状态横幅的 SMTP 未配置警告
    getUserInfo()
      .then((res) => {
        if (mounted && res.data?.security) setSecurity(res.data.security)
      })
      .catch(() => undefined)
    return () => {
      mounted = false
    }
    // setSecurity 为 store 稳定引用，仅挂载时刷新一次
    // eslint-disable-next-line react-hooks/exhaustive-deps
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
      <HostStatusBanner stats={stats} smtpConfigured={security ? security.smtp_configured : undefined} />
      <AdminStats stats={stats} vms={vms} />
      <HostMonitorCharts externalStats={stats} />
      <AdminBottom vms={vms} />
    </>
  )
}
