/**
 * 磁盘扩容弹窗（编辑模式，仅扩大）
 */
import { useEffect, useState } from 'react'
import { InputNumber, Modal, Toast } from '@douyinfe/semi-ui'
import type { VmDiskItem } from '@/api/vm'
import type { VmEditDevices } from '../useVmEditDevices'
import FormField from '../sections/FormField'

interface ResizeDiskDialogProps {
  visible: boolean
  disk: VmDiskItem | null
  devices: VmEditDevices
  onClose: () => void
}

export default function ResizeDiskDialog({ visible, disk, devices, onClose }: ResizeDiskDialogProps) {
  const [size, setSize] = useState<number>(0)
  const [submitting, setSubmitting] = useState(false)
  const currentCapacity = Number(disk?.capacity_gb || 0)

  useEffect(() => {
    if (visible) setSize(0)
  }, [visible])

  const handleOk = async () => {
    if (!disk) return
    if (!Number.isFinite(size) || size <= 0) {
      Toast.error('容量必须大于 0')
      return
    }
    if (size < currentCapacity) {
      Toast.error(`新容量不能小于当前容量（${currentCapacity} GB）`)
      return
    }
    if (size === currentCapacity) {
      onClose()
      return
    }
    setSubmitting(true)
    try {
      await devices.resizeDiskAction(disk.device, size)
      onClose()
    } catch {
      // 错误由请求层统一提示
    } finally {
      setSubmitting(false)
    }
  }

  return (
    <Modal
      title={`扩容磁盘 ${disk?.device || ''}`}
      visible={visible}
      onCancel={onClose}
      onOk={() => void handleOk()}
      okText="扩容"
      cancelText="取消"
      confirmLoading={submitting}
      width={420}
      closeOnEsc
    >
      <FormField label="新容量（GB）" tip={`当前容量 ${currentCapacity} GB，只能扩大不能缩小`}>
        <InputNumber
          style={{ width: '100%' }}
          value={size || undefined}
          min={currentCapacity}
          max={8192}
          placeholder="请输入新的容量（GB）"
          onChange={(v) => setSize(Number(v || 0))}
        />
      </FormField>
    </Modal>
  )
}
