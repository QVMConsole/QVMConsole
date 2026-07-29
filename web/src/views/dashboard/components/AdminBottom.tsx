/**
 * 管理员仪表盘：底部行（最近虚拟机列表）
 */
import { useNavigate } from 'react-router'
import { IconDesktop } from '@douyinfe/semi-icons'
import type { VmListItem } from '@/api/vm'
import { formatRuntime } from '@/utils/format'
import { StatusPill } from './widgets'

interface AdminBottomProps {
  vms: VmListItem[]
}

const RECENT_VM_LIMIT = 5

export default function AdminBottom({ vms }: AdminBottomProps) {
  const navigate = useNavigate()
  const recentVms = vms.slice(0, RECENT_VM_LIMIT)

  return (
    <section className="qvm-bottom-row">
      {/* 最近虚拟机 */}
      <div className="qvm-panel-card qvm-g-border qvm-fade-up" style={{ '--qvm-delay': '360ms' } as React.CSSProperties}>
        <div className="qvm-panel-head">
          <span className="qvm-panel-title">虚拟机</span>
          <span className="qvm-panel-sub">最近活跃</span>
          <span className="qvm-panel-link" onClick={() => navigate('/vm')}>
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
                      onClick={() => navigate(`/vm/detail/${encodeURIComponent(vm.name)}`)}
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
    </section>
  )
}
