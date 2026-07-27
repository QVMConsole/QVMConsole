/**
 * 磁盘 IOPS 设置弹窗（通用受控组件，创建 / 编辑共用）
 */
import { useEffect, useState } from 'react'
import { Banner, InputNumber, Modal } from '@douyinfe/semi-ui'
import FormField from '../sections/FormField'

export interface DiskIopsValues {
  total: number
  read: number
  write: number
}

interface DiskIopsDialogProps {
  visible: boolean
  /** 标题副文本（磁盘描述） */
  subtitle?: string
  initial: DiskIopsValues
  onApply: (values: DiskIopsValues) => void
  onClose: () => void
}

export default function DiskIopsDialog({ visible, subtitle, initial, onApply, onClose }: DiskIopsDialogProps) {
  const [values, setValues] = useState<DiskIopsValues>(initial)

  useEffect(() => {
    if (visible) setValues(initial)
  }, [visible, initial])

  const handleOk = () => {
    onApply(values)
    onClose()
  }

  return (
    <Modal
      title="设置磁盘 IOPS 限制"
      visible={visible}
      onCancel={onClose}
      onOk={handleOk}
      okText="保存 IOPS 设置"
      cancelText="取消"
      width={480}
      closeOnEsc
    >
      <Banner
        type="warning"
        closeIcon={null}
        style={{ marginBottom: 12 }}
        description="总 IOPS 与 读/写 IOPS 互斥，请只设置其中一组"
      />
      {subtitle && <div className="qvm-vf-dialog-subtitle">{subtitle}</div>}
      <FormField label="总 IOPS" tip="磁盘每秒总 I/O 操作数限制，设置后将忽略读/写 IOPS">
        <InputNumber
          style={{ width: '100%' }}
          value={values.total}
          min={0}
          step={100}
          placeholder="0 表示不限制"
          onChange={(v) => setValues((prev) => ({ ...prev, total: Number(v || 0) }))}
        />
      </FormField>
      <FormField label="读 IOPS" tip="磁盘每秒读取操作数限制（总 IOPS > 0 时无效）">
        <InputNumber
          style={{ width: '100%' }}
          value={values.read}
          min={0}
          step={100}
          placeholder="0 表示不限制"
          disabled={values.total > 0}
          onChange={(v) => setValues((prev) => ({ ...prev, read: Number(v || 0) }))}
        />
      </FormField>
      <FormField label="写 IOPS" tip="磁盘每秒写入操作数限制（总 IOPS > 0 时无效）">
        <InputNumber
          style={{ width: '100%' }}
          value={values.write}
          min={0}
          step={100}
          placeholder="0 表示不限制"
          disabled={values.total > 0}
          onChange={(v) => setValues((prev) => ({ ...prev, write: Number(v || 0) }))}
        />
      </FormField>
    </Modal>
  )
}
