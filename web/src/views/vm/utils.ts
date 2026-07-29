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

/** 判断点击目标是否属于列表中的选择或行内操作控件。 */
export function shouldOpenVmDetail(target: EventTarget | null): boolean {
  if (!(target instanceof Element)) return true
  return !target.closest('.semi-checkbox, .qvm-act-cell, .qvm-tag-editor')
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

/** 分组维度：'' 为不分组（全部平铺） */
export type VmGroupBy = '' | 'status' | 'template' | 'custom'

/** 分组桶（分组视图渲染单元） */
export interface VmGroupBucket {
  key: string
  label: string
  /** Semi Tag 颜色 */
  color: 'green' | 'orange' | 'grey' | 'red' | 'blue' | 'violet'
  vms: VmListItem[]
}

/** 按状态分组的固定定义（排序权重与标签颜色） */
const STATUS_GROUP_DEFS: Array<Omit<VmGroupBucket, 'vms'> & { order: number }> = [
  { key: 'running', label: '运行中', order: 1, color: 'green' },
  { key: 'paused', label: '已暂停', order: 2, color: 'orange' },
  { key: 'shut off', label: '已关机', order: 3, color: 'grey' },
  { key: 'migrating', label: '迁移中', order: 4, color: 'red' },
]

/** 构建分组列表（按状态 / 按模板 / 自定义分组），空组自动过滤 */
export function buildVmGroups(groupBy: VmGroupBy, vms: VmListItem[]): VmGroupBucket[] {
  if (!groupBy) return []

  if (groupBy === 'status') {
    const grouped = new Map<string, VmGroupBucket & { order: number }>()
    STATUS_GROUP_DEFS.forEach((d) => grouped.set(d.key, { ...d, vms: [] }))
    vms.forEach((vm) => {
      const key = vm.status || 'shut off'
      let bucket = grouped.get(key)
      if (!bucket) {
        bucket = { key, label: vmStatusText(key), order: 99, color: 'grey', vms: [] }
        grouped.set(key, bucket)
      }
      bucket.vms.push(vm)
    })
    return [...grouped.values()].filter((g) => g.vms.length > 0).sort((a, b) => a.order - b.order)
  }

  if (groupBy === 'template') {
    const grouped = new Map<string, VmGroupBucket>()
    vms.forEach((vm) => {
      const key = vm.template || '__none__'
      let bucket = grouped.get(key)
      if (!bucket) {
        bucket = { key, label: vm.template || '无模板', color: 'blue', vms: [] }
        grouped.set(key, bucket)
      }
      bucket.vms.push(vm)
    })
    return [...grouped.values()].sort((a, b) => a.label.localeCompare(b.label))
  }

  // 自定义分组：未分组排在最后
  const grouped = new Map<string, VmGroupBucket>()
  vms.forEach((vm) => {
    const key = vm.group || '__ungrouped__'
    let bucket = grouped.get(key)
    if (!bucket) {
      bucket = { key, label: vm.group || '未分组', color: vm.group ? 'violet' : 'grey', vms: [] }
      grouped.set(key, bucket)
    }
    bucket.vms.push(vm)
  })
  return [...grouped.values()].sort((a, b) => {
    if (a.key === '__ungrouped__') return 1
    if (b.key === '__ungrouped__') return -1
    return a.label.localeCompare(b.label)
  })
}

/** 电源操作文案 */
export const POWER_ACTION_TEXT: Record<string, string> = {
  start: '开机',
  shutdown: '关机',
  reboot: '重启',
  destroy: '强制断电',
  reset: '重置',
}
