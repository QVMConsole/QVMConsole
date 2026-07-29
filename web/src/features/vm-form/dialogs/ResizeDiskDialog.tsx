/**
 * 磁盘扩容弹窗（编辑模式，仅扩大）
 */
import { useEffect, useState } from 'react'
import { Banner, InputNumber, Modal, Switch, Toast } from '@douyinfe/semi-ui'
import type { VmDiskItem } from '@/api/vm'
import type { VmEditDevices } from '../useVmEditDevices'
import FormField from '../sections/FormField'
import { useVmFormScope } from '../scopeContext'

interface ResizeDiskDialogProps {
  visible: boolean
  disk: VmDiskItem | null
  devices: VmEditDevices
  onClose: () => void
}

export default function ResizeDiskDialog({ visible, disk, devices, onClose }: ResizeDiskDialogProps) {
  const { ctx } = useVmFormScope()
  const [size, setSize] = useState<number>(0)
  const [autoGrow, setAutoGrow] = useState(false)
  const [modalVisible, setModalVisible] = useState(false)
  const [lastDisk, setLastDisk] = useState<VmDiskItem | null>(disk)
  const [submitting, setSubmitting] = useState(false)
  const activeDisk = disk || lastDisk
  const currentCapacity = Number(activeDisk?.capacity_gb || 0)
  const canAutoGrow =
    ctx.vmStatus === 'running' &&
    ctx.guestType === 'linux' &&
    !!ctx.guestAgentConnected &&
    !!activeDisk?.is_system

  useEffect(() => {
    if (disk) setLastDisk(disk)
    if (visible) {
      setSize(0)
      setAutoGrow(false)
      setModalVisible(true)
    }
  }, [visible, disk])

  const handleOk = async () => {
    if (!activeDisk) return
    if (!Number.isFinite(size) || size <= 0) {
      Toast.error('容量必须大于 0')
      return
    }
    if (size < currentCapacity) {
      Toast.error(`新容量不能小于当前容量（${currentCapacity} GB）`)
      return
    }
    if (size === currentCapacity) {
      setModalVisible(false)
      return
    }
    setSubmitting(true)
    try {
      await devices.resizeDiskAction(activeDisk.device, size, autoGrow)
      setModalVisible(false)
    } catch {
      // 错误由请求层统一提示
    } finally {
      setSubmitting(false)
    }
  }

  return (
    <Modal
      title={`扩容磁盘 ${activeDisk?.device || ''}`}
      visible={modalVisible}
      afterClose={onClose}
      onCancel={() => setModalVisible(false)}
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
      {canAutoGrow && (
        <FormField label="自动扩容系统分区" tip="宿主机磁盘扩容成功后，通过 QEMU Guest Agent 扩展根分区及 ext4、XFS 或 Btrfs 文件系统">
          <Switch checked={autoGrow} checkedText="开" uncheckedText="关" onChange={setAutoGrow} />
        </FormField>
      )}
      {autoGrow && (
        <Banner
          type="warning"
          closeIcon={null}
          description="该操作将异步修改来宾系统分区。宿主机扩容成功而来宾阶段失败时，可从任务结果重试来宾阶段。"
        />
      )}
    </Modal>
  )
}
