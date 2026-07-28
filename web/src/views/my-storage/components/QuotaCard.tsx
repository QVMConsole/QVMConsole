/**
 * 配额概览卡：已用空间 / 配额上限 + 使用率进度条 + 只读模式警告
 * 迁移自旧前端 views/storage/index.vue 的 quota-card
 */
import { useMemo } from 'react'
import { Banner, Progress, Tag } from '@douyinfe/semi-ui'
import { IconFolder } from '@douyinfe/semi-icons'
import type { UserStorageInfo } from '@/api/storage'

interface QuotaCardProps {
  info: UserStorageInfo
}

export default function QuotaCard({ info }: QuotaCardProps) {
  const usagePercent = useMemo(() => {
    if (!info.max_bytes || info.max_bytes <= 0) return 0
    return Math.min(Math.round(((info.used_bytes || 0) / info.max_bytes) * 100), 100)
  }, [info.used_bytes, info.max_bytes])

  const strokeColor = useMemo(() => {
    if (usagePercent > 90) return 'var(--semi-color-danger)'
    if (usagePercent > 70) return 'var(--semi-color-warning)'
    return 'var(--semi-color-primary)'
  }, [usagePercent])

  return (
    <div className="mst-quota-card qvm-fade-up" style={{ '--qvm-delay': '60ms' } as React.CSSProperties}>
      <div className="mst-quota-header">
        <div className="mst-quota-title">
          <IconFolder />
          存储配额
        </div>
        {info.readonly && <Tag color="red">只读模式</Tag>}
      </div>

      {info.readonly && (
        <Banner
          type="danger"
          closeIcon={null}
          description="存储空间已超出配额，当前处于只读模式。请删除部分文件后恢复正常使用。"
          style={{ marginBottom: 12 }}
        />
      )}

      <div className="mst-quota-stats">
        <div className="mst-quota-item">
          <span className="mst-quota-label">已用空间</span>
          <span className="mst-quota-value">{info.used_display || '0 B'}</span>
        </div>
        <div className="mst-quota-item">
          <span className="mst-quota-label">配额上限</span>
          <span className="mst-quota-value">
            {info.max_storage && info.max_storage > 0 ? `${info.max_storage} GB` : '不限'}
          </span>
        </div>
      </div>

      {info.max_storage != null && info.max_storage > 0 && (
        <Progress
          percent={usagePercent}
          stroke={strokeColor}
          showInfo
          format={(p) => `${p}%`}
          aria-label="存储配额使用率"
        />
      )}
    </div>
  )
}
