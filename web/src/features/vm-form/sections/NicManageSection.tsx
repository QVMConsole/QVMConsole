/**
 * 网口管理分区（编辑模式）
 * 主网口 VPC 绑定切换（交换机/安全组）+ 多网口列表（增删改，仅管理员）。
 * 轻量云用户禁止切换（VPC 由管理员分配）。
 */
import { useCallback, useEffect, useMemo, useState } from 'react'
import { Banner, Button, Modal, Select, Table, Tag, Toast } from '@douyinfe/semi-ui'
import { IconGlobe, IconPlus, IconRefresh } from '@douyinfe/semi-icons'
import {
  bindVMVPC,
  getVMVPCBinding,
  listVMInterfaces,
  removeVMInterface,
  switchVMSecurityGroup,
  type VMInterfaceInfo,
  type VpcBindingInfo,
} from '@/api/vpc'
import { getVMNetworkStatus, type VmNetworkStatus } from '@/api/vm'
import { useUserStore } from '@/stores/user'
import { CLOUD_TYPES, ROLES } from '@/config/constants'
import {
  filterSecurityGroupsForSwitch,
  formatSecurityGroupOptionLabel,
} from '../vpcOptionUtils'
import SectionCard from './SectionCard'
import FormField from './FormField'
import { useVmFormScope } from '../scopeContext'
import NicEditDialog from '../dialogs/NicEditDialog'

interface NicManageSectionProps {
  vmName: string
  vmStatus: string
}

