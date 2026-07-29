/**
 * 创建/编辑交换机对话框
 * - 管理员可指定所属用户与目标网桥；选中桥接网桥时隐藏网段配置，显示桥接 VLAN 与桥接安全
 * - 网段/网关创建后不可修改；留空自动分配
 * - 流量/带宽配额按用户剩余配额动态限制上下限（编辑时可为负表示归还配额）
 */
import { useEffect, useMemo, useState } from 'react'
import { Input, InputNumber, Modal, Select, Switch, Toast } from '@douyinfe/semi-ui'
import type { NetworkBridge } from '@/api/network'
import { createVPCSwitch, updateVPCSwitch, type VpcQuota, type VpcSwitch } from '@/api/vpc'
import { getUserList, type UserListItem } from '@/api/user'
import { bridgeModeText } from '../utils'
import { useMountModalLifecycle } from '@/hooks/useMountModalLifecycle'

interface SwitchDialogProps {
  row?: VpcSwitch
  isAdmin: boolean
  bridges: NetworkBridge[]
  quota: VpcQuota | null
  defaultUsername: string
  onClose: () => void
  onSaved: () => void
}

interface SwitchFormState {
  username: string
  name: string
  bridge_name: string
  bridge_vlan_id: number
  allow_promiscuous: boolean
  allow_mac_change: boolean
  allow_forged_transmits: boolean
  cidr: string
  gateway_ip: string
  dhcp_start: string
  dhcp_end: string
  traffic_down_gb: number
  traffic_up_gb: number
  bandwidth_down_mbps: number
  bandwidth_up_mbps: number
}

/** 配额上下限（编辑时允许填负数以归还配额） */
function quotaRange(quota: VpcQuota | null, maxField: keyof VpcQuota, remainingField: keyof VpcQuota, editing: boolean) {
  const max = Number(quota?.[maxField]) || 0
  const remaining = Number(quota?.[remainingField]) || 0
  const defaultVal = max > 0 ? remaining : 0
  return {
    min: editing ? -defaultVal : 0,
    max: max > 0 ? remaining : 999999,
    defaultVal,
  }
}

/** 桥接安全开关行（状态文字内嵌） */
function SecuritySwitchRow({
  label,
  tip,
  checked,
  onChange,
}: {
  label: string
  tip: string
  checked: boolean
  onChange: (v: boolean) => void
}) {
  return (
    <div className="qvm-form-item">
      <div className="net-switch-row">
        <div>
          <div className="qvm-form-label">{label}</div>
          <div className="qvm-form-tip">{tip}</div>
        </div>
        <div className="net-switch-control">
          <Switch checked={checked} onChange={onChange} size="small" checkedText="允" uncheckedText="拒" />
        </div>
      </div>
    </div>
  )
}

