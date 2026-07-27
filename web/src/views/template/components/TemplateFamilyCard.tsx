/**
 * 模板族卡片：族头（图标/名称/统计） + 节点树列表
 * 迁移自旧前端 views/template/index.vue 的 family-card
 */
import { memo } from 'react'
import { IconDesktop, IconLayers, IconServer } from '@douyinfe/semi-icons'
import { computeGuideFlags, familyTypeClass, familyTypeEmoji } from '../utils'
import type { TemplateFamily, TemplateNodeView } from '../types'
import TemplateNodeRow, { type TemplateNodeHandlers } from './TemplateNodeRow'

interface TemplateFamilyCardProps {
  family: TemplateFamily
  byNodeId: Map<string, TemplateNodeView>
  childrenMap: Map<string, TemplateNodeView[]>
  expandState: Record<string, boolean>
  exportingName: string
  deletingExportName: string
  preparingLinuxName: string
  handlers: TemplateNodeHandlers
}

function TemplateFamilyCard({
  family,
  byNodeId,
  childrenMap,
  expandState,
  exportingName,
  deletingExportName,
  preparingLinuxName,
  handlers,
}: TemplateFamilyCardProps) {
  return (
    <section className="tpl-family-card qvm-fade-up">
      <header className="tpl-family-header">
        <div className="tpl-family-id">
          <div className={`tpl-family-icon ${familyTypeClass(family.type)}`}>
            <span>{familyTypeEmoji(family.type)}</span>
          </div>
          <div>
            <div className="tpl-family-title">{family.root_name || family.template_uid}</div>
            <div className="tpl-family-meta">模板族 {family.template_uid}</div>
          </div>
        </div>
        <div className="tpl-family-stats">
          <span className="tpl-family-stat">
            <IconLayers />
            <strong>{family.node_count}</strong> 节点
          </span>
          <span className="tpl-family-stat">
            <IconDesktop />
            <strong>{family.vm_count}</strong> 关联 VM
          </span>
          <span className="tpl-family-stat">
            <IconServer />
            <strong>{family.disk_size}</strong> 磁盘
          </span>
        </div>
      </header>

      <ul className="tpl-node-list">
        {family.visible_nodes.map((node) => (
          <TemplateNodeRow
            key={node.node_id}
            node={node}
            guides={computeGuideFlags(node, byNodeId, childrenMap)}
            expanded={!!expandState[node.node_id]}
            exportingName={exportingName}
            deletingExportName={deletingExportName}
            preparingLinuxName={preparingLinuxName}
            handlers={handlers}
          />
        ))}
      </ul>
    </section>
  )
}

export default memo(TemplateFamilyCard)
