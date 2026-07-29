/**
 * 磁盘/卷组卡片
 * - 头部：类型图标（hdd/ssd/nvme/vg/usb）、名称与徽章、元信息、行内操作
 * - 空间分配条：分段显示 sys/used/held/free，含图例
 * - 待初始化盘：显示初始化引导区（格式化/分区操作按钮）
 * - 历史数据警告：卡片顶部展示警告 Banner
 */
import { Banner, Button, Dropdown, Tooltip } from '@douyinfe/semi-ui'
import {
  IconBox,
  IconDelete,
  IconEditStroked,
  IconLayers,
  IconMore,
  IconPlus,
  IconStarStroked,
  IconWrenchStroked,
} from '@douyinfe/semi-icons'
import { IconChevronRight } from '@douyinfe/semi-icons'
import type { HostStoragePoolInfo } from '@/api/storagePool'
import { formatBytes } from '@/utils/format'
import {
  clearDiskDisabled,
  flattenChildren,
  getDiskCategory,
  hasLVChildren,
  hasPVChildren,
  isVGNode,
  poolTypeLabel,
} from '../utils'
import PartitionRow, { type PartitionRowHandlers } from './PartitionRow'
import VMUsageSection from './VMUsageSection'

/** 磁盘卡操作回调（分区级操作复用 PartitionRowHandlers） */
export interface DiskCardHandlers extends PartitionRowHandlers {
  onCreatePartition: (disk: HostStoragePoolInfo) => void
  onClearDisk: (disk: HostStoragePoolInfo) => void
  onDeleteVolume: (disk: HostStoragePoolInfo) => void
}

interface DiskCardProps {
  disk: HostStoragePoolInfo
  collapsed: boolean
  onToggle: (id: string) => void
  handlers: DiskCardHandlers
}

/** 磁盘图标类型判断 */
function getDiskIconType(disk: HostStoragePoolInfo): string {
  if (disk.type === 'vg' || disk.is_lvm_vg) return 'vg'
  if (disk.removable || disk.readonly) return 'usb'
  if (disk.tran === 'nvme') return 'nvme'
  if (disk.rota === false) return 'ssd' // SSD 无机械转动
  return 'hdd'
}

/** 磁盘图标组件 */
function DiskIcon({ type }: { type: string }) {
  const icons: Record<string, React.ReactElement> = {
    hdd: (
      <svg width="21" height="21" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.8" strokeLinecap="round" strokeLinejoin="round">
        <circle cx="12" cy="12" r="8"/><circle cx="12" cy="12" r="2.5"/><path d="M12 4v3M15 13.5l4 4"/>
      </svg>
    ),
    ssd: (
      <svg width="21" height="21" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.8" strokeLinecap="round" strokeLinejoin="round">
        <rect x="4" y="6" width="16" height="12" rx="2"/><circle cx="12" cy="12" r="3"/><path d="M12 9V7.5M8 18v2M16 18v2"/>
      </svg>
    ),
    nvme: (
      <svg width="21" height="21" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.8" strokeLinecap="round" strokeLinejoin="round">
        <rect x="3" y="8" width="18" height="8" rx="2"/><path d="M7 12h4M17 12h.01"/>
      </svg>
    ),
    vg: (
      <svg width="21" height="21" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.8" strokeLinecap="round" strokeLinejoin="round">
        <circle cx="6" cy="6" r="2.4"/><circle cx="6" cy="18" r="2.4"/><circle cx="18" cy="6" r="2.4"/><path d="M6 8.4v7.2M8 6h5.5a3 3 0 0 0 3-3"/>
      </svg>
    ),
    usb: (
      <svg width="21" height="21" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.8" strokeLinecap="round" strokeLinejoin="round">
        <path d="M12 2v14"/><path d="m9 5 3-3 3 3"/><circle cx="12" cy="19" r="3"/><path d="M12 8H8a2 2 0 0 0-2 2v2"/><path d="M12 12h4a2 2 0 0 1 2 2v1"/>
      </svg>
    ),
  }
  return icons[type] || icons.hdd
}

