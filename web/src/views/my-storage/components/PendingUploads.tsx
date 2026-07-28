/**
 * 未完成上传恢复区：列出断点续传会话，支持继续上传 / 取消
 * 迁移自旧前端 views/storage/index.vue 的 pending-uploads 卡片
 */
import { Button, Progress, Tag } from '@douyinfe/semi-ui'
import { IconUpload } from '@douyinfe/semi-icons'
import type { PendingUploadItem } from '@/api/storage'

/** 类别标签 */
const CATEGORY_MAP: Record<string, { label: string; color: string }> = {
  iso: { label: 'ISO', color: 'green' },
  share: { label: '共享', color: 'blue' },
  disk: { label: '磁盘', color: 'orange' },
}

interface PendingUploadsProps {
  items: PendingUploadItem[]
  /** 点击「继续」：弹出文件选择器让用户选回同一文件 */
  onResume: (item: PendingUploadItem) => void
  /** 点击「取消」：取消上传并清理 */
  onCancel: (item: PendingUploadItem) => void
}

export default function PendingUploads({ items, onResume, onCancel }: PendingUploadsProps) {
  if (items.length === 0) return null

  return (
    <div className="mst-pending-card qvm-fade-up" style={{ '--qvm-delay': '30ms' } as React.CSSProperties}>
      <div className="mst-pending-title">
        <IconUpload />
        未完成的上传（可继续或取消）
      </div>
      {items.map((item) => {
        const cat = CATEGORY_MAP[item.category] || { label: item.category, color: 'grey' }
        return (
          <div key={item.session_key} className="mst-pending-row">
            <Tag size="small" color={cat.color as 'green'}>
              {cat.label}
            </Tag>
            <span className="mst-pending-name" title={item.file_name}>
              {item.file_name}
            </span>
            <div className="mst-pending-progress">
              <Progress percent={item.progress} showInfo size="small" aria-label="上传进度" />
            </div>
            <Button size="small" type="primary" onClick={() => onResume(item)}>
              继续
            </Button>
            <Button size="small" type="danger" theme="light" onClick={() => onCancel(item)}>
              取消
            </Button>
          </div>
        )
      })}
    </div>
  )
}
