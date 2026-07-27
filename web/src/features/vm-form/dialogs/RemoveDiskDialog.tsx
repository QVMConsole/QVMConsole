/**
 * 删除磁盘弹窗（编辑模式）
 * 三种处理：连同文件删除 / 转移到我的存储 / 取消。
 */
import { useState } from 'react'
import { Banner, Button, Modal } from '@douyinfe/semi-ui'
import type { VmDiskItem } from '@/api/vm'
import type { VmEditDevices } from '../useVmEditDevices'

interface RemoveDiskDialogProps {
  visible: boolean
  disk: VmDiskItem | null
  devices: VmEditDevices
  onClose: () => void
}

export default function RemoveDiskDialog({ visible, disk, devices, onClose }: RemoveDiskDialogProps) {
  const [submitting, setSubmitting] = useState<'delete' | 'transfer' | ''>('')

  const handle = async (action: 'delete' | 'transfer') => {
    if (!disk) return
    setSubmitting(action)
    try {
      if (action === 'delete') {
        await devices.removeDiskAction(disk.device, true, false)
      } else {
        await devices.removeDiskAction(disk.device, false, true)
      }
      onClose()
    } catch {
      // 错误由请求层统一提示
    } finally {
      setSubmitting('')
    }
  }

  return (
    <Modal
      title="删除磁盘"
      visible={visible}
      onCancel={onClose}
      width={480}
      closeOnEsc
      footer={
        <>
          <Button onClick={onClose}>取消</Button>
          <Button
            type="primary"
            theme="solid"
            loading={submitting === 'transfer'}
            disabled={!!submitting && submitting !== 'transfer'}
            onClick={() => void handle('transfer')}
          >
            转移到我的存储
          </Button>
          <Button
            type="danger"
            theme="solid"
            loading={submitting === 'delete'}
            disabled={!!submitting && submitting !== 'delete'}
            onClick={() => void handle('delete')}
          >
            连同文件删除
          </Button>
        </>
      }
    >
      <Banner
        type="warning"
        closeIcon={null}
        description={`确定要删除磁盘 ${disk?.device || ''}（${disk?.path || '-'}）吗？可选择连同文件一起删除，或卸载后转移到「我的存储 - 虚拟磁盘」。`}
      />
    </Modal>
  )
}
