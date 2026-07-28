/**
 * 创建分区弹窗（高风险，任务队列）
 * - 在磁盘上创建新分区；无分区表时自动创建 GPT 分区表
 * - 分区大小 0 或留空表示使用全部剩余空间
 */
import { useState } from 'react'
import { Banner, Checkbox, Descriptions, InputNumber, Modal, Toast } from '@douyinfe/semi-ui'
import { IconInfoCircle } from '@douyinfe/semi-icons'
import type { HostStoragePoolInfo } from '@/api/storagePool'
import { createStoragePartition } from '@/api/storagePool'
import { formatBytes } from '@/utils/format'
import { useMountModalLifecycle } from '@/hooks/useMountModalLifecycle'

interface CreatePartitionDialogProps {
  row: HostStoragePoolInfo
  onClose: () => void
  onSubmitted: () => void
}

export default function CreatePartitionDialog({
  row,
  onClose,
  onSubmitted,
}: CreatePartitionDialogProps) {
  const { modalVisible, requestClose, afterModalClose } = useMountModalLifecycle(onClose)
  const [sizeGB, setSizeGB] = useState<number>(0)
  const [confirmed, setConfirmed] = useState(false)
  const [submitting, setSubmitting] = useState(false)

  const handleSubmit = async () => {
    setSubmitting(true)
    try {
      await createStoragePartition(row.id, { size_gb: sizeGB || 0 })
      Toast.success('创建分区任务已提交，请在任务中心查看进度')
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
      title="创建分区"
      visible={modalVisible}
      afterClose={afterModalClose}
      onCancel={requestClose}
      onOk={() => void handleSubmit()}
      okText="提交任务"
      cancelText="取消"
      okButtonProps={{ loading: submitting, disabled: !confirmed }}
      width={520}
      closeOnEsc
    >
      <Banner
        type="warning"
        closeIcon={null}
        style={{ marginBottom: 14 }}
        description="此操作会在磁盘上创建新分区。若磁盘无分区表将自动创建 GPT 分区表。"
      />
      <Descriptions align="left" size="small" className="sp-desc">
        <Descriptions.Item itemKey="设备">{row.device_path}</Descriptions.Item>
        <Descriptions.Item itemKey="容量">{formatBytes(row.size)}</Descriptions.Item>
        <Descriptions.Item itemKey="已有分区">
          {(row.children || []).length} 个
        </Descriptions.Item>
      </Descriptions>

      <div className="qvm-form-item" style={{ marginTop: 14 }}>
        <div className="qvm-form-label">分区大小</div>
        <InputNumber
          value={sizeGB}
          onChange={(v) => setSizeGB(Number(v) || 0)}
          min={0}
          max={100000}
          step={1}
          style={{ width: '100%' }}
          placeholder="留空则使用全部剩余空间"
        />
        <div className="qvm-form-tip">
          <IconInfoCircle size="small" style={{ marginRight: 4, verticalAlign: -2 }} />
          单位为 GB，输入 0 或留空表示使用磁盘全部剩余空间
        </div>
      </div>

      <div className="sp-confirm-line">
        <Checkbox checked={confirmed} onChange={(e) => setConfirmed(!!e.target.checked)}>
          我确认要在该磁盘上创建新分区
        </Checkbox>
      </div>
    </Modal>
  )
}
