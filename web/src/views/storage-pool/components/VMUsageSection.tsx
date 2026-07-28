/**
 * 虚拟机磁盘占用展示区（默认折叠）
 * - 折叠态：一行汇总（台数 + 实际占用 / 虚拟总配），点击展开明细
 * - 展开态：总占用进度条（实际 vs 虚拟配置，≥70 橙 / ≥90 红）+ 各 VM 占用列表
 * - 同一 VM 的多块盘已由后端按名称聚合
 * - 供 PartitionRow（分区行）与 DiskCard（整盘挂载磁盘）复用
 */
import { useState } from 'react'
import { IconChevronRight } from '@douyinfe/semi-icons'
import type { HostStoragePoolInfo } from '@/api/storagePool'
import { formatBytes } from '@/utils/format'

export default function VMUsageSection({ node }: { node: HostStoragePoolInfo }) {
  const [expanded, setExpanded] = useState(false)
  const vmList = node.vm_usage_list || []
  if (vmList.length === 0) return null

  const totalVirtual = node.vm_total_virtual || 0
  const totalActual = node.vm_total_actual || 0
  const virtualPercent = totalVirtual > 0 ? Math.round((totalActual / totalVirtual) * 100) : 0

  return (
    <div className="sp-vm-usage">
      {/* 标题行：点击展开/折叠明细 */}
      <button
        type="button"
        className={`sp-vm-header clickable ${expanded ? 'open' : ''}`}
        onClick={() => setExpanded((v) => !v)}
      >
        <IconChevronRight className="sp-vm-chevron" />
        <span className="sp-vm-title">虚拟机占用</span>
        <span className="sp-vm-summary">
          共 {vmList.length} 台 · 实际占用 {formatBytes(totalActual)} / 虚拟总配 {formatBytes(totalVirtual)}
        </span>
      </button>

      {expanded && (
        <>
          {/* 总占用进度条：实际占用 vs 虚拟配置 */}
          <div className="sp-vm-progress">
            <div className="sp-vm-bar-bg">
              <div
                className="sp-vm-bar-fill"
                style={{
                  width: `${virtualPercent}%`,
                  background: virtualPercent >= 90 ? '#fb7185' : virtualPercent >= 70 ? '#f59e0b' : '#2dd4bf',
                }}
              />
            </div>
            <div className="sp-vm-bar-text">
              <span>实际占用率 {virtualPercent}%</span>
              <span className="sp-mono">
                {formatBytes(totalActual)} / {formatBytes(totalVirtual)}
              </span>
            </div>
          </div>

          {/* VM 列表 */}
          <div className="sp-vm-list">
            {vmList.map((vm) => (
              <div key={vm.name} className="sp-vm-item">
                <span className="sp-vm-name">{vm.name}</span>
                <span className="sp-vm-size">
                  <span className="sp-mono">{formatBytes(vm.actual_size)}</span>
                  <span className="sp-vm-sep">/</span>
                  <span className="sp-vm-virtual sp-mono">{formatBytes(vm.virtual_size)}</span>
                </span>
              </div>
            ))}
          </div>
        </>
      )}
    </div>
  )
}
