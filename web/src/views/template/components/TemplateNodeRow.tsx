/**
 * 模板节点行：树状引导线 + 展开箭头 + 层级色条 + 节点信息 + 行内操作
 * 迁移自旧前端 views/template/index.vue 的节点行
 */
import { memo } from 'react'
import { Dropdown, Tag, Tooltip } from '@douyinfe/semi-ui'
import {
  IconBranch,
  IconDelete,
  IconDeleteStroked,
  IconDownload,
  IconExport,
  IconMore,
  IconRefresh,
  IconSetting,
  IconWrench,
} from '@douyinfe/semi-icons'
import { templateCategoryLabel, templateTypeLabel, normalizeTemplateType } from '@/utils/templateCategory'
import { hashStatusColor, hashStatusText, linuxInitStatusColor, linuxInitStatusText } from '../utils'
import type { GuideKind, TemplateNodeView } from '../types'

/** 节点行操作回调（由模板页主入口统一注入） */
export interface TemplateNodeHandlers {
  onToggle: (node: TemplateNodeView) => void
  onExport: (node: TemplateNodeView, scope: 'root' | 'node') => void
  onDownloadExport: (node: TemplateNodeView) => void
  onDeleteExport: (node: TemplateNodeView) => void
  onOpenPublish: (node: TemplateNodeView) => void
  onPrepareLinux: (node: TemplateNodeView) => void
  onOpenDelete: (node: TemplateNodeView) => void
}

interface TemplateNodeRowProps {
  node: TemplateNodeView
  guides: GuideKind[]
  expanded: boolean
  exportingName: string
  deletingExportName: string
  preparingLinuxName: string
  handlers: TemplateNodeHandlers
}

/** OS 类型标签颜色 */
const OS_TAG_COLOR: Record<string, 'green' | 'blue' | 'red' | 'teal' | 'orange'> = {
  linux: 'green',
  windows: 'blue',
  fnos: 'red',
  openwrt: 'teal',
  other: 'orange',
}

