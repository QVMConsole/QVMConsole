/**
 * 模板管理页视图层类型
 */
import type { TemplateItem } from '@/api/template'

/** 视图层模板节点（含前端计算的树结构信息） */
export interface TemplateNodeView extends TemplateItem {
  node_id: string
  /** 派生深度（根节点为 0） */
  depth: number
  has_children: boolean
  is_root: boolean
  /** 从根到当前节点的名称链（用于收起时的链状摘要） */
  chain_labels: string[]
  chain_depth: number
  /** 子树节点总数（含自身） */
  chain_total: number
}

/** 模板族（同一 template_uid 的节点集合） */
export interface TemplateFamily {
  template_uid: string
  type: string
  root_name: string
  node_count: number
  vm_count: number
  disk_size: string
  root_nodes: TemplateNodeView[]
  visible_nodes: TemplateNodeView[]
}

/** 树状引导线类型：垂直线 / 空白 / 中间分支 / 末尾分支 */
export type GuideKind = 'line' | 'blank' | 'elbow' | 'elbow-end'
