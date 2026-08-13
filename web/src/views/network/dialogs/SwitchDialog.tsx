/**
 * ESXi 风格交换机创建/编辑弹窗。
 * 交换机直接管理零或一个物理上行；管理员可配置托管 DHCP/NAT，普通用户固定创建空交换机。
 */
import { useEffect, useMemo, useState } from 'react'
import { Collapse, Input, InputNumber, Modal, Select, Switch, TextArea, Toast } from '@douyinfe/semi-ui'
import type { HostInterface } from '@/api/network'
import {
  createVPCSwitch,
  reconfigureVPCSwitch,
  updateVPCSwitch,
  type VpcQuota,
  type VpcSwitch,
  type VpcSwitchPayload,
} from '@/api/vpc'
import { getUserList, type UserListItem } from '@/api/user'
import { useMountModalLifecycle } from '@/hooks/useMountModalLifecycle'

interface SwitchDialogProps {
  row?: VpcSwitch
  isAdmin: boolean
  hostInterfaces: HostInterface[]
  quota: VpcQuota | null
  defaultUsername: string
  portSecurityEnabled: boolean
  onClose: () => void
  onSaved: () => void
  onTaskSubmitted: (switchId: number, taskId: number) => void
}

interface SwitchFormState {
  username: string
  name: string
  uplink_if: string
  uplink_gateway: string
  dhcp_enabled: boolean
  migrate_host_ip: boolean
  bridge_vlan_id: number
  allow_promiscuous: boolean
  allow_mac_change: boolean
  allow_forged_transmits: boolean
  ipv6_security_enabled: boolean
  trusted_ipv6_prefixes: string
  cidr: string
  gateway_ip: string
  dhcp_start: string
  dhcp_end: string
  traffic_down_gb: number
  traffic_up_gb: number
  bandwidth_down_mbps: number
  bandwidth_up_mbps: number
}

function quotaRange(quota: VpcQuota | null, maxField: keyof VpcQuota, remainingField: keyof VpcQuota, editing: boolean) {
  const max = Number(quota?.[maxField]) || 0
  const remaining = Number(quota?.[remainingField]) || 0
  const defaultVal = max > 0 ? remaining : 0
  return { min: editing ? -defaultVal : 0, max: max > 0 ? remaining : 999999, defaultVal }
}

function SecuritySwitchRow({
  label,
  tip,
  checked,
  onChange,
}: {
  label: string
  tip: string
  checked: boolean
  onChange: (value: boolean) => void
}) {
  return (
    <div className="qvm-form-item">
      <div className="net-switch-row">
        <div>
          <div className="qvm-form-label">{label}</div>
          <div className="qvm-form-tip">{tip}</div>
        </div>
        <Switch checked={checked} onChange={onChange} checkedText="允" uncheckedText="拒" />
      </div>
    </div>
  )
}

