/**
 * 编辑虚拟机备注弹窗
 */
import { useState } from 'react'
import { Input, Modal, TextArea, Toast } from '@douyinfe/semi-ui'
import type { VmListItem } from '@/api/vm'
import { updateVm } from '@/api/vm'
import { useMountModalLifecycle } from '@/hooks/useMountModalLifecycle'

interface VmRemarkDialogProps {
  vm: VmListItem
  onClose: () => void
  onSuccess: (name: string, remark: string) => void
}

export default function VmRemarkDialog({ vm, onClose, onSuccess }: VmRemarkDialogProps) {
  const { modalVisible, requestClose, afterModalClose } = useMountModalLifecycle(onClose)
  const [remark, setRemark] = useState(vm.remark || '')
  const [submitting, setSubmitting] = useState(false)

  const handleSubmit = async () => {
    setSubmitting(true)
    try {
      const nextRemark = remark.trim()
      const res = await updateVm(vm.name, { remark: nextRemark })
      Toast.success(res.message || '备注已更新')
      onSuccess(vm.name, nextRemark)
      requestClose()
    } catch {
      // 错误提示由请求层统一处理
    } finally {
      setSubmitting(false)
    }
  }

  return (
    <Modal
      title={`编辑备注 - ${vm.name}`}
      visible={modalVisible}
      afterClose={afterModalClose}
      onCancel={requestClose}
      onOk={() => void handleSubmit()}
      okText="保存备注"
      cancelText="取消"
      confirmLoading={submitting}
      width={520}
      closeOnEsc
    >
      <div className="qvm-form-item">
        <div className="qvm-form-label">虚拟机</div>
        <Input value={vm.name} disabled />
      </div>
      <div className="qvm-form-item">
        <div className="qvm-form-label">备注</div>
        <TextArea
          value={remark}
          onChange={setRemark}
          rows={4}
          maxCount={200}
          placeholder="用于记录用途、环境或业务信息；留空保存将清空备注"
        />
      </div>
    </Modal>
  )
}
