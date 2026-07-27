/**
 * 网口编辑弹窗（编辑模式 · 仅管理员）
 * 添加 / 编辑虚拟机网口：网卡型号 + VPC 交换机 + 安全组 + 上下行速率限制。
 */
import { useEffect, useMemo, useState } from 'react'
import { Divider, InputNumber, Modal, Select, Tag, Toast } from '@douyinfe/semi-ui'
import {
  addVMInterface,
  updateVMInterface,
  type VMInterfaceInfo,
} from '@/api/vpc'
import { useVmFormScope } from '../scopeContext'
import { NIC_MODEL_OPTIONS } from '../constants'
import FormField from '../sections/FormField'

interface NicEditDialogProps {
  visible: boolean
  vmName: string
  /** 编辑模式传入网口信息；添加模式传 null */
  editing: VMInterfaceInfo | null
  onClose: () => void
  onSaved: () => void | Promise<void>
}

export default function NicEditDialog({ visible, vmName, editing, onClose, onSaved }: NicEditDialogProps) {
  const { options } = useVmFormScope()
  const [nicModel, setNicModel] = useState('virtio')
  const [switchId, setSwitchId] = useState<number | null>(null)
  const [securityGroupId, setSecurityGroupId] = useState<number | null>(null)
  const [bandwidthIn, setBandwidthIn] = useState(0)
  const [bandwidthOut, setBandwidthOut] = useState(0)
  const [submitting, setSubmitting] = useState(false)

  useEffect(() => {
    if (!visible) return
    void options.loadVPCOptions()
    if (editing) {
      setNicModel(editing.binding?.nic_model || 'virtio')
      setSwitchId(editing.binding?.switch_id || editing.switch?.id || null)
      setSecurityGroupId(editing.binding?.security_group_id || editing.security_group?.id || null)
      setBandwidthIn(editing.binding?.bandwidth_inbound_avg || 0)
      setBandwidthOut(editing.binding?.bandwidth_outbound_avg || 0)
    } else {
      setNicModel('virtio')
      setSwitchId(options.vpcSwitches[0]?.id ?? null)
      setSecurityGroupId(null)
      setBandwidthIn(0)
      setBandwidthOut(0)
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [visible, editing])

  const selectedSwitch = useMemo(
    () => options.vpcSwitches.find((item) => item.id === switchId) || null,
    [options.vpcSwitches, switchId],
  )
  const isBridge = selectedSwitch?.bridge_mode === 'bridge'

  // 切换交换机时：桥接直通清空安全组；安全组不属于新交换机用户时清空
  const handleSwitchChange = (value: unknown) => {
    const id = Number(value)
    setSwitchId(id)
    const sw = options.vpcSwitches.find((item) => item.id === id)
    if (sw?.bridge_mode === 'bridge') {
      setSecurityGroupId(null)
      return
    }
    const currentGroup = options.vpcSecurityGroups.find((g) => g.id === securityGroupId)
    if (currentGroup && sw?.username && currentGroup.username !== sw.username) {
      setSecurityGroupId(null)
    }
  }

  const handleSubmit = async () => {
    if (!switchId) {
      Toast.warning('请选择 VPC 交换机')
      return
    }
    setSubmitting(true)
    try {
      const payload = {
        nic_model: nicModel,
        switch_id: switchId,
        security_group_id: securityGroupId || 0,
        bandwidth_inbound_avg: bandwidthIn || 0,
        bandwidth_outbound_avg: bandwidthOut || 0,
      }
      if (editing) {
        await updateVMInterface(vmName, editing.binding?.interface_order ?? 0, payload)
        Toast.success('网口已更新')
      } else {
        await addVMInterface(vmName, payload)
        Toast.success('网口已添加')
      }
      onClose()
      await onSaved()
    } catch {
      // 错误由请求层统一提示
    } finally {
      setSubmitting(false)
    }
  }

  return (
    <Modal
      title={editing ? '编辑网口' : '添加网口'}
      visible={visible}
      onCancel={onClose}
      onOk={() => void handleSubmit()}
      okText={editing ? '保存' : '确定添加'}
      cancelText="取消"
      confirmLoading={submitting}
      width={500}
      closeOnEsc
    >
      <FormField label="网卡型号">
        <Select
          style={{ width: '100%' }}
          value={nicModel}
          onChange={(v) => setNicModel(v as string)}
          optionList={NIC_MODEL_OPTIONS.map((item) => ({ value: item.value, label: item.label }))}
        />
      </FormField>
      <FormField label="VPC 交换机" required>
        <Select
          style={{ width: '100%' }}
          value={switchId ?? undefined}
          placeholder="选择交换机"
          filter
          onChange={handleSwitchChange}
        >
          {options.vpcSwitches.map((item) => (
            <Select.Option key={item.id} value={item.id}>
              <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', gap: 8 }}>
                <span>{item.name}</span>
                <Tag size="small" color={item.bridge_mode === 'bridge' ? 'orange' : 'blue'}>
                  {item.bridge_mode === 'bridge'
                    ? `${item.bridge_name || '桥接'}${item.bridge_vlan_id > 0 ? ` / VLAN ${item.bridge_vlan_id}` : ''}`
                    : item.cidr}
                </Tag>
              </div>
            </Select.Option>
          ))}
        </Select>
        {isBridge && <div className="qvm-vf-tip">桥接直通由上级路由器分配 IP，不使用内部 DHCP 和安全组</div>}
      </FormField>
      {!isBridge && (
        <FormField label="安全组" tip="不选则使用该交换机用户默认安全组">
          <Select
            style={{ width: '100%' }}
            value={securityGroupId ?? undefined}
            placeholder="选择安全组（可选）"
            filter
            showClear
            onChange={(v) => setSecurityGroupId(v === undefined ? null : Number(v))}
            optionList={options.vpcSecurityGroups.map((item) => ({
              value: item.id,
              label: item.is_default ? `${item.name}（默认）` : item.name,
            }))}
          />
        </FormField>
      )}
      <Divider margin="12px">速率限制</Divider>
      <div className="qvm-vf-grid-2">
        <FormField label="下行速率 (Mbps)">
          <InputNumber
            style={{ width: '100%' }}
            value={bandwidthIn}
            min={0}
            max={100000}
            onChange={(v) => setBandwidthIn(Number(v || 0))}
          />
        </FormField>
        <FormField label="上行速率 (Mbps)">
          <InputNumber
            style={{ width: '100%' }}
            value={bandwidthOut}
            min={0}
            max={100000}
            onChange={(v) => setBandwidthOut(Number(v || 0))}
          />
        </FormField>
      </div>
      <div className="qvm-vf-tip">0 表示不限制，设置后通过 libvirt domiftune 对该网口生效</div>
    </Modal>
  )
}
