/**
 * 节点管理页（仅管理员）
 * - 管理跨节点迁移目标节点：面板 API + SSH 双通道连接信息
 * - 行内操作：高频「探测」图标外露（行级 loading），编辑/删除收进 ⋯ 下拉菜单
 * - ≤768px 切换移动端卡片视图
 */
import { useCallback, useEffect, useMemo, useState } from 'react'
import {
  Button,
  Dropdown,
  Input,
  Pagination,
  Select,
  Spin,
  Table,
  Tag,
  Toast,
  Tooltip,
} from '@douyinfe/semi-ui'
import {
  IconDelete,
  IconEditStroked,
  IconLock,
  IconMore,
  IconPlus,
  IconPulse,
  IconRefresh,
  IconSearch,
  IconServerStroked,
} from '@douyinfe/semi-icons'
import type { ColumnProps } from '@douyinfe/semi-ui/lib/es/table'
import {
  deleteHostNode,
  listHostNodes,
  probeHostNode,
  type HostNodeItem,
} from '@/api/node'
import { confirmModal } from '@/utils/confirm'
import { formatDateTime } from '@/utils/format'
import { useUserStore } from '@/stores/user'
import { ROLES } from '@/config/constants'
import NodeDialog from './dialogs/NodeDialog'
import './node.css'

const PAGE_SIZE = 100

/** 节点状态中文文案 */
function statusText(status: string): string {
  if (status === 'online') return '在线'
  if (status === 'error') return '异常'
  return '未知'
}

/** 节点状态标签配色 */
function statusTagColor(status: string): 'green' | 'red' | 'grey' {
  if (status === 'online') return 'green'
  if (status === 'error') return 'red'
  return 'grey'
}

/** 弹窗状态 */
type DialogState = { type: 'edit'; row?: HostNodeItem } | null

