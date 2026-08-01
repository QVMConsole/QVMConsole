/**
 * 用户管理页（仅管理员）
 * - 用户列表：紧凑配额单元格（悬浮查看完整明细）+ 纯图标行内操作
 * - 支持新增用户、编辑配置、分配/注册 VM、封禁/解封、重发邀请、重置流量、SSH 开关、删除
 * - 删除用户 / 封禁等敏感操作由请求层统一处理 428 二次验证
 */
import { useCallback, useEffect, useMemo, useState } from 'react'
import {
  Button,
  Dropdown,
  Input,
  Pagination,
  Select,
  Switch,
  Table,
  Tag,
  Toast,
  Tooltip,
} from '@douyinfe/semi-ui'
import {
  IconAlertTriangle,
  IconDelete,
  IconLock,
  IconMailStroked1,
  IconMore,
  IconPlus,
  IconRefresh,
  IconSearch,
  IconSetting,
  IconServerStroked,
  IconShield,
  IconUnlock,
  IconUserGroup,
} from '@douyinfe/semi-icons'
import type { ColumnProps } from '@douyinfe/semi-ui/lib/es/table'
import {
  deleteUser,
  getUserList,
  resendInvite,
  resetUserTraffic,
  toggleUserSSH,
  updateUserStatus,
  type UserListItem,
} from '@/api/user'
import { getVPCSwitches, type VpcSwitch } from '@/api/vpc'
import { confirmModal } from '@/utils/confirm'
import { useUserStore } from '@/stores/user'
import { ROLES } from '@/config/constants'
import QuotaOverviewCell from './components/QuotaOverviewCell'
import CreateUserDialog from './dialogs/CreateUserDialog'
import EditQuotaDialog from './dialogs/EditQuotaDialog'
import AssignVmDialog from './dialogs/AssignVmDialog'
import RegistrationDialog from './dialogs/RegistrationDialog'
import { filterNatVpcSwitches } from './dialogs/vpcOption'
import {
  cloudTypeLabel,
  isLightweightUser,
  registrationStatusLabel,
  registrationStatusTagColor,
  userStatusLabel,
  userStatusTagColor,
} from './utils'
import './user.css'

const PAGE_SIZE = 100

/** 弹窗状态 */
type DialogState =
  | { type: 'create' }
  | { type: 'quota'; row: UserListItem }
  | { type: 'assign'; row: UserListItem }
  | { type: 'registration'; row: UserListItem }
  | null

