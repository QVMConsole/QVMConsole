/**
 * 网络分区（创建 / 编辑共用）
 * 编辑：仅网卡类型（运行中禁用）；创建：默认网卡型号 + 网口列表。
 */
import { Button, Empty, Select, Tag, TextArea, Tooltip } from '@douyinfe/semi-ui'
import { useEffect, useState } from 'react'
import { IconGlobe, IconDelete, IconPlus } from '@douyinfe/semi-icons'
import { IllustrationNoContent, IllustrationNoContentDark } from '@douyinfe/semi-illustrations'
import { useUserStore } from '@/stores/user'
import SectionCard from './SectionCard'
import FormField from './FormField'
import { useVmFormScope } from '../scopeContext'
import { NIC_MODEL_OPTIONS } from '../constants'
import { vpcSwitchModeDetail, type VpcSwitch } from '@/api/vpc'
import { getPortSecurityStatus } from '@/api/ovs'
import {
  filterSecurityGroupsForSwitch,
  formatSecurityGroupOptionLabel,
} from '../vpcOptionUtils'

/** 交换机选项展示文案。 */
function switchOptionLabel(item: VpcSwitch, isAdmin: boolean): string {
  const prefix = isAdmin && item.username ? `${item.username} / ` : ''
  return `${prefix}${item.name}（${vpcSwitchModeDetail(item)}）`
}

