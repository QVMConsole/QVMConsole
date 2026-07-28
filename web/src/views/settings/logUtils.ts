/**
 * 日志管理辅助逻辑：日志类型标签配色
 */
import type { TagColor } from '@douyinfe/semi-ui/lib/es/tag'

/** 日志类型标签配色（app/request/cmd/libvirt） */
export function categoryTagColor(category: string): TagColor {
  const map: Record<string, TagColor> = {
    app: 'blue',
    request: 'green',
    cmd: 'orange',
    libvirt: 'red',
  }
  return map[category] || 'grey'
}
