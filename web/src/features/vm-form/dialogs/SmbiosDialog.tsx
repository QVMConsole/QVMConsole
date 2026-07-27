/**
 * SMBIOS 配置弹窗（类型 1，创建 / 编辑共用）
 */
import { Banner, Button, Input, Modal } from '@douyinfe/semi-ui'
import { useVmFormScope } from '../scopeContext'
import { ADVANCED_HELP_TEXT } from '../constants'
import { createEmptySMBIOS1Config } from '../defaults'
import FormField from '../sections/FormField'
import TextSwitch from '../sections/TextSwitch'

interface SmbiosDialogProps {
  visible: boolean
  onClose: () => void
  /** 编辑模式传入当前虚拟机 UUID（只读展示） */
  currentVmUUID?: string
}

export default function SmbiosDialog({ visible, onClose, currentVmUUID }: SmbiosDialogProps) {
  const { form, ctx } = useVmFormScope()
  const { form: f, setField } = form
  const isEdit = ctx.mode === 'edit'

  const update = (key: keyof typeof f.smbios1, value: string | boolean) => {
    setField('smbios1', { ...f.smbios1, [key]: value })
  }

  const resetDefaults = () => {
    setField('smbios1', createEmptySMBIOS1Config())
  }

  return (
    <Modal
      title="SMBIOS 配置（类型 1）"
      visible={visible}
      onCancel={onClose}
      footer={
        <>
          <Button onClick={resetDefaults}>恢复默认</Button>
          <Button type="primary" theme="solid" onClick={onClose}>
            关闭
          </Button>
        </>
      }
      width={680}
      closeOnEsc
    >
      <Banner
        type="warning"
        closeIcon={null}
        style={{ marginBottom: 16 }}
        description="SMBIOS 用于向虚拟机暴露厂商、产品、序列号、UUID 等机器身份信息。一般保持默认，只有在授权绑定、资产识别或迁移兼容场景下才建议修改。"
      />
      <FormField
        label="Base64"
        help={ADVANCED_HELP_TEXT.smbiosBase64}
        tip="启用后，下面填写的 SMBIOS 字段会先按 Base64 解码，再写入虚拟机定义。回显始终显示解码后的实际值。"
      >
        <TextSwitch checked={f.smbios1.base64} onChange={(v) => update('base64', v)} />
      </FormField>
      <div className="qvm-vf-grid-2">
        <FormField label="厂商名">
          <Input value={f.smbios1.manufacturer} onChange={(v) => update('manufacturer', v)} placeholder="例如 QEMU" showClear />
        </FormField>
        <FormField label="产品 ID">
          <Input value={f.smbios1.product} onChange={(v) => update('product', v)} placeholder="例如 Standard PC" showClear />
        </FormField>
        <FormField label="版本">
          <Input value={f.smbios1.version} onChange={(v) => update('version', v)} placeholder="例如 1.0" showClear />
        </FormField>
        <FormField label="序列号">
          <Input value={f.smbios1.serial} onChange={(v) => update('serial', v)} placeholder="例如 SN-001" showClear />
        </FormField>
        <FormField label="SKU">
          <Input value={f.smbios1.sku} onChange={(v) => update('sku', v)} placeholder="例如 SKU-001" showClear />
        </FormField>
        <FormField label="家族名称">
          <Input value={f.smbios1.family} onChange={(v) => update('family', v)} placeholder="例如 Virtual Machine" showClear />
        </FormField>
      </div>
      <FormField
        label="UUID"
        help={ADVANCED_HELP_TEXT.smbiosUUID}
        tip={
          isEdit
            ? '已存在虚拟机的 SMBIOS UUID 必须与当前虚拟机 UUID 保持一致，因此编辑页仅支持查看，不支持直接修改。'
            : '创建、导入或克隆时可选填标准 UUID。若填写，系统会同时使用该 UUID 作为虚拟机 UUID，避免 libvirt 拒绝定义。'
        }
      >
        {isEdit ? (
          <Input value={currentVmUUID || f.smbios1.uuid || ''} disabled />
        ) : (
          <Input
            value={f.smbios1.uuid}
            onChange={(v) => update('uuid', v)}
            placeholder="留空时由系统自动生成"
            showClear
          />
        )}
      </FormField>
    </Modal>
  )
}
