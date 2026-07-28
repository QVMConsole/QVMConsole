/**
 * 概览页顶部宿主机状态横幅
 * - 正常：各项资源指标处于健康区间
 * - 警告：CPU 使用率 ≥ 90% / 内存使用率 ≥ 90% / 存储剩余 < 10G 时触发，并列出具体原因
 * - 数据来自宿主机 SSE 实时推送，状态随推送自动切换
 */
import { IconTickCircle, IconAlertTriangle } from '@douyinfe/semi-icons'
import type { HostStats } from '@/api/host'
import { formatKB } from '@/utils/format'

/** CPU 使用率警告阈值（%） */
const CPU_WARN_PERCENT = 90
/** 内存使用率警告阈值（%） */
const MEM_WARN_PERCENT = 90
/** 存储剩余警告阈值（KB，10 GB） */
const DISK_FREE_WARN_KB = 10 * 1024 * 1024

interface HostStatusBannerProps {
  stats: HostStats | null
}

export default function HostStatusBanner({ stats }: HostStatusBannerProps) {
  // 首屏数据未到达时不渲染，避免状态闪烁
  if (!stats) return null

  const cpuPercent = stats.cpu_percent || 0
  const memPercent = stats.mem_total > 0 ? (stats.mem_used / stats.mem_total) * 100 : 0
  const diskFreeKB = stats.disk_free || 0

  // 汇总警告原因
  const reasons: string[] = []
  if (cpuPercent >= CPU_WARN_PERCENT) {
    reasons.push(`CPU 使用率已达 ${cpuPercent.toFixed(0)}%`)
  }
  if (memPercent >= MEM_WARN_PERCENT) {
    reasons.push(`内存使用率已达 ${memPercent.toFixed(0)}%`)
  }
  if (diskFreeKB > 0 && diskFreeKB < DISK_FREE_WARN_KB) {
    reasons.push(`存储剩余仅 ${formatKB(diskFreeKB)}（不足 10 GB）`)
  }

  const isWarn = reasons.length > 0

  return (
    <div className={`qvm-status-banner qvm-fade-up ${isWarn ? 'warn' : ''}`}>
      {isWarn ? <IconAlertTriangle /> : <IconTickCircle />}
      {isWarn ? (
        <span>
          <b>警告：</b>
          {reasons.join('，')}，请及时关注宿主机负载
        </span>
      ) : (
        <span>宿主机运行正常，CPU / 内存 / 存储各项指标处于健康区间</span>
      )}
    </div>
  )
}