export default function NicManageSection({ vmName, vmStatus }: NicManageSectionProps) {
  const { options } = useVmFormScope()
  const role = useUserStore((s) => s.role)
  const username = useUserStore((s) => s.username)
  const cloudType = useUserStore((s) => s.cloudType)
  const isAdmin = role === ROLES.admin
  const isLightweight = !isAdmin && cloudType === CLOUD_TYPES.lightweight

  const [vpcInfo, setVpcInfo] = useState<VpcBindingInfo | null>(null)
  const [interfaces, setInterfaces] = useState<VMInterfaceInfo[]>([])
  const [runtimeStatus, setRuntimeStatus] = useState<VmNetworkStatus | null>(null)
  const [loading, setLoading] = useState(false)
  const [submitting, setSubmitting] = useState(false)
  const [bindSwitchId, setBindSwitchId] = useState<number | null>(null)
  const [bindGroupId, setBindGroupId] = useState<number | null>(null)
  const [nicDialog, setNicDialog] = useState<{ open: boolean; editing: VMInterfaceInfo | null }>({
    open: false,
    editing: null,
  })

  const ownerUsername = useMemo(
    () => vpcInfo?.owner_username || vpcInfo?.binding?.username || username || '',
    [vpcInfo?.owner_username, vpcInfo?.binding?.username, username],
  )
  const availableSwitches = useMemo(
    () => (vpcInfo?.switches && vpcInfo.switches.length > 0 ? vpcInfo.switches : options.vpcSwitches),
    [options.vpcSwitches, vpcInfo?.switches],
  )
  const availableGroups = useMemo(
    () => (vpcInfo?.groups && vpcInfo.groups.length > 0 ? vpcInfo.groups : options.vpcSecurityGroups),
    [options.vpcSecurityGroups, vpcInfo?.groups],
  )

  // ==================== 数据加载 ====================

  const refresh = useCallback(async () => {
    if (!vmName) return
    setLoading(true)
    try {
      const res = await getVMVPCBinding(vmName)
      const info = res.data || {}
      setVpcInfo(info)
      await options.loadVPCOptions()
      // 主绑定表单回填
      setBindSwitchId(info.binding?.switch_id || info.switch?.id || null)
      setBindGroupId(info.binding?.security_group_id || info.security_group?.id || null)
      // 多网口列表：管理员走专用接口；普通用户由 bindings 构建
      if (isAdmin) {
        try {
          const ifRes = await listVMInterfaces(vmName)
          setInterfaces(ifRes.data || [])
        } catch {
          setInterfaces([])
        }
      } else {
        const bindings = info.bindings || []
        const switches = info.switches || []
        const groups = info.groups || []
        setInterfaces(
          bindings.map((b) => ({
            binding: b,
            switch: switches.find((s) => s.id === b.switch_id) || null,
            security_group: groups.find((g) => g.id === b.security_group_id) || null,
          })),
        )
      }
      // 运行状态（取网口实时 IP）
      try {
        const rtRes = await getVMNetworkStatus(vmName)
        setRuntimeStatus(rtRes.data || null)
      } catch {
        setRuntimeStatus(null)
      }
    } finally {
      setLoading(false)
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [vmName, isAdmin])

  useEffect(() => {
    void refresh()
  }, [refresh])

  // ==================== 主网口 VPC 绑定 ====================

  const bindSelectedSwitch = useMemo(
    () => availableSwitches.find((item) => item.id === bindSwitchId) || null,
    [availableSwitches, bindSwitchId],
  )
  const bindIsBridge = bindSelectedSwitch?.bridge_mode === 'bridge'
  const bindSecurityGroups = useMemo(
    () => filterSecurityGroupsForSwitch(availableGroups, bindSelectedSwitch, ownerUsername, vmName),
    [availableGroups, bindSelectedSwitch, ownerUsername, vmName],
  )

  useEffect(() => {
    if (bindIsBridge) {
      if (bindGroupId !== null) setBindGroupId(null)
      return
    }
    if (bindGroupId && !bindSecurityGroups.some((group) => group.id === bindGroupId)) {
      setBindGroupId(null)
    }
  }, [bindGroupId, bindIsBridge, bindSecurityGroups])

  const handleBindSwitchChange = (value: unknown) => {
    const nextSwitchId = Number(value)
    setBindSwitchId(nextSwitchId)
    const nextSwitch = availableSwitches.find((item) => item.id === nextSwitchId) || null
    if (nextSwitch?.bridge_mode === 'bridge') {
      setBindGroupId(null)
      return
    }
    const nextGroups = filterSecurityGroupsForSwitch(availableGroups, nextSwitch, ownerUsername, vmName)
    if (bindGroupId && !nextGroups.some((group) => group.id === bindGroupId)) {
      setBindGroupId(null)
    }
  }

  const submitBind = async () => {
    if (isLightweight) {
      Toast.warning('轻量云服务器的 VPC 由管理员分配，不能自行切换')
      return
    }
    if (!bindSwitchId || (!bindIsBridge && !bindGroupId)) {
      Toast.warning(bindIsBridge ? '请选择交换机' : '请选择交换机和安全组')
      return
    }
    setSubmitting(true)
    try {
      await bindVMVPC(vmName, {
        switch_id: bindSwitchId,
        security_group_id: bindIsBridge ? 0 : (bindGroupId as number),
      })
      if (vmStatus === 'running' && !bindIsBridge) {
        Toast.warning('VPC 绑定已更新；运行中的虚拟机需要重新获取 DHCP 或重启后才会显示新 IP')
      } else {
        Toast.success('VPC 绑定已更新')
      }
      await refresh()
    } catch {
      // 错误由请求层统一提示
    } finally {
      setSubmitting(false)
    }
  }

  const submitSecurityGroupOnly = async () => {
    if (isLightweight) {
      Toast.warning('轻量云服务器使用专属安全组，不能切换安全组')
      return
    }
    if (!vpcInfo?.binding) {
      Toast.warning('请先保存 VPC 绑定，再单独切换安全组')
      return
    }
    if (bindIsBridge) {
      Toast.warning('桥接直通交换机不使用安全组')
      return
    }
    if (!bindGroupId) {
      Toast.warning('请选择安全组')
      return
    }
    setSubmitting(true)
    try {
      await switchVMSecurityGroup(vmName, bindGroupId)
      Toast.success('安全组已切换')
      await refresh()
    } catch {
      // 错误由请求层统一提示
    } finally {
      setSubmitting(false)
    }
  }

  // ==================== 多网口操作 ====================

  const handleRemoveNic = (row: VMInterfaceInfo) => {
    const order = row.binding?.interface_order
    if (order == null) {
      Toast.warning('无效的网口序号')
      return
    }
    Modal.confirm({
      title: '删除网口',
      content: '确定要删除此网口吗？删除后虚拟机内对应网卡将不可用。',
      okText: '删除',
      cancelText: '取消',
      okButtonProps: { type: 'danger' },
      onOk: async () => {
        await removeVMInterface(vmName, order)
        Toast.success('网口已删除')
        await refresh()
      },
    })
  }

  /** 按网口序号取运行态 IP */
  const getInterfaceIP = (row: VMInterfaceInfo): string => {
    const order = row.binding?.interface_order
    if (order === undefined || order === null) return ''
    const ifaces = runtimeStatus?.interfaces || []
    if (order < ifaces.length) return ifaces[order].ip || ''
    return ''
  }

  const nicColumns = [
    {
      title: '序号',
      width: 80,
      align: 'center' as const,
      render: (_: unknown, row: VMInterfaceInfo) => (
        <span>
          {row.binding?.interface_order ?? 0}
          {(row.binding?.interface_order ?? 0) === 0 && (
            <Tag size="small" style={{ marginLeft: 4 }}>主</Tag>
          )}
        </span>
      ),
    },
    {
      title: '网卡型号',
      width: 100,
      render: (_: unknown, row: VMInterfaceInfo) => row.binding?.nic_model || 'virtio',
    },
    {
      title: 'IP 地址',
      render: (_: unknown, row: VMInterfaceInfo) => {
        const ip = getInterfaceIP(row)
        return ip ? <code className="qvm-vf-ip-code">{ip}</code> : <span className="qvm-vf-empty-text">-</span>
      },
    },
    {
      title: 'VPC 交换机',
      render: (_: unknown, row: VMInterfaceInfo) =>
        row.switch ? (
          <span>
            {row.switch.name}{' '}
            <Tag size="small" color={row.switch.bridge_mode === 'bridge' ? 'orange' : 'blue'}>
              {row.switch.bridge_mode === 'bridge' ? '桥接直通' : row.switch.cidr || '-'}
            </Tag>
          </span>
        ) : (
          '-'
        ),
    },
    {
      title: '安全组',
      render: (_: unknown, row: VMInterfaceInfo) => {
        if (row.security_group) {
          return `${row.security_group.name}${row.security_group.is_default ? '（默认）' : ''}`
        }
        if (row.switch?.bridge_mode === 'bridge') {
          return <span className="qvm-vf-tip">桥接直通不使用安全组</span>
        }
        return '-'
      },
    },
    {
      title: '下行速率',
      width: 100,
      align: 'center' as const,
      render: (_: unknown, row: VMInterfaceInfo) =>
        (row.binding?.bandwidth_inbound_avg || 0) > 0 ? (
          `${row.binding?.bandwidth_inbound_avg} Mbps`
        ) : (
          <span className="qvm-vf-empty-text">未限制</span>
        ),
    },
    {
      title: '上行速率',
      width: 100,
      align: 'center' as const,
      render: (_: unknown, row: VMInterfaceInfo) =>
        (row.binding?.bandwidth_outbound_avg || 0) > 0 ? (
          `${row.binding?.bandwidth_outbound_avg} Mbps`
        ) : (
          <span className="qvm-vf-empty-text">未限制</span>
        ),
    },
    ...(isAdmin
      ? [
          {
            title: '操作',
            width: 140,
            align: 'center' as const,
            render: (_: unknown, row: VMInterfaceInfo) => (
              <div style={{ display: 'flex', gap: 6, justifyContent: 'center' }}>
                <Button size="small" type="primary" theme="light" onClick={() => setNicDialog({ open: true, editing: row })}>
                  编辑
                </Button>
                <Button size="small" type="danger" theme="light" onClick={() => handleRemoveNic(row)}>
                  删除
                </Button>
              </div>
            ),
          },
        ]
      : []),
  ]

  return (
    <>
      {/* 主网口 VPC 绑定 */}
      <SectionCard icon={<IconGlobe />} title="VPC 网络绑定">
        {isLightweight && (
          <Banner
            type="info"
            closeIcon={null}
            style={{ marginBottom: 12 }}
            description="轻量云服务器的 VPC 由管理员分配，不能自行切换。"
          />
        )}
        <div className="qvm-vf-grid-2">
          <FormField label="VPC 交换机" required>
            <Select
              style={{ width: '100%' }}
              value={bindSwitchId ?? undefined}
              placeholder="选择交换机"
              filter
              disabled={isLightweight}
              onChange={handleBindSwitchChange}
            >
              {availableSwitches.map((item) => (
                <Select.Option key={item.id} value={item.id}>
                  <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', gap: 8 }}>
                    <span>{isAdmin && item.username ? `${item.username} / ${item.name}` : item.name}</span>
                    <Tag size="small" color={item.bridge_mode === 'bridge' ? 'orange' : 'blue'}>
                      {item.bridge_mode === 'bridge' ? '桥接直通' : item.cidr}
                    </Tag>
                  </div>
                </Select.Option>
              ))}
            </Select>
            {bindIsBridge && (
              <div className="qvm-vf-tip">桥接直通由上级路由器分配 IP，不使用内部 DHCP 和安全组</div>
            )}
          </FormField>
          {!bindIsBridge && (
            <FormField label="安全组" required>
              <Select
                style={{ width: '100%' }}
                value={bindGroupId ?? undefined}
                placeholder="选择安全组"
                filter
                disabled={isLightweight}
                onChange={(v) => setBindGroupId(Number(v))}
                optionList={bindSecurityGroups.map((item) => ({
                  value: item.id,
                  label: formatSecurityGroupOptionLabel(item, false),
                }))}
              />
            </FormField>
          )}
        </div>
        {!isLightweight && (
          <div style={{ display: 'flex', gap: 10 }}>
            <Button type="primary" theme="solid" loading={submitting} onClick={() => void submitBind()}>
              保存 VPC 绑定
            </Button>
            <Button loading={submitting} disabled={bindIsBridge} onClick={() => void submitSecurityGroupOnly()}>
              仅切换安全组
            </Button>
          </div>
        )}
      </SectionCard>

      {/* 多网口列表 */}
      <SectionCard
        icon={<IconGlobe />}
        title="网口列表"
        extra={
          <div style={{ display: 'flex', gap: 8 }}>
            {isAdmin && (
              <Button size="small" type="primary" theme="light" icon={<IconPlus />} onClick={() => setNicDialog({ open: true, editing: null })}>
                添加网口
              </Button>
            )}
            <Button size="small" icon={<IconRefresh />} loading={loading} onClick={() => void refresh()} />
          </div>
        }
      >
        {interfaces.length > 1 && (
          <Banner
            type="info"
            closeIcon={null}
            style={{ marginBottom: 12 }}
            description="此虚拟机配置了多个网口，每个网口可接入不同的 VPC 交换机。运行中热插拔需要虚拟机操作系统支持。"
          />
        )}
        <Table
          rowKey={(row) => String(row?.binding?.interface_order ?? Math.random())}
          size="small"
          bordered
          loading={loading}
          columns={nicColumns}
          dataSource={interfaces}
          pagination={false}
          empty="暂无网口"
        />
      </SectionCard>

      <NicEditDialog
        visible={nicDialog.open}
        vmName={vmName}
        editing={nicDialog.editing}
        ownerUsername={ownerUsername}
        switches={availableSwitches}
        securityGroups={availableGroups}
        onClose={() => setNicDialog({ open: false, editing: null })}
        onSaved={refresh}
      />
    </>
  )
}
