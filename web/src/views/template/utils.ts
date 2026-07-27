/**
 * 模板管理页工具函数：模板族树构建、可见节点计算、状态标签映射
 * 迁移自旧前端 views/template/index.vue 中的相关逻辑
 */
import type { TemplateItem } from '@/api/template'
import { normalizeTemplateType } from '@/utils/templateCategory'
import { formatBytes } from '@/utils/format'
import type { GuideKind, TemplateFamily, TemplateNodeView } from './types'

/** 解析 "20 GiB" / "1.5 GB" 等容量文本为字节数 */
export function parseSizeToBytes(sizeStr?: string): number {
  if (!sizeStr) return 0
  const num = parseFloat(sizeStr)
  if (Number.isNaN(num)) return 0
  if (/GiB/i.test(sizeStr) || /GB/i.test(sizeStr)) return num * 1024 * 1024 * 1024
  if (/MiB/i.test(sizeStr) || /MB/i.test(sizeStr)) return num * 1024 * 1024
  if (/KiB/i.test(sizeStr) || /KB/i.test(sizeStr)) return num * 1024
  return num * 1024 * 1024 * 1024
}

/** 节点排序键 */
const nodeSortKey = (n: TemplateItem) => n.admin_name || n.name || ''

/** 统计子树节点总数（含自身） */
function countSubtree(nodeId: string, childrenMap: Map<string, TemplateNodeView[]>): number {
  let count = 1
  const children = childrenMap.get(nodeId)
  if (children) {
    children.forEach((child) => {
      count += countSubtree(child.node_id, childrenMap)
    })
  }
  return count
}

/** 从根到当前节点的名称链 */
function getChainLabels(
  node: TemplateNodeView,
  byNodeId: Map<string, TemplateNodeView>,
): string[] {
  const labels: string[] = []
  let current: TemplateNodeView | undefined = node
  while (current) {
    labels.unshift(current.admin_name || current.name)
    if (current.parent_node_id && byNodeId.has(current.parent_node_id)) {
      current = byNodeId.get(current.parent_node_id)
    } else {
      break
    }
  }
  return labels
}

export interface TemplateTreeData {
  families: TemplateFamily[]
  byNodeId: Map<string, TemplateNodeView>
  /** 父节点 → 已排序子节点列表 */
  childrenMap: Map<string, TemplateNodeView[]>
}

/**
 * 将扁平模板列表构建为模板族树。
 * - 按 parent_node_id 建立父子关系（父节点缺失时视为根）
 * - 按 template_uid 聚合为模板族卡片
 */
export function buildTemplateTree(items: TemplateItem[]): TemplateTreeData {
  const nodes: TemplateNodeView[] = (items || [])
    .filter((n) => n.node_id)
    .map((n) => ({
      ...n,
      node_id: n.node_id as string,
      depth: 0,
      has_children: false,
      is_root: false,
      chain_labels: [],
      chain_depth: 1,
      chain_total: 1,
    }))

  const byNodeId = new Map<string, TemplateNodeView>()
  nodes.forEach((n) => byNodeId.set(n.node_id, n))

  // 父子关系映射（子节点按名称排序）
  const childrenMap = new Map<string, TemplateNodeView[]>()
  nodes.forEach((n) => {
    const pid = n.parent_node_id || ''
    if (pid && byNodeId.has(pid)) {
      const list = childrenMap.get(pid) || []
      list.push(n)
      childrenMap.set(pid, list)
    }
  })
  childrenMap.forEach((list) => list.sort((a, b) => nodeSortKey(a).localeCompare(nodeSortKey(b))))

  // 深度计算（带记忆化）
  const depthMap = new Map<string, number>()
  const calcDepth = (nodeId: string): number => {
    const cached = depthMap.get(nodeId)
    if (cached !== undefined) return cached
    const node = byNodeId.get(nodeId)
    if (!node || !node.parent_node_id || !byNodeId.has(node.parent_node_id)) {
      depthMap.set(nodeId, 0)
      return 0
    }
    const depth = calcDepth(node.parent_node_id) + 1
    depthMap.set(nodeId, depth)
    return depth
  }

  nodes.forEach((n) => {
    n.depth = calcDepth(n.node_id)
    n.has_children = (childrenMap.get(n.node_id)?.length || 0) > 0
    n.is_root = !n.parent_node_id || !byNodeId.has(n.parent_node_id)
    n.chain_labels = getChainLabels(n, byNodeId)
    n.chain_depth = n.chain_labels.length
    n.chain_total = countSubtree(n.node_id, childrenMap)
  })

  // 按模板族聚合
  const familyMap = new Map<string, TemplateFamily>()
  nodes.forEach((n) => {
    const uid = n.template_uid || n.node_id
    let fam = familyMap.get(uid)
    if (!fam) {
      fam = {
        template_uid: uid,
        type: n.type || 'other',
        root_name: '',
        node_count: 0,
        vm_count: 0,
        disk_size: '-',
        root_nodes: [],
        visible_nodes: [],
      }
      familyMap.set(uid, fam)
    }
    if (n.is_root) {
      fam.root_nodes.push(n)
      if (!fam.root_name) fam.root_name = n.admin_name || n.name
    }
    if (!fam.type) fam.type = n.type || 'other'
  })

  familyMap.forEach((fam) => {
    // 族统计：节点数 / 关联 VM 总数 / 磁盘总量
    const uidNodeIds = nodes.filter((n) => (n.template_uid || n.node_id) === fam.template_uid)
    fam.node_count = uidNodeIds.length
    fam.vm_count = uidNodeIds.reduce((sum, n) => sum + (n.tree_vm_count || n.direct_vm_count || 0), 0)
    const totalBytes = uidNodeIds.reduce((sum, n) => {
      const bytes = n.virtual_size ? parseSizeToBytes(n.virtual_size) : parseSizeToBytes(n.actual_size)
      return sum + bytes
    }, 0)
    if (totalBytes > 0) fam.disk_size = formatBytes(totalBytes)
    fam.root_nodes.sort((a, b) => nodeSortKey(a).localeCompare(nodeSortKey(b)))
  })

  const families = Array.from(familyMap.values()).sort((a, b) =>
    a.root_name.localeCompare(b.root_name),
  )
  return { families, byNodeId, childrenMap }
}