export default function NicSection() {
  const { form, options, ctx } = useVmFormScope()
  const username = useUserStore((s) => s.username)
  const { form: f, setField } = form
  const isEdit = ctx.mode === 'edit'
  const running = ctx.vmStatus === 'running'
  const [portSecurityEnabled, setPortSecurityEnabled] = useState(false)

  useEffect(() => {
    if (isEdit || !ctx.isAdmin) return
    void getPortSecurityStatus()
      .then((res) => setPortSecurityEnabled(!!res.data?.enabled))
      .catch(() => setPortSecurityEnabled(false))
  }, [ctx.isAdmin, isEdit])

  const addNic = () => {
    setField('extra_nics', [
      ...f.extra_nics,
      {
        nic_model: f.nic_model || 'virtio',
        switch_id: options.vpcSwitches.length > 0 ? options.vpcSwitches[0].id : null,
        security_group_id: null,
        allowed_ipv4_addresses: '',
        allowed_ipv6_addresses: '',
      },
    ])
  }

  const removeNic = (index: number) => {
    setField('extra_nics', f.extra_nics.filter((_, i) => i !== index))
  }

  const getSecurityGroupsForSwitch = (switchId: number | null) => {
    const sw = options.vpcSwitches.find((item) => item.id === switchId) || null
    return filterSecurityGroupsForSwitch(options.vpcSecurityGroups, sw, username)
  }

  const updateNic = (
    index: number,
    key:
      | 'nic_model'
      | 'switch_id'
      | 'security_group_id'
      | 'allowed_ipv4_addresses'
      | 'allowed_ipv6_addresses',
    value: unknown,
  ) => {
    setField(
      'extra_nics',
      f.extra_nics.map((nic, i) => {
        if (i !== index) return nic
        const next = { ...nic, [key]: value }
        if (key === 'switch_id' && next.security_group_id) {
          const groups = getSecurityGroupsForSwitch(Number(value) || null)
          if (!groups.some((group) => group.id === next.security_group_id)) {
            next.security_group_id = null
          }
        }
        return next
      }),
    )
  }

  return (
    <>
      <SectionCard icon={<IconGlobe />} title={isEdit ? '网络配置' : '默认网卡型号'}>
        <FormField
          label="网卡类型"
          tip={
            isEdit
              ? running
                ? '修改网卡类型需要先关机'
                : 'VirtIO 性能最佳，部分系统需安装驱动'
              : '新建网口时将默认使用此网卡型号'
          }
          tipType={isEdit && running ? 'warn' : 'info'}
        >
          <Select
            style={{ width: isEdit ? 280 : '100%' }}
            value={f.nic_model}
            disabled={isEdit && running}
            onChange={(v) => setField('nic_model', v as string)}
            optionList={NIC_MODEL_OPTIONS.map((item) => ({
              value: item.value,
              label: (
                <div style={{ display: 'flex', justifyContent: 'space-between', gap: 12 }}>
                  <span>{item.label}</span>
                  <Tag
                    size="small"
                    color={item.tagType === 'success' ? 'green' : item.tagType === 'warning' ? 'orange' : 'blue'}
                  >
                    {item.tag}
                  </Tag>
                </div>
              ),
            }))}
          />
        </FormField>
      </SectionCard>

      {!isEdit && !ctx.registration.enabled && (
        <SectionCard
          icon={<IconPlus />}
          title="网口"
          extra={
            <Button size="small" type="primary" theme="light" icon={<IconPlus />} onClick={addNic}>
              添加网口
            </Button>
          }
        >
          {f.extra_nics.length === 0 ? (
            <Empty
              image={<IllustrationNoContent style={{ width: 96, height: 96 }} />}
              darkModeImage={<IllustrationNoContentDark style={{ width: 96, height: 96 }} />}
              description="暂无网口，点击上方「添加网口」为虚拟机配置网络。未添加网口时虚拟机将无物理网卡。"
            />
          ) : (
            f.extra_nics.map((nic, index) => (
              <div key={index} className="qvm-vf-nic-row">
                <div className="qvm-vf-nic-header">
                  <Tag size="small" color="blue">网口 #{index + 1}</Tag>
                  <Tooltip content="删除网口" position="top">
                    <span className="qvm-act-ic danger" onClick={() => removeNic(index)}>
                      <IconDelete />
                    </span>
                  </Tooltip>
                </div>
                <div className="qvm-vf-grid-3">
                  <FormField label="网卡型号">
                    <Select
                      style={{ width: '100%' }}
                      value={nic.nic_model}
                      onChange={(v) => updateNic(index, 'nic_model', v)}
                      optionList={NIC_MODEL_OPTIONS.map((item) => ({ value: item.value, label: item.label }))}
                    />
                  </FormField>
                  <FormField label="VPC 交换机">
                    <Select
                      style={{ width: '100%' }}
                      value={nic.switch_id ?? undefined}
                      placeholder="选择交换机"
                      filter
                      onFocus={() => void options.loadVPCOptions()}
                      onChange={(v) => updateNic(index, 'switch_id', v)}
                      optionList={options.vpcSwitches.map((item) => ({
                        value: item.id,
                        label: switchOptionLabel(item, ctx.isAdmin),
                      }))}
                    />
                  </FormField>
                  {options.vpcSwitches.find((item) => item.id === nic.switch_id)?.bridge_mode !== 'bridge' && (
                    <FormField label="安全组">
                      <Select
                        style={{ width: '100%' }}
                        value={nic.security_group_id ?? undefined}
                        placeholder="可选"
                        filter
                        onFocus={() => void options.loadVPCOptions()}
                        onChange={(v) => updateNic(index, 'security_group_id', v)}
                        optionList={getSecurityGroupsForSwitch(nic.switch_id).map((item) => ({
                          value: item.id,
                          label: formatSecurityGroupOptionLabel(item, ctx.isAdmin),
                        }))}
                      />
                    </FormField>
                  )}
                </div>
                {portSecurityEnabled && (() => {
                  const selectedSwitch = options.vpcSwitches.find((item) => item.id === nic.switch_id)
                  const directBridge = selectedSwitch?.bridge_mode === 'bridge'
                  if (directBridge && !selectedSwitch?.uplink_if) return null
                  return (
                    <div className="qvm-vf-grid-2">
                      <FormField
                        label="允许的 IPv4 地址"
                        tip={directBridge ? '可选；留空时保持兼容保护，填写后启用精确地址校验' : '静态地址可在此登记，DHCP 租约会自动加入策略'}
                      >
                        <TextArea
                          value={nic.allowed_ipv4_addresses}
                          onChange={(v) => updateNic(index, 'allowed_ipv4_addresses', v)}
                          placeholder="每行一个精确 IPv4 地址"
                          autosize={{ minRows: 2, maxRows: 4 }}
                        />
                      </FormField>
                      {directBridge && selectedSwitch?.ipv6_security_enabled && (
                        <FormField label="允许的 IPv6 地址" required tip="须位于交换机可信前缀内">
                          <TextArea
                            value={nic.allowed_ipv6_addresses}
                            onChange={(v) => updateNic(index, 'allowed_ipv6_addresses', v)}
                            placeholder="每行一个精确 IPv6 地址"
                            autosize={{ minRows: 2, maxRows: 4 }}
                          />
                        </FormField>
                      )}
                    </div>
                  )
                })()}
              </div>
            ))
          )}
        </SectionCard>
      )}
    </>
  )
}
