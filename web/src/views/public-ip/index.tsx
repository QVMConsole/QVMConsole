/**
 * 公网 IP 管理页（仅管理员）
 * - 管理 1:1 NAT、经典网络（路由/桥接）公网 IP 资源与绑定关系
 * - 行内操作：高频「绑定/迁移」图标外露，其余收进 ⋯ 下拉菜单
 * - 批量操作：勾选行后顶部出现工具栏，支持批量绑定/解绑/删除
 * - 绑定/解绑/迁移/重载为高风险操作，提交任务队列异步应用（428 二次验证由请求层处理）
 */
import { useCallback, useEffect, useMemo, useState } from 'react'
import {
  Banner,
  Button,
  Dropdown,
  Input,
  Pagination,
  Select,
  Table,
  Tag,
  Toast,
  Tooltip,
} from '@douyinfe/semi-ui'
import {
  IconDelete,
  IconEditStroked,
  IconEyeOpenedStroked,
  IconGlobeStroke,
  IconLink,
  IconLock,
  IconMore,
  IconPlus,
  IconRedo,
  IconRefresh,
  IconSearch,
  IconSend,
  IconUnChainStroked,
} from '@douyinfe/semi-icons'
import type { ColumnProps } from '@douyinfe/semi-ui/lib/es/table'
import {
  applyPublicIPRules,
  batchDeletePublicIPs,
  batchUnbindPublicIPs,
  deletePublicIP,
  getPublicIPs,
  previewPublicIP,
  unbindPublicIP,
  type PublicIpBatchOpSummary,
  type PublicIpItem,
  type PublicIpPreview,
} from '@/api/publicIp'
import { getUserList } from '@/api/user'
import { getVmList } from '@/api/vm'
import { confirmModal } from '@/utils/confirm'
import { useUserStore } from '@/stores/user'
import { ROLES } from '@/config/constants'
import {
  guestIPv6StatusLabel,
  publicIpModeLabel,
  publicIpRowStatus,
  publicIpStatusLabel,
  publicIpStatusTagColor,
  publicIpTaskToast,
} from './utils'
import PublicIpDialog from './dialogs/PublicIpDialog'
import BindPublicIpDialog, { type BindVmOption } from './dialogs/BindPublicIpDialog'
import BatchBindPublicIpDialog from './dialogs/BatchBindPublicIpDialog'
import ImportIPv6PrefixDialog from './dialogs/ImportIPv6PrefixDialog'
import PreviewModal from '@/components/common/PreviewModal'
import './public-ip.css'

const PAGE_SIZE = 100

/** 弹窗状态 */
type DialogState =
  | { type: 'edit'; row?: PublicIpItem }
  | { type: 'bind'; row: PublicIpItem; action: 'bind' | 'migrate' }
  | { type: 'batch-bind'; rows: PublicIpItem[] }
  | { type: 'preview'; row: PublicIpItem; preview: PublicIpPreview }
  | { type: 'ipv6-import' }
  | null

