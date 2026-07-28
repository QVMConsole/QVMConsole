/**
 * 通用格式化工具（容量 / 速率 / 时长）
 */

/** KB 容量格式化（自动换算 GB / TB） */
export function formatKB(kb: number, digits = 1): string {
  if (!Number.isFinite(kb) || kb <= 0) return '0 GB'
  const gb = kb / 1024 / 1024
  if (gb >= 1024) return `${(gb / 1024).toFixed(digits)} TB`
  return `${gb.toFixed(digits)} GB`
}

/** Bytes 容量格式化 */
export function formatBytes(bytes: number, digits = 1): string {
  if (!Number.isFinite(bytes) || bytes <= 0) return '0 GB'
  const gb = bytes / 1024 / 1024 / 1024
  if (gb >= 1024) return `${(gb / 1024).toFixed(digits)} TB`
  if (gb >= 1) return `${gb.toFixed(digits)} GB`
  const mb = bytes / 1024 / 1024
  return `${mb.toFixed(0)} MB`
}

/** MB 容量格式化 */
export function formatMB(mb: number, digits = 1): string {
  if (!Number.isFinite(mb) || mb <= 0) return '0 GB'
  if (mb >= 1024 * 1024) return `${(mb / 1024 / 1024).toFixed(digits)} TB`
  if (mb >= 1024) return `${(mb / 1024).toFixed(digits)} GB`
  return `${mb.toFixed(0)} MB`
}

/** 文件字节数格式化（B / KB / MB / GB / TB，适用于日志等小文件展示） */
export function formatFileSize(bytes: number): string {
  if (!Number.isFinite(bytes) || bytes <= 0) return '0 B'
  const units = ['B', 'KB', 'MB', 'GB', 'TB']
  const i = Math.min(Math.floor(Math.log(bytes) / Math.log(1024)), units.length - 1)
  return `${parseFloat((bytes / 1024 ** i).toFixed(1))} ${units[i]}`
}

/** 解析 "20 GB" / "1.5 TB" 之类的容量文本为 GB 数值 */
export function parseSizeToGB(text: string): number {
  if (!text) return 0
  const m = text.trim().match(/^([\d.]+)\s*(GB|TB|MB|G|T|M)?/i)
  if (!m) return 0
  const val = parseFloat(m[1])
  if (!Number.isFinite(val)) return 0
  const unit = (m[2] || 'GB').toUpperCase()
  if (unit === 'TB' || unit === 'T') return val * 1024
  if (unit === 'MB' || unit === 'M') return val / 1024
  return val
}

/** 连续运行时长格式化（x 天 x 小时 / x 小时 x 分钟 / x 分钟） */
export function formatRuntime(seconds: number): string {
  if (!Number.isFinite(seconds) || seconds <= 0) return '—'
  const days = Math.floor(seconds / 86400)
  const hours = Math.floor((seconds % 86400) / 3600)
  const minutes = Math.floor((seconds % 3600) / 60)
  if (days > 0) return `${days} 天${hours > 0 ? ` ${hours} 小时` : ''}`
  if (hours > 0) return `${hours} 小时${minutes > 0 ? ` ${minutes} 分` : ''}`
  if (minutes > 0) return `${minutes} 分钟`
  return '刚刚'
}

/** 完整日期时间格式化（中文本地格式，无效值原样返回） */
export function formatDateTime(value?: string | null): string {
  if (!value) return '-'
  const d = new Date(value)
  if (Number.isNaN(d.getTime())) return value
  return d.toLocaleString('zh-CN', {
    year: 'numeric',
    month: '2-digit',
    day: '2-digit',
    hour: '2-digit',
    minute: '2-digit',
    second: '2-digit',
    hour12: false,
  })
}

/** 问候语（按小时） */
export function greetingByHour(date = new Date()): string {
  const h = date.getHours()
  if (h < 6) return '夜深了'
  if (h < 12) return '早上好'
  if (h < 14) return '中午好'
  if (h < 18) return '下午好'
  return '晚上好'
}
