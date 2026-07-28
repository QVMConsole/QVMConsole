/**
 * 清除磁盘弹窗（高风险，任务队列）
 * - 有分区：卸载并删除所有分区、清除分区表
 * - 无分区：卸载挂载点并擦除文件系统签名
 * - 标题/警告/确认文案按磁盘状态动态切换（与旧版一致）
 */
import { useState } from 'react'
import { Banner, Checkbox, Descriptions, Modal, Toast } from '@douyinfe/semi-ui'
import type { HostStoragePoolInfo } from '@/api/storagePool'
import { deleteStoragePartitions } from '@/api/storagePool'
import { formatBytes } from '@/utils/format'
import { useMountModalLifecycle } from '@/hooks/useMountModalLifecycle'

interface ClearDiskDialogProps {
  row: HostStoragePoolInfo
  onClose: () => void
  onSubmitted: () => void
}

export default function ClearDiskDialog({ row, onClose, onSubmitted }: ClearDiskDialogProps) {
  const { modalVisible, requestClose, afterModalClose } = useMountModalLifecycle(onClose)
  const [confirmed, setConfirmed] = useState(false)
  const [submitting, setSubmitting] = useState(false)

  const hasChildren = (row.children || []).length > 0
  const title = hasChildren ? '删除所有分区' : '清除磁盘挂载'
  const warning = hasChildren
    ? '此操作将卸载并删除该磁盘上的所有分区，清除分区表，相关数据将不可恢复！'
    : '此操作将卸载该磁盘并清除文件系统签名，相关数据将不可恢复！'
  const confirmText = hasChildren
    ? '我确认要删除该磁盘上的所有分区'
    : '我确认要清除该磁盘上的挂载并擦除数据'

  const handleSubmit = async () => {
    setSubmitting(true)
    try {
      await deleteStoragePartitions(row.id)
      Toast.success('删除分区任务已提交，请在任务中心查看进度')
      onSubmitted()
      requestClose()
    } catch {
      // 错误提示由请求层统一处理
    } finally {
      setSubmitting(false)
    }
  }

  return (
    <Modal
      title={title}
      visible={modalVisible}
      afterClose={afterModalClose}
      onCancel={requestClose}
      onOk={() => void handleSubmit()}
      okText="提交任务"
      cancelText="取消"
      okButtonProps={{ type: 'danger', loading: submitting, disabled: !confirmed }}
      width={520}
      closeOnEsc
    >
      <Banner type="danger" closeIcon={null} style={{ marginBottom: 14 }} description={warning} />
      <Descriptions align="left" size="small" className="sp-desc">
        <Descriptions.Item itemKey="设备">{row.device_path}</Descriptions.Item>
        <Descriptions.Item itemKey="容量">{formatBytes(row.size)}</Descriptions.Item>
        {hasChildren ? (
          <Descriptions.Item itemKey="分区数">
            {(row.children || []).length} 个
          </Descriptions.Item>
        ) : (
          (row.mountpoints || []).length > 0 && (
            <Descriptions.Item itemKey="挂载点">
              <span className="sp-mono">{(row.mountpoints || []).join(', ')}</span>
            </Descriptions.Item>
          )
        )}
        {row.fstype && (
          <Descriptions.Item itemKey="文件系统">{row.fstype}</Descriptions.Item>
        )}
      </Descriptions>

      <div className="sp-confirm-line">
        <Checkbox checked={confirmed} onChange={(e) => setConfirmed(!!e.target.checked)}>
          {confirmText}
        </Checkbox>
      </div>
    </Modal>
  )
}