export default function UserPage() {
  const role = useUserStore((s) => s.role)
  const currentUsername = useUserStore((s) => s.username)
  const isAdmin = role === ROLES.admin

  const [list, setList] = useState<UserListItem[]>([])
  const [vpcSwitches, setVpcSwitches] = useState<VpcSwitch[]>([])
  const [loading, setLoading] = useState(false)
  const [dialog, setDialog] = useState<DialogState>(null)

  // 筛选
  const [searchText, setSearchText] = useState('')
  const [emailSearch, setEmailSearch] = useState('')
  const [roleFilter, setRoleFilter] = useState('')
  const [statusFilter, setStatusFilter] = useState('')
  const [cloudTypeFilter, setCloudTypeFilter] = useState('')
  const [page, setPage] = useState(1)

  const natVpcSwitches = useMemo(() => filterNatVpcSwitches(vpcSwitches), [vpcSwitches])

  // ==================== 数据加载 ====================
  const loadData = useCallback(async () => {
    setLoading(true)
    try {
      const res = await getUserList()
      setList(res.data || [])
    } catch {
      // 请求层已提示
    } finally {
      setLoading(false)
    }
  }, [])

  useEffect(() => {
    void loadData()
    // VPC 选项供轻量云相关弹窗使用，加载失败不影响列表
    getVPCSwitches()
      .then((res) => setVpcSwitches(res.data || []))
      .catch(() => undefined)
  }, [loadData])

  /** 任务型操作后延迟刷新（等待任务队列执行） */
  const refreshAfterTask = useCallback(
    (delay = 2000) => {
      window.setTimeout(() => void loadData(), delay)
    },
    [loadData],
  )

  // ==================== 筛选与分页 ====================
  const filtered = useMemo(() => {
    let data = list
    if (searchText) {
      const q = searchText.toLowerCase()
      data = data.filter((u) => u.username.toLowerCase().includes(q))
    }
    if (emailSearch) {
      const q = emailSearch.toLowerCase()
      data = data.filter((u) => (u.email || '').toLowerCase().includes(q))
    }
    if (roleFilter) {
      data = data.filter((u) => u.role === roleFilter)
    }
    if (statusFilter) {
      data = data.filter((u) => u.status === statusFilter)
    }
    if (cloudTypeFilter) {
      data = data.filter((u) => u.role !== 'admin' && u.cloud_type === cloudTypeFilter)
    }
    return data
  }, [list, searchText, emailSearch, roleFilter, statusFilter, cloudTypeFilter])

  useEffect(() => {
    setPage(1)
  }, [searchText, emailSearch, roleFilter, statusFilter, cloudTypeFilter])

  const paged = useMemo(
    () => filtered.slice((page - 1) * PAGE_SIZE, page * PAGE_SIZE),
    [filtered, page],
  )

  // ==================== 操作 ====================
  /** SSH 开关（失败回滚） */
  const handleToggleSSH = async (row: UserListItem, enabled: boolean) => {
    setList((prev) =>
      prev.map((u) => (u.username === row.username ? { ...u, ssh_enabled: enabled } : u)),
    )
    try {
      await toggleUserSSH(row.username, enabled)
      Toast.success(`用户 ${row.username} 的 SSH 访问已${enabled ? '开启' : '关闭'}`)
    } catch {
      setList((prev) =>
        prev.map((u) => (u.username === row.username ? { ...u, ssh_enabled: !enabled } : u)),
      )
    }
  }

  /** 封禁 / 解封 */
  const handleToggleStatus = async (row: UserListItem, targetStatus: 'active' | 'disabled') => {
    const isDisable = targetStatus === 'disabled'
    const ok = await confirmModal({
      title: isDisable ? '封禁账户' : '解封账户',
      content: isDisable
        ? `确定封禁用户 ${row.username}？封禁后该用户将立即退出登录，系统会尝试关闭该用户下所有运行中的虚拟机，并同步关闭 SSH 访问。此操作不会删除用户资产，解封后可继续使用。`
        : `确定解封用户 ${row.username}？解封后用户可重新登录面板，但之前被关闭的虚拟机不会自动恢复启动。`,
      okText: isDisable ? '确定封禁' : '确定解封',
      danger: isDisable,
    })
    if (!ok) return
    try {
      const res = await updateUserStatus(row.username, { status: targetStatus })
      if (res.data?.task_id) {
        Toast.success(
          `${isDisable ? '封禁' : '解封'}任务已提交（任务ID: ${res.data.task_id}），请在任务中心查看进度`,
        )
      } else {
        Toast.success(res.message || (isDisable ? '用户已封禁' : '用户已解封'))
      }
      refreshAfterTask(isDisable ? 2000 : 500)
    } catch {
      // 请求层已提示
    }
  }

  /** 重发邀请邮件 */
  const handleResendInvite = async (row: UserListItem) => {
    try {
      await resendInvite(row.username)
      Toast.success(`已向 ${row.email} 重发邀请邮件`)
    } catch {
      // 请求层已提示
    }
  }

  /** 重置流量配额 */
  const handleResetTraffic = async (row: UserListItem) => {
    const ok = await confirmModal({
      title: '重置流量配额',
      content: `确定重置用户 ${row.username} 的本月流量配额？重置后将恢复正常网络速率。`,
      okText: '确定重置',
      danger: true,
    })
    if (!ok) return
    try {
      await resetUserTraffic(row.username)
      Toast.success(`用户 ${row.username} 的流量配额已重置`)
      void loadData()
    } catch {
      // 请求层已提示
    }
  }

  /** 删除用户及所有资产 */
  const handleDelete = async (row: UserListItem) => {
    const vmCount = (row.vms || []).length
    const ok = await confirmModal({
      title: '删除用户及所有资产',
      content: (
        <div>
          <p>确定删除用户 {row.username}？</p>
          {vmCount > 0 && (
            <p className="usr-del-warn">
              <IconAlertTriangle />
              将同时删除该用户的 {vmCount} 台虚拟机（{(row.vms || []).join('、')}
              ）及其所有磁盘、快照、网络配置。
            </p>
          )}
          <p className="usr-del-warn">
            <IconAlertTriangle />
            用户的存储池（ISO 镜像、共享文件）也将被清除。
          </p>
          <p className="usr-del-danger">此操作不可恢复！</p>
        </div>
      ),
      okText: '确定删除',
      danger: true,
    })
    if (!ok) return
    try {
      const res = await deleteUser(row.username)
      if (res.data?.task_id) {
        Toast.success(`删除用户任务已提交（任务ID: ${res.data.task_id}），请在任务中心查看进度`)
      } else {
        Toast.success('删除用户任务已提交')
      }
      refreshAfterTask()
    } catch {
      // 请求层已提示
    }
  }

  // ==================== 表格 ====================
  const columns: ColumnProps<UserListItem>[] = [
    {
      title: '用户',
      dataIndex: 'username',
      width: 200,
      render: (_text, row) => (
        <div>
          <div className="usr-name">{row.username}</div>
          <div className="usr-muted sm">{row.email || '-'}</div>
        </div>
      ),
    },
    {
      title: '角色/类型',
      dataIndex: 'role',
      width: 150,
      render: (_text, row) => (
        <div className="usr-tag-group">
          <Tag size="small" color={row.role === 'admin' ? 'red' : 'green'}>
            {row.role === 'admin' ? '管理员' : '普通用户'}
          </Tag>
          {row.role !== 'admin' && (
            <Tag size="small" color={row.cloud_type === 'lightweight' ? 'orange' : 'teal'}>
              {cloudTypeLabel(row.cloud_type)}
            </Tag>
          )}
        </div>
      ),
    },
    {
      title: '状态',
      dataIndex: 'status',
      width: 90,
      align: 'center',
      render: (_text, row) => (
        <Tag size="small" color={userStatusTagColor(row.status)}>
          {userStatusLabel(row.status)}
        </Tag>
      ),
    },
    {
      title: '配额',
      dataIndex: 'quota',
      width: 250,
      render: (_text, row) => <QuotaOverviewCell row={row} />,
    },
    {
      title: '虚拟机',
      dataIndex: 'vms',
      render: (_text, row) => {
        const vms = row.vms || []
        const regs = row.lightweight_vm_registrations || []
        return (
          <div className="usr-vm-cell">
            {vms.length > 0 ? (
              <div className="usr-vm-tags">
                {vms.slice(0, 5).map((vm) => (
                  <Tag key={vm} size="small">
                    {vm}
                  </Tag>
                ))}
                {vms.length > 5 && <span className="usr-muted sm">+{vms.length - 5}</span>}
              </div>
            ) : (
              <span className="usr-muted">未分配</span>
            )}
            {isLightweightUser(row) && regs.length > 0 && (
              <div className="usr-vm-tags">
                {regs.slice(0, 3).map((item) => (
                  <Tag key={item.id} size="small" color={registrationStatusTagColor(item.status)}>
                    {registrationStatusLabel(item.status)}：{item.vm_name}
                  </Tag>
                ))}
              </div>
            )}
          </div>
        )
      },
    },
    {
      title: 'SSH',
      dataIndex: 'ssh_enabled',
      width: 70,
      align: 'center',
      render: (_text, row) =>
        row.role !== 'admin' ? (
          <Switch
            size="small"
            checked={!!row.ssh_enabled}
            disabled={row.status !== 'active'}
            onChange={(v) => void handleToggleSSH(row, v)}
            checkedText="开"
            uncheckedText="关"
          />
        ) : (
          <span className="usr-muted">-</span>
        ),
    },
    {
      title: '操作',
      dataIndex: 'actions',
      width: 100,
      render: (_text, row) => {
        const isSelf = row.username === currentUsername
        const isBuiltinAdmin = row.username === 'admin'
        const configDisabled = row.role === 'admin' && isSelf
        const deleteDisabled = (row.role === 'admin' && isSelf) || isBuiltinAdmin
        const trafficDisabled =
          row.role === 'admin' ||
          row.cloud_type === 'lightweight' ||
          !(row.quota && (row.quota.is_limited_down || row.quota.is_limited_up))
        return (
          <div className="usr-act-cell">
            <Tooltip content="编辑用户" position="top">
              <span
                className={`usr-act-ic config${configDisabled ? ' disabled' : ''}`}
                onClick={() => {
                  if (configDisabled) return
                  setDialog({ type: 'quota', row })
                }}
              >
                <IconSetting />
              </span>
            </Tooltip>
            <Dropdown
              trigger="click"
              position="bottomRight"
              clickToHide
              render={
                <Dropdown.Menu>
                  <Dropdown.Item
                    icon={<IconServerStroked />}
                    disabled={row.role === 'admin'}
                    onClick={() =>
                      setDialog({
                        type: row.cloud_type === 'lightweight' ? 'registration' : 'assign',
                        row,
                      })
                    }
                  >
                    {row.cloud_type === 'lightweight' ? '注册 VM' : '分配 VM'}
                  </Dropdown.Item>
                  {row.status === 'pending_invite' && (
                    <Dropdown.Item
                      icon={<IconMailStroked1 />}
                      onClick={() => void handleResendInvite(row)}
                    >
                      重发邀请
                    </Dropdown.Item>
                  )}
                  <Dropdown.Item
                    icon={<IconRefresh />}
                    disabled={trafficDisabled}
                    onClick={() => void handleResetTraffic(row)}
                  >
                    重置流量
                  </Dropdown.Item>
                  <Dropdown.Divider />
                  {!isBuiltinAdmin && !isSelf && row.status === 'active' && (
                    <Dropdown.Item
                      icon={<IconLock />}
                      type="danger"
                      onClick={() => void handleToggleStatus(row, 'disabled')}
                    >
                      封禁
                    </Dropdown.Item>
                  )}
                  {!isBuiltinAdmin && !isSelf && row.status === 'disabled' && (
                    <Dropdown.Item
                      icon={<IconUnlock />}
                      onClick={() => void handleToggleStatus(row, 'active')}
                    >
                      解封
                    </Dropdown.Item>
                  )}
                  <Dropdown.Item
                    icon={<IconDelete />}
                    type="danger"
                    disabled={deleteDisabled}
                    onClick={() => void handleDelete(row)}
                  >
                    删除
                  </Dropdown.Item>
                </Dropdown.Menu>
              }
            >
              <span className="usr-act-ic more">
                <IconMore />
              </span>
            </Dropdown>
          </div>
        )
      },
    },
  ]

  // ==================== 渲染 ====================
  if (!isAdmin) {
    return (
      <div className="usr-page">
        <div className="usr-empty">
          <div className="usr-empty-icon">
            <IconShield />
          </div>
          <div>用户管理仅对管理员开放</div>
        </div>
      </div>
    )
  }

  return (
    <div className="usr-page">
      <div className="usr-page-header qvm-fade-up">
        <div>
          <h2>
            <IconUserGroup style={{ marginRight: 8, color: 'var(--qvm-acc-ink)' }} />
            用户管理
          </h2>
          <p className="usr-page-sub">管理面板用户、资源配额与虚拟机分配</p>
        </div>
        <div className="usr-header-actions">
          <Button icon={<IconRefresh />} loading={loading} onClick={() => void loadData()}>
            刷新
          </Button>
          <Button
            type="primary"
            theme="light"
            icon={<IconPlus />}
            onClick={() => setDialog({ type: 'create' })}
          >
            新增用户
          </Button>
        </div>
      </div>

      <div className="usr-filter-bar qvm-fade-up">
        <Input
          prefix={<IconSearch />}
          placeholder="搜索用户名"
          value={searchText}
          onChange={setSearchText}
          showClear
          style={{ width: 160 }}
        />
        <Input
          prefix={<IconSearch />}
          placeholder="搜索邮箱"
          value={emailSearch}
          onChange={setEmailSearch}
          showClear
          style={{ width: 200 }}
        />
        <Select
          value={roleFilter}
          onChange={(v) => setRoleFilter((v as string) || '')}
          placeholder="角色筛选"
          showClear
          style={{ width: 130 }}
          optionList={[
            { label: '管理员', value: 'admin' },
            { label: '普通用户', value: 'user' },
          ]}
        />
        <Select
          value={statusFilter}
          onChange={(v) => setStatusFilter((v as string) || '')}
          placeholder="状态筛选"
          showClear
          style={{ width: 130 }}
          optionList={[
            { label: '正常', value: 'active' },
            { label: '待激活', value: 'pending_invite' },
            { label: '已封禁', value: 'disabled' },
          ]}
        />
        <Select
          value={cloudTypeFilter}
          onChange={(v) => setCloudTypeFilter((v as string) || '')}
          placeholder="用户类型"
          showClear
          style={{ width: 130 }}
          optionList={[
            { label: '弹性云', value: 'elastic' },
            { label: '轻量云', value: 'lightweight' },
          ]}
        />
      </div>

      <div className="usr-table-card qvm-fade-up">
        <Table<UserListItem>
          rowKey="username"
          columns={columns}
          dataSource={paged}
          loading={loading}
          pagination={false}
          size="small"
          empty="暂无用户"
        />
        {filtered.length > PAGE_SIZE && (
          <div className="usr-pagination">
            <Pagination
              total={filtered.length}
              pageSize={PAGE_SIZE}
              currentPage={page}
              onPageChange={setPage}
              showTotal
            />
          </div>
        )}
      </div>

      {/* ==================== 弹窗 ==================== */}
      {dialog?.type === 'create' && (
        <CreateUserDialog
          users={list}
          natVpcSwitches={natVpcSwitches}
          onClose={() => setDialog(null)}
          onSaved={() => {
            void loadData()
          }}
        />
      )}
      {dialog?.type === 'quota' && (
        <EditQuotaDialog
          row={dialog.row}
          natVpcSwitches={natVpcSwitches}
          onClose={() => setDialog(null)}
          onSaved={() => {
            void loadData()
          }}
        />
      )}
      {dialog?.type === 'assign' && (
        <AssignVmDialog
          row={dialog.row}
          onClose={() => setDialog(null)}
          onSaved={() => {
            void loadData()
          }}
        />
      )}
      {dialog?.type === 'registration' && (
        <RegistrationDialog
          row={dialog.row}
          users={list}
          natVpcSwitches={natVpcSwitches}
          onClose={() => setDialog(null)}
          onChanged={() => void loadData()}
        />
      )}
    </div>
  )
}