/** 空间段类型 */
export type SpaceSegmentType = 'sys' | 'used' | 'used-warn' | 'used-danger' | 'held' | 'free'

/** 空间段数据 */
export interface SpaceSegment {
  type: SpaceSegmentType
  width: number // 百分比
  size: number // 字节
  label: string
  title?: string
}

/** 计算磁盘空间分配 */
function computeSpaceSegments(disk: HostStoragePoolInfo): SpaceSegment[] {
  const segments: SpaceSegment[] = []
  const children = disk.children || []
  const totalSize = disk.size || 1

  let allocatedSize = 0

  // 遍历子分区
  for (const child of children) {
    const childSize = child.size || 0
    allocatedSize += childSize

    // 判断分区类型
    if (child.system_disk || child.type === 'rom') {
      // 系统分区
      segments.push({
        type: 'sys',
        width: (childSize / totalSize) * 100,
        size: childSize,
        label: child.display_name || child.name,
      })
    } else if (child.configured && child.can_use_for_vm) {
      // 已用于虚拟机的存储池
      const usePercent = child.use_percent || 0
      let segType: SpaceSegmentType = 'used'
      if (usePercent >= 90) segType = 'used-danger'
      else if (usePercent >= 70) segType = 'used-warn'

      segments.push({
        type: segType,
        width: (childSize / totalSize) * 100,
        size: childSize,
        label: child.display_name || child.name,
        title: `已用 ${usePercent}%`,
      })
    } else if (child.has_existing_data) {
      // 旧数据（未挂载但有数据）
      segments.push({
        type: 'held',
        width: (childSize / totalSize) * 100,
        size: childSize,
        label: child.display_name || child.name,
        title: '旧数据 · 未挂载',
      })
    }
  }

  // 未分配空间
  const freeSize = Math.max(totalSize - allocatedSize, 0)
  if (freeSize > 0) {
    segments.push({
      type: 'free',
      width: (freeSize / totalSize) * 100,
      size: freeSize,
      label: '未分配',
    })
  }

  // 按宽度排序（sys 在最前，free 在最后）
  const typeOrder: Record<SpaceSegmentType, number> = {
    sys: 0,
    used: 1,
    'used-warn': 2,
    'used-danger': 3,
    held: 4,
    free: 5,
  }
  segments.sort((a, b) => typeOrder[a.type] - typeOrder[b.type])

  return segments
}

