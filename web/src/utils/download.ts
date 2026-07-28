/**
 * 浏览器端文件下载工具
 * 用于日志导出 / 诊断导出等 blob 下载场景
 */

/** 触发浏览器下载 blob 内容 */
export function downloadBlob(blob: Blob, filename: string) {
  const url = URL.createObjectURL(blob)
  const a = document.createElement('a')
  a.href = url
  a.download = filename
  document.body.appendChild(a)
  a.click()
  document.body.removeChild(a)
  URL.revokeObjectURL(url)
}

/** 生成带时间戳的导出文件名（如 qvmconsole-logs-2026-07-28T10-00-00.zip） */
export function timestampFilename(prefix: string, ext: string): string {
  const dateStr = new Date().toISOString().replace(/[:.]/g, '-').slice(0, 19)
  return `${prefix}-${dateStr}.${ext}`
}