export default function SwitchDialog({
  row,
  isAdmin,
  hostInterfaces,
  quota,
  defaultUsername,
  portSecurityEnabled,
  onClose,
  onSaved,
  onTaskSubmitted,
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
      uplink_if: row?.uplink_if || '',
      uplink_gateway: row?.uplink_gateway || hostInterfaces.find((item) => item.name === row?.uplink_if)?.gateway || '',
      dhcp_enabled: !!row?.dhcp_enabled,
      migrate_host_ip: !!row?.migrate_host_ip,
      bridge_vlan_id: row?.bridge_vlan_id || 0,
      allow_promiscuous: !!row?.allow_promiscuous,
      allow_mac_change: !!row?.allow_mac_change,
      allow_forged_transmits: !!row?.allow_forged_transmits,
      ipv6_security_enabled: !!row?.ipv6_security_enabled,
      trusted_ipv6_prefixes: row?.trusted_ipv6_prefixes || '',
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

  useEffect(() => {
    if (!isAdmin) return
    setUserLoading(true)
    getUserList()
      .then((res) => setUserOptions(res.data || []))
      .catch(() => setUserOptions([]))
      .finally(() => setUserLoading(false))
  }, [isAdmin])

  const patch = (value: Partial<SwitchFormState>) => setForm((current) => ({ ...current, ...value }))
  const hasPhysicalUplink = isAdmin && !!form.uplink_if
  const isManaged = hasPhysicalUplink && form.dhcp_enabled
  const isPhysicalDirect = hasPhysicalUplink && !form.dhcp_enabled
  const modeText = isManaged ? '内置 DHCP/NAT' : isPhysicalDirect ? '物理直通' : '空交换机'

  const uplinkOptions = useMemo(
    () => hostInterfaces
      .filter((item) => item.physical !== false)
      .map((item) => {
        // 尚未选择上行时同时展示可用于直通或托管 NAT 的物理口；已有直通口仍可继续作为 NAT 出口。
        const available = form.dhcp_enabled
          ? item.can_use_nat !== false
          : item.can_use_direct !== false || item.can_use_nat !== false
        const selectedByCurrent = !!row && item.name === row.uplink_if
        const detail = [
          item.state,
          item.effective_l3_if && item.effective_l3_if !== item.name ? `经 ${item.effective_l3_if}` : '',
          item.gateway ? `网关 ${item.gateway}` : form.dhcp_enabled ? '需填写网关' : '',
          item.direct_switch_name ? `直通：${item.direct_switch_name}` : '',
          item.direct_vlan_ids?.length ? `已用 VLAN ${item.direct_vlan_ids.join('/')}` : '',
          item.nat_switch_count ? `${item.nat_switch_count} 个 NAT` : '',
        ]
          .filter(Boolean)
          .join(' · ')
        return {
          value: item.name,
          label: `${item.name}${detail ? `（${detail}）` : ''}`,
          disabled: !available && !selectedByCurrent,
        }
      }),
    [hostInterfaces, form.dhcp_enabled, row],
  )

  const buildPayload = (): VpcSwitchPayload => ({
    username: form.username,
    name: form.name.trim(),
    uplink_mode: hasPhysicalUplink ? 'physical' : 'none',
    uplink_if: hasPhysicalUplink ? form.uplink_if : '',
    uplink_gateway: isManaged ? form.uplink_gateway.trim() : '',
    dhcp_enabled: isManaged,
    migrate_host_ip: isPhysicalDirect && form.migrate_host_ip,
    bridge_vlan_id: isPhysicalDirect ? form.bridge_vlan_id : 0,
    allow_promiscuous: isPhysicalDirect && form.allow_promiscuous,
    allow_mac_change: isPhysicalDirect && form.allow_mac_change,
    allow_forged_transmits: isPhysicalDirect && form.allow_forged_transmits,
    ipv6_security_enabled: isPhysicalDirect && form.ipv6_security_enabled,
    trusted_ipv6_prefixes: isPhysicalDirect ? form.trusted_ipv6_prefixes : '',
    cidr: form.cidr,
    gateway_ip: form.gateway_ip,
    dhcp_start: form.dhcp_start,
    dhcp_end: form.dhcp_end,
    traffic_down_gb: form.traffic_down_gb,
    traffic_up_gb: form.traffic_up_gb,
    bandwidth_mbps: 0,
    bandwidth_down_mbps: form.bandwidth_down_mbps,
    bandwidth_up_mbps: form.bandwidth_up_mbps,
  })

  const topologyChanged = (payload: VpcSwitchPayload) => !!row && (
    !!row.dhcp_enabled !== !!payload.dhcp_enabled ||
    (row.uplink_if || '') !== (payload.uplink_if || '') ||
    (row.uplink_gateway || '') !== (payload.uplink_gateway || '') ||
    !!row.migrate_host_ip !== !!payload.migrate_host_ip ||
    Number(row.bridge_vlan_id || 0) !== Number(payload.bridge_vlan_id || 0)
  )

  const handleSubmit = async () => {
    if (!form.name.trim()) {
      Toast.warning('请输入交换机名称')
      return
    }
    const managedFields = [form.cidr, form.gateway_ip, form.dhcp_start, form.dhcp_end].map((value) => value.trim())
    if (isManaged && managedFields.some(Boolean) && managedFields.some((value) => !value)) {
      Toast.warning('请完整填写托管网络的网段、网关和 DHCP 地址池，首次创建也可全部留空自动分配')
      return
    }
    const selectedUplink = hostInterfaces.find((item) => item.name === form.uplink_if)
    const occupiedDirectVLANs = (selectedUplink?.direct_vlan_ids || []).filter(
      (vlanID) => !(editing && row?.uplink_if === form.uplink_if && Number(row.bridge_vlan_id || 0) === vlanID),
    )
    if (isPhysicalDirect && occupiedDirectVLANs.length > 0 && form.bridge_vlan_id === 0) {
      Toast.warning('该物理口已由直通交换机使用，共享上行时 VLAN ID 必须为 1-4094')
      return
    }
    if (isPhysicalDirect && occupiedDirectVLANs.includes(form.bridge_vlan_id)) {
      Toast.warning(`该物理口的 VLAN ID ${form.bridge_vlan_id} 已被其它直通交换机使用`)
      return
    }
    if (isManaged && !form.uplink_gateway.trim() && !selectedUplink?.gateway) {
      Toast.warning('当前物理出口未检测到默认网关，请填写上行网关')
      return
    }
    if (portSecurityEnabled && isPhysicalDirect && form.ipv6_security_enabled && !form.trusted_ipv6_prefixes.trim()) {
      Toast.warning('启用 IPv6 防护时请填写可信 IPv6 前缀')
      return
    }
    const payload = buildPayload()
    setSubmitting(true)
    try {
      if (!editing || !row) {
        await createVPCSwitch(payload)
        Toast.success('交换机已创建')
      } else {
        await updateVPCSwitch(row.id, {
          username: payload.username,
          name: payload.name,
          traffic_down_gb: payload.traffic_down_gb,
          traffic_up_gb: payload.traffic_up_gb,
          bandwidth_mbps: 0,
          bandwidth_down_mbps: payload.bandwidth_down_mbps,
          bandwidth_up_mbps: payload.bandwidth_up_mbps,
          allow_promiscuous: payload.allow_promiscuous,
          allow_mac_change: payload.allow_mac_change,
          allow_forged_transmits: payload.allow_forged_transmits,
          ipv6_security_enabled: payload.ipv6_security_enabled,
          trusted_ipv6_prefixes: payload.trusted_ipv6_prefixes,
        })
        if (isAdmin && topologyChanged(payload)) {
          const response = await reconfigureVPCSwitch(row.id, payload)
          const taskId = response.data?.task_id
          if (taskId) onTaskSubmitted(row.id, taskId)
          Toast.success(response.message || '交换机重配置任务已提交')
        } else {
          Toast.success('交换机已更新')
        }
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
      okText={editing && isAdmin && topologyChanged(buildPayload()) ? '提交重配置' : '保存'}
      cancelText="取消"
      confirmLoading={submitting}
      width={860}
      closeOnEsc
    >
      <div className="net-switch-mode-banner">
        <span>当前模式</span>
        <strong>{modeText}</strong>
        <small>
          {isManaged
            ? '面板提供 DHCP、网关、DNS 与 NAT，并通过所选物理口出站。'
            : isPhysicalDirect
              ? '虚拟机直接接入上级二层网络，由上级网络分配地址。'
              : '独立纯二层信任网络，适合连接软路由 LAN 口和内部虚拟机。'}
        </small>
      </div>
      <div className="net-switch-grid">
        <div className="net-switch-col">
          <Collapse defaultActiveKey={['basic']} keepDOM className="net-switch-collapse">
            <Collapse.Panel itemKey="basic" header={<span className="net-switch-collapse-title">基本信息</span>}>
              {isAdmin && (
                <div className="qvm-form-item">
                  <div className="qvm-form-label">所属用户</div>
                  <Select
                    style={{ width: '100%' }}
                    placeholder="选择用户"
                    filter
                    loading={userLoading}
                    value={form.username}
                    onChange={(value) => patch({ username: String(value || '') })}
                    optionList={userOptions.map((user) => ({
                      value: user.username,
                      label: user.email ? `${user.username} (${user.email})` : user.username,
                    }))}
                  />
                </div>
              )}
              <div className="qvm-form-item">
                <div className="qvm-form-label required">名称</div>
                <Input value={form.name} onChange={(value) => patch({ name: value })} placeholder="请输入交换机名称" />
              </div>
              {isAdmin ? (
                <>
                  <div className="qvm-form-item">
                    <div className="qvm-form-label">上行链路</div>
                    <Select
                      style={{ width: '100%' }}
                      placeholder="不绑定物理网口"
                      showClear
                      value={form.uplink_if || undefined}
                      onChange={(value) => {
                        const uplink = String(value || '')
                        const selected = hostInterfaces.find((item) => item.name === uplink)
                        const managedOnly = !!uplink && selected?.can_use_direct === false && selected?.can_use_nat !== false
                        patch({
                          uplink_if: uplink,
                          uplink_gateway: selected?.gateway || '',
                          dhcp_enabled: uplink ? (managedOnly || form.dhcp_enabled) : false,
                          migrate_host_ip: !!uplink && !!(selected?.default_route || selected?.gateway || selected?.addresses?.length),
                        })
                      }}
                      optionList={uplinkOptions}
                    />
                    <div className="qvm-form-tip">第一版每个交换机最多绑定一个物理网口。</div>
                  </div>
                  {hasPhysicalUplink && (
                    <div className="qvm-form-item">
                      <div className="net-switch-row">
                        <div>
                          <div className="qvm-form-label">内置 DHCP</div>
                          <div className="qvm-form-tip">开启后同时启用网关、DNS、NAT 与专属策略路由。</div>
                        </div>
                        <Switch
                          checked={form.dhcp_enabled}
                          onChange={(value) => {
                            const selected = hostInterfaces.find((item) => item.name === form.uplink_if)
                            if (!value && selected?.can_use_direct === false) {
                              Toast.warning('该物理口当前不可用于新的直通交换机')
                              return
                            }
                            patch({ dhcp_enabled: value, uplink_gateway: value ? (form.uplink_gateway || selected?.gateway || '') : form.uplink_gateway })
                          }}
                          checkedText="开"
                          uncheckedText="关"
                        />
                      </div>
                    </div>
                  )}
                </>
              ) : (
                <div className="net-switch-inline-note">普通用户创建的交换机固定为空交换机；仍可在虚拟机网络设置中选择系统基础网络。</div>
              )}
            </Collapse.Panel>
          </Collapse>

          <Collapse defaultActiveKey={[]} keepDOM className="net-switch-collapse">
            <Collapse.Panel itemKey="quota" header={<span className="net-switch-collapse-title">配额设置</span>}>
              <div className="net-switch-quota-grid">
                <div className="qvm-form-item">
                  <div className="qvm-form-label">下行月配额(GB)</div>
                  <InputNumber style={{ width: '100%' }} min={trafficDown.min} max={trafficDown.max} value={form.traffic_down_gb} onChange={(value) => patch({ traffic_down_gb: Number(value) || 0 })} />
                </div>
                <div className="qvm-form-item">
                  <div className="qvm-form-label">上行月配额(GB)</div>
                  <InputNumber style={{ width: '100%' }} min={trafficUp.min} max={trafficUp.max} value={form.traffic_up_gb} onChange={(value) => patch({ traffic_up_gb: Number(value) || 0 })} />
                </div>
                <div className="qvm-form-item">
                  <div className="qvm-form-label">下行总带宽(Mbps)</div>
                  <InputNumber style={{ width: '100%' }} min={bandwidthDown.min} max={bandwidthDown.max} value={form.bandwidth_down_mbps} onChange={(value) => patch({ bandwidth_down_mbps: Number(value) || 0 })} />
                </div>
                <div className="qvm-form-item">
                  <div className="qvm-form-label">上行总带宽(Mbps)</div>
                  <InputNumber style={{ width: '100%' }} min={bandwidthUp.min} max={bandwidthUp.max} value={form.bandwidth_up_mbps} onChange={(value) => patch({ bandwidth_up_mbps: Number(value) || 0 })} />
                </div>
              </div>
              <div className="qvm-form-tip">0 表示不限；编辑时可按现有配额规则归还额度。</div>
            </Collapse.Panel>
          </Collapse>
        </div>

        <div className="net-switch-col">
          <Collapse defaultActiveKey={['network']} keepDOM className="net-switch-collapse">
            <Collapse.Panel itemKey="network" header={<span className="net-switch-collapse-title">动态网络配置</span>}>
              {isManaged && (
                <>
                  <div className="qvm-form-item">
                    <div className="qvm-form-label">上行网关（自动检测）</div>
                    <Input
                      value={form.uplink_gateway}
                      onChange={(value) => patch({ uplink_gateway: value })}
                      placeholder="自动检测；未安装默认路由时请填写，例如 192.168.10.1"
                    />
                    <div className="qvm-form-tip">用于交换机专属策略路由；已有物理直通网桥也可作为托管 NAT 出口。</div>
                  </div>
                  <div className="qvm-form-item">
                    <div className="qvm-form-label">网段(CIDR)</div>
                    <Input value={form.cidr} onChange={(value) => patch({ cidr: value })} placeholder="如 10.0.1.0/24；四项全空时自动分配" />
                  </div>
                  <div className="qvm-form-item">
                    <div className="qvm-form-label">网关地址</div>
                    <Input value={form.gateway_ip} onChange={(value) => patch({ gateway_ip: value })} placeholder="如 10.0.1.1" />
                  </div>
                  <div className="net-switch-quota-grid">
                    <div className="qvm-form-item">
                      <div className="qvm-form-label">DHCP 起始地址</div>
                      <Input value={form.dhcp_start} onChange={(value) => patch({ dhcp_start: value })} />
                    </div>
                    <div className="qvm-form-item">
                      <div className="qvm-form-label">DHCP 结束地址</div>
                      <Input value={form.dhcp_end} onChange={(value) => patch({ dhcp_end: value })} />
                    </div>
                  </div>
                  <div className="qvm-form-tip warn">关闭 DHCP 后会保留这组网段配置，重新开启时可直接复用。</div>
                </>
              )}
              {isPhysicalDirect && (
                <>
                  <div className="qvm-form-item">
                    <div className="qvm-form-label">桥接 VLAN ID</div>
                    <InputNumber style={{ width: '100%' }} min={0} max={4094} value={form.bridge_vlan_id} onChange={(value) => patch({ bridge_vlan_id: Number(value) || 0 })} />
                    <div className="qvm-form-tip">
                      0 表示不打标签；1-4094 表示以指定 VLAN 接入上级网络。同一物理口被多个直通交换机共享时，必须分别使用不同的非零 VLAN ID。
                    </div>
                  </div>
                  <div className="qvm-form-item">
                    <div className="net-switch-row">
                      <div>
                        <div className="qvm-form-label">迁移宿主机 IP</div>
                        <div className="qvm-form-tip">物理口承载宿主机地址或默认路由时，将地址、网关与 DNS 迁移到交换机网桥。</div>
                      </div>
                      <Switch checked={form.migrate_host_ip} onChange={(value) => patch({ migrate_host_ip: value })} checkedText="开" uncheckedText="关" />
                    </div>
                  </div>
                </>
              )}
              {!isManaged && !isPhysicalDirect && (
                <div className="net-switch-inline-note">
                  此交换机不连接宿主机或外部网络，不运行内置 DHCP，并放行来宾 DHCP、DHCPv6 与 RA。可将软路由 LAN 口和内部虚拟机接入同一交换机。
                </div>
              )}
            </Collapse.Panel>
          </Collapse>

          {isPhysicalDirect && (
            <Collapse defaultActiveKey={[]} keepDOM className="net-switch-collapse">
              <Collapse.Panel itemKey="security" header={<span className="net-switch-collapse-title">桥接安全</span>}>
                <SecuritySwitchRow label="混杂模式" tip="允许接收并非发往当前 VM MAC 的二层帧。" checked={form.allow_promiscuous} onChange={(value) => patch({ allow_promiscuous: value })} />
                <SecuritySwitchRow label="MAC 地址更改" tip="允许来宾修改网卡 MAC 地址。" checked={form.allow_mac_change} onChange={(value) => patch({ allow_mac_change: value })} />
                <SecuritySwitchRow label="伪传输" tip="允许源 MAC 与配置 MAC 不一致的报文。" checked={form.allow_forged_transmits} onChange={(value) => patch({ allow_forged_transmits: value })} />
                {portSecurityEnabled && (
                  <>
                    <div className="qvm-form-item">
                      <div className="net-switch-row">
                        <div>
                          <div className="qvm-form-label">IPv6 防护</div>
                          <div className="qvm-form-tip">仅允许可信前缀内登记到网卡的精确 IPv6 地址。</div>
                        </div>
                        <Switch checked={form.ipv6_security_enabled} onChange={(value) => patch({ ipv6_security_enabled: value })} checkedText="开" uncheckedText="关" />
                      </div>
                    </div>
                    {form.ipv6_security_enabled && (
                      <div className="qvm-form-item">
                        <div className="qvm-form-label required">可信 IPv6 前缀</div>
                        <TextArea value={form.trusted_ipv6_prefixes} onChange={(value) => patch({ trusted_ipv6_prefixes: value })} placeholder={'每行一个 CIDR，例如：\n2001:db8:100::/64'} autosize={{ minRows: 2, maxRows: 5 }} />
                      </div>
                    )}
                  </>
                )}
              </Collapse.Panel>
            </Collapse>
          )}
        </div>
      </div>
    </Modal>
  )
}
