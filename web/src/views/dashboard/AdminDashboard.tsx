/**
 * 管理员仪表盘
 * - 顶部状态横幅（正常 / 警告）+ 统计卡片（含理论最大量）+ 宿主机资源监控四图 + 最近虚拟机
 * - 宿主机实时数据来自 SSE 推送
 */
import { useEffect, useState } from 'react'
import { useHostStatsSSE } from '@/hooks/useHostStatsSSE'
import { getHostDisks, type HostDisk } from '@/api/host'
import { getVmList, type VmListItem } from '@/api/vm'
import { getUserInfo } from '@/api/auth'
import { getPasswordBreachStatus, type PasswordBreachStatus } from '@/api/passwordBreach'
import { useUserStore } from '@/stores/user'
import TopLine from './components/TopLine'
import HostStatusBanner from './components/HostStatusBanner'
import AdminStats from './components/AdminStats'
import HostMonitorCharts from './components/HostMonitorCharts'
import AdminBottom from './components/AdminBottom'

export default function AdminDashboard() {
  const { stats } = useHostStatsSSE()
  const [vms, setVms] = useState<VmListItem[]>([])
  const [disks, setDisks] = useState<HostDisk[]>([])
  const [passwordBreachStatus, setPasswordBreachStatus] = useState<PasswordBreachStatus | null>(null)
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

  useEffect(() => {
    let mounted = true
    const refresh = () => {
      void getHostDisks()
        .then((res) => {
          if (mounted) setDisks(res.data || [])
        })
        .catch(() => undefined)
    }
    refresh()
    const timer = window.setInterval(refresh, 5_000)
    return () => {
      mounted = false
      window.clearInterval(timer)
    }
  }, [])

  useEffect(() => {
    let mounted = true
    const refresh = () => {
      void getPasswordBreachStatus()
        .then((res) => {
          if (mounted) setPasswordBreachStatus(res.data.status)
        })
        .catch(() => undefined)
    }
    refresh()
    const timer = window.setInterval(refresh, 60_000)
    window.addEventListener('focus', refresh)
    return () => {
      mounted = false
      window.clearInterval(timer)
      window.removeEventListener('focus', refresh)
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
      <HostStatusBanner
        stats={stats}
        disks={disks}
        smtpConfigured={security ? security.smtp_configured : undefined}
        passwordBreachStatus={passwordBreachStatus}
      />
      <AdminStats stats={stats} vms={vms} />
      <HostMonitorCharts externalStats={stats} />
      <AdminBottom vms={vms} />
    </>
  )
}
