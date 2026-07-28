/**
 * 普通用户仪表盘
 * - 弹性云：资源总览 5 卡 + 配额详情折叠分类 + 我的虚拟机资源追踪
 * - 轻量云：无用户级配额（为单 VM 配额），仅展示虚拟机资源追踪
 */
import { useEffect, useState } from 'react'
import { getSelfQuota, getSelfVMs, type QuotaUsage, type VmListItem } from '@/api/vm'
import { useUserStore } from '@/stores/user'
import { CLOUD_TYPES } from '@/config/constants'
import TopLine from './components/TopLine'
import UserQuotaCards from './components/UserQuotaCards'
import UserQuotaDetails from './components/UserQuotaDetails'
import UserVmTracker from './components/UserVmTracker'

export default function UserDashboard() {
  const cloudType = useUserStore((s) => s.cloudType)
  const isLightweight = cloudType === CLOUD_TYPES.lightweight
  const [quota, setQuota] = useState<QuotaUsage | null>(null)
  const [vms, setVms] = useState<VmListItem[]>([])

  useEffect(() => {
    let mounted = true
    // 轻量云无用户级配额，不请求配额接口
    void Promise.all([isLightweight ? Promise.resolve(null) : getSelfQuota(), getSelfVMs()])
      .then(([quotaRes, vmsRes]) => {
        if (!mounted) return
        setQuota(quotaRes?.data || null)
        setVms(vmsRes.data || [])
      })
      .catch(() => undefined)
    return () => {
      mounted = false
    }
  }, [isLightweight])

  const runningCount = vms.filter((v) => v.status === 'running').length
  const diskDanger =
    !isLightweight && !!quota?.max_disk && quota.used_disk / quota.max_disk >= 0.9

  return (
    <>
      <TopLine
        cloudTag={isLightweight ? '轻量云' : '弹性云'}
        subtitle={
          <>
            {runningCount} 台虚拟机运行中
            {diskDanger ? ' · 磁盘配额即将耗尽，请及时清理' : ' · 资源使用正常'}
          </>
        }
      />
      {!isLightweight && (
        <>
          <UserQuotaCards quota={quota} />
          <UserQuotaDetails quota={quota} />
        </>
      )}
      <UserVmTracker vms={vms} />
    </>
  )
}
