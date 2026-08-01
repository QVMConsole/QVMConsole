/**
 * 轻量云 VM 注册管理弹窗
 * - 注册新 VM：复用创建向导登记模式生成草稿，保存后随邀请流程开通
 * - 分配已有 VM：选择未占用 VM 并逐台设置单 VM 配额
 * - 支持编辑单 VM 配额、删除待注册项，以及移除或删除已开通 VM
 */
import { useEffect, useMemo, useState } from 'react'
import {
  Banner,
  Button,
  Modal,
  RadioGroup,
  Radio,
  Select,
  Table,
  Tag,
  Toast,
  Tooltip,
} from '@douyinfe/semi-ui'
import { IconDelete, IconEditStroked, IconPlus, IconRefresh } from '@douyinfe/semi-icons'
import type { ColumnProps } from '@douyinfe/semi-ui/lib/es/table'
import {
  assignVms,
  createLightweightVmRegistrations,
  deleteLightweightRegisteredVm,
  deleteLightweightVmRegistration,
  removeLightweightRegisteredVm,
  type LightweightVmQuotaPayload,
  type UserListItem,
} from '@/api/user'
import { getVmList, type VmListItem } from '@/api/vm'
import { vmConfigText } from '@/views/vm/utils'
import type { VpcSwitch } from '@/api/vpc'
import { confirmModal } from '@/utils/confirm'
import CreateVmWizard, { type RegistrationDraft } from '@/features/vm-form/CreateVmWizard'
import LightweightQuotaTable from '../components/LightweightQuotaTable'
import RegistrationQuotaDialog from './RegistrationQuotaDialog'
import {
  buildLightweightQuotaPayload,
  defaultLightweightQuotaRow,
  formatRegistrationQuota,
  registrationStatusLabel,
  registrationStatusTagColor,
} from '../utils'
import { vpcOptionLabel } from './vpcOption'
import { useMountModalLifecycle } from '@/hooks/useMountModalLifecycle'

interface RegistrationDialogProps {
  row: UserListItem
  users: UserListItem[]
  natVpcSwitches: VpcSwitch[]
  onClose: () => void
  /** 任一提交成功后回调（父级刷新用户列表） */
  onChanged: () => void
}

/** 注册列表统一行结构（已保存注册 / 仅配额的已开通 VM / 本地草稿） */
interface RegRow {
  key: string
  id: number | null
  client_id?: string
  vm_name: string
  template: string
  vcpu: number | string
  ram: number | string
  disk_size: number | string
  status: string
  quota_only?: boolean
  draft?: RegistrationDraft
  traffic_down_gb?: number
  traffic_up_gb?: number
  bandwidth_down_mbps?: number
  bandwidth_up_mbps?: number
  max_port_forwards?: number
  max_snapshots?: number
  max_runtime_hours?: number
}

