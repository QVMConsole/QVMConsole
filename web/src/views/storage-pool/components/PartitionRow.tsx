/**
 * 分区/逻辑卷行
 * - 自绘 capbar（细条进度条 + 容量文字）
 * - 徽章格式统一为 .sp-badge（系统分区/存储池/LVM/只读/存在数据等）
 * - 行内操作：配置图标外露，「设为默认/格式化挂载」收 ⋯ 下拉
 * - VM 占用展示：显示该分区上的虚拟机列表及磁盘占用
 */
import { Dropdown, Tooltip } from '@douyinfe/semi-ui'
import { IconChevronRight, IconEditStroked, IconMore, IconStarStroked, IconWrenchStroked } from '@douyinfe/semi-icons'
import type { HostStoragePoolInfo } from '@/api/storagePool'
import { formatBytes } from '@/utils/format'
import { usageColor, type FlatNode } from '../utils'
import VMUsageSection from './VMUsageSection'

/** 分区行操作回调 */
export interface PartitionRowHandlers {
  onConfig: (node: HostStoragePoolInfo) => void
  onSetDefault: (node: HostStoragePoolInfo) => void
  onFormat: (node: HostStoragePoolInfo) => void
}

interface PartitionRowProps {
  node: FlatNode
  handlers: PartitionRowHandlers
}

/** 徽章类型映射 */
const BADGE_MAP: Record<string, { text: string; cls: string }> = {
  system: { text: '系统分区', cls: 'system' },
  default: { text: '默认', cls: 'default' },
  enabled: { text: '已启用', cls: 'enabled' },
  vg: { text: 'LVM 卷组', cls: 'vg' },
  readonly: { text: '只读', cls: 'readonly' },
  held: { text: '存在数据', cls: 'pending' },
  lvm: { text: 'LVM', cls: 'purple' },
  pv: { text: 'PV', cls: 'purple' },
}

/** 分区徽章组件 */
function PartitionBadges({ node }: { node: HostStoragePoolInfo }) {
  const badges: Array<{ key: string; text: string; cls: string }> = []

  // 系统分区
  if (node.system_disk) badges.push({ key: 'system', ...BADGE_MAP.system })

  // 默认存储
  if (node.is_default) badges.push({ key: 'default', ...BADGE_MAP.default })

  // 已启用
  if (node.enabled) badges.push({ key: 'enabled', ...BADGE_MAP.enabled })

  // LVM / PV 标记
  if (node.type === 'lvm') badges.push({ key: 'lvm', ...BADGE_MAP.lvm })
  if (node.type === 'pv') badges.push({ key: 'pv', ...BADGE_MAP.pv })

  // 只读
  if (node.readonly) badges.push({ key: 'readonly', ...BADGE_MAP.readonly })

  // 存在数据（未挂载）
  if (node.has_existing_data) badges.push({ key: 'held', ...BADGE_MAP.held })

  return (
    <>
      {badges.map((badge) => (
        <span key={badge.key} className={`sp-badge ${badge.cls}`}>
          {badge.text}
        </span>
      ))}
    </>
  )
}

/** 容量文字颜色判断 */
function getCapTextColor(usePercent = 0): string {
  if (usePercent >= 90) return 'var(--qvm-danger-ink)'
  if (usePercent >= 70) return 'var(--qvm-warn-ink)'
  return 'var(--qvm-acc-ink)'
}

export default function PartitionRow({ node, handlers }: PartitionRowProps) {
  const isPV = node.type === 'pv'
  const isSystemPart = node.system_disk || node.type === 'rom'
  const isStoragePool = node.configured && node.can_use_for_vm
  const hasData = node.has_existing_data

  return (
    <div className={`sp-part-row ${node.depth > 0 ? `depth-${node.depth}` : ''}`}>
      <div className="sp-part-main">
        <div className="sp-part-name-row">
          {node.depth > 0 && <IconChevronRight />}
          <span className="sp-part-name">{node.display_name}</span>
          <PartitionBadges node={node} />
        </div>
        <div className="sp-part-meta">
          <span className="sp-mono">{node.device_path}</span>
          {!isPV && (
            <>
              <span className="sp-meta-sep">·</span>
              <span>{node.fstype || '未知文件系统'}</span>
            </>
          )}
          {(node.mountpoints || []).length > 0 && (
            <>
              <span className="sp-meta-sep">·</span>
              <span className="sp-mono">{(node.mountpoints || []).join(', ')}</span>
            </>
          )}
          {isPV && node.size > 0 && (
            <>
              <span className="sp-meta-sep">·</span>
              <span>{formatBytes(node.size)}</span>
            </>
          )}
        </div>
      </div>

      {/* 容量条：系统分区/未挂载盘/存储池共用细条 */}
      {!isPV && node.size > 0 && (
        <div className="sp-part-cap">
          <div className="sp-capbar">
            <i
              style={{
                width: `${node.use_percent || 0}%`,
                background: isSystemPart ? 'rgba(139, 151, 173, 0.5)' : usageColor(node.use_percent),
              }}
            />
          </div>
          <div className="sp-cap-text">
            {isStoragePool ? (
              <>
                <b style={{ color: getCapTextColor(node.use_percent) }}>{node.use_percent || 0}%</b>
                <span>{formatBytes(node.used || 0)} / {formatBytes(node.size)}</span>
              </>
            ) : hasData ? (
              <span>{formatBytes(node.size)} · 未挂载</span>
            ) : isSystemPart ? (
              <span>{formatBytes(node.size)} · 只读</span>
            ) : (
              <span>{formatBytes(node.size)}</span>
            )}
          </div>
        </div>
      )}

      {/* VM 占用展示：存储池分区，或存在 VM 磁盘的已挂载分区均显示 */}
      {(isStoragePool || (node.vm_usage_list?.length ?? 0) > 0) && <VMUsageSection node={node} />}

      {/* 行内操作：PV 无操作 */}
      {!isPV && (
        <div className="sp-act-cell">
          <Tooltip content="配置" position="top">
            <span className="sp-part-act-ic" onClick={() => handlers.onConfig(node)}>
              <IconEditStroked />
            </span>
          </Tooltip>
          <Dropdown
            trigger="click"
            position="bottomRight"
            clickToHide
            render={
              <Dropdown.Menu>
                <Dropdown.Item
                  icon={<IconStarStroked />}
                  disabled={!node.can_use_for_vm || node.is_default}
                  onClick={() => handlers.onSetDefault(node)}
                >
                  设为默认
                </Dropdown.Item>
                <Dropdown.Item
                  icon={<IconWrenchStroked />}
                  disabled={!node.can_format}
                  onClick={() => handlers.onFormat(node)}
                >
                  格式化挂载
                </Dropdown.Item>
              </Dropdown.Menu>
            }
          >
            <span className="sp-part-act-ic">
              <IconMore />
            </span>
          </Dropdown>
        </div>
      )}
    </div>
  )
}
