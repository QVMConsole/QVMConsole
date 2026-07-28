/**
 * 编辑虚拟机分组弹窗（可选择已有分组或输入新分组）
 */
import { useState } from 'react'
import { Input, Modal, Select, Toast } from '@douyinfe/semi-ui'
import type { VmListItem } from '@/api/vm'
import { updateVm } from '@/api/vm'
import { useMountModalLifecycle } from '@/hooks/useMountModalLifecycle'

interface VmGroupDialogProps {
  vm: VmListItem
  /** 现有全部分组（候选项） */
  groups: string[]
  onClose: () => void
  onSuccess: (name: string, group: string) => void
}

export default function VmGroupDialog({ vm, groups, onClose, onSuccess }: VmGroupDialogProps) {
  const { modalVisible, requestClose, afterModalClose } = useMountModalLifecycle(onClose)
  const [groupName, setGroupName] = useState(vm.group || '')
  const [submitting, setSubmitting] = useState(false)

  const handleSubmit = async () => {
    setSubmitting(true)
    try {
      const nextGroup = (groupName || '').trim()
      const res = await updateVm(vm.name, { group: nextGroup })
      Toast.success(res.message || '分组已更新')
      onSuccess(vm.name, nextGroup)
      requestClose()
    } catch {
      // 错误提示由请求层统一处理
    } finally {
      setSubmitting(false)
    }
  }

  return (
    <Modal
      title={`编辑分组 - ${vm.name}`}
      visible={modalVisible}
      afterClose={afterModalClose}
      onCancel={requestClose}
      onOk={() => void handleSubmit()}
      okText="保存分组"
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
        <div className="qvm-form-label">分组名称</div>
        <Select
          style={{ width: '100%' }}
          value={groupName || undefined}
          onChange={(value) => setGroupName((value as string) || '')}
          filter
          allowCreate
          showClear
          placeholder="输入或选择分组名称，留空则取消分组"
          optionList={groups.map((g) => ({ label: g, value: g }))}
        />
      </div>
    </Modal>
  )
}
