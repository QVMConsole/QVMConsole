/**
 * 虚拟机列表页共享工具（状态文案 / 排序权重 / 容量解析）
 */
import type { VmListItem } from '@/api/vm'

/** 虚拟机状态文案映射 */
export function vmStatusText(status: string): string {
  const map: Record<string, string> = {
    running: '运行中',
    'shut off': '已关机',
    paused: '已暂停',
    migrating: '迁移中',
  }
  return map[status] || status || '未知'
}

/** 状态分类（图标着色用） */
export function vmStatusKind(status: string): 'run' | 'stop' | 'warn' | 'move' {
  if (status === 'running') return 'run'
  if (status === 'paused') return 'warn'
  if (status === 'migrating') return 'move'
  return 'stop'
}

/** 是否迁移中（迁移中禁止一切操作） */
export function isVmMigrating(vm?: VmListItem | null): boolean {
  return vm?.status === 'migrating'
}

/** 内存 MB → 可读文本 */
export function formatMemoryMB(memory: number): string {
  if (!Number.isFinite(memory) || memory <= 0) return '-'
  return memory >= 1024 ? `${(memory / 1024).toFixed(1)} GB` : `${memory} MB`
}

/** 内存 MB → 紧凑 G 文本（配置列用） */
export function formatMemoryGB(memory: number): string {
  if (!Number.isFinite(memory) || memory <= 0) return '-'
  const gb = memory / 1024
  return `${Number.isInteger(gb) ? gb : gb.toFixed(1)}G`
}

/** 配置摘要文本：4C / 8G / 100G */
export function vmConfigText(vm: VmListItem): string {
  return `${vm.vcpu}C / ${formatMemoryGB(vm.memory)} / ${vm.disk_size || '-'}`
}

/** 解析 "20 GB" 之类的磁盘容量文本为 GB 整数 */
export function parseDiskSizeGB(value?: string): number {
  const text = `${value || ''}`.trim()
  const matched = text.match(/([\d.]+)\s*GB/i)
  if (!matched) return 0
  const parsed = Number.parseFloat(matched[1])
  if (!Number.isFinite(parsed) || parsed <= 0) return 0
  return Math.ceil(parsed)
}

/** 解析模板虚拟磁盘大小（"20 GiB" / "20 GB"）为 GB 整数 */
export function resolveTemplateMinDiskSize(template?: { virtual_size?: string } | null): number {
  if (!template) return 0
  const text = `${template.virtual_size || ''}`.trim()
  const gibMatch = text.match(/([\d.]+)\s*GiB/i)
  if (gibMatch) {
    const parsed = Number.parseFloat(gibMatch[1])
    return Number.isFinite(parsed) && parsed > 0 ? Math.ceil(parsed) : 0
  }
  const gbMatch = text.match(/([\d.]+)\s*GB/i)
  if (gbMatch) {
    const parsed = Number.parseFloat(gbMatch[1])
    return Number.isFinite(parsed) && parsed > 0 ? Math.ceil(parsed) : 0
  }
  return 0
}

/** 电源操作文案 */
export const POWER_ACTION_TEXT: Record<string, string> = {
  start: '开机',
  shutdown: '关机',
  reboot: '重启',
  destroy: '强制断电',
  reset: '重置',
}
