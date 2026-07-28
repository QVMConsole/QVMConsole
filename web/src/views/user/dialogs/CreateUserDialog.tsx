/**
 * 新增用户弹窗
 * - SMTP 已配置：发送邀请邮件激活；未配置：直接设置初始密码（接入密码泄露检测）
 * - 普通用户支持弹性云（用户级配额）与轻量云（分配已有 VM / 登记待开通 VM）
 * - 登记新 VM 复用创建向导的轻量云登记模式（onDraft 回传草稿）
 */
import { useMemo, useState } from 'react'
import {
  Banner,
  Button,
  Empty,
  Input,
  InputNumber,
  Modal,
  RadioGroup,
  Radio,
  Select,
  Table,
  Tag,
  Toast,
} from '@douyinfe/semi-ui'
import { IconPlus } from '@douyinfe/semi-icons'
import type { ColumnProps } from '@douyinfe/semi-ui/lib/es/table'
import {
  createUser,
  type CreateUserPayload,
  type LightweightVmQuotaPayload,
  type UserListItem,
  type UserQuotaPayload,
} from '@/api/user'
import { getVmList } from '@/api/vm'
import type { VpcSwitch } from '@/api/vpc'
import { useUserStore } from '@/stores/user'
import { checkPasswordBreachAsync, validatePassword } from '@/utils/validate'
import CreateVmWizard, { type RegistrationDraft } from '@/features/vm-form/CreateVmWizard'
import QuotaFormFields from '../components/QuotaFormFields'
import LightweightQuotaTable from '../components/LightweightQuotaTable'
import {
  buildLightweightQuotaPayload,
  defaultLightweightQuotaRow,
  formatRegistrationQuota,
} from '../utils'
import { vpcOptionLabel } from './vpcOption'
import { useMountModalLifecycle } from '@/hooks/useMountModalLifecycle'

interface CreateUserDialogProps {
  users: UserListItem[]
  natVpcSwitches: VpcSwitch[]
  onClose: () => void
  onSaved: () => void
}

/** 注册草稿行（本地状态，附带客户端唯一标识） */
type DraftRow = RegistrationDraft & { client_id: string }

/** 普通用户默认配额 */
const DEFAULT_QUOTA: UserQuotaPayload = {
  max_cpu: 4,
  max_memory: 8,
  max_disk: 100,
  max_vm: 5,
  max_storage: 10,
  max_runtime_hours: 0,
  enable_port_forward: true,
  max_port_forwards: 10,
  max_snapshots: 5,
  max_public_ips: 0,
  max_bandwidth_up: 0,
  max_bandwidth_down: 0,
  max_traffic_down: 0,
  max_traffic_up: 0,
}

