/**
 * 添加安全组规则对话框
 * - 方向 / 协议 / 端口（支持单端口、范围、全端口）
 * - 目标类型：CIDR/IP、指定交换机、指定安全组（仅允许选择当前用户可见资源）
 */
import { useMemo, useState } from 'react'
import { Checkbox, Input, Modal, Select, TextArea, Toast } from '@douyinfe/semi-ui'
import { addVPCSecurityGroupRule, type VpcSecurityGroup, type VpcSwitch } from '@/api/vpc'
import { useMountModalLifecycle } from '@/hooks/useMountModalLifecycle'

interface RuleDialogProps {
  group: VpcSecurityGroup
  switches: VpcSwitch[]
  securityGroups: VpcSecurityGroup[]
  onClose: () => void
  onSaved: () => void
}

interface RuleFormState {
  direction: string
  protocol: string
  port_text: string
  port_all: boolean
  target_type: string
  target_value: string
  remark: string
}

const INITIAL_FORM: RuleFormState = {
  direction: 'ingress',
  protocol: 'tcp',
  port_text: '',
  port_all: false,
  target_type: 'cidr',
  target_value: '0.0.0.0/0',
  remark: '',
}

export default function RuleDialog({ group, switches, securityGroups, onClose, onSaved }: RuleDialogProps) {
  const { modalVisible, requestClose, afterModalClose } = useMountModalLifecycle(onClose)
  const [submitting, setSubmitting] = useState(false)
  const [form, setForm] = useState<RuleFormState>(INITIAL_FORM)

  const patch = (p: Partial<RuleFormState>) => setForm((f) => ({ ...f, ...p }))

  const switchOptions = useMemo(
    () => switches.map((s) => ({ value: String(s.id), label: `${s.name} (${s.cidr})` })),
    [switches],
  )
  const groupOptions = useMemo(
    () => securityGroups.map((g) => ({ value: String(g.id), label: g.name })),
    [securityGroups],
  )

  const targetHelp =
    form.target_type === 'cidr'
      ? '支持 IPv4 地址或 CIDR 格式，如 0.0.0.0/0 表示所有'
      : form.target_type === 'switch'
        ? '选择当前用户可访问的交换机'
        : '选择当前用户拥有的安全组'

  const handleTargetTypeChange = (type: string) => {
    if (type === 'cidr') {
      patch({ target_type: type, target_value: '0.0.0.0/0' })
      return
    }
    const first = type === 'switch' ? switchOptions[0]?.value : groupOptions[0]?.value
    patch({ target_type: type, target_value: first || '' })
  }

  const handleSubmit = async () => {
    // 解析端口
    let port_start = 0
    let port_end = 0
    if (form.port_all) {
      if (form.protocol === 'icmp' || form.protocol === 'all') {
        port_start = 0
        port_end = 0
      } else {
        port_start = 1
        port_end = 65535
      }
    } else if (form.port_text) {
      const parts = form.port_text.split('-')
      port_start = parseInt(parts[0]) || 0
      port_end = parts.length > 1 ? parseInt(parts[1]) || 65535 : port_start
    }

    let targetValue = form.target_value
    if (form.target_type !== 'cidr' && !targetValue) {
      const first = form.target_type === 'switch' ? switchOptions[0]?.value : groupOptions[0]?.value
      targetValue = first || ''
    }
    if (!targetValue) {
      Toast.warning(form.target_type === 'cidr' ? '请填写 CIDR/IP' : '请选择目标值')
      return
    }
    if (
      form.target_type === 'switch' &&
      !switchOptions.some((o) => o.value === String(targetValue))
    ) {
      Toast.warning('请选择当前用户可用的交换机')
      return
    }
    if (
      form.target_type === 'security_group' &&
      !groupOptions.some((o) => o.value === String(targetValue))
    ) {
      Toast.warning('请选择当前用户可用的安全组')
      return
    }

    setSubmitting(true)
    try {
      await addVPCSecurityGroupRule(group.id, {
        direction: form.direction,
        protocol: form.protocol,
        port_start,
        port_end,
        target_type: form.target_type,
        target_value: targetValue,
        remark: form.remark,
      })
      Toast.success('规则已添加')
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
      title={`添加规则 — ${group.name}`}
      visible={modalVisible}
      afterClose={afterModalClose}
      onCancel={requestClose}
      onOk={() => void handleSubmit()}
      okText="保存"
      cancelText="取消"
      confirmLoading={submitting}
      width={600}
      closeOnEsc
    >
      <div style={{ display: 'grid', gridTemplateColumns: '1fr 1fr', gap: '0 16px' }}>
        <div className="qvm-form-item">
          <div className="qvm-form-label">方向</div>
          <Select
            style={{ width: '100%' }}
            value={form.direction}
            onChange={(v) => patch({ direction: String(v) })}
            optionList={[
              { value: 'ingress', label: '入站' },
              { value: 'egress', label: '出站' },
            ]}
          />
        </div>
        <div className="qvm-form-item">
          <div className="qvm-form-label">协议</div>
          <Select
            style={{ width: '100%' }}
            value={form.protocol}
            onChange={(v) => patch({ protocol: String(v) })}
            optionList={[
              { value: 'tcp', label: 'TCP' },
              { value: 'udp', label: 'UDP' },
              { value: 'icmp', label: 'ICMP' },
              { value: 'all', label: '全部' },
            ]}
          />
        </div>
      </div>

      <div className="qvm-form-item">
        <div className="qvm-form-label">端口</div>
        <div style={{ display: 'flex', alignItems: 'center', gap: 12 }}>
          <Input
            style={{ flex: 1 }}
            value={form.port_text}
            onChange={(v) => patch({ port_text: v })}
            disabled={form.port_all}
            placeholder="例如 80 或 80-90"
          />
          <Checkbox checked={form.port_all} onChange={(e) => patch({ port_all: !!e.target.checked })}>
            全端口
          </Checkbox>
        </div>
      </div>

      <div className="qvm-form-item">
        <div className="qvm-form-label">目标类型</div>
        <Select
          style={{ width: '100%' }}
          value={form.target_type}
          onChange={(v) => handleTargetTypeChange(String(v))}
          optionList={[
            { value: 'cidr', label: 'CIDR / IP 地址' },
            { value: 'switch', label: '指定交换机' },
            { value: 'security_group', label: '指定安全组' },
          ]}
        />
      </div>

      <div className="qvm-form-item">
        <div className="qvm-form-label">目标值</div>
        {form.target_type === 'cidr' && (
          <Input
            value={form.target_value}
            onChange={(v) => patch({ target_value: v })}
            placeholder="例如 0.0.0.0/0、192.168.1.10 或 10.200.1.0/24"
          />
        )}
        {form.target_type === 'switch' && (
          <Select
            style={{ width: '100%' }}
            filter
            placeholder="选择允许访问的交换机"
            emptyContent="当前用户没有可选交换机"
            value={form.target_value}
            onChange={(v) => patch({ target_value: String(v || '') })}
            optionList={switchOptions}
          />
        )}
        {form.target_type === 'security_group' && (
          <Select
            style={{ width: '100%' }}
            filter
            placeholder="选择允许访问的安全组"
            emptyContent="当前用户没有可选安全组"
            value={form.target_value}
            onChange={(v) => patch({ target_value: String(v || '') })}
            optionList={groupOptions}
          />
        )}
        <div className="qvm-form-tip">{targetHelp}</div>
      </div>

      <div className="qvm-form-item">
        <div className="qvm-form-label">备注</div>
        <TextArea
          rows={2}
          value={form.remark}
          onChange={(v) => patch({ remark: v })}
          placeholder="规则说明"
        />
      </div>
    </Modal>
  )
}
