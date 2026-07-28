/**
 * 导入区域 CIDR 对话框
 * - 本地导入 GeoIP 区域数据（代码/名称/来源/CIDR 列表）
 * - CIDR 列表为每行一个 IPv4 CIDR 的原始文本，由后端解析
 */
import { useState } from 'react'
import { Input, Modal, TextArea, Toast } from '@douyinfe/semi-ui'
import { importFirewallRegion } from '@/api/firewall'
import { useMountModalLifecycle } from '@/hooks/useMountModalLifecycle'

interface ImportRegionDialogProps {
  onClose: () => void
  onSaved: () => void
}

export default function ImportRegionDialog({ onClose, onSaved }: ImportRegionDialogProps) {
  const { modalVisible, requestClose, afterModalClose } = useMountModalLifecycle(onClose)
  const [submitting, setSubmitting] = useState(false)
  const [form, setForm] = useState({ code: '', name: '', source: 'local-import', cidrs: '' })

  const patch = (p: Partial<typeof form>) => setForm((f) => ({ ...f, ...p }))

  const handleSubmit = async () => {
    if (!form.code.trim()) {
      Toast.warning('请输入区域代码')
      return
    }
    if (!form.cidrs.trim()) {
      Toast.warning('请输入 CIDR 列表')
      return
    }
    setSubmitting(true)
    try {
      await importFirewallRegion({
        code: form.code.trim(),
        name: form.name.trim(),
        source: form.source.trim() || 'local-import',
        cidrs: form.cidrs,
      })
      Toast.success('区域 CIDR 已导入')
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
      title="导入区域 CIDR"
      visible={modalVisible}
      afterClose={afterModalClose}
      onCancel={requestClose}
      onOk={() => void handleSubmit()}
      okText="导入"
      cancelText="取消"
      confirmLoading={submitting}
      width={620}
      closeOnEsc
    >
      <div className="qvm-form-item">
        <div className="qvm-form-label required">区域代码</div>
        <Input
          value={form.code}
          onChange={(v) => patch({ code: v })}
          placeholder="如 cn"
        />
      </div>
      <div className="qvm-form-item">
        <div className="qvm-form-label">区域名称</div>
        <Input
          value={form.name}
          onChange={(v) => patch({ name: v })}
          placeholder="如 中国大陆"
        />
      </div>
      <div className="qvm-form-item">
        <div className="qvm-form-label">来源说明</div>
        <Input
          value={form.source}
          onChange={(v) => patch({ source: v })}
          placeholder="local-import"
        />
      </div>
      <div className="qvm-form-item">
        <div className="qvm-form-label required">CIDR 列表</div>
        <TextArea
          rows={10}
          value={form.cidrs}
          onChange={(v) => patch({ cidrs: v })}
          placeholder="每行一个 IPv4 CIDR"
        />
      </div>
    </Modal>
  )
}