export default function PublicIpPage() {
  const role = useUserStore((s) => s.role)
  const isAdmin = role === ROLES.admin
  const [list, setList] = useState<PublicIpItem[]>([])
  const [users, setUsers] = useState<string[]>([])
  const [vms, setVms] = useState<BindVmOption[]>([])
  const [loading, setLoading] = useState(false)
  const [dialog, setDialog] = useState<DialogState>(null)

  // 筛选
  const [searchText, setSearchText] = useState('')
  const [statusFilter, setStatusFilter] = useState('')
  const [modeFilter, setModeFilter] = useState('')
  const [familyFilter, setFamilyFilter] = useState('')
  const [page, setPage] = useState(1)

  // 批量选择
  const [selectedKeys, setSelectedKeys] = useState<(string | number)[]>([])

  // ==================== 数据加载 ====================
  const loadData = useCallback(async () => {
    setLoading(true)
    try {
      const [ipRes, userRes, vmRes] = await Promise.all([
        getPublicIPs(),
        getUserList(),
        getVmList(),
      ])
      setList(ipRes.data || [])
      const userItems = userRes.data || []
      setUsers(userItems.map((u) => u.username))
      // 用户 → 虚拟机 归属映射（管理员接口返回每个用户已分配的 VM 名称）
      const ownerMap: Record<string, string> = {}
      userItems.forEach((u) => {
        ;(u.vms || []).forEach((vmName) => {
          ownerMap[vmName] = u.username
        })
      })
      setVms((vmRes.data || []).map((vm) => ({ name: vm.name, username: ownerMap[vm.name] || '' })))
    } catch {
      // 请求层已提示
    } finally {
      setLoading(false)
    }
  }, [])

  useEffect(() => {
    void loadData()
  }, [loadData])

  // ==================== 筛选与分页 ====================
  const filtered = useMemo(() => {
    let data = list
    if (searchText) {
      const q = searchText.toLowerCase()
      data = data.filter((row) => row.ip.toLowerCase().includes(q))
    }
    if (statusFilter) {
      data = data.filter((row) => publicIpRowStatus(row) === statusFilter)
    }
    if (modeFilter) {
      data = data.filter((row) => (row.modes || []).includes(modeFilter as never))
    }
	if (familyFilter) {
	  data = data.filter((row) => (row.address_family || (row.ip.includes(':') ? 'ipv6' : 'ipv4')) === familyFilter)
	}
    return data
  }, [list, searchText, statusFilter, modeFilter, familyFilter])

  useEffect(() => {
    setPage(1)
  }, [searchText, statusFilter, modeFilter, familyFilter])

  const paged = useMemo(
    () => filtered.slice((page - 1) * PAGE_SIZE, page * PAGE_SIZE),
    [filtered, page],
  )

  /** 任务提交后延迟刷新（等待任务队列应用规则） */
  const refreshAfterTask = useCallback(() => {
    window.setTimeout(() => void loadData(), 1200)
  }, [loadData])

  // ==================== 操作 ====================
  const handlePreview = useCallback(
    async (row: PublicIpItem) => {
      // 未绑定：打开绑定弹窗进行试算
      if (!row.binding) {
        setDialog({ type: 'bind', row, action: 'bind' })
        return
      }
      // 已绑定：直接预览当前绑定关系对应的规则
      try {
        const res = await previewPublicIP(row.id, {
          username: row.binding.username,
          vm_name: row.binding.vm_name,
          vm_private_ip: row.binding.vm_private_ip,
          mode: row.binding.mode,
        })
        setDialog({ type: 'preview', row, preview: res.data || {} })
      } catch {
        // 请求层已提示
      }
    },
    [],
  )

  const handleUnbind = useCallback(
    async (row: PublicIpItem) => {
      const ok = await confirmModal({
        title: '解绑公网 IP',
        content: `确定解绑公网 IP ${row.ip}？现有公网访问会中断。`,
        okText: '确定解绑',
        danger: true,
      })
      if (!ok) return
      try {
        const res = await unbindPublicIP(row.id)
        Toast.success(publicIpTaskToast('解绑任务已提交', res.data?.task_id))
        refreshAfterTask()
      } catch {
        // 请求层已提示
      }
    },
    [refreshAfterTask],
  )

  const handleDelete = useCallback(
    async (row: PublicIpItem) => {
      const ok = await confirmModal({
        title: '删除公网 IP',
        content: `确定删除公网 IP ${row.ip}？`,
        okText: '确定删除',
        danger: true,
      })
      if (!ok) return
      try {
        await deletePublicIP(row.id)
        Toast.success('公网 IP 已删除')
        void loadData()
      } catch {
        // 请求层已提示
      }
    },
    [loadData],
  )

  const handleApplyAll = useCallback(async () => {
    const ok = await confirmModal({
      title: '重载公网 IP 规则',
      content: '确定按当前绑定关系重新应用全部公网 IP 规则？',
      okText: '确定重载',
      danger: true,
    })
    if (!ok) return
    try {
      const res = await applyPublicIPRules()
      Toast.success(publicIpTaskToast('重载任务已提交', res.data?.task_id))
    } catch {
      // 请求层已提示
    }
  }, [])

  // ==================== 批量操作 ====================
  // 选中行对应的原始数据（跨页保留）
  const selectedRows = useMemo(() => {
    const idSet = new Set(selectedKeys)
    return list.filter((row) => idSet.has(row.id))
  }, [list, selectedKeys])

  // 批量绑定：仅未绑定的行可参与
  const bindableRows = useMemo(
    () => selectedRows.filter((row) => !row.binding),
    [selectedRows],
  )
  // 批量解绑：仅已绑定的行可参与
  const boundRows = useMemo(
    () => selectedRows.filter((row) => !!row.binding),
    [selectedRows],
  )

  const handleBatchBind = useCallback(() => {
    if (bindableRows.length === 0) {
      Toast.warning('请选择至少一条未绑定的公网 IP')
      return
    }
    setDialog({ type: 'batch-bind', rows: bindableRows })
  }, [bindableRows])

  const handleBatchUnbind = useCallback(async () => {
    if (boundRows.length === 0) {
      Toast.warning('请选择至少一条已绑定的公网 IP')
      return
    }
    const ok = await confirmModal({
      title: '批量解绑公网 IP',
      content: `确定批量解绑已选中的 ${boundRows.length} 条已绑定公网 IP？现有公网访问会中断。`,
      okText: '确定批量解绑',
      danger: true,
    })
    if (!ok) return
    try {
      const res = await batchUnbindPublicIPs(boundRows.map((row) => row.id))
      Toast.success(publicIpTaskToast('批量解绑任务已提交', res.data?.task_id))
      setSelectedKeys([])
      refreshAfterTask()
    } catch {
      // 请求层已提示
    }
  }, [boundRows, refreshAfterTask])

  const handleBatchDelete = useCallback(async () => {
    if (selectedRows.length === 0) {
      Toast.warning('请先选择要删除的公网 IP')
      return
    }
    const ok = await confirmModal({
      title: '批量删除公网 IP',
      content: `确定批量删除已选中的 ${selectedRows.length} 条公网 IP？已绑定的会自动跳过。`,
      okText: '确定批量删除',
      danger: true,
    })
    if (!ok) return
    try {
      const res = await batchDeletePublicIPs(selectedRows.map((row) => row.id))
      const summary: PublicIpBatchOpSummary = (res.data as PublicIpBatchOpSummary) || {
        success: 0,
        failed: 0,
        skipped: 0,
        items: [],
      }
      Toast.success(
        `批量删除完成（成功 ${summary.success} / 失败 ${summary.failed} / 跳过 ${summary.skipped}）`,
      )
      setSelectedKeys([])
      void loadData()
    } catch {
      // 请求层已提示
    }
  }, [selectedRows, loadData])

  // ==================== 表格 ====================
  const columns: ColumnProps<PublicIpItem>[] = [
    {
      title: '公网 IP',
      dataIndex: 'ip',
      width: 275,
      render: (text, row) => (
        <div className="pip-address-cell">
          <span className="pip-ip-text">{text}</span>
          <Tag size="small" color={row.address_family === 'ipv6' || row.ip.includes(':') ? 'purple' : 'cyan'}>
            {row.address_family === 'ipv6' || row.ip.includes(':') ? 'IPv6' : 'IPv4'}
          </Tag>
          {row.auto_ipv6 && <Tag size="small" color="green">动态</Tag>}
        </div>
      ),
    },
    {
      title: '掩码/网关',
      dataIndex: 'cidr',
      width: 190,
      render: (_text, row) => (
        <div>
          <div className="qvm-mono">{row.cidr || '-'}</div>
          <div className="pip-muted">网关：{row.gateway || '-'}</div>
        </div>
      ),
    },
    {
      title: '出口网卡',
      dataIndex: 'uplink_if',
      width: 120,
      render: (text) => text || '自动检测',
    },
    {
      title: '支持模式',
      dataIndex: 'mode_labels',
      width: 220,
      render: (_text, row) => (
        <div className="pip-mode-tags">
          {(row.mode_labels || []).map((label) => (
            <Tag key={label} size="small" color="blue">
              {label}
            </Tag>
          ))}
        </div>
      ),
    },
    {
      title: '状态',
      dataIndex: 'status',
      width: 90,
      align: 'center',
      render: (_text, row) => {
        const status = publicIpRowStatus(row)
        return (
          <Tag size="small" color={publicIpStatusTagColor(status)}>
            {publicIpStatusLabel(status)}
          </Tag>
        )
      },
    },
    {
      title: '绑定 VM',
      dataIndex: 'binding',
      width: 240,
      render: (_text, row) =>
        row.binding ? (
          <div>
            <div>
              {row.binding.vm_name}{' '}
              <Tag size="small">{publicIpModeLabel(row.binding.mode)}</Tag>
            </div>
            <div className="pip-muted">用户：{row.binding.username}</div>
            <div className="pip-muted">私网：{row.binding.vm_private_ip || '-'}</div>
            <div className="pip-muted">运行态：{row.binding.runtime_status || '-'}</div>
            {row.address_family === 'ipv6' && (
              <Tooltip content={row.binding.guest_ipv6_message || '来宾系统 IPv6 配置状态'}>
                <div className="pip-muted">
                  来宾 IPv6：{guestIPv6StatusLabel(row.binding.guest_ipv6_status)}
                </div>
              </Tooltip>
            )}
          </div>
        ) : (
          <span className="pip-muted">未绑定</span>
        ),
    },
    {
      title: '备注',
      dataIndex: 'remark',
      render: (text) => text || '-',
    },
    {
      title: '操作',
      dataIndex: 'actions',
      width: 110,
      render: (_text, row) => {
        const bound = !!row.binding
        return (
          <div className="pip-act-cell">
            {bound ? (
              <Tooltip content="迁移" position="top">
                <span
                  className="pip-act-ic migrate"
                  onClick={() => setDialog({ type: 'bind', row, action: 'migrate' })}
                >
                  <IconSend />
                </span>
              </Tooltip>
            ) : (
              <Tooltip content="绑定" position="top">
                <span
                  className="pip-act-ic bind"
                  onClick={() => setDialog({ type: 'bind', row, action: 'bind' })}
                >
                  <IconLink />
                </span>
              </Tooltip>
            )}
            <Dropdown
              trigger="click"
              position="bottomRight"
              clickToHide
              render={
                <Dropdown.Menu>
                  <Dropdown.Item
                    icon={<IconEditStroked />}
                    onClick={() => setDialog({ type: 'edit', row })}
                  >
                    编辑
                  </Dropdown.Item>
                  <Dropdown.Item
                    icon={<IconEyeOpenedStroked />}
                    onClick={() => void handlePreview(row)}
                  >
                    {bound ? '预览规则' : '试算规则'}
                  </Dropdown.Item>
                  <Dropdown.Divider />
                  {bound ? (
                    <Dropdown.Item
                      icon={<IconUnChainStroked />}
                      type="danger"
                      onClick={() => void handleUnbind(row)}
                    >
                      解绑
                    </Dropdown.Item>
                  ) : (
                    <Dropdown.Item
                      icon={<IconDelete />}
                      type="danger"
                      onClick={() => void handleDelete(row)}
                    >
                      删除
                    </Dropdown.Item>
                  )}
                </Dropdown.Menu>
              }
            >
              <span className="pip-act-ic more">
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
      <div className="pip-page">
        <div className="pip-empty">
          <div className="pip-empty-icon">
            <IconLock />
          </div>
          <div>公网 IP 管理仅对管理员开放</div>
        </div>
      </div>
    )
  }

  return (
    <div className="pip-page">
      <div className="pip-page-header qvm-fade-up">
        <div>
          <h2>
            <IconGlobeStroke style={{ marginRight: 8, color: 'var(--qvm-acc-ink)' }} />
            公网 IP（IPv4 / IPv6）
          </h2>
          <p className="pip-page-sub">管理 IPv4 NAT、IPv6 前缀路由、经典网络和浮动 IP 迁移</p>
        </div>
        <div className="pip-header-actions">
          <Button icon={<IconRefresh />} loading={loading} onClick={() => void loadData()}>
            刷新
          </Button>
          <Button type="warning" theme="light" icon={<IconRedo />} onClick={() => void handleApplyAll()}>
            重载规则
          </Button>
          <Button
			theme="light"
			icon={<IconGlobeStroke />}
			onClick={() => setDialog({ type: 'ipv6-import' })}
		  >
			导入 IPv6 前缀
		  </Button>
		  <Button
            type="primary"
            theme="light"
            icon={<IconPlus />}
            onClick={() => setDialog({ type: 'edit' })}
          >
            新增公网 IP
          </Button>
        </div>
      </div>

      <Banner
        type="warning"
        closeIcon={null}
        className="qvm-fade-up"
        description="公网 IP 绑定、解绑、迁移会触发高风险验证，并通过任务队列应用规则。IPv6 使用 Proxy NDP + /128 路由，VM 内地址与默认路由按预览提示配置。"
        style={{ marginBottom: 16 }}
      />

      <div className="pip-filter-bar qvm-fade-up">
        <Input
          prefix={<IconSearch />}
          placeholder="搜索公网 IP"
          value={searchText}
          onChange={setSearchText}
          showClear
          style={{ width: 180 }}
        />
        <Select
		  value={familyFilter}
		  onChange={(v) => setFamilyFilter(v as string)}
		  placeholder="地址族"
		  showClear
		  style={{ width: 120 }}
		  optionList={[
			{ label: 'IPv4', value: 'ipv4' },
			{ label: 'IPv6', value: 'ipv6' },
		  ]}
		/>
		<Select
          value={statusFilter}
          onChange={(v) => setStatusFilter(v as string)}
          placeholder="状态筛选"
          showClear
          style={{ width: 130 }}
          optionList={[
            { label: '已绑定', value: 'bound' },
            { label: '空闲', value: 'free' },
            { label: '保留', value: 'reserved' },
          ]}
        />
        <Select
          value={modeFilter}
          onChange={(v) => setModeFilter(v as string)}
          placeholder="模式筛选"
          showClear
          style={{ width: 150 }}
          optionList={[
            { label: '1:1 NAT', value: 'nat' },
            { label: '经典网络-路由', value: 'classic_route' },
            { label: '经典网络-桥接', value: 'classic_bridge' },
          ]}
        />
      </div>

      <div className="pip-table-card qvm-fade-up">
        {selectedKeys.length > 0 && (
          <div className="pip-batch-bar">
            <span className="pip-batch-bar-count">
              已选 {selectedKeys.length} 项（可绑定 {bindableRows.length} / 可解绑 {boundRows.length}）
            </span>
            <div className="pip-batch-bar-actions">
              <Button
                size="small"
                theme="light"
                icon={<IconLink />}
                disabled={bindableRows.length === 0}
                onClick={() => handleBatchBind()}
              >
                批量绑定
              </Button>
              <Button
                size="small"
                theme="light"
                type="warning"
                icon={<IconUnChainStroked />}
                disabled={boundRows.length === 0}
                onClick={() => void handleBatchUnbind()}
              >
                批量解绑
              </Button>
              <Button
                size="small"
                theme="light"
                type="danger"
                icon={<IconDelete />}
                onClick={() => void handleBatchDelete()}
              >
                批量删除
              </Button>
            </div>
            <div className="pip-batch-bar-spacer" />
            <Button size="small" theme="borderless" onClick={() => setSelectedKeys([])}>
              清除选择
            </Button>
          </div>
        )}
        <Table<PublicIpItem>
          rowKey="id"
          columns={columns}
          dataSource={paged}
          loading={loading}
          pagination={false}
          size="small"
          empty="暂无公网 IP"
          rowSelection={{
            selectedRowKeys: selectedKeys,
            onChange: (keys) => setSelectedKeys(keys || []),
          }}
        />
        {filtered.length > PAGE_SIZE && (
          <div className="pip-pagination">
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
      {dialog?.type === 'edit' && (
        <PublicIpDialog
          row={dialog.row}
          onClose={() => setDialog(null)}
          onSaved={() => {
            void loadData()
          }}
        />
      )}
      {dialog?.type === 'bind' && (
        <BindPublicIpDialog
          row={dialog.row}
          action={dialog.action}
          users={users}
          vms={vms}
          onClose={() => setDialog(null)}
          onSubmitted={() => {
            refreshAfterTask()
          }}
        />
      )}
      {dialog?.type === 'batch-bind' && (
        <BatchBindPublicIpDialog
          rows={dialog.rows}
          users={users}
          vms={vms}
          onClose={() => setDialog(null)}
          onSubmitted={() => {
            setSelectedKeys([])
            refreshAfterTask()
          }}
        />
      )}
	  {dialog?.type === 'ipv6-import' && (
		<ImportIPv6PrefixDialog
		  suggestedCount={Math.max(1, vms.length)}
		  onClose={() => setDialog(null)}
		  onSaved={() => void loadData()}
		/>
	  )}
      {dialog?.type === 'preview' && (
        <PreviewModal
          title={`规则预览：${dialog.row.ip}`}
          onClose={() => setDialog(null)}
          width={720}
        >
          <pre className="pip-preview-commands">
            {(dialog.preview.commands || []).join('\n')}
          </pre>
          {dialog.preview.config_hint && (
            <Banner
              type="info"
              closeIcon={null}
              description={dialog.preview.config_hint}
              style={{ marginTop: 12 }}
            />
          )}
          {(dialog.preview.warnings || []).map((item) => (
            <Banner
              key={item}
              type="warning"
              closeIcon={null}
              description={item}
              style={{ marginTop: 8 }}
            />
          ))}
        </PreviewModal>
      )}
    </div>
  )
}
