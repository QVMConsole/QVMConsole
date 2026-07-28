/**
 * 创建/编辑安全组对话框
 * - 管理员可指定所属用户（编辑时不可改）
 * - 默认安全组名称不可改
 */
import { useState } from 'react'
import { Input, Modal, TextArea, Toast } from '@douyinfe/semi-ui'
import {
  createVPCSecurityGroup,
  updateVPCSecurityGroup,
  type VpcSecurityGroup,
} from '@/api/vpc'
import { useMountModalLifecycle } from '@/hooks/useMountModalLifecycle'

interface SecurityGroupDialogProps {
  row?: VpcSecurityGroup
  isAdmin: boolean
  defaultUsername: string
  onClose: () => void
  onSaved: () => void
}

export default function SecurityGroupDialog({
  row,
  isAdmin,
  defaultUsername,
  onClose,
  onSaved,
}: SecurityGroupDialogProps) {
  const { modalVisible, requestClose, afterModalClose } = useMountModalLifecycle(onClose)
  const editing = !!row
  const [submitting, setSubmitting] = useState(false)
  const [form, setForm] = useState({
    username: row?.username || defaultUsername || '',
    name: row?.name || '',
    remark: row?.remark || '',
  })

  const handleSubmit = async () => {
    if (!form.name.trim()) {
      Toast.warning('请输入安全组名称')
      return
    }
    setSubmitting(true)
    try {
      if (editing && row) {
        await updateVPCSecurityGroup(row.id, form)
        Toast.success('安全组已更新')
      } else {
        await createVPCSecurityGroup(form)
        Toast.success('安全组已创建')
      }
      onSaved()
      requestClose()
    } catch {
      // 请求层已提示
    } finally {
      setSubmitting(false)
    }
  }

  return (
    <Modal
      title={editing ? '编辑安全组' : '创建安全组'}
      visible={modalVisible}
      afterClose={afterModalClose}
      onCancel={requestClose}
      onOk={() => void handleSubmit()}
      okText="保存"
      cancelText="取消"
      confirmLoading={submitting}
      width={500}
      closeOnEsc
    >
      {isAdmin && (
        <div className="qvm-form-item">
          <div className="qvm-form-label">所属用户</div>
          <Input
            value={form.username}
            onChange={(v) => setForm((f) => ({ ...f, username: v }))}
            disabled={editing}
            placeholder="留空使用筛选用户或当前管理员"
          />
        </div>
      )}
      <div className="qvm-form-item">
        <div className="qvm-form-label required">名称</div>
        <Input
          value={form.name}
          onChange={(v) => setForm((f) => ({ ...f, name: v }))}
          disabled={!!row?.is_default}
          placeholder="请输入安全组名称"
        />
      </div>
      <div className="qvm-form-item">
        <div className="qvm-form-label">备注</div>
        <TextArea
          rows={2}
          value={form.remark}
          onChange={(v) => setForm((f) => ({ ...f, remark: v }))}
          placeholder="请输入备注信息"
        />
      </div>
    </Modal>
  )
}
