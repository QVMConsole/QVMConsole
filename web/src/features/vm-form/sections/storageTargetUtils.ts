/**
 * 存储位置选项标签工具（创建向导 / 确认摘要 / 挂载弹窗共用）
 */
import { formatBytes } from '@/utils/format'
import type { VmStorageTarget } from '@/api/infra'

/** 存储位置选项标签（含可用容量或默认标记） */
export function storageTargetLabel(target: VmStorageTarget): string {
  const suffix = target.is_default ? '默认' : `${formatBytes(target.available || 0)} 可用`
  return `${target.display_name}（${suffix}）`
}

/** 当前选中的存储位置名称（确认摘要复用） */
export function resolveStorageTargetLabel(targets: VmStorageTarget[], id: string): string {
  const target = targets.find((item) => item.id === id)
  if (!target) return '默认存储位置'
  return target.display_name || target.name || target.id
}