function TemplateNodeRow({
  node,
  guides,
  expanded,
  exportingName,
  deletingExportName,
  preparingLinuxName,
  handlers,
}: TemplateNodeRowProps) {
  const osType = normalizeTemplateType(node.type)
  const categoryLabel = templateCategoryLabel(node.type || '', node.category)
  // 该行存在进行中的导出/删除导出包/预处理任务时，⋯ 图标显示为加载态
  const rowBusy =
    exportingName === `${node.name}:root` ||
    exportingName === `${node.name}:node` ||
    deletingExportName === node.name ||
    preparingLinuxName === node.name
  const visibilityTag = node.disabled ? (
    <Tag color="red" size="small">已禁用</Tag>
  ) : node.clone_visible ? (
    <Tag color="green" size="small">用户可见</Tag>
  ) : (
    <Tag color="orange" size="small">仅管理员</Tag>
  )

  const handleRowClick = () => {
    if (node.has_children) handlers.onToggle(node)
  }

  return (
    <li className={`tpl-node-item${node.disabled ? ' disabled' : ''}`}>
      {/* eslint-disable-next-line jsx-a11y/click-events-have-key-events, jsx-a11y/no-static-element-interactions */}
      <div className="tpl-node-row" onClick={handleRowClick}>
        {/* 树状引导线 */}
        {guides.length > 0 && (
          <div className="tpl-guides">
            {guides.map((kind, i) => (
              <span key={i} className={`tpl-guide tpl-guide-${kind}`} />
            ))}
          </div>
        )}
        {/* 展开/收起箭头 */}
        <span
          className={`tpl-toggle${node.has_children ? '' : ' no-children'}${expanded ? '' : ' collapsed'}`}
          onClick={(e) => {
            e.stopPropagation()
            handlers.onToggle(node)
          }}
        >
          ▼
        </span>
        {/* 层级色条 */}
        <span className={`tpl-level-bar l${node.depth % 5}`} />

        <div className="tpl-node-content">
          <div className="tpl-node-identity">
            <div className="tpl-node-name" title={node.admin_name || node.name}>
              {node.admin_name || node.name}
            </div>
            <div className="tpl-node-file">{node.name}.qcow2</div>
          </div>

          <Tag color={OS_TAG_COLOR[osType] || 'orange'} size="small">
            {templateTypeLabel(node.type)}
          </Tag>
          {categoryLabel && <Tag size="small">{categoryLabel}</Tag>}
          {visibilityTag}

          <Tooltip content="直接关联 / 子树关联 VM 数">
            <div className="tpl-node-vm-stat">
              <span className="vm-count">{node.tree_vm_count || node.direct_vm_count || 0}</span>
              <span className="vm-label">VM</span>
            </div>
          </Tooltip>

          <div className="tpl-node-size" title="虚拟大小 / 实际占用">
            <div className="size-value">{node.virtual_size || '-'}</div>
            <div className="size-label">{node.actual_size || '-'}</div>
          </div>

          {node.exported ? (
            <Tag color="blue" size="small">已导出</Tag>
          ) : (
            <Tag color="grey" size="small">未导出</Tag>
          )}
          <Tag color={hashStatusColor(node.hash_status)} size="small">
            {hashStatusText(node.hash_status)}
          </Tag>
          {node.type === 'linux' && (
            <Tag color={linuxInitStatusColor(node.linux_init_status)} size="small">
              {linuxInitStatusText(node.linux_init_status)}
            </Tag>
          )}
        </div>

        {/* 行内操作：发布设置图标 + ⋯ 下拉菜单（防误触，文字写到菜单里） */}
        <div className="tpl-act-cell" onClick={(e) => e.stopPropagation()}>
          <Tooltip content="发布设置" position="top">
            <span className="tpl-act-ic" onClick={() => handlers.onOpenPublish(node)}>
              <IconSetting />
            </span>
          </Tooltip>
          <Dropdown
            trigger="click"
            position="bottomRight"
            clickToHide
            render={
              <Dropdown.Menu>
                {node.is_root && (
                  <Dropdown.Item
                    icon={<IconBranch />}
                    onClick={() => handlers.onExport(node, 'root')}
                  >
                    导出整树
                  </Dropdown.Item>
                )}
                <Dropdown.Item icon={<IconExport />} onClick={() => handlers.onExport(node, 'node')}>
                  导出节点
                </Dropdown.Item>
                {node.exported && (
                  <Dropdown.Item
                    icon={<IconDownload />}
                    onClick={() => handlers.onDownloadExport(node)}
                  >
                    下载导出包
                  </Dropdown.Item>
                )}
                {node.exported && (
                  <Dropdown.Item
                    icon={<IconDeleteStroked />}
                    type="warning"
                    onClick={() => handlers.onDeleteExport(node)}
                  >
                    删除导出包
                  </Dropdown.Item>
                )}
                {node.type === 'linux' && (
                  <Dropdown.Item
                    icon={<IconWrench />}
                    onClick={() => handlers.onPrepareLinux(node)}
                  >
                    离线预处理
                  </Dropdown.Item>
                )}
                <Dropdown.Divider />
                <Dropdown.Item
                  icon={<IconDelete />}
                  type="danger"
                  onClick={() => handlers.onOpenDelete(node)}
                >
                  删除模板链路
                </Dropdown.Item>
              </Dropdown.Menu>
            }
          >
            <span className="tpl-act-ic more">
              {rowBusy ? <IconRefresh spin /> : <IconMore />}
            </span>
          </Dropdown>
        </div>
      </div>

      {/* 收起时的派生链摘要 */}
      {node.has_children && !expanded && (
        // eslint-disable-next-line jsx-a11y/click-events-have-key-events, jsx-a11y/no-static-element-interactions
        <div className="tpl-chain-summary" onClick={() => handlers.onToggle(node)}>
          📌 收起 · 最新派生链：
          {node.chain_labels.map((label, idx) => (
            <span key={idx}>
              {idx > 0 && <span className="chain-arrow">→</span>}
              <span className={idx === node.chain_labels.length - 1 ? 'chain-leaf' : ''}>
                {label}
              </span>
            </span>
          ))}
          <span className="chain-depth">
            (深{node.chain_depth || node.depth + 1}层 / 共{node.chain_total || 1}节点)
          </span>
        </div>
      )}
    </li>
  )
}

export default memo(TemplateNodeRow)
