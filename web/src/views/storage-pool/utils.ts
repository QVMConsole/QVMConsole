/**
 * 存储池页工具函数
 * - 树形块设备扁平化 / 叶子节点收集
 * - 类型文案 / 容量使用率配色 / 可用磁盘过滤
 * - 磁盘分类 / 空间段计算 / 存储卷统计
 */
import type { HostStoragePoolInfo } from '@/api/storagePool'

/** 块设备类型文案 */
export function poolTypeLabel(type?: string): string {
  const map: Record<string, string> = {
    disk: '硬盘',
    part: '分区',
    lvm: 'LVM',
    loop: 'Loop',
    rom: '光驱',
    vg: '卷组',
    lv: '逻辑卷',
    pv: '物理卷',
  }
  return (type && map[type]) || type || '-'
}

/** 容量使用率配色（进度条 / 图表共用） */
export function usageColor(percent = 0): string {
  if (percent >= 90) return '#fb7185'
  if (percent >= 70) return '#f59e0b'
  return '#2dd4bf'
}

/** 带层级的扁平节点 */
export interface FlatNode extends HostStoragePoolInfo {
  depth: number
}

/** 树形 children 扁平化（记录层级深度，用于缩进渲染） */
export function flattenChildren(nodes: HostStoragePoolInfo[], depth = 0): FlatNode[] {
  const result: FlatNode[] = []
  for (const node of nodes) {
    result.push({ ...node, depth })
    if (node.children && node.children.length > 0) {
      result.push(...flattenChildren(node.children, depth + 1))
    }
  }
  return result
}

/** 收集树形结构中的所有叶子节点（用于容量统计/图表） */
export function collectLeafNodes(nodes: HostStoragePoolInfo[]): HostStoragePoolInfo[] {
  const leaves: HostStoragePoolInfo[] = []
  for (const node of nodes) {
    if (!node.children || node.children.length === 0) {
      leaves.push(node)
    } else {
      leaves.push(...collectLeafNodes(node.children))
    }
  }
  return leaves
}

/** VG 卡：是否含 PV 子节点 */
export function hasPVChildren(disk: HostStoragePoolInfo): boolean {
  return (disk.children || []).some((c) => c.type === 'pv')
}

/** VG 卡：是否含 LV 子节点 */
export function hasLVChildren(disk: HostStoragePoolInfo): boolean {
  return (disk.children || []).some((c) => c.type === 'lv')
}

/** 是否为 VG 节点（type=vg 或 LVM VG 合成节点） */
export function isVGNode(disk: HostStoragePoolInfo): boolean {
  return disk.type === 'vg' || !!disk.is_lvm_vg
}

/** 清除磁盘按钮禁用逻辑（与旧版一致） */
export function clearDiskDisabled(disk: HostStoragePoolInfo): boolean {
  const noChildren = !disk.children || disk.children.length === 0
  const noMounts = !disk.mountpoints || disk.mountpoints.length === 0
  return (noChildren && noMounts) || !!disk.system_disk || !!disk.readonly
}

/** 容量概览统计（基于叶子节点 + 物理盘总量） */
export interface PoolOverviewStats {
  totalSize: number // 物理盘总容量
  totalUsed: number // 已挂载节点已用
  totalAvail: number // 已挂载节点可用
  heldSize: number // 未挂载节点占用（旧数据 / PV / 未使用分区）
  unallocated: number // 未分配空间
  diskCount: number
  mountedCount: number
}

export function computeOverviewStats(pools: HostStoragePoolInfo[]): PoolOverviewStats {
  const leaves = collectLeafNodes(pools)
  const mounted = leaves.filter((p) => (p.mountpoints || []).length > 0)
  const unmounted = leaves.filter((p) => (p.mountpoints || []).length === 0)
  const diskTotal = pools
    .filter((d) => d.type === 'disk')
    .reduce((sum, d) => sum + (d.size || 0), 0)
  const leafTotal = leaves.reduce((sum, p) => sum + (p.size || 0), 0)
  const totalSize = diskTotal > 0 ? diskTotal : leafTotal
  const totalUsed = mounted.reduce((sum, p) => sum + (p.used || 0), 0)
  const totalAvail = mounted.reduce((sum, p) => sum + (p.available || 0), 0)
  const heldSize = unmounted.reduce((sum, p) => sum + (p.size || 0), 0)
  return {
    totalSize,
    totalUsed,
    totalAvail,
    heldSize,
    unallocated: Math.max(totalSize - totalUsed - totalAvail - heldSize, 0),
    diskCount: pools.filter((d) => d.type === 'disk').length,
    mountedCount: mounted.length,
  }
}

/** 磁盘分类枚举 */
export type DiskCategory = 'all' | 'pending' | 'inuse' | 'vg' | 'other'

/** 磁盘分类判断 */
export function getDiskCategory(disk: HostStoragePoolInfo): DiskCategory {
  if (disk.type === 'vg' || disk.is_lvm_vg) return 'vg'
  if (disk.configured) return 'inuse'
  if (disk.can_format && !disk.readonly && !disk.removable) return 'pending'
  return 'other'
}

/** VG 节点计数统计 */
export interface VGBriefStats {
  count: number
  vgNames: string[]
}

export function computeVGStats(pools: HostStoragePoolInfo[]): VGBriefStats {
  const vgs = pools.filter((p) => p.type === 'vg' || p.is_lvm_vg)
  return {
    count: vgs.length,
    vgNames: vgs.map((v) => v.display_name),
  }
}