export default function RegistrationDialog({
  row,
  users,
  natVpcSwitches,
  onClose,
  onChanged,
}: RegistrationDialogProps) {
  const { modalVisible, requestClose, afterModalClose } = useMountModalLifecycle(onClose)
  const [vmSource, setVmSource] = useState<'existing' | 'register'>('register')
  const [submitting, setSubmitting] = useState(false)
  const [removingVm, setRemovingVm] = useState('')
  // 已保存注册项 / 已开通 VM 单配额（本地副本，操作后局部更新）
  const [registrations, setRegistrations] = useState(
    (row.lightweight_vm_registrations || []).map((item) => ({ ...item })),
  )
  const [quotas, setQuotas] = useState((row.lightweight_quotas || []).map((item) => ({ ...item })))
  const [drafts, setDrafts] = useState<(RegistrationDraft & { client_id: string })[]>([])
  // 分配已有 VM（列表同时用于已开通 VM 规格回显）
  const [existingVms, setExistingVms] = useState<string[]>([])
  const [existingQuotas, setExistingQuotas] = useState<LightweightVmQuotaPayload[]>([])
  const [allVms, setAllVms] = useState<VmListItem[]>([])
  const [loadingVms, setLoadingVms] = useState(false)
  const [vmsLoaded, setVmsLoaded] = useState(false)
  const [wizardVisible, setWizardVisible] = useState(false)
  const [quotaEditRow, setQuotaEditRow] = useState<RegRow | null>(null)
  const [deleteTarget, setDeleteTarget] = useState<RegRow | null>(null)
  const [deleteMode, setDeleteMode] = useState<'remove' | 'delete'>('remove')

  const vpcLabel = useMemo(() => {
    const vpc = natVpcSwitches.find(
      (item) => Number(item.id) === Number(row.dedicated_vpc_switch_id),
    )
    return vpc ? vpcOptionLabel(vpc) : `交换机 ID ${row.dedicated_vpc_switch_id || '-'}`
  }, [natVpcSwitches, row.dedicated_vpc_switch_id])

  /** 懒加载全部 VM */
  const ensureVmList = async () => {
    if (vmsLoaded || loadingVms) return
    setLoadingVms(true)
    try {
      const res = await getVmList()
      setAllVms(res.data || [])
      setVmsLoaded(true)
    } catch {
      // 请求层已提示
    } finally {
      setLoadingVms(false)
    }
  }

  // 存在仅配额的已开通 VM（无注册记录）时，加载 VM 列表用于规格回显
  useEffect(() => {
    const registeredNames = new Set(registrations.map((item) => item.vm_name))
    if (quotas.some((item) => !registeredNames.has(item.vm_name))) {
      void ensureVmList()
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [])

  /** VM 是否已被其他用户占用 */
  const isVmAssigned = (vmName: string) =>
    users.some((u) => u.username !== row.username && (u.vms || []).includes(vmName))

  /** 合并展示行：已保存注册 + 仅配额的已开通 VM + 本地草稿 */
  const rows = useMemo<RegRow[]>(() => {
    const registeredNames = new Set(registrations.map((item) => item.vm_name))
    const persisted: RegRow[] = registrations.map((item) => ({
      key: `reg-${item.id}`,
      id: item.id,
      vm_name: item.vm_name,
      template: item.template || '-',
      vcpu: item.vcpu,
      ram: item.ram,
      disk_size: item.disk_size,
      status: item.status || 'pending',
      traffic_down_gb: item.traffic_down_gb,
      traffic_up_gb: item.traffic_up_gb,
      bandwidth_down_mbps: item.bandwidth_down_mbps,
      bandwidth_up_mbps: item.bandwidth_up_mbps,
      max_port_forwards: item.max_port_forwards,
      max_snapshots: item.max_snapshots,
      max_runtime_hours: item.max_runtime_hours,
    }))
    const quotaOnly: RegRow[] = quotas
      .filter((item) => !registeredNames.has(item.vm_name))
      .map((item) => ({
        key: `quota-${item.vm_name}`,
        id: null,
        vm_name: item.vm_name,
        template: '已开通 VM',
        vcpu: '-',
        ram: '-',
        disk_size: '-',
        status: 'active',
        quota_only: true,
        traffic_down_gb: item.traffic_down_gb,
        traffic_up_gb: item.traffic_up_gb,
        bandwidth_down_mbps: item.bandwidth_down_mbps,
        bandwidth_up_mbps: item.bandwidth_up_mbps,
        max_port_forwards: item.max_port_forwards,
        max_snapshots: item.max_snapshots,
        max_runtime_hours: item.max_runtime_hours,
      }))
    const draftRows: RegRow[] = drafts.map((item) => ({
      key: `draft-${item.client_id}`,
      id: null,
      client_id: item.client_id,
      vm_name: item.vm_name,
      template: item.template || '-',
      vcpu: item.vcpu,
      ram: item.ram,
      disk_size: item.disk_size,
      status: 'draft',
      draft: item,
      traffic_down_gb: item.traffic_down_gb,
      traffic_up_gb: item.traffic_up_gb,
      bandwidth_down_mbps: item.bandwidth_down_mbps,
      bandwidth_up_mbps: item.bandwidth_up_mbps,
      max_port_forwards: item.max_port_forwards,
      max_runtime_hours: item.max_runtime_hours,
    }))
    return [...persisted, ...quotaOnly, ...draftRows]
  }, [registrations, quotas, drafts])

  /** 已选已有 VM 的配额编辑行 */
  const existingQuotaRows = useMemo(
    () =>
      existingVms.map(
        (vmName) =>
          existingQuotas.find((item) => item.vm_name === vmName) ||
          defaultLightweightQuotaRow(vmName),
      ),
    [existingVms, existingQuotas],
  )

  const patchExistingQuota = (vmName: string, patch: Partial<LightweightVmQuotaPayload>) => {
    setExistingQuotas((prev) => {
      const exist = prev.find((item) => item.vm_name === vmName)
      if (exist) {
        return prev.map((item) => (item.vm_name === vmName ? { ...item, ...patch } : item))
      }
      return [...prev, { ...defaultLightweightQuotaRow(vmName), ...patch }]
    })
  }

  /** 登记向导回传草稿（同名去重） */
  const handleDraft = (draft: RegistrationDraft) => {
    if (rows.some((item) => item.vm_name === draft.vm_name)) {
      Toast.warning('注册列表中已存在同名 VM')
      return
    }
    setDrafts((prev) => [
      ...prev,
      { ...draft, client_id: `${Date.now()}-${Math.random().toString(16).slice(2)}` },
    ])
  }

  /** 删除待注册项（草稿直接移除；已保存的调接口） */
  const handleRemoveRegistration = async (regRow: RegRow) => {
    if (!regRow.id) {
      setDrafts((prev) => prev.filter((item) => item.client_id !== regRow.client_id))
      return
    }
    const ok = await confirmModal({
      title: '删除注册项',
      content: `确定删除待注册 VM ${regRow.vm_name}？`,
      okText: '确定删除',
      danger: true,
    })
    if (!ok) return
    setRemovingVm(regRow.vm_name)
    try {
      await deleteLightweightVmRegistration(row.username, regRow.id)
      Toast.success('注册项已删除')
      setRegistrations((prev) => prev.filter((item) => item.id !== regRow.id))
      onChanged()
    } catch {
      // 请求层已提示
    } finally {
      setRemovingVm('')
    }
  }

  /** 处理已开通 VM 的移除或删除。 */
  const handleDeleteActive = async () => {
    if (!deleteTarget) return
    const regRow = deleteTarget
    setRemovingVm(regRow.vm_name)
    try {
      const res =
        deleteMode === 'delete'
          ? await deleteLightweightRegisteredVm(row.username, regRow.vm_name)
          : await removeLightweightRegisteredVm(row.username, regRow.vm_name)
      Toast.success(res.message || (deleteMode === 'delete' ? '删除任务已提交' : '轻量云 VM 已移除'))
      setRegistrations((prev) => prev.filter((item) => item.vm_name !== regRow.vm_name))
      setQuotas((prev) => prev.filter((item) => item.vm_name !== regRow.vm_name))
      setDeleteTarget(null)
      onChanged()
    } catch {
      // 请求层已提示
    } finally {
      setRemovingVm('')
    }
  }

  /** 保存新增注册草稿 */
  const handleSubmitDrafts = async () => {
    if (!drafts.length) return
    setSubmitting(true)
    try {
      const res = await createLightweightVmRegistrations(row.username, {
        registrations: drafts.map(({ client_id: _clientId, ...item }) => ({ ...item })),
      })
      Toast.success(res.message || '注册 VM 已保存')
      setDrafts([])
      onChanged()
      requestClose()
    } catch {
      // 请求层已提示
    } finally {
      setSubmitting(false)
    }
  }

  /** 分配已有 VM（附带单 VM 配额） */
  const handleSubmitExisting = async () => {
    if (!existingVms.length) return
    setSubmitting(true)
    try {
      const res = await assignVms(row.username, {
        vms: existingVms,
        lightweight_quotas: existingQuotaRows.map(buildLightweightQuotaPayload),
      })
      Toast.success(res.message || '已有 VM 分配成功')
      onChanged()
      requestClose()
    } catch {
      // 请求层已提示
    } finally {
      setSubmitting(false)
    }
  }

  /** 单 VM 配额保存回填 */
  const handleQuotaSaved = (payload: LightweightVmQuotaPayload) => {
    const patch = { ...payload }
    setDrafts((prev) =>
      prev.map((item) => (item.vm_name === payload.vm_name ? { ...item, ...patch } : item)),
    )
    setRegistrations((prev) =>
      prev.map((item) => (item.vm_name === payload.vm_name ? { ...item, ...patch } : item)),
    )
    setQuotas((prev) => {
      const exist = prev.some((item) => item.vm_name === payload.vm_name)
      if (exist) {
        return prev.map((item) =>
          item.vm_name === payload.vm_name ? { ...item, ...patch } : item,
        )
      }
      return prev
    })
    if (!quotaEditRow?.draft) onChanged()
  }

  const columns: ColumnProps<RegRow>[] = [
    { title: '名称', dataIndex: 'vm_name', width: 140 },
    { title: '模板', dataIndex: 'template', width: 150, ellipsis: true },
    {
      title: '规格',
      dataIndex: 'vcpu',
      width: 130,
      render: (_t, r) => {
        // 已开通 VM 无注册记录，从 VM 列表回显真实规格
        if (r.quota_only) {
          const vm = allVms.find((item) => item.name === r.vm_name)
          return vm ? vmConfigText(vm) : '-'
        }
        return `${r.vcpu}C / ${r.ram}GB / ${r.disk_size}GB`
      },
    },
    {
      title: '状态',
      dataIndex: 'status',
      width: 90,
      render: (_t, r) => (
        <Tag size="small" color={registrationStatusTagColor(r.status)}>
          {registrationStatusLabel(r.status)}
        </Tag>
      ),
    },
    {
      title: '网络配额',
      dataIndex: 'traffic_down_gb',
      render: (_t, r) => <span className="usr-muted sm">{formatRegistrationQuota(r)}</span>,
    },
    {
      title: '操作',
      dataIndex: 'actions',
      width: 90,
      render: (_t, r) => {
        const removing = removingVm === r.vm_name
        return (
          <div className="usr-act-cell">
            <Tooltip content="编辑配额" position="top">
              <span
                className={`usr-act-ic${r.status === 'provisioning' ? ' disabled' : ''}`}
                onClick={() => {
                  if (r.status === 'provisioning') return
                  setQuotaEditRow(r)
                }}
              >
                <IconEditStroked />
              </span>
            </Tooltip>
            {r.status === 'active' && (
              <Tooltip content="删除" position="top">
                <span
                  className="usr-act-ic danger"
                  onClick={() => {
                    setDeleteMode('remove')
                    setDeleteTarget(r)
                  }}
                >
                  {removing ? <IconRefresh spin /> : <IconDelete />}
                </span>
              </Tooltip>
            )}
            {r.status !== 'active' && r.status !== 'provisioning' && (
              <Tooltip content="删除" position="top">
                <span
                  className="usr-act-ic danger"
                  onClick={() => void handleRemoveRegistration(r)}
                >
                  {removing ? <IconRefresh spin /> : <IconDelete />}
                </span>
              </Tooltip>
            )}
          </div>
        )
      },
    },
  ]

  const footer = (
    <>
      <Button onClick={requestClose}>关闭</Button>
      {vmSource === 'existing' && (
        <Button
          type="primary"
          loading={submitting}
          disabled={!existingVms.length}
          onClick={() => void handleSubmitExisting()}
        >
          分配已有 VM
        </Button>
      )}
      {vmSource === 'register' && (
        <Button
          type="primary"
          loading={submitting}
          disabled={!drafts.length}
          onClick={() => void handleSubmitDrafts()}
        >
          保存新增注册
        </Button>
      )}
    </>
  )

  return (
    <>
      <Modal
        title="注册轻量云 VM"
        visible={modalVisible}
        afterClose={afterModalClose}
        onCancel={requestClose}
        footer={footer}
        width={920}
        closeOnEsc
        bodyStyle={{ maxHeight: '68vh', overflowY: 'auto' }}
      >
        <Banner
          type="info"
          closeIcon={null}
          description={
            <span>
              用户：<strong>{row.username}</strong>，可注册新 VM 或分配已有 VM。
            </span>
          }
          style={{ marginBottom: 16 }}
        />

        <div className="qvm-form-item">
          <div className="qvm-form-label">VM 来源</div>
          <RadioGroup
            value={vmSource}
            onChange={(e) => {
              const v = e.target.value as 'existing' | 'register'
              setVmSource(v)
              if (v === 'existing') void ensureVmList()
            }}
          >
            <Radio value="existing">选择已有 VM</Radio>
            <Radio value="register">注册新 VM</Radio>
          </RadioGroup>
        </div>

        {vmSource === 'existing' && (
          <>
            <div className="qvm-form-item">
              <div className="qvm-form-label">选择 VM</div>
              <Select
                multiple
                filter
                value={existingVms}
                onChange={(v) => setExistingVms(v as string[])}
                onFocus={() => void ensureVmList()}
                loading={loadingVms}
                placeholder="选择要分配给该用户的已有 VM"
                style={{ width: '100%' }}
                maxTagCount={6}
                optionList={allVms.map((vm) => ({
                  label: isVmAssigned(vm.name) ? `${vm.name}（已被占用）` : vm.name,
                  value: vm.name,
                  disabled: isVmAssigned(vm.name),
                }))}
              />
            </div>
            {existingVms.length > 0 && (
              <div className="usr-reg-panel">
                <div className="usr-reg-panel-header">
                  <div>
                    <strong>已有 VM 配额</strong>
                    <span className="usr-reg-hint">
                      为每个已选 VM 设置流量、带宽和端口转发等配额。
                    </span>
                  </div>
                </div>
                <LightweightQuotaTable
                  rows={existingQuotaRows}
                  onRowChange={patchExistingQuota}
                />
              </div>
            )}
          </>
        )}

        {vmSource === 'register' && (
          <div className="usr-reg-panel-header">
            <div>
              <strong>当前注册 VM</strong>
              <span className="usr-reg-hint">专用 VPC：{vpcLabel}</span>
            </div>
            <Button
              type="primary"
              theme="light"
              size="small"
              icon={<IconPlus />}
              disabled={!row.dedicated_vpc_switch_id}
              onClick={() => setWizardVisible(true)}
            >
              添加注册VM
            </Button>
          </div>
        )}
        {vmSource === 'register' && !row.dedicated_vpc_switch_id && (
          <Banner
            type="warning"
            closeIcon={null}
            description="该用户尚未配置专用 VPC，请先在用户配置中设置。"
            style={{ marginBottom: 12 }}
          />
        )}

        <Table<RegRow>
          rowKey="key"
          columns={columns}
          dataSource={rows}
          pagination={false}
          size="small"
          empty="暂无注册 VM"
        />
      </Modal>

      <Modal
        title="删除轻量云 VM"
        visible={!!deleteTarget}
        onCancel={() => setDeleteTarget(null)}
        onOk={() => void handleDeleteActive()}
        okText={deleteMode === 'delete' ? '确认删除' : '确认移除'}
        cancelText="取消"
        okButtonProps={{
          type: 'danger',
          loading: !!deleteTarget && removingVm === deleteTarget.vm_name,
        }}
        width={520}
        closeOnEsc
      >
        <RadioGroup
          value={deleteMode}
          onChange={(event) => setDeleteMode(event.target.value as 'remove' | 'delete')}
        >
          <Radio value="remove">仅移除分配</Radio>
          <Radio value="delete">同时删除虚拟机</Radio>
        </RadioGroup>
        <Banner
          type={deleteMode === 'delete' ? 'danger' : 'warning'}
          closeIcon={null}
          style={{ marginTop: 16 }}
          description={
            deleteMode === 'delete'
              ? `将删除虚拟机 ${deleteTarget?.vm_name || ''} 及其磁盘文件。删除成功后会同时清理注册记录、单 VM 配额和访问授权，此操作不可恢复。`
              : `将仅移除虚拟机 ${deleteTarget?.vm_name || ''} 的注册记录、单 VM 配额和访问授权，虚拟机本体及磁盘会保留。`
          }
        />
      </Modal>

      {/* 轻量云登记向导（复用创建虚拟机向导） */}
      <CreateVmWizard
        visible={wizardVisible}
        initialMode="template"
        registration={{
          enabled: true,
          dedicated_vpc_switch_id: row.dedicated_vpc_switch_id || 0,
          dedicated_vpc_label: vpcLabel,
        }}
        onDraft={(draft) => {
          handleDraft(draft)
          setWizardVisible(false)
        }}
        onClose={() => setWizardVisible(false)}
        onSuccess={() => setWizardVisible(false)}
      />

      {/* 编辑单 VM 配额 */}
      {quotaEditRow && (
        <RegistrationQuotaDialog
          username={row.username}
          row={{
            ...buildLightweightQuotaPayload(quotaEditRow),
            draft: quotaEditRow.status === 'draft',
          }}
          onClose={() => setQuotaEditRow(null)}
          onSaved={handleQuotaSaved}
        />
      )}
    </>
  )
}
