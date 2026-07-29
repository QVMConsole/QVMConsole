/**
 * 配置存储池弹窗
 * - 修改显示名称与启用状态（启用 = 允许用户创建虚拟机到此硬盘）
 * - 不可用于虚拟机的盘禁止开启启用开关，并展示原因
 */
import { useState } from 'react'
import { Input, Modal, Toast } from '@douyinfe/semi-ui'
import { IconInfoCircle } from '@douyinfe/semi-icons'
import type { HostStoragePoolInfo } from '@/api/storagePool'
import { updateStoragePoolConfig } from '@/api/storagePool'
import TextSwitch from '@/features/vm-form/sections/TextSwitch'
import { useMountModalLifecycle } from '@/hooks/useMountModalLifecycle'

interface ConfigDialogProps {
  row: HostStoragePoolInfo
  onClose: () => void
  onSaved: () => void
}

export default function ConfigDialog({ row, onClose, onSaved }: ConfigDialogProps) {
  const { modalVisible, requestClose, afterModalClose } = useMountModalLifecycle(onClose)
  const [displayName, setDisplayName] = useState(row.display_name || '')
  const [enabled, setEnabled] = useState(!!row.enabled)
  const [saving, setSaving] = useState(false)

  const handleSave = async () => {
    setSaving(true)
    try {
      await updateStoragePoolConfig(row.id, { display_name: displayName, enabled })
      Toast.success('存储池配置已保存')
      onSaved()
      requestClose()
    } catch {
      // 错误提示由请求层统一处理
    } finally {
      setSaving(false)
    }
  }

  return (
    <Modal
      title="配置存储池"
      visible={modalVisible}
      afterClose={afterModalClose}
      onCancel={requestClose}
      onOk={() => void handleSave()}
      okText="保存"
      cancelText="取消"
      okButtonProps={{ loading: saving }}
      width={520}
      closeOnEsc
    >
      <div className="qvm-form-item">
        <div className="qvm-form-label">设备</div>
        <Input value={row.device_path} disabled />
      </div>
      <div className="qvm-form-item">
        <div className="qvm-form-label">显示名称</div>
        <Input
          value={displayName}
          onChange={setDisplayName}
          placeholder="请输入用户侧显示名称"
          showClear
        />
      </div>
      <div className="qvm-form-item">
        <div className="qvm-form-label">启用</div>
        <TextSwitch
          checked={enabled}
          onChange={setEnabled}
          checkedText="允"
          uncheckedText="禁"
          disabled={!row.can_use_for_vm}
        />
        {!row.can_use_for_vm && (
          <div className="qvm-form-tip warn">
            <IconInfoCircle size="small" style={{ marginRight: 4, verticalAlign: -2 }} />
            {row.status_reason || '该硬盘当前不可用于虚拟机存储'}
          </div>
        )}
      </div>
    </Modal>
  )
}
