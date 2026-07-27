/**
 * 管理员仪表盘：底部行（最近虚拟机列表 + 存储池用量）
 */
import { useEffect, useState } from 'react'
import { Toast } from '@douyinfe/semi-ui'
import { IconDesktop } from '@douyinfe/semi-icons'
import type { VmListItem } from '@/api/vm'
import { getStoragePoolList, type StoragePoolInfo } from '@/api/storage'
import { formatBytes, formatRuntime } from '@/utils/format'
import { StatusPill } from './widgets'

interface AdminBottomProps {
  vms: VmListItem[]
}

/** 存储池用量条配色（按使用率） */
function poolColor(percent: number): string {
  if (percent >= 85) return 'linear-gradient(90deg,#FBBF24,#F59E0B)'
  if (percent >= 60) return 'linear-gradient(90deg,#2DD4BF,#38BDF8)'
  return 'linear-gradient(90deg,#8B5CF6,#C084FC)'
}

const RECENT_VM_LIMIT = 5

export default function AdminBottom({ vms }: AdminBottomProps) {
  const [pools, setPools] = useState<StoragePoolInfo[]>([])

  useEffect(() => {
    let mounted = true
    getStoragePoolList()
      .then((res) => {
        if (mounted) setPools((res.data || []).filter((p) => p.enabled))
      })
      .catch(() => undefined)
    return () => {
      mounted = false
    }
  }, [])

  const recentVms = vms.slice(0, RECENT_VM_LIMIT)

  return (
    <section className="qvm-bottom-row">
      {/* 最近虚拟机 */}
      <div className="qvm-panel-card qvm-g-border qvm-fade-up" style={{ '--qvm-delay': '360ms' } as React.CSSProperties}>
        <div className="qvm-panel-head">
          <span className="qvm-panel-title">虚拟机</span>
          <span className="qvm-panel-sub">最近活跃</span>
          <span
            className="qvm-panel-link"
            onClick={() => Toast.info({ content: '虚拟机列表页将在后续迭代提供', duration: 2 })}
          >
            查看全部 →
          </span>
        </div>
        {recentVms.length === 0 ? (
          <div className="qvm-empty-text">暂无虚拟机</div>
        ) : (
          <table className="qvm-table">
            <thead>
              <tr>
                <th>名称</th>
                <th>状态</th>
                <th>配置</th>
                <th>IP 地址</th>
                <th>运行时长</th>
                <th></th>
              </tr>
            </thead>
            <tbody>
              {recentVms.map((vm) => (
                <tr key={vm.name}>
                  <td>
                    <div className="qvm-vm-name">
                      <div className={`qvm-vm-ic ${vm.status === 'running' ? '' : 'off'}`}>
                        <IconDesktop size="small" />
                      </div>
                      {vm.name}
                    </div>
                  </td>
                  <td>
                    <StatusPill status={vm.status} />
                  </td>
                  <td className="qvm-mono">
                    {vm.vcpu}C / {Math.round((vm.memory || 0) / 1024)}G / {vm.disk_size || '-'}
                  </td>
                  <td className="qvm-mono">{vm.ip || '—'}</td>
                  <td className="qvm-mono">{formatRuntime(vm.continuous_runtime_seconds)}</td>
                  <td>
                    <span
                      className="qvm-act-btn"
                      onClick={() => Toast.info({ content: '虚拟机详情页将在后续迭代提供', duration: 2 })}
                    >
                      管理
                    </span>
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        )}
      </div>

      {/* 存储池用量 */}
      <div className="qvm-panel-card qvm-g-border qvm-fade-up" style={{ '--qvm-delay': '420ms' } as React.CSSProperties}>
        <div className="qvm-panel-head">
          <span className="qvm-panel-title">存储池用量</span>
          <span
            className="qvm-panel-link"
            onClick={() => Toast.info({ content: '存储池管理将在后续迭代提供', duration: 2 })}
          >
            管理 →
          </span>
        </div>
        {pools.length === 0 ? (
          <div className="qvm-pool-empty">暂无存储池</div>
        ) : (
          pools.map((pool) => (
            <div className="qvm-pool-row" key={pool.id}>
              <div className="qvm-pool-top">
                <span className="qvm-pool-name">{pool.display_name || pool.name}</span>
                <span className="qvm-pool-tag">{pool.fstype || pool.type || '目录'}</span>
                <span className="qvm-pool-val">
                  {formatBytes(pool.used)} / {formatBytes(pool.size)} · {pool.use_percent}%
                </span>
              </div>
              <div className="qvm-pool-track">
                <div
                  className="qvm-pool-fill"
                  style={{ width: `${Math.min(pool.use_percent, 100)}%`, background: poolColor(pool.use_percent) }}
                />
              </div>
            </div>
          ))
        )}
      </div>
    </section>
  )
}
