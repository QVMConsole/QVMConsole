/**
 * 添加/编辑宿主机防火墙规则对话框
 * - 协议支持 TCP / UDP / TCP+UDP
 * - 端口区间 1-65535，结束端口不小于起始端口
 * - 创建/更新为高风险操作（428 二次验证由请求层处理）
 */
import { useState } from 'react'
import { Col, Input, InputNumber, Modal, Radio, Row, Select, Toast } from '@douyinfe/semi-ui'
import {
  createHostFirewallRule,
  updateHostFirewallRule,
  type HostFirewallRule,
} from '@/api/firewall'
import { createDefaultRule, normalizeRulePayload } from '../utils'
import { useMountModalLifecycle } from '@/hooks/useMountModalLifecycle'

interface HostRuleDialogProps {
  /** 编辑时传入规则行，新增时不传 */
  row?: HostFirewallRule
  onClose: () => void
  onSaved: () => void
}

export default function HostRuleDialog({ row, onClose, onSaved }: HostRuleDialogProps) {
  const { modalVisible, requestClose, afterModalClose } = useMountModalLifecycle(onClose)
  const editing = !!row?.id
  const [submitting, setSubmitting] = useState(false)
  const [form, setForm] = useState(() => ({
    ...createDefaultRule(),
    ...(row ? normalizeRulePayload(row) : {}),
  }))

  const patch = (p: Partial<typeof form>) => setForm((f) => ({ ...f, ...p }))

  const handleSubmit = async () => {
    if (form.port_start && form.port_end && form.port_end < form.port_start) {
      Toast.warning('结束端口不能小于起始端口')
      return
    }
    setSubmitting(true)
    try {
      const payload = normalizeRulePayload(form)
      if (editing && row) {
        await updateHostFirewallRule(row.id, payload)
        Toast.success('宿主机防火墙规则已更新')
      } else {
        await createHostFirewallRule(payload)
        Toast.success('宿主机防火墙规则已添加')
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
      title={editing ? '编辑宿主机规则' : '添加宿主机规则'}
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
        <div className="qvm-form-label">动作</div>
        <Radio.Group
          type="button"
          value={form.action}
          onChange={(e) => patch({ action: e.target.value as string })}
        >
          <Radio value="allow">允许</Radio>
          <Radio value="deny">拒绝</Radio>
        </Radio.Group>
      </div>
      <div className="qvm-form-item">
        <div className="qvm-form-label">协议</div>
        <Select
          value={form.protocol}
          onChange={(v) => patch({ protocol: v as string })}
          style={{ width: '100%' }}
          optionList={[
            { label: 'TCP', value: 'tcp' },
            { label: 'UDP', value: 'udp' },
            { label: 'TCP + UDP', value: 'both' },
          ]}
        />
      </div>
      <Row gutter={16}>
        <Col span={12}>
          <div className="qvm-form-item">
            <div className="qvm-form-label">起始端口</div>
            <InputNumber
              value={form.port_start ?? undefined}
              min={1}
              max={65535}
              onChange={(v) => patch({ port_start: Number(v) || null })}
              style={{ width: '100%' }}
            />
          </div>
        </Col>
        <Col span={12}>
          <div className="qvm-form-item">
            <div className="qvm-form-label">结束端口</div>
            <InputNumber
              value={form.port_end ?? undefined}
              min={form.port_start || 1}
              max={65535}
              onChange={(v) => patch({ port_end: Number(v) || null })}
              style={{ width: '100%' }}
            />
          </div>
        </Col>
      </Row>
      <div className="qvm-form-item">
        <div className="qvm-form-label">来源 CIDR</div>
        <Input
          value={form.source_cidr || ''}
          onChange={(v) => patch({ source_cidr: v })}
          placeholder="留空表示 any，例如 203.0.113.0/24"
        />
      </div>
      <div className="qvm-form-item">
        <div className="qvm-form-label">备注</div>
        <Input
          value={form.comment || ''}
          onChange={(v) => patch({ comment: v })}
          placeholder="规则说明"
        />
      </div>
    </Modal>
  )
}