/** 计算族内当前可见节点（按展开状态深度优先展开） */
export function computeVisibleNodes(
  family: TemplateFamily,
  childrenMap: Map<string, TemplateNodeView[]>,
  expandState: Record<string, boolean>,
): TemplateNodeView[] {
  const visible: TemplateNodeView[] = []
  const walk = (list: TemplateNodeView[]) => {
    list.forEach((n) => {
      visible.push(n)
      if (!expandState[n.node_id]) return
      const children = childrenMap.get(n.node_id)
      if (children && children.length > 0) walk(children)
    })
  }
  walk(family.root_nodes)
  return visible
}

/**
 * 计算节点的树状引导线（每层一段）。
 * - 非末层：路径上的祖先若为其父的末子 → 空白，否则 → 垂直线
 * - 末层：当前节点为父的末子 → 末尾分支，否则 → 中间分支
 */
export function computeGuideFlags(
  node: TemplateNodeView,
  byNodeId: Map<string, TemplateNodeView>,
  childrenMap: Map<string, TemplateNodeView[]>,
): GuideKind[] {
  const depth = node.depth || 0
  if (depth === 0) return []

  // 祖先链（含自身）：[root, ..., parent, node]
  const chain: TemplateNodeView[] = [node]
  let current: TemplateNodeView | undefined = node
  while (current && current.parent_node_id && byNodeId.has(current.parent_node_id)) {
    current = byNodeId.get(current.parent_node_id)
    if (current) chain.unshift(current)
  }

  const isLastAmongSiblings = (target: TemplateNodeView): boolean => {
    const siblings = target.parent_node_id ? childrenMap.get(target.parent_node_id) : undefined
    if (!siblings || siblings.length === 0) return true
    return siblings[siblings.length - 1].node_id === target.node_id
  }

  const guides: GuideKind[] = []
  for (let i = 0; i < depth; i++) {
    if (i === depth - 1) {
      guides.push(isLastAmongSiblings(node) ? 'elbow-end' : 'elbow')
    } else {
      // i+1 层祖先是否为其父的末子
      const ancestor = chain[i + 1]
      guides.push(ancestor && isLastAmongSiblings(ancestor) ? 'blank' : 'line')
    }
  }
  return guides
}

// ==================== 状态标签映射 ====================

/** 哈希校验状态 */
export function hashStatusText(status?: string): string {
  const map: Record<string, string> = { ok: '已记录', missing: '缺失', size_mismatch: '大小变化' }
  return (status && map[status]) || '未知'
}

export function hashStatusColor(status?: string): 'green' | 'orange' | 'red' | 'grey' {
  const map: Record<string, 'green' | 'orange' | 'red'> = {
    ok: 'green',
    missing: 'orange',
    size_mismatch: 'red',
  }
  return (status && map[status]) || 'grey'
}

/** Linux 离线预处理状态 */
export function linuxInitStatusText(status?: string): string {
  const map: Record<string, string> = { ready: '离线就绪', failed: '预处理失败', unknown: '待预处理' }
  return (status && map[status]) || '待预处理'
}

export function linuxInitStatusColor(status?: string): 'green' | 'red' | 'orange' {
  const map: Record<string, 'green' | 'red' | 'orange'> = { ready: 'green', failed: 'red' }
  return (status && map[status]) || 'orange'
}

/** 模板族类型图标（emoji） */
export function familyTypeEmoji(type?: string): string {
  const map: Record<string, string> = { windows: '🪟', fnos: '📦', other: '💾' }
  return map[normalizeTemplateType(type)] || '🐧'
}

/** 模板族类型样式类名（用于图标底色） */
export function familyTypeClass(type?: string): string {
  return normalizeTemplateType(type) || 'other'
}