/** 空间分配条组件 */
function SpaceBar({ segments, totalSize, vmTotalVirtual, vmTotalActual }: {
  segments: SpaceSegment[];
  totalSize: number;
  vmTotalVirtual?: number;
  vmTotalActual?: number;
}) {
  const legendItems: Array<{ type: SpaceSegmentType; label: string; size: number; percent?: number }> = []

  // 合并同类项
  const merged = new Map<SpaceSegmentType, { size: number; count: number; maxPercent: number }>()
  for (const seg of segments) {
    const existing = merged.get(seg.type) || { size: 0, count: 0, maxPercent: 0 }
    merged.set(seg.type, {
      size: existing.size + seg.size,
      count: existing.count + 1,
      maxPercent: Math.max(existing.maxPercent, parseFloat(seg.title?.match(/(\d+)%/)?.[1] || '0')),
    })
  }

  // 生成图例
  const typeLabels: Record<SpaceSegmentType, string> = {
    sys: '系统占用',
    used: '存储池',
    'used-warn': '存储池',
    'used-danger': '存储池',
    held: '旧数据',
    free: '未分配',
  }

  for (const [type, data] of merged) {
    if (data.size === 0) continue
    const label = typeLabels[type] || type
    legendItems.push({
      type,
      label,
      size: data.size,
      percent: type.startsWith('used') ? data.maxPercent : undefined,
    })
  }

  // VM 虚拟总量进度条数据
  const vmVirtualPercent = vmTotalVirtual && vmTotalVirtual > 0 ? Math.round(((vmTotalActual || 0) / vmTotalVirtual) * 100) : 0
  const showVMSpaceBar = vmTotalVirtual && vmTotalVirtual > 0

  return (
    <div className="sp-dc-space">
      <div className="sp-space-title">
        <span>空间分配</span>
        <span className="total">共 {formatBytes(totalSize)}</span>
      </div>
      <div className="sp-spacebar">
        {segments.map((seg, idx) => (
          <Tooltip
            key={idx}
            content={seg.title ? `${seg.label} · ${formatBytes(seg.size)}（${seg.title}）` : `${seg.label} · ${formatBytes(seg.size)}`}
            position="top"
          >
            <div className={`seg ${seg.type}`} style={{ width: `${seg.width}%` }} />
          </Tooltip>
        ))}
      </div>

      {/* VM 虚拟总量进度条（当有虚拟机时显示） */}
      {showVMSpaceBar && (
        <div className="sp-vm-virtual-bar">
          <div className="sp-vm-bar-header">
            <span className="sp-vm-bar-title">虚拟机磁盘占用</span>
            <span className="sp-vm-bar-summary">
              实际 {formatBytes(vmTotalActual || 0)} / 虚拟 {formatBytes(vmTotalVirtual || 0)}
            </span>
          </div>
          <div className="sp-vm-bar">
            <div
              className="sp-vm-bar-fill"
              style={{
                width: `${vmVirtualPercent}%`,
                background: vmVirtualPercent >= 90 ? '#fb7185' : vmVirtualPercent >= 70 ? '#f59e0b' : '#2dd4bf',
              }}
            />
          </div>
          <div className="sp-vm-bar-text">
            <span>实际占用率 {vmVirtualPercent}%</span>
            <span className="sp-mono">
              {formatBytes(vmTotalActual || 0)} / {formatBytes(vmTotalVirtual || 0)}
            </span>
          </div>
        </div>
      )}

      <div className="sp-space-legend">
        {legendItems.map((item) => (
          <span key={item.type} className="lg-item">
            <i className={`lg-dot ${item.type}`} />
            {item.label}{' '}
            <span className="lg-num">
              {formatBytes(item.size)}
              {item.percent !== undefined && ` · 已用 ${item.percent}%`}
            </span>
          </span>
        ))}
      </div>
    </div>
  )
}