export default function CreateUserDialog({
  users,
  natVpcSwitches,
  onClose,
  onSaved,
}: CreateUserDialogProps) {
  const { modalVisible, requestClose, afterModalClose } = useMountModalLifecycle(onClose)
  const security = useUserStore((s) => s.security)
  const smtpConfigured = security?.smtp_configured === true

  const [submitting, setSubmitting] = useState(false)
  const [username, setUsername] = useState('')
  const [email, setEmail] = useState('')
  const [password, setPassword] = useState('')
  const [confirmPassword, setConfirmPassword] = useState('')
  const [role, setRole] = useState('user')
  const [cloudType, setCloudType] = useState('elastic')
  const [vpcSwitchId, setVpcSwitchId] = useState<number | null>(null)
  const [quota, setQuota] = useState<UserQuotaPayload>({ ...DEFAULT_QUOTA })
  // 轻量云：VM 来源与选择状态
  const [vmSource, setVmSource] = useState<'existing' | 'register'>('existing')
  const [existingVms, setExistingVms] = useState<string[]>([])
  const [existingQuotas, setExistingQuotas] = useState<LightweightVmQuotaPayload[]>([])
  const [drafts, setDrafts] = useState<DraftRow[]>([])
  const [allVms, setAllVms] = useState<string[]>([])
  const [loadingVms, setLoadingVms] = useState(false)
  const [vmsLoaded, setVmsLoaded] = useState(false)
  const [wizardVisible, setWizardVisible] = useState(false)

  /** 角色切换时同步存储配额默认值（管理员默认不限） */
  const handleRoleChange = (v: string) => {
    setRole(v)
    setQuota((q) => ({ ...q, max_storage: v === 'admin' ? 0 : 10 }))
  }

  /** 懒加载全部 VM 列表（选择已有 VM 时） */
  const ensureVmList = async () => {
    if (vmsLoaded || loadingVms) return
    setLoadingVms(true)
    try {
      const res = await getVmList()
      setAllVms((res.data || []).map((vm) => vm.name))
      setVmsLoaded(true)
    } catch {
      // 请求层已提示
    } finally {
      setLoadingVms(false)
    }
  }

  /** VM 是否已被其他用户占用 */
  const isVmAssigned = (vmName: string) =>
    users.some((u) => (u.vms || []).includes(vmName))

  /** 已选 VM 的配额编辑行（保持与选择联动） */
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

  const selectedVpcLabel = useMemo(() => {
    const item = natVpcSwitches.find((vpc) => Number(vpc.id) === Number(vpcSwitchId))
    return item ? vpcOptionLabel(item) : ''
  }, [natVpcSwitches, vpcSwitchId])

  /** 登记向导回传草稿 */
  const handleDraft = (draft: RegistrationDraft) => {
    if (drafts.some((item) => item.vm_name === draft.vm_name)) {
      Toast.warning('注册列表中已存在同名 VM')
      return
    }
    setDrafts((prev) => [
      ...prev,
      { ...draft, client_id: `${Date.now()}-${Math.random().toString(16).slice(2)}` },
    ])
  }

  const draftColumns: ColumnProps<DraftRow>[] = [
    { title: '名称', dataIndex: 'vm_name', width: 130 },
    { title: '模板', dataIndex: 'template', ellipsis: true },
    {
      title: '规格',
      dataIndex: 'vcpu',
      width: 140,
      render: (_t, row) => `${row.vcpu}C / ${row.ram}GB / ${row.disk_size}GB`,
    },
    {
      title: '网络配额',
      dataIndex: 'traffic_down_gb',
      width: 260,
      render: (_t, row) => (
        <span className="usr-muted sm">{formatRegistrationQuota(row)}</span>
      ),
    },
    {
      title: '操作',
      dataIndex: 'actions',
      width: 70,
      render: (_t, row) => (
        <Button
          type="danger"
          theme="borderless"
          size="small"
          onClick={() =>
            setDrafts((prev) => prev.filter((item) => item.client_id !== row.client_id))
          }
        >
          移除
        </Button>
      ),
    },
  ]

  const handleSubmit = async () => {
    // 基础校验
    if (!username.trim()) {
      Toast.warning('请输入用户名')
      return
    }
    if (!email.trim()) {
      Toast.warning('请输入邮箱')
      return
    }
    if (!smtpConfigured) {
      if (!password) {
        Toast.warning('SMTP 未配置，请为用户设置初始密码')
        return
      }
      const local = validatePassword(password)
      if (!local.valid) {
        Toast.error(local.message)
        return
      }
      if (password !== confirmPassword) {
        Toast.error('两次输入的密码不一致')
        return
      }
    }
    // 轻量云校验
    if (role === 'user' && cloudType === 'lightweight') {
      if (vmSource === 'register' && !vpcSwitchId) {
        Toast.warning('请先为轻量云用户选择专用 VPC')
        return
      }
      if (vmSource === 'existing' && !existingVms.length) {
        Toast.warning('请至少选择一台已有 VM')
        return
      }
    }
    // 密码泄露检测（SMTP 未配置时用户将直接使用该密码登录）
    if (!smtpConfigured && password) {
      const breach = await checkPasswordBreachAsync(password)
      if (breach.enabled && breach.breached) {
        Toast.error('该密码已在已知泄露数据库中发现，请更换为更安全的密码')
        return
      }
    }

    setSubmitting(true)
    try {
      const payload: CreateUserPayload = {
        username: username.trim(),
        email: email.trim(),
        role,
        ...quota,
      }
      if (!smtpConfigured) {
        payload.password = password
      }
      if (role === 'user') {
        payload.cloud_type = cloudType
        if (cloudType === 'lightweight') {
          if (vmSource === 'existing') {
            payload.lightweight_existing_vms = existingVms
            payload.lightweight_existing_vm_quotas = existingQuotaRows.map(
              buildLightweightQuotaPayload,
            )
          } else {
            payload.dedicated_vpc_switch_id = vpcSwitchId
            payload.lightweight_vm_registrations = drafts.map(
              ({ client_id: _clientId, ...item }) => ({ ...item }),
            )
          }
        }
      }
      const res = await createUser(payload)
      Toast.success(res.message || (smtpConfigured ? '邀请邮件已发送' : '用户创建成功'))
      onSaved()
      requestClose()
    } catch {
      // 请求层已提示
    } finally {
      setSubmitting(false)
    }
  }

  return (
    <>
      <Modal
        title="新增用户"
        visible={modalVisible}
        afterClose={afterModalClose}
        onCancel={requestClose}
        onOk={() => void handleSubmit()}
        okText="确定"
        cancelText="取消"
        confirmLoading={submitting}
        width={820}
        closeOnEsc
        bodyStyle={{ maxHeight: '68vh', overflowY: 'auto' }}
      >
        <div className="usr-quota-grid cols-2">
          <div className="qvm-form-item">
            <div className="qvm-form-label required">用户名</div>
            <Input value={username} onChange={setUsername} placeholder="登录用户名" />
          </div>
          <div className="qvm-form-item">
            <div className="qvm-form-label required">邮箱</div>
            <Input value={email} onChange={setEmail} placeholder="接收邀请/通知邮件" />
          </div>
        </div>

        {!smtpConfigured && (
          <>
            <Banner
              type="warning"
              closeIcon={null}
              description="SMTP 未配置，用户将直接使用初始密码登录，无需邮件邀请。"
              style={{ marginBottom: 12 }}
            />
            <div className="usr-quota-grid cols-2">
              <div className="qvm-form-item">
                <div className="qvm-form-label required">初始密码</div>
                <Input
                  mode="password"
                  value={password}
                  onChange={setPassword}
                  placeholder="为用户设置初始密码"
                />
              </div>
              <div className="qvm-form-item">
                <div className="qvm-form-label required">确认密码</div>
                <Input
                  mode="password"
                  value={confirmPassword}
                  onChange={setConfirmPassword}
                  placeholder="请再次输入密码"
                />
              </div>
            </div>
          </>
        )}

        <div className="usr-quota-grid cols-2">
          <div className="qvm-form-item">
            <div className="qvm-form-label">角色</div>
            <Select
              value={role}
              onChange={(v) => handleRoleChange(v as string)}
              style={{ width: '100%' }}
              optionList={[
                { label: '普通用户', value: 'user' },
                { label: '管理员', value: 'admin' },
              ]}
            />
          </div>
          {role === 'user' && (
            <div className="qvm-form-item">
              <div className="qvm-form-label">用户类型</div>
              <Select
                value={cloudType}
                onChange={(v) => setCloudType(v as string)}
                style={{ width: '100%' }}
                optionList={[
                  { label: '弹性云', value: 'elastic' },
                  { label: '轻量云', value: 'lightweight' },
                ]}
              />
            </div>
          )}
        </div>

        {/* 轻量云：VM 来源 */}
        {role === 'user' && cloudType === 'lightweight' && (
          <>
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
                  <div className="qvm-form-label required">选择 VM</div>
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
                    optionList={allVms.map((name) => ({
                      label: isVmAssigned(name) ? `${name}（已被占用）` : name,
                      value: name,
                      disabled: isVmAssigned(name),
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
              <>
                <div className="qvm-form-item">
                  <div className="qvm-form-label required">专用 VPC</div>
                  <Select
                    value={vpcSwitchId ?? undefined}
                    onChange={(v) => setVpcSwitchId(v as number)}
                    filter
                    placeholder="请选择管理员创建的 NAT VPC"
                    style={{ width: '100%' }}
                    optionList={natVpcSwitches.map((item) => ({
                      label: vpcOptionLabel(item),
                      value: item.id,
                    }))}
                  />
                </div>
                <div className="usr-reg-panel">
                  <div className="usr-reg-panel-header">
                    <div>
                      <strong>待注册 VM</strong>
                      <span className="usr-reg-hint">
                        配置会随邀请邮件发送，用户确认凭据后才开通。
                      </span>
                    </div>
                    <Button
                      type="primary"
                      theme="light"
                      size="small"
                      icon={<IconPlus />}
                      disabled={!vpcSwitchId}
                      onClick={() => setWizardVisible(true)}
                    >
                      添加注册VM
                    </Button>
                  </div>
                  {drafts.length > 0 ? (
                    <Table<DraftRow>
                      rowKey="client_id"
                      columns={draftColumns}
                      dataSource={drafts}
                      pagination={false}
                      size="small"
                    />
                  ) : (
                    <Empty description="还没有待注册 VM" style={{ padding: '16px 0' }} />
                  )}
                </div>
              </>
            )}
          </>
        )}

        {/* 弹性云普通用户：完整配额表单 */}
        {role === 'user' && cloudType !== 'lightweight' && (
          <QuotaFormFields value={quota} onChange={(p) => setQuota((q) => ({ ...q, ...p }))} />
        )}

        {/* 管理员：仅存储配额 */}
        {role === 'admin' && (
          <div className="qvm-form-item">
            <div className="qvm-form-label">
              存储配额 (GB) <Tag size="small">设为 0 表示不限制</Tag>
            </div>
            <InputNumber
              value={quota.max_storage}
              onNumberChange={(v) => setQuota((q) => ({ ...q, max_storage: Number(v) || 0 }))}
              min={0}
              max={102400}
              style={{ width: 240 }}
            />
          </div>
        )}
      </Modal>

      {/* 轻量云登记向导（复用创建虚拟机向导） */}
      <CreateVmWizard
        visible={wizardVisible}
        initialMode="template"
        registration={{
          enabled: true,
          dedicated_vpc_switch_id: vpcSwitchId || 0,
          dedicated_vpc_label: selectedVpcLabel,
        }}
        onDraft={(draft) => {
          handleDraft(draft)
          setWizardVisible(false)
        }}
        onClose={() => setWizardVisible(false)}
        onSuccess={() => setWizardVisible(false)}
      />
    </>
  )
}
