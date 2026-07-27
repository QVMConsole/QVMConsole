/**
 * 虚拟机状态归一化与文案（供仪表盘各组件复用）
 */

/** 状态归一化：运行 / 警告 / 停止 */
export function vmStatusKind(status: string): 'run' | 'warn' | 'stop' {
  const s = (status || '').toLowerCase()
  if (s === 'running') return 'run'
  if (s === 'paused' || s === 'migrating' || s === 'pmsuspended') return 'warn'
  return 'stop'
}

/** 状态中文文案 */
export function vmStatusText(status: string): string {
  const s = (status || '').toLowerCase()
  const map: Record<string, string> = {
    running: '运行中',
    'shut off': '已停止',
    paused: '已暂停',
    migrating: '迁移中',
    pmsuspended: '已挂起',
  }
  return map[s] || status || '未知'
}
