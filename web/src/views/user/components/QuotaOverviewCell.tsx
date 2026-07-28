/**
 * 用户列表紧凑配额单元格
 * - 弹性云用户：展示 CPU/内存/磁盘/VM 四项迷你进度条，完整明细放悬浮卡片
 * - 轻量云用户：提示单 VM 配额，悬浮卡片展示每台 VM 的配额摘要
 * - 管理员：仅展示存储配额
 */
import { Popover, Progress, Tag } from '@douyinfe/semi-ui'
import type { UserListItem } from '@/api/user'
import {
  formatRegistrationQuota,
  isLightweightUser,
  quotaPercent,
  trafficLimitLabel,
} from '../utils'

/** 单条迷你进度行 */
function MiniBar({
  label,
  text,
  percent,
  danger,
}: {
  label: string
  text: string
  percent: number
  danger?: boolean
}) {
  return (
    <div className="usr-mini-bar">
      <span className="usr-mini-bar-label">{label}</span>
      <span className="usr-mini-bar-text">{text}</span>
      <Progress
        percent={percent}
        showInfo={false}
        stroke={danger ? 'var(--semi-color-danger)' : undefined}
        className="usr-mini-bar-progress"
      />
    </div>
  )
}

/** 悬浮明细里的一行 */
function DetailRow({ label, value, danger }: { label: string; value: string; danger?: boolean }) {
  return (
    <div className="usr-quota-detail-row">
      <span className="usr-quota-detail-label">{label}</span>
      <span className={`usr-quota-detail-value${danger ? ' danger' : ''}`}>{value}</span>
    </div>
  )
}

/** 上限展示：0 表示不限 */
const limitText = (max?: number, unit = '') => (max && max > 0 ? `${max}${unit}` : '不限')

export default function QuotaOverviewCell({ row }: { row: UserListItem }) {
  const quota = row.quota

  // 管理员：仅存储配额
  if (row.role === 'admin') {
    return (
      <div className="usr-quota-cell">
        {quota ? (
          <MiniBar
            label="存储"
            text={`${quota.used_storage_gb || '0 B'} / ${limitText(row.max_storage, 'GB')}`}
            percent={quotaPercent((row.max_storage || 0) * 1073741824, quota.used_storage)}
          />
        ) : (
          <span className="usr-muted">-</span>
        )}
      </div>
    )
  }

  // 轻量云用户：单 VM 配额，悬浮展示每台 VM 摘要
  if (isLightweightUser(row)) {
    const quotas = row.lightweight_quotas || []
    const cell = (
      <div className="usr-quota-cell lw">
        <Tag size="small" color="cyan">
          单 VM 配额 × {quotas.length}
        </Tag>
        <span className="usr-muted sm">计算资源由管理员分配</span>
      </div>
    )
    if (!quotas.length) return cell
    return (
      <Popover
        position="right"
        showArrow
        content={
          <div className="usr-quota-detail">
            <div className="usr-quota-detail-title">轻量云单 VM 配额</div>
            {quotas.map((q) => (
              <DetailRow
                key={q.vm_name}
                label={q.vm_name}
                value={formatRegistrationQuota(q)}
                danger={q.is_limited_down || q.is_limited_up || q.runtime_quota_reached}
              />
            ))}
          </div>
        }
      >
        {cell}
      </Popover>
    )
  }

  // 弹性云用户
  if (!quota) {
    return <span className="usr-muted">-</span>
  }

  const cell = (
    <div className="usr-quota-cell">
      <MiniBar
        label="CPU"
        text={`${quota.used_cpu} / ${limitText(row.max_cpu)}`}
        percent={quotaPercent(row.max_cpu, quota.used_cpu)}
      />
      <MiniBar
        label="内存"
        text={`${quota.used_memory} / ${limitText(row.max_memory, 'GB')}`}
        percent={quotaPercent(row.max_memory, quota.used_memory)}
      />
      <MiniBar
        label="磁盘"
        text={`${quota.used_disk} / ${limitText(row.max_disk, 'GB')}`}
        percent={quotaPercent(row.max_disk, quota.used_disk)}
      />
      <MiniBar
        label="VM"
        text={`${quota.used_vm} / ${limitText(row.max_vm)}`}
        percent={quotaPercent(row.max_vm, quota.used_vm)}
      />
      {(quota.is_limited_down || quota.is_limited_up) && (
        <Tag size="small" color="red">
          已限速
        </Tag>
      )}
      {quota.runtime_quota_reached && (
        <Tag size="small" color="red">
          时长耗尽
        </Tag>
      )}
    </div>
  )

  return (
    <Popover
      position="right"
      showArrow
      content={
        <div className="usr-quota-detail">
          <div className="usr-quota-detail-title">配额明细</div>
          <DetailRow
            label="快照数量"
            value={`${quota.used_snapshots || 0} / ${limitText(row.max_snapshots)}`}
          />
          <DetailRow
            label="端口转发"
            value={
              row.enable_port_forward
                ? `${quota.used_port_forwards || 0} / ${limitText(row.max_port_forwards)}`
                : '未开通'
            }
          />
          <DetailRow
            label="公网 IP"
            value={`${quota.used_public_ips || 0} / ${limitText(row.max_public_ips)}`}
          />
          <DetailRow
            label="存储配额"
            value={`${quota.used_storage_gb || '0 B'} / ${limitText(row.max_storage, 'GB')}`}
          />
          <DetailRow
            label="运行时长"
            value={`${quota.used_runtime_display || '0秒'} / ${limitText(row.max_runtime_hours, '小时')}`}
            danger={quota.runtime_quota_reached}
          />
          <DetailRow
            label="下行流量"
            value={`${quota.used_traffic_down_gb || '0 B'} / ${trafficLimitLabel(row.max_traffic_down)}`}
            danger={quota.is_limited_down}
          />
          <DetailRow
            label="上行流量"
            value={`${quota.used_traffic_up_gb || '0 B'} / ${trafficLimitLabel(row.max_traffic_up)}`}
            danger={quota.is_limited_up}
          />
          <DetailRow
            label="带宽上限"
            value={`↓${limitText(row.max_bandwidth_down, 'Mbps')} / ↑${limitText(row.max_bandwidth_up, 'Mbps')}`}
          />
        </div>
      }
    >
      {cell}
    </Popover>
  )
}
