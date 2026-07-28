/**
 * 格式化并挂载弹窗（高风险，任务队列）
 * - 清空目标硬盘/分区全部数据，格式化为选定文件系统并写入开机自动挂载
 * - 挂载目录固定为 /var/lib/kvm-storage/{设备ID}
 */
import { useState } from 'react'
import { Banner, Checkbox, Descriptions, Modal, Select, Toast } from '@douyinfe/semi-ui'
import type { HostStoragePoolInfo } from '@/api/storagePool'
import { formatMountStoragePool } from '@/api/storagePool'
import { formatBytes } from '@/utils/format'
import { useMountModalLifecycle } from '@/hooks/useMountModalLifecycle'

interface FormatDialogProps {
  row: HostStoragePoolInfo
  onClose: () => void
  onSubmitted: () => void
}

export default function FormatDialog({ row, onClose, onSubmitted }: FormatDialogProps) {
  const { modalVisible, requestClose, afterModalClose } = useMountModalLifecycle(onClose)
  const [fsType, setFsType] = useState('ext4')
  const [confirmed, setConfirmed] = useState(false)
  const [submitting, setSubmitting] = useState(false)

  const handleSubmit = async () => {
    setSubmitting(true)
    try {
      await formatMountStoragePool(row.id, fsType)
      Toast.success('格式化并挂载任务已提交，请在任务中心查看进度')
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
      title="格式化并挂载硬盘"
      visible={modalVisible}
      afterClose={afterModalClose}
      onCancel={requestClose}
      onOk={() => void handleSubmit()}
      okText="提交任务"
      cancelText="取消"
      okButtonProps={{ type: 'danger', loading: submitting, disabled: !confirmed }}
      width={560}
      closeOnEsc
    >
      <Banner
        type="danger"
        closeIcon={null}
        style={{ marginBottom: 14 }}
        description="此操作会清空目标硬盘或分区上的全部数据，并写入开机自动挂载配置。"
      />
      <Descriptions align="left" size="small" className="sp-desc">
        <Descriptions.Item itemKey="设备">{row.device_path}</Descriptions.Item>
        <Descriptions.Item itemKey="容量">{formatBytes(row.size)}</Descriptions.Item>
        <Descriptions.Item itemKey="当前文件系统">{row.fstype || '无'}</Descriptions.Item>
        <Descriptions.Item itemKey="挂载目录">
          <span className="sp-mono">/var/lib/kvm-storage/{row.id}</span>
        </Descriptions.Item>
      </Descriptions>

      <div className="qvm-form-item" style={{ marginTop: 14 }}>
        <div className="qvm-form-label">文件系统</div>
        <Select
          value={fsType}
          onChange={(v) => setFsType(v as string)}
          style={{ width: '100%' }}
          optionList={[
            { label: 'ext4（推荐，稳定兼容）', value: 'ext4' },
            { label: 'xfs（高性能，大文件优化）', value: 'xfs' },
            { label: 'btrfs（快照/压缩等高级特性）', value: 'btrfs' },
          ]}
        />
      </div>

      <div className="sp-confirm-line">
        <Checkbox checked={confirmed} onChange={(e) => setConfirmed(!!e.target.checked)}>
          我确认要格式化该设备并挂载为虚拟机存储池
        </Checkbox>
      </div>
    </Modal>
  )
}
