/**
 * 删除存储卷弹窗（高风险，任务队列）
 * - 删除卷组及其所有逻辑卷和物理卷，数据不可恢复
 */
import { useState } from 'react'
import { Banner, Checkbox, Descriptions, Modal, Toast } from '@douyinfe/semi-ui'
import type { HostStoragePoolInfo } from '@/api/storagePool'
import { deleteLVMVolume } from '@/api/storagePool'
import { formatBytes } from '@/utils/format'
import { useMountModalLifecycle } from '@/hooks/useMountModalLifecycle'

interface DeleteVolumeDialogProps {
  row: HostStoragePoolInfo
  onClose: () => void
  onSubmitted: () => void
}

export default function DeleteVolumeDialog({ row, onClose, onSubmitted }: DeleteVolumeDialogProps) {
  const { modalVisible, requestClose, afterModalClose } = useMountModalLifecycle(onClose)
  const [confirmed, setConfirmed] = useState(false)
  const [submitting, setSubmitting] = useState(false)

  const handleSubmit = async () => {
    if (!row.name) return
    setSubmitting(true)
    try {
      await deleteLVMVolume(row.name)
      Toast.success('删除 LVM 存储卷任务已提交，请在任务中心查看进度')
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
      title="删除存储卷"
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
      <Banner
        type="danger"
        closeIcon={null}
        style={{ marginBottom: 14 }}
        description={`此操作将删除卷组「${row.name}」及其所有逻辑卷和物理卷，数据将不可恢复！`}
      />
      <Descriptions align="left" size="small" className="sp-desc">
        <Descriptions.Item itemKey="卷组名称">{row.name}</Descriptions.Item>
        <Descriptions.Item itemKey="总容量">{formatBytes(row.size)}</Descriptions.Item>
        <Descriptions.Item itemKey="逻辑卷数">{row.lv_count || 0} 个</Descriptions.Item>
        <Descriptions.Item itemKey="物理卷数">{row.pv_count || 0} 个</Descriptions.Item>
      </Descriptions>

      <div className="sp-confirm-line">
        <Checkbox checked={confirmed} onChange={(e) => setConfirmed(!!e.target.checked)}>
          我确认要删除该卷组及其所有逻辑卷和物理卷
        </Checkbox>
      </div>
    </Modal>
  )
}