export default function DiskCard({ disk, collapsed, onToggle, handlers }: DiskCardProps) {
  const isVG = isVGNode(disk)
  const category = getDiskCategory(disk)
  const iconType = getDiskIconType(disk)
  const flatNodes = flattenChildren(disk.children || [])
  const pvNodes = flatNodes.filter((n) => n.type === 'pv')
  const lvNodes = flatNodes.filter((n) => n.type === 'lv')
  const spaceSegments = computeSpaceSegments(disk)
  const unallocatedSize = Math.max(disk.size - (disk.children || []).reduce((sum, c) => sum + (c.size || 0), 0), 0)

  // 收集所有子分区的 VM 统计（用于 SpaceBar 显示虚拟总量进度条）
  // 初始值含磁盘节点自身：整盘挂载（无子分区）时 VM 占用数据挂在磁盘节点上
  const allVMStats = flatNodes.reduce(
    (acc, node) => {
      if (node.vm_total_virtual) acc.vmTotalVirtual += node.vm_total_virtual
      if (node.vm_total_actual) acc.vmTotalActual += node.vm_total_actual
      return acc
    },
    { vmTotalVirtual: disk.vm_total_virtual || 0, vmTotalActual: disk.vm_total_actual || 0 }
  )

  // 判断是否为待初始化盘（全新盘或历史数据盘）
  const isPending = category === 'pending'
  const isNewDisk = isPending && !disk.has_existing_data
  const hasUnallocatedSpace = unallocatedSize > 0
  const hasPartitionDetails = isVG || flatNodes.length > 0

  // 卡片样式类
  const cardClasses = ['sp-disk-card']
  if (isPending) cardClasses.push('pending')
  if (isVG) cardClasses.push('vg-card')

  return (
    <div className={cardClasses.join(' ')}>
      {/* ==================== 卡片头 ==================== */}
      <div className="sp-dc-head">
        <div className={`sp-dc-icon ${iconType}`}>
          <DiskIcon type={iconType} />
        </div>
        <div className="sp-dc-main">
          <div className="sp-dc-name-row">
            <span className="sp-dc-name">{disk.display_name}</span>
            {/* 徽章 */}
            {isVG && <span className="sp-badge vg">LVM 卷组</span>}
            {isPending && <span className="sp-badge pending">待初始化</span>}
            {disk.is_default && <span className="sp-badge default">默认存储</span>}
            {disk.enabled && <span className="sp-badge enabled">已启用</span>}
            {disk.system_disk && <span className="sp-badge system">系统盘</span>}
            {disk.removable && <span className="sp-badge readonly">可移除</span>}
            {disk.readonly && <span className="sp-badge readonly">只读</span>}
            {disk.has_existing_data && <span className="sp-badge pending">存在数据</span>}
          </div>
          <div className="sp-dc-meta">
            <span className="sp-mono">{disk.device_path}</span>
            <span className="sp-meta-sep">·</span>
            <span>{poolTypeLabel(disk.type)}</span>
            {isVG && (disk.pv_count || 0) > 0 && (
              <>
                <span className="sp-meta-sep">·</span>
                <span>{disk.pv_count} 个物理卷</span>
              </>
            )}
            {isVG && (disk.lv_count || 0) > 0 && (
              <>
                <span className="sp-meta-sep">·</span>
                <span>{disk.lv_count} 个逻辑卷</span>
              </>
            )}
            {!isVG && disk.model && (
              <>
                <span className="sp-meta-sep">·</span>
                <span>{disk.model}</span>
              </>
            )}
            {disk.size > 0 && (
              <>
                <span className="sp-meta-sep">·</span>
                <span>{formatBytes(disk.size)}</span>
              </>
            )}
          </div>
        </div>

        <div className="sp-dc-acts">
          {isVG ? (
            <Button
              className="sp-btn danger-ghost sm"
              onClick={() => handlers.onDeleteVolume(disk)}
              disabled={disk.system_disk}
            >
              <IconDelete /> 删除存储卷
            </Button>
          ) : (
            <>
              <Tooltip content="配置" position="top">
                <span className="sp-part-act-ic" onClick={() => handlers.onConfig(disk)}>
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
                      disabled={!disk.can_use_for_vm || disk.is_default}
                      onClick={() => handlers.onSetDefault(disk)}
                    >
                      设为默认
                    </Dropdown.Item>
                    <Dropdown.Item
                      icon={<IconWrenchStroked />}
                      disabled={!disk.can_format}
                      onClick={() => handlers.onFormat(disk)}
                    >
                      格式化挂载
                    </Dropdown.Item>
                    <Dropdown.Item
                      icon={<IconPlus />}
                      disabled={!disk.can_format && !disk.configured}
                      onClick={() => handlers.onCreatePartition(disk)}
                    >
                      创建分区
                    </Dropdown.Item>
                    <Dropdown.Divider />
                    <Dropdown.Item
                      icon={<IconDelete />}
                      type="danger"
                      disabled={clearDiskDisabled(disk)}
                      onClick={() => handlers.onClearDisk(disk)}
                    >
                      清除磁盘
                    </Dropdown.Item>
                  </Dropdown.Menu>
                }
              >
                <span className="sp-part-act-ic">
                  <IconMore />
                </span>
              </Dropdown>
            </>
          )}
        </div>
      </div>

      {/* ==================== 历史数据警告 ==================== */}
      {disk.has_existing_data && disk.existing_data_warning && (
        <Banner
          type="warning"
          closeIcon={null}
          className="sp-existing-alert"
          description={disk.existing_data_warning}
        />
      )}

      {/* ==================== 空间分配条 ==================== */}
      <SpaceBar
        segments={spaceSegments}
        totalSize={disk.size}
        vmTotalVirtual={allVMStats.vmTotalVirtual}
        vmTotalActual={allVMStats.vmTotalActual}
      />

      {/* ==================== 磁盘级 VM 占用 ==================== */}
      {/* 整盘挂载（无子分区）时数据在磁盘节点本身；常驻显示（无子分区的盘默认折叠且无展开入口），点击标题行展开明细 */}
      <div className="sp-dc-vm">
        <VMUsageSection node={disk} />
      </div>

      {/* ==================== 初始化引导区（待初始化盘）==================== */}
      {isPending && (
        <div className="sp-dc-guide">
          <span className="guide-icon">
            <svg width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.8" strokeLinecap="round" strokeLinejoin="round">
              <path d="M9 18h6M10 21h4"/><path d="M12 3a6 6 0 0 0-4 10.5c.8.7 1 1.5 1 2.5h6c0-1 .2-1.8 1-2.5A6 6 0 0 0 12 3Z"/>
            </svg>
          </span>
          <div className="guide-text">
            <div className="guide-title">此磁盘尚未用于虚拟机存储</div>
            <div className="guide-sub">
              {isNewDisk
                ? '可整盘格式化为存储池，或先创建分区再使用；操作将提交任务队列并需二次验证'
                : `可在剩余 ${formatBytes(unallocatedSize)} 空间上创建分区，或格式化整盘 / 单个分区后使用`}
            </div>
          </div>
          <div className="guide-acts">
            {isNewDisk ? (
              <Button
                className="sp-btn primary"
                onClick={() => handlers.onFormat(disk)}
              >
                <IconWrenchStroked /> 格式化挂载为存储池
              </Button>
            ) : (
              <Button
                className="sp-btn primary"
                onClick={() => handlers.onCreatePartition(disk)}
              >
                <IconPlus /> 创建分区
              </Button>
            )}
            {isNewDisk && (
              <Button className="sp-btn" onClick={() => handlers.onCreatePartition(disk)}>
                <IconPlus /> 创建分区
              </Button>
            )}
          </div>
        </div>
      )}

      {/* ==================== 分区区块 ==================== */}
      {hasPartitionDetails && (
        <div className="sp-dc-parts">
          <button
            className={`sp-parts-toggle ${!collapsed ? 'open' : ''}`}
            type="button"
            aria-expanded={!collapsed}
            onClick={() => onToggle(disk.id)}
          >
            <IconChevronRight />
            分区 <span className="cnt">{(flatNodes.filter((n) => n.type === 'part').length || 0)}</span>
            {isPending && hasUnallocatedSpace && (
              <span style={{ marginLeft: 'auto', fontWeight: 400, fontSize: 12, color: 'var(--qvm-text-2)' }}>
                剩余空间可继续创建分区
              </span>
            )}
          </button>
          {!collapsed && (
            <div className="sp-parts-body">
              {isVG ? (
                <>
                  {hasPVChildren(disk) && (
                    <div className="sp-vg-sec-title">
                      <IconBox size="small" />
                      物理卷 (PV)
                    </div>
                  )}
                  {pvNodes.map((node) => (
                    <PartitionRow key={node.id} node={node} handlers={handlers} />
                  ))}
                  {hasLVChildren(disk) && (
                    <div className="sp-vg-sec-title">
                      <IconLayers size="small" />
                      逻辑卷 (LV)
                    </div>
                  )}
                  {lvNodes.map((node) => (
                    <PartitionRow key={node.id} node={node} handlers={handlers} />
                  ))}
                  {!hasPVChildren(disk) && !hasLVChildren(disk) && (
                    <div className="sp-empty-sm">无卷信息</div>
                  )}
                </>
              ) : (
                flatNodes.map((node) => <PartitionRow key={node.id} node={node} handlers={handlers} />)
              )}
            </div>
          )}
        </div>
      )}
    </div>
  )
}
