/**
 * 上传进度弹窗：分片上传过程中显示进度，支持暂停/继续/取消
 * 迁移自旧前端 views/storage/index.vue 的 upload dialog
 */
import { Button, Modal, Progress } from '@douyinfe/semi-ui'
import { IconRefresh } from '@douyinfe/semi-icons'
import type { ChunkUploader } from '@/utils/chunkUploader'

/** 上传状态 */
export type UploadStatus = 'hashing' | 'uploading' | 'done'

interface UploadDialogProps {
  visible: boolean
  fileName: string
  progress: number // 0-100
  status: UploadStatus
  paused: boolean
  uploader: ChunkUploader | null
  /** 暂停 / 继续切换 */
  onTogglePause: () => void
  /** 取消上传 */
  onCancel: () => void
  /** 上传完成后关闭弹窗 */
  onClose: () => void
}

export default function UploadDialog({
  visible,
  fileName,
  progress,
  status,
  paused,
  uploader,
  onTogglePause,
  onCancel,
  onClose,
}: UploadDialogProps) {
  /** 关闭拦截：上传中先取消，由 upload() 的 catch 负责关闭 */
  const handleClose = () => {
    if (uploader && status !== 'done') {
      uploader.cancel()
      return
    }
    onClose()
  }

  const statusText = () => {
    if (status === 'hashing') {
      return (
        <p style={{ marginTop: 12, color: 'var(--qvm-text-2)', display: 'flex', alignItems: 'center', gap: 6, justifyContent: 'center' }}>
          <IconRefresh spin />
          正在校验文件，准备分片上传...
        </p>
      )
    }
    if (status === 'uploading') {
      return (
        <p style={{ marginTop: 12, color: 'var(--qvm-text-2)' }}>
          分片上传中（1MB / 片，并发 3）{paused ? ' · 已暂停' : ''}
        </p>
      )
    }
    if (status === 'done') {
      return (
        <p style={{ marginTop: 12, color: 'var(--semi-color-success)' }}>
          上传完成，正在校验并落盘...
        </p>
      )
    }
    return null
  }

  return (
    <Modal
      title="上传文件"
      visible={visible}
      onCancel={handleClose}
      width={480}
      maskClosable={false}
      closeOnEsc={false}
      footer={null}
    >
      <div style={{ textAlign: 'center' }}>
        <p style={{ marginBottom: 12, wordBreak: 'break-all', color: 'var(--qvm-text-0)' }}>
          {fileName}
        </p>
        <Progress
          percent={progress}
          showInfo
          // 关闭数字动画：percent 每传一个分片变一次，间隔远小于动画时长 300ms，
          // 动画会被频繁销毁重建而来不及触发帧回调，导致 showInfo 数字卡在初值 0%。
          // 关闭后数字直接跟随 percent（与填充条同步）。
          motion={false}
          stroke={progress >= 100 ? 'var(--semi-color-success)' : undefined}
          aria-label="上传进度"
        />
        {statusText()}
        {(status === 'uploading' || status === 'hashing') && (
          <div style={{ marginTop: 16, display: 'flex', gap: 8, justifyContent: 'center' }}>
            {status === 'uploading' && (
              <Button size="small" onClick={onTogglePause}>
                {paused ? '继续' : '暂停'}
              </Button>
            )}
            <Button size="small" type="danger" theme="light" onClick={onCancel}>
              取消
            </Button>
          </div>
        )}
      </div>
    </Modal>
  )
}
