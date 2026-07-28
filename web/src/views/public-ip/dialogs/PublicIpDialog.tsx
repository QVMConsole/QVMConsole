/**
 * 新增/编辑公网 IP 对话框
 * - 支持模式多选（1:1 NAT / 经典网络-路由 / 经典网络-桥接）
 * - 已绑定的公网 IP 不允许修改 IP 地址与状态（后端强制）
 */
import { useState } from 'react'
import { Checkbox, Input, Modal, Select, TextArea, Toast } from '@douyinfe/semi-ui'
import {
  createPublicIP,
  updatePublicIP,
  type PublicIpItem,
  type PublicIpMode,
} from '@/api/publicIp'
import { ALL_PUBLIC_IP_MODES, publicIpModeLabel } from '../utils'
import { useMountModalLifecycle } from '@/hooks/useMountModalLifecycle'

interface PublicIpDialogProps {
  row?: PublicIpItem
  onClose: () => void
  onSaved: () => void
}

export default function PublicIpDialog({ row, onClose, onSaved }: PublicIpDialogProps) {
  const { modalVisible, requestClose, afterModalClose } = useMountModalLifecycle(onClose)
  const editing = !!row
  const bound = !!row?.binding
  const [submitting, setSubmitting] = useState(false)
  const [form, setForm] = useState({
    ip: row?.ip || '',
    cidr: row?.cidr || '',
    gateway: row?.gateway || '',
    uplink_if: row?.uplink_if || '',
    modes: (row?.modes?.length ? row.modes : ALL_PUBLIC_IP_MODES) as PublicIpMode[],
    status: row?.status === 'reserved' ? 'reserved' : 'free',
    remark: row?.remark || '',
  })

  const patch = (p: Partial<typeof form>) => setForm((f) => ({ ...f, ...p }))

  const handleSubmit = async () => {
    if (!form.ip.trim()) {
      Toast.warning('请输入公网 IP')
      return
    }
    if (form.modes.length === 0) {
      Toast.warning('请至少选择一种支持模式')
      return
    }
    setSubmitting(true)
    try {
      const payload = {
        ip: form.ip.trim(),
        cidr: form.cidr.trim(),
        gateway: form.gateway.trim(),
        uplink_if: form.uplink_if.trim(),
        supported_modes: form.modes.join(','),
        status: form.status,
        remark: form.remark,
      }
      if (editing && row) {
        await updatePublicIP(row.id, payload)
      } else {
        await createPublicIP(payload)
      }
      Toast.success('公网 IP 已保存')
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
      title={editing ? '编辑公网 IP' : '新增公网 IP'}
      visible={modalVisible}
      afterClose={afterModalClose}
      onCancel={requestClose}
      onOk={() => void handleSubmit()}
      okText="保存"
      cancelText="取消"
      confirmLoading={submitting}
      width={560}
      closeOnEsc
    >
      <div className="qvm-form-item">
        <div className="qvm-form-label required">公网 IP</div>
        <Input
          value={form.ip}
          onChange={(v) => patch({ ip: v })}
          disabled={bound}
          placeholder="例如 203.0.113.10"
        />
        {bound && <div className="qvm-form-tip warn">公网 IP 已绑定，不能修改 IP 地址</div>}
      </div>
      <div className="qvm-form-item">
        <div className="qvm-form-label">CIDR/掩码</div>
        <Input
          value={form.cidr}
          onChange={(v) => patch({ cidr: v })}
          placeholder="例如 203.0.113.10/32 或 203.0.113.0/29"
        />
      </div>
      <div className="qvm-form-item">
        <div className="qvm-form-label">网关</div>
        <Input
          value={form.gateway}
          onChange={(v) => patch({ gateway: v })}
          placeholder="经典网络给 VM 使用的网关"
        />
      </div>
      <div className="qvm-form-item">
        <div className="qvm-form-label">出口网卡</div>
        <Input
          value={form.uplink_if}
          onChange={(v) => patch({ uplink_if: v })}
          placeholder="留空时自动检测默认出口"
        />
      </div>
      <div className="qvm-form-item">
        <div className="qvm-form-label required">支持模式</div>
        <Checkbox.Group
          value={form.modes}
          onChange={(v) => patch({ modes: v as PublicIpMode[] })}
        >
          {ALL_PUBLIC_IP_MODES.map((mode) => (
            <Checkbox key={mode} value={mode}>
              {publicIpModeLabel(mode)}
            </Checkbox>
          ))}
        </Checkbox.Group>
      </div>
      <div className="qvm-form-item">
        <div className="qvm-form-label">状态</div>
        <Select
          value={form.status}
          onChange={(v) => patch({ status: v as string })}
          disabled={bound}
          style={{ width: '100%' }}
          optionList={[
            { label: '空闲', value: 'free' },
            { label: '保留', value: 'reserved' },
          ]}
        />
        {bound && <div className="qvm-form-tip">已绑定状态下由系统维护，不可手动调整</div>}
      </div>
      <div className="qvm-form-item">
        <div className="qvm-form-label">备注</div>
        <TextArea
          rows={2}
          value={form.remark}
          onChange={(v) => patch({ remark: v })}
          placeholder="请输入备注信息"
        />
      </div>
    </Modal>
  )
}
