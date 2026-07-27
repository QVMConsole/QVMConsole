/**
 * 普通用户仪表盘
 * - 资源总览 5 卡 + 配额详情折叠分类 + 我的虚拟机资源追踪
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
  const [quota, setQuota] = useState<QuotaUsage | null>(null)
  const [vms, setVms] = useState<VmListItem[]>([])

  useEffect(() => {
    let mounted = true
    void Promise.all([getSelfQuota(), getSelfVMs()])
      .then(([quotaRes, vmsRes]) => {
        if (!mounted) return
        setQuota(quotaRes.data || null)
        setVms(vmsRes.data || [])
      })
      .catch(() => undefined)
    return () => {
      mounted = false
    }
  }, [])

  const runningCount = vms.filter((v) => v.status === 'running').length
  const diskDanger = !!quota?.max_disk && quota.used_disk / quota.max_disk >= 0.9

  return (
    <>
      <TopLine
        cloudTag={cloudType === CLOUD_TYPES.lightweight ? '轻量云' : '弹性云'}
        subtitle={
          <>
            {runningCount} 台虚拟机运行中
            {diskDanger ? ' · 磁盘配额即将耗尽，请及时清理' : ' · 资源使用正常'}
          </>
        }
        actionText="从模板创建"
      />
      <UserQuotaCards quota={quota} />
      <UserQuotaDetails quota={quota} />
      <UserVmTracker vms={vms} />
    </>
  )
}