export default function NodePage() {
  const role = useUserStore((s) => s.role)
  const isAdmin = role === ROLES.admin

  const [list, setList] = useState<HostNodeItem[]>([])
  const [loading, setLoading] = useState(false)
  const [probing, setProbing] = useState<Record<number, boolean>>({})
  const [dialog, setDialog] = useState<DialogState>(null)

  // 筛选
  const [searchText, setSearchText] = useState('')
  const [statusFilter, setStatusFilter] = useState('')
  const [enabledFilter, setEnabledFilter] = useState('')
  const [page, setPage] = useState(1)

  // ==================== 数据加载 ====================
  const loadData = useCallback(async () => {
    setLoading(true)
    try {
      const res = await listHostNodes()
      setList(res.data || [])
    } catch {
      // 请求层已统一提示
    } finally {
      setLoading(false)
    }
  }, [])

  useEffect(() => {
    if (isAdmin) void loadData()
  }, [isAdmin, loadData])

  // ==================== 筛选与分页 ====================
  const filtered = useMemo(() => {
    let data = list
    if (searchText) {
      const q = searchText.toLowerCase()
      data = data.filter((row) => row.name.toLowerCase().includes(q))
    }
    if (statusFilter) {
      data = data.filter((row) => row.status === statusFilter)
    }
    if (enabledFilter) {
      data = data.filter((row) => row.enabled === (enabledFilter === 'enabled'))
    }
    return data
  }, [list, searchText, statusFilter, enabledFilter])

  useEffect(() => {
    setPage(1)
  }, [searchText, statusFilter, enabledFilter])

  const paged = useMemo(
    () => filtered.slice((page - 1) * PAGE_SIZE, page * PAGE_SIZE),
    [filtered, page],
  )

  // ==================== 操作 ====================
  const handleProbe = useCallback(
    async (row: HostNodeItem) => {
      setProbing((p) => ({ ...p, [row.id]: true }))
      try {
        const res = await probeHostNode(row.id)
        Toast.success(res.message || '节点探测通过')
      } catch {
        // 探测失败由请求层提示，仍刷新列表以同步最新探测状态
      } finally {
        setProbing((p) => ({ ...p, [row.id]: false }))
        void loadData()
      }
    },
    [loadData],
  )

  const handleDelete = useCallback(
    async (row: HostNodeItem) => {
      const ok = await confirmModal({
        title: '删除节点',
        content: `确定删除节点 ${row.name} 吗？删除后跨节点迁移将无法选择该节点。`,
        okText: '确定删除',
        danger: true,
      })
      if (!ok) return
      try {
        await deleteHostNode(row.id)
        Toast.success('节点已删除')
        void loadData()
      } catch {
        // 请求层已统一提示
      }
    },
    [loadData],
  )

  /** 行内操作区（表格与移动卡片共用） */
  const renderActions = useCallback(
    (row: HostNodeItem) => (
      <div className="node-act-cell">
        <Tooltip content="探测" position="top">
          <span
            className={`node-act-ic probe${probing[row.id] ? ' loading' : ''}`}
            onClick={() => {
              if (!probing[row.id]) void handleProbe(row)
            }}
          >
            {probing[row.id] ? <IconRefresh spin /> : <IconPulse />}
          </span>
        </Tooltip>
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
              <Dropdown.Divider />
              <Dropdown.Item
                icon={<IconDelete />}
                type="danger"
                onClick={() => void handleDelete(row)}
              >
                删除
              </Dropdown.Item>
            </Dropdown.Menu>
          }
        >
          <span className="node-act-ic more">
            <IconMore />
          </span>
        </Dropdown>
      </div>
    ),
    [probing, handleProbe, handleDelete],
  )

  // ==================== 表格列 ====================
  const columns: ColumnProps<HostNodeItem>[] = [
    {
      title: '节点名称',
      dataIndex: 'name',
      width: 150,
      render: (text) => <span className="node-name-text">{text}</span>,
    },
    {
      title: '面板地址',
      dataIndex: 'api_base_url',
      width: 220,
      ellipsis: { showTitle: true },
      render: (text) => <span className="qvm-mono">{text}</span>,
    },
    {
      title: 'SSH',
      dataIndex: 'ssh_host',
      width: 200,
      render: (_text, row) => (
        <span className="qvm-mono">
          {row.ssh_user}@{row.ssh_host}:{row.ssh_port}
        </span>
      ),
    },
    {
      title: '状态',
      dataIndex: 'status',
      width: 90,
      align: 'center',
      render: (text) => (
        <Tag size="small" color={statusTagColor(text)}>
          {statusText(text)}
        </Tag>
      ),
    },
    {
      title: '最近探测',
      dataIndex: 'last_probe_message',
      ellipsis: { showTitle: true },
      render: (text, row) => (
        <div>
          <div className="node-probe-msg">{text || '-'}</div>
          {row.last_probed_at && (
            <div className="node-muted">{formatDateTime(row.last_probed_at)}</div>
          )}
        </div>
      ),
    },
    {
      title: '启用',
      dataIndex: 'enabled',
      width: 80,
      align: 'center',
      render: (enabled) => (
        <Tag size="small" color={enabled ? 'green' : 'grey'}>
          {enabled ? '启用' : '禁用'}
        </Tag>
      ),
    },
    {
      title: '操作',
      dataIndex: 'actions',
      width: 100,
      render: (_text, row) => renderActions(row),
    },
  ]

  // ==================== 渲染 ====================
  if (!isAdmin) {
    return (
      <div className="node-page">
        <div className="node-empty">
          <div className="node-empty-icon">
            <IconLock />
          </div>
          <div>节点管理仅对管理员开放</div>
        </div>
      </div>
    )
  }

  return (
    <div className="node-page">
      <div className="node-page-header qvm-fade-up">
        <div>
          <h2>
            <IconServerStroked style={{ marginRight: 8, color: 'var(--qvm-acc-ink)' }} />
            节点管理
          </h2>
          <p className="node-page-sub">管理跨节点迁移的目标节点，探测校验面板 API 与 SSH 连通性</p>
        </div>
        <div className="node-header-actions">
          <Button icon={<IconRefresh />} loading={loading} onClick={() => void loadData()}>
            刷新
          </Button>
          <Button
            type="primary"
            theme="light"
            icon={<IconPlus />}
            onClick={() => setDialog({ type: 'edit' })}
          >
            添加节点
          </Button>
        </div>
      </div>

      <div className="node-filter-bar qvm-fade-up">
        <Input
          prefix={<IconSearch />}
          placeholder="搜索名称"
          value={searchText}
          onChange={setSearchText}
          showClear
          style={{ width: 180 }}
        />
        <Select
          value={statusFilter || undefined}
          onChange={(v) => setStatusFilter((v as string) || '')}
          placeholder="状态筛选"
          showClear
          style={{ width: 130 }}
          optionList={[
            { label: '在线', value: 'online' },
            { label: '异常', value: 'error' },
            { label: '未知', value: 'unknown' },
          ]}
        />
        <Select
          value={enabledFilter || undefined}
          onChange={(v) => setEnabledFilter((v as string) || '')}
          placeholder="启用状态"
          showClear
          style={{ width: 130 }}
          optionList={[
            { label: '启用', value: 'enabled' },
            { label: '禁用', value: 'disabled' },
          ]}
        />
      </div>

      {/* ==================== 桌面表格视图 ==================== */}
      <div className="node-table-card qvm-fade-up">
        <Table<HostNodeItem>
          rowKey="id"
          columns={columns}
          dataSource={paged}
          loading={loading}
          pagination={false}
          size="small"
          empty="暂无节点，点击右上角添加"
        />
        {filtered.length > PAGE_SIZE && (
          <div className="node-pagination">
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

      {/* ==================== 移动端卡片视图（≤768px） ==================== */}
      <div className="node-mobile-list qvm-fade-up">
        <Spin spinning={loading}>
          {paged.length === 0 && !loading && (
            <div className="node-empty">暂无节点，点击右上角添加</div>
          )}
          {paged.map((row) => (
            <div className="node-mobile-card" key={row.id}>
              <div className="node-mobile-head">
                <span className="node-name-text">{row.name}</span>
                <span className="node-mobile-tags">
                  <Tag size="small" color={statusTagColor(row.status)}>
                    {statusText(row.status)}
                  </Tag>
                  <Tag size="small" color={row.enabled ? 'green' : 'grey'}>
                    {row.enabled ? '启用' : '禁用'}
                  </Tag>
                </span>
              </div>
              <div className="node-mobile-body">
                <div className="node-mobile-row">
                  <span className="node-mobile-label">面板地址</span>
                  <span className="node-mobile-value qvm-mono">{row.api_base_url}</span>
                </div>
                <div className="node-mobile-row">
                  <span className="node-mobile-label">SSH</span>
                  <span className="node-mobile-value qvm-mono">
                    {row.ssh_user}@{row.ssh_host}:{row.ssh_port}
                  </span>
                </div>
                {row.last_probe_message && (
                  <div className="node-mobile-row">
                    <span className="node-mobile-label">最近探测</span>
                    <span className="node-mobile-value">{row.last_probe_message}</span>
                  </div>
                )}
              </div>
              <div className="node-mobile-actions">{renderActions(row)}</div>
            </div>
          ))}
        </Spin>
      </div>

      {/* ==================== 弹窗 ==================== */}
      {dialog?.type === 'edit' && (
        <NodeDialog
          row={dialog.row}
          onClose={() => setDialog(null)}
          onSaved={() => {
            void loadData()
          }}
        />
      )}
    </div>
  )
}