export default function SwitchDialog({
  row,
  isAdmin,
  bridges,
  quota,
  defaultUsername,
  onClose,
  onSaved,
}: SwitchDialogProps) {
  const { modalVisible, requestClose, afterModalClose } = useMountModalLifecycle(onClose)
  const editing = !!row
  const [submitting, setSubmitting] = useState(false)
  const [userOptions, setUserOptions] = useState<UserListItem[]>([])
  const [userLoading, setUserLoading] = useState(false)

  const trafficDown = quotaRange(quota, 'max_traffic_down', 'remaining_traffic_down', editing)
  const trafficUp = quotaRange(quota, 'max_traffic_up', 'remaining_traffic_up', editing)
  const bandwidthDown = quotaRange(quota, 'max_bandwidth_down', 'remaining_bandwidth_down', editing)
  const bandwidthUp = quotaRange(quota, 'max_bandwidth_up', 'remaining_bandwidth_up', editing)

  const [form, setForm] = useState<SwitchFormState>(() => {
    const legacyBandwidth = row?.bandwidth_mbps || 0
    return {
      username: row?.username || defaultUsername || '',
      name: row?.name || '',
      bridge_name: row?.bridge_name || bridges[0]?.name || 'br-ovs',
      bridge_vlan_id: row?.bridge_vlan_id || 0,
      allow_promiscuous: !!row?.allow_promiscuous,
      allow_mac_change: !!row?.allow_mac_change,
      allow_forged_transmits: !!row?.allow_forged_transmits,
      cidr: row?.cidr || '',
      gateway_ip: row?.gateway_ip || '',
      dhcp_start: row?.dhcp_start || '',
      dhcp_end: row?.dhcp_end || '',
      traffic_down_gb: row ? row.traffic_down_gb ?? 0 : trafficDown.defaultVal,
      traffic_up_gb: row ? row.traffic_up_gb ?? 0 : trafficUp.defaultVal,
      bandwidth_down_mbps: row ? row.bandwidth_down_mbps ?? legacyBandwidth : bandwidthDown.defaultVal,
      bandwidth_up_mbps: row ? row.bandwidth_up_mbps ?? legacyBandwidth : bandwidthUp.defaultVal,
    }
  })

  /** 当前选中的目标网桥 */
  const selectedBridge = useMemo(
    () => bridges.find((b) => b.name === form.bridge_name),
    [bridges, form.bridge_name],
  )
  const isBridgeMode = selectedBridge?.mode === 'bridge'

  // 管理员加载用户选项
  useEffect(() => {
    if (!isAdmin) return
    setUserLoading(true)
    getUserList()
      .then((res) => setUserOptions(res.data || []))
      .catch(() => setUserOptions([]))
      .finally(() => setUserLoading(false))
  }, [isAdmin])

  const patch = (p: Partial<SwitchFormState>) => setForm((f) => ({ ...f, ...p }))

  const handleSubmit = async () => {
    if (!form.name.trim()) {
      Toast.warning('请输入交换机名称')
      return
    }
    setSubmitting(true)
    try {
      if (editing && row) {
        await updateVPCSwitch(row.id, form)
        Toast.success('交换机已更新')
      } else {
        await createVPCSwitch({ ...form, bandwidth_mbps: 0 })
        Toast.success('交换机已创建')
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
      title={editing ? '编辑交换机' : '创建交换机'}
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
      {isAdmin && (
        <div className="qvm-form-item">
          <div className="qvm-form-label">所属用户</div>
          <Select
            style={{ width: '100%' }}
            placeholder="选择用户"
            filter
            showClear
            loading={userLoading}
            value={form.username}
            onChange={(v) => patch({ username: String(v || '') })}
            optionList={userOptions.map((u) => ({
              value: u.username,
              label: u.email ? `${u.username} (${u.email})` : u.username,
            }))}
          />
        </div>
      )}
      <div className="qvm-form-item">
        <div className="qvm-form-label required">名称</div>
        <Input value={form.name} onChange={(v) => patch({ name: v })} placeholder="请输入交换机名称" />
      </div>

      {!isBridgeMode && (
        <>
          <div className="qvm-form-divider">网段配置</div>
          <div className="qvm-form-item">
            <div className="qvm-form-label">网段(CIDR)</div>
            <Input
              value={form.cidr}
              onChange={(v) => patch({ cidr: v })}
              placeholder="如 10.0.1.0/24，留空自动分配"
              disabled={editing}
            />
            <div className="qvm-form-tip">创建后不可修改。留空时系统将自动分配未使用的子网。</div>
            <div className="qvm-form-tip warn">注意：网段不能与宿主机网段相同，否则会导致网络冲突。</div>
          </div>
          <div className="qvm-form-item">
            <div className="qvm-form-label">网关地址</div>
            <Input
              value={form.gateway_ip}
              onChange={(v) => patch({ gateway_ip: v })}
              placeholder="如 10.0.1.1，留空自动计算"
              disabled={editing}
            />
            <div className="qvm-form-tip">创建后不可修改。留空时自动取网段内第一个可用 IP。</div>
          </div>
        </>
      )}

      {isAdmin && (
        <div className="qvm-form-item">
          <div className="qvm-form-label">目标网桥</div>
          <Select
            style={{ width: '100%' }}
            placeholder="选择目标网桥"
            value={form.bridge_name}
            onChange={(v) => patch({ bridge_name: String(v) })}
            optionList={bridges.map((b) => ({
              value: b.name,
              label: `${b.name} - ${bridgeModeText(b.mode)}`,
            }))}
          />
          {isBridgeMode && (
            <div className="qvm-form-tip">
              桥接直通由上级路由器分配 IP，不启用内部 DHCP、NAT、安全组和端口转发。
            </div>
          )}
        </div>
      )}

      {isBridgeMode && (
        <>
          <div className="qvm-form-item">
            <div className="qvm-form-label">桥接 VLAN ID</div>
            <InputNumber
              style={{ width: '100%' }}
              min={0}
              max={4094}
              value={form.bridge_vlan_id}
              onChange={(v) => patch({ bridge_vlan_id: Number(v) || 0 })}
            />
            <div className="qvm-form-tip">0 表示不打 VLAN；填写 1-4094 时 VM 会以该 VLAN 接入上级网络。</div>
          </div>
          <div className="qvm-form-divider">桥接安全</div>
          <SecuritySwitchRow
            label="混杂模式"
            tip="拒绝时会对 VM 端口启用 no-flood，减少未知单播泛洪到该 VM。"
            checked={form.allow_promiscuous}
            onChange={(v) => patch({ allow_promiscuous: v })}
          />
          <SecuritySwitchRow
            label="MAC 地址更改"
            tip="拒绝时 VM 只能使用 XML 中配置的 MAC 作为源 MAC 发包。"
            checked={form.allow_mac_change}
            onChange={(v) => patch({ allow_mac_change: v })}
          />
          <SecuritySwitchRow
            label="伪传输"
            tip="拒绝时源 MAC 与配置 MAC 不一致的发包会被 OVS 丢弃。"
            checked={form.allow_forged_transmits}
            onChange={(v) => patch({ allow_forged_transmits: v })}
          />
        </>
      )}

      <div className="qvm-form-divider">流量配额</div>
      <div className="qvm-form-item">
        <div className="qvm-form-label">下行月配额(GB)</div>
        <InputNumber
          style={{ width: '100%' }}
          min={trafficDown.min}
          max={trafficDown.max}
          value={form.traffic_down_gb}
          onChange={(v) => patch({ traffic_down_gb: Number(v) || 0 })}
        />
        <div className="qvm-form-tip">用户下行总配额不限时可填 0，表示不限</div>
      </div>
      <div className="qvm-form-item">
        <div className="qvm-form-label">上行月配额(GB)</div>
        <InputNumber
          style={{ width: '100%' }}
          min={trafficUp.min}
          max={trafficUp.max}
          value={form.traffic_up_gb}
          onChange={(v) => patch({ traffic_up_gb: Number(v) || 0 })}
        />
        <div className="qvm-form-tip">用户上行总配额不限时可填 0，表示不限</div>
      </div>

      <div className="qvm-form-divider">带宽配额</div>
      <div className="qvm-form-item">
        <div className="qvm-form-label">下行总带宽(Mbps)</div>
        <InputNumber
          style={{ width: '100%' }}
          min={bandwidthDown.min}
          max={bandwidthDown.max}
          value={form.bandwidth_down_mbps}
          onChange={(v) => patch({ bandwidth_down_mbps: Number(v) || 0 })}
        />
        <div className="qvm-form-tip">用户下行带宽配额不限时可填 0，表示不限</div>
      </div>
      <div className="qvm-form-item">
        <div className="qvm-form-label">上行总带宽(Mbps)</div>
        <InputNumber
          style={{ width: '100%' }}
          min={bandwidthUp.min}
          max={bandwidthUp.max}
          value={form.bandwidth_up_mbps}
          onChange={(v) => patch({ bandwidth_up_mbps: Number(v) || 0 })}
        />
        <div className="qvm-form-tip">用户上行带宽配额不限时可填 0，表示不限</div>
      </div>
    </Modal>
  )
}
