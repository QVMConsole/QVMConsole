/**
 * 概览页顶部宿主机状态横幅
 * - 正常：各项资源指标处于健康区间
 * - 警告：CPU 使用率 ≥ 90% / 内存使用率 ≥ 90% / 存储剩余 < 10G 时触发，并列出具体原因；
 *         任一可写挂载点可用空间不足 10 GiB 时同样告警（排除 /boot 与 /boot/efi）；
 *         系统未配置 SMTP 时同样触发警告，提示前往系统设置配置
 * - 数据来自宿主机 SSE 实时推送，状态随推送自动切换；SMTP 状态来自当前用户安全状态
 */
import { IconTickCircle, IconAlertTriangle } from '@douyinfe/semi-icons'
import type { HostDisk, HostStats } from '@/api/host'
import type { PasswordBreachStatus } from '@/api/passwordBreach'
import { formatKB } from '@/utils/format'

/** CPU 使用率警告阈值（%） */
const CPU_WARN_PERCENT = 90
/** 内存使用率警告阈值（%） */
const MEM_WARN_PERCENT = 90
/** 存储剩余警告阈值（KB，10 GiB） */
const DISK_FREE_WARN_KB = 10 * 1024 * 1024
const HEALTH_EXCLUDED_MOUNT_POINTS = new Set(['/boot', '/boot/efi'])

interface HostStatusBannerProps {
  stats: HostStats | null
  /** 宿主机挂载盘，轮询获取以检测单个文件系统空间 */
  disks?: HostDisk[]
  /** 系统是否已配置 SMTP（undefined 表示未知，不触发警告） */
  smtpConfigured?: boolean
  passwordBreachStatus?: PasswordBreachStatus | null
}

export default function HostStatusBanner({ stats, disks = [], smtpConfigured, passwordBreachStatus }: HostStatusBannerProps) {
  // 首屏数据未到达时不渲染，避免状态闪烁
  if (!stats) return null

  const cpuPercent = stats.cpu_percent || 0
  const memPercent = stats.mem_total > 0 ? (stats.mem_used / stats.mem_total) * 100 : 0
  const diskFreeKB = stats.disk_free || 0

  // 汇总资源负载警告原因
  const loadReasons: string[] = []
  if (cpuPercent >= CPU_WARN_PERCENT) {
    loadReasons.push(`CPU 使用率已达 ${cpuPercent.toFixed(0)}%`)
  }
  if (memPercent >= MEM_WARN_PERCENT) {
    loadReasons.push(`内存使用率已达 ${memPercent.toFixed(0)}%`)
  }
  if (diskFreeKB > 0 && diskFreeKB < DISK_FREE_WARN_KB) {
    loadReasons.push(`存储剩余仅 ${formatKB(diskFreeKB)}（不足 10 GB）`)
  }
  const warnedDevices = new Set<string>()
  for (const disk of disks) {
    const mountPoint = disk.mount_point.replace(/\/+$/, '') || '/'
    if (
      disk.read_only ||
      HEALTH_EXCLUDED_MOUNT_POINTS.has(mountPoint) ||
      disk.free_kb >= DISK_FREE_WARN_KB ||
      warnedDevices.has(disk.device)
    ) {
      continue
    }
    warnedDevices.add(disk.device)
    loadReasons.push(`${disk.device}（${mountPoint}）可用空间仅 ${formatKB(disk.free_kb)}（不足 10 GB）`)
  }

  // 汇总警告消息（负载类原因合并为一句，SMTP 未配置单独一句）
  const messages: string[] = []
  if (loadReasons.length > 0) {
    messages.push(`${loadReasons.join('，')}，请及时关注宿主机负载`)
  }
  if (smtpConfigured === false) {
    messages.push('您当前没有设置 SMTP，请尽快前往系统设置进行设置')
  }

  const breachReasons: string[] = []
  if ((passwordBreachStatus?.breached_total || 0) > 0) {
    const accounts = (passwordBreachStatus?.affected_accounts || []).slice(0, 3).map((item) => item.username)
    const more = (passwordBreachStatus?.breached_total || 0) - accounts.length
    breachReasons.push(
      `检测到 ${passwordBreachStatus?.breached_total} 个账户密码泄露（管理员 ${passwordBreachStatus?.breached_admins}，普通用户 ${passwordBreachStatus?.breached_users}）：${accounts.join('、')}${more > 0 ? ` 等 ${more + accounts.length} 个账户` : ''}`,
    )
  }

  const isError = breachReasons.length > 0
  const isWarn = !isError && messages.length > 0

  return (
    <div className={`qvm-status-banner qvm-fade-up ${isError ? 'error' : isWarn ? 'warn' : ''}`}>
      {isError || isWarn ? <IconAlertTriangle /> : <IconTickCircle />}
      {isError ? (
        <span>
          <b>异常：</b>
          {[...breachReasons, ...messages].join('；')}
        </span>
      ) : isWarn ? (
        <span>
          <b>警告：</b>
          {messages.join('；')}
        </span>
      ) : (
        <span>宿主机运行正常，CPU / 内存 / 存储各项指标处于健康区间</span>
      )}
    </div>
  )
}
