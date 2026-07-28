/**
 * 任务中心页（所有登录用户可见，普通用户仅能看到自己的任务）
 * - 任务列表：支持状态/类型筛选，服务端分页
 * - 实时进度：复用全局任务 Store 的 SSE 连接（不重复建连）
 * - 行内操作：详情 / 取消（纯图标 + Tooltip 模式）
 */
import { useCallback, useEffect, useRef, useState } from 'react'
import { Button, Pagination, Progress, Select, Table, Tag, Toast, Tooltip } from '@douyinfe/semi-ui'
import {
  IconCheckList,
  IconClose,
  IconDelete,
  IconEyeOpenedStroked,
  IconPulse,
  IconRefresh,
} from '@douyinfe/semi-icons'
import type { ColumnProps } from '@douyinfe/semi-ui/lib/es/table'
import {
  cancelTask,
  clearFinishedTasks,
  getTaskDetail,
  getTaskList,
  type TaskItem,
} from '@/api/task'
import {
  TASK_TYPE_TEXT,
  TERMINAL_STATUSES,
  taskStatusText,
  taskTypeColor,
  taskTypeText,
  useTaskStore,
} from '@/stores/task'
import TaskDetailSheet from '@/components/business/TaskDetailSheet'
import { confirmModal } from '@/utils/confirm'
import { formatDateTime } from '@/utils/format'
import './task.css'

/** 任务状态标签配色 */
function statusTagColor(status: string): 'grey' | 'blue' | 'green' | 'red' | 'amber' {
  const map: Record<string, 'grey' | 'blue' | 'green' | 'red' | 'amber'> = {
    pending: 'grey',
    running: 'blue',
    success: 'green',
    failed: 'red',
    canceled: 'amber',
  }
  return map[status] || 'grey'
}

/** 进度条配色 */
function progressStroke(status: string): string {
  if (status === 'success') return 'var(--semi-color-success)'
  if (status === 'failed') return 'var(--semi-color-danger)'
  if (status === 'canceled') return 'var(--semi-color-warning)'
  return 'var(--semi-color-primary)'
}

const STATUS_OPTIONS = [
  { label: '等待中', value: 'pending' },
  { label: '执行中', value: 'running' },
  { label: '成功', value: 'success' },
  { label: '失败', value: 'failed' },
  { label: '已取消', value: 'canceled' },
]

const TYPE_OPTIONS = Object.entries(TASK_TYPE_TEXT).map(([value, label]) => ({ label, value }))

export default function TaskCenterPage() {
  // 列表数据（本页独立分页，与全局任务栏的首页数据互不影响）
  const [list, setList] = useState<TaskItem[]>([])
  const [total, setTotal] = useState(0)
  const [loading, setLoading] = useState(false)
  const [statusFilter, setStatusFilter] = useState('')
  const [typeFilter, setTypeFilter] = useState('')
  const [page, setPage] = useState(1)
  const [pageSize, setPageSize] = useState(20)

  // 详情抽屉
  const [detailVisible, setDetailVisible] = useState(false)
  const [detailTask, setDetailTask] = useState<TaskItem | null>(null)

  // 复用全局 SSE：订阅任务 Store 增量更新
  const storeTasks = useTaskStore((s) => s.tasks)
  const sseStatus = useTaskStore((s) => s.sseStatus)
  const fetchStoreTasks = useTaskStore((s) => s.fetchTasks)

  // SSE 合并回调中读取最新状态（避免闭包过期）
  const listRef = useRef(list)
  listRef.current = list
  const pageRef = useRef(page)
  pageRef.current = page
  const filterRef = useRef({ status: statusFilter, type: typeFilter })
  filterRef.current = { status: statusFilter, type: typeFilter }
  const pageSizeRef = useRef(pageSize)
  pageSizeRef.current = pageSize
  const detailRef = useRef(detailTask)
  detailRef.current = detailTask

  // ==================== 数据加载 ====================
  const fetchData = useCallback(
    async (targetPage: number, targetPageSize: number, status: string, type: string) => {
      setLoading(true)
      try {
        const res = await getTaskList({
          page: targetPage,
          page_size: targetPageSize,
          status: status || undefined,
          type: type || undefined,
        })
        setList(res.data?.list || [])
        setTotal(res.data?.total || 0)
      } catch {
        // 请求层已统一提示
      } finally {
        setLoading(false)
      }
    },
    [],
  )

  const refresh = useCallback(() => {
    void fetchData(pageRef.current, pageSize, filterRef.current.status, filterRef.current.type)
  }, [fetchData, pageSize])

  useEffect(() => {
    void fetchData(1, 20, '', '')
  }, [fetchData])

  /** 筛选变化：重置到第一页并重新拉取 */
  const handleFilterChange = useCallback(
    (status: string, type: string) => {
      setStatusFilter(status)
      setTypeFilter(type)
      setPage(1)
      void fetchData(1, pageSize, status, type)
    },
    [fetchData, pageSize],
  )

  // ==================== SSE 增量合并 ====================
  // 全局 Store 的任务进度变化时，同步合并到本页列表与详情抽屉
  useEffect(() => {
    if (storeTasks.length === 0) return
    const current = listRef.current

    // 更新本页已有任务行
    let changed = false
    const next = current.map((t) => {
      const s = storeTasks.find((x) => x.id === t.id)
      if (s && (s.status !== t.status || s.progress !== t.progress || s.message !== t.message)) {
        changed = true
        return { ...t, status: s.status, progress: s.progress, message: s.message }
      }
      return t
    })
    if (changed) setList(next)

    // 新任务出现（首页且无筛选时刷新本页列表）
    const f = filterRef.current
    if (
      pageRef.current === 1 &&
      !f.status &&
      !f.type &&
      storeTasks.some((s) => !current.some((t) => t.id === s.id))
    ) {
      void fetchData(1, pageSizeRef.current, '', '')
    }

    // 详情抽屉同步：任务到达终态时补拉完整详情（含执行结果）
    const detail = detailRef.current
    if (detail) {
      const s = storeTasks.find((x) => x.id === detail.id)
      if (s && (s.status !== detail.status || s.progress !== detail.progress || s.message !== detail.message)) {
        setDetailTask({ ...detail, status: s.status, progress: s.progress, message: s.message })
        if (TERMINAL_STATUSES.includes(s.status)) {
          void getTaskDetail(detail.id)
            .then((res) => {
              if (res.data && detailRef.current?.id === res.data.id) setDetailTask(res.data)
            })
            .catch(() => {
              // 静默失败，保留 SSE 已合并的状态
            })
        }
      }
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [storeTasks, fetchData])

  // ==================== 操作 ====================
  const openDetail = useCallback(async (row: TaskItem) => {
    setDetailTask(row)
    setDetailVisible(true)
    try {
      const res = await getTaskDetail(row.id)
      if (res.data) setDetailTask(res.data)
    } catch {
      // 请求层已提示
    }
  }, [])

  const handleCancel = useCallback(
    async (row: TaskItem) => {
      const isRunning = row.status === 'running'
      const ok = await confirmModal({
        title: '取消任务',
        content: isRunning
          ? `任务 #${row.id} 正在执行中，取消后已创建的资源将被自动清理。确定要取消吗？`
          : `确定要取消任务 #${row.id} 吗？`,
        okText: '确定取消',
        danger: true,
      })
      if (!ok) return
      try {
        await cancelTask(row.id)
        Toast.success(isRunning ? '取消信号已发送，任务将尽快停止' : '任务已取消')
        refresh()
        void fetchStoreTasks()
      } catch {
        // 请求层已提示
      }
    },
    [refresh, fetchStoreTasks],
  )

  const hasFinished = list.some((t) => TERMINAL_STATUSES.includes(t.status))

  const handleClear = useCallback(async () => {
    const ok = await confirmModal({
      title: '清理任务记录',
      content: '确定要清理所有已完成/失败/已取消的任务记录吗？',
      okText: '确定清理',
      danger: true,
    })
    if (!ok) return
    try {
      const res = await clearFinishedTasks()
      Toast.success(res.message || '清理完成')
      setPage(1)
      void fetchData(1, pageSize, statusFilter, typeFilter)
      void fetchStoreTasks()
    } catch {
      // 请求层已提示
    }
  }, [fetchData, pageSize, statusFilter, typeFilter, fetchStoreTasks])

  // ==================== 表格列 ====================
  const columns: ColumnProps<TaskItem>[] = [
    {
      title: 'ID',
      dataIndex: 'id',
      width: 70,
      align: 'center',
      render: (text) => <span className="qvm-mono">{text}</span>,
    },
    {
      title: '类型',
      dataIndex: 'type',
      width: 140,
      render: (text) => {
        const tc = taskTypeColor(text)
        return (
          <Tag
            size="small"
            style={{ color: tc.color, backgroundColor: tc.bg, border: `1px solid ${tc.border}` }}
          >
            {taskTypeText(text)}
          </Tag>
        )
      },
    },
    {
      title: '状态',
      dataIndex: 'status',
      width: 90,
      align: 'center',
      render: (text) => (
        <Tag size="small" color={statusTagColor(text)}>
          {taskStatusText(text)}
        </Tag>
      ),
    },
    {
      title: '进度',
      dataIndex: 'progress',
      width: 180,
      render: (_text, row) => (
        <div className="tsk-progress-cell">
          <Progress
            percent={row.progress || 0}
            stroke={progressStroke(row.status)}
            aria-label="任务进度"
          />
          <span className="tsk-progress-num qvm-mono">{row.progress || 0}%</span>
        </div>
      ),
    },
    {
      title: '状态消息',
      dataIndex: 'message',
      ellipsis: { showTitle: true },
      render: (text) => text || '-',
    },
    {
      title: '创建人',
      dataIndex: 'created_by',
      width: 100,
      align: 'center',
      render: (text) => text || '-',
    },
    {
      title: '创建时间',
      dataIndex: 'created_at',
      width: 170,
      align: 'center',
      render: (text) => <span className="qvm-mono">{formatDateTime(text)}</span>,
    },
    {
      title: '操作',
      dataIndex: 'actions',
      width: 90,
      render: (_text, row) => (
        <div className="tsk-act-cell">
          <Tooltip content="详情" position="top">
            <span className="tsk-act-ic detail" onClick={() => void openDetail(row)}>
              <IconEyeOpenedStroked />
            </span>
          </Tooltip>
          {(row.status === 'pending' || row.status === 'running') && (
            <Tooltip content="取消任务" position="top">
              <span className="tsk-act-ic cancel" onClick={() => void handleCancel(row)}>
                <IconClose />
              </span>
            </Tooltip>
          )}
        </div>
      ),
    },
  ]

  // ==================== 渲染 ====================
  return (
    <div className="tsk-page">
      <div className="tsk-page-header qvm-fade-up">
        <div>
          <h2>
            <IconCheckList style={{ marginRight: 8, color: 'var(--qvm-acc-ink)' }} />
            任务中心
          </h2>
          <p className="tsk-page-sub">异步任务队列执行记录与实时进度</p>
        </div>
        <div className="tsk-header-actions">
          <span className={`tsk-sse-tag ${sseStatus}`}>
            <IconPulse size="small" />
            {sseStatus === 'connected' ? '实时连接' : sseStatus === 'connecting' ? '连接中…' : '已断开'}
          </span>
          <Button icon={<IconRefresh />} loading={loading} onClick={refresh}>
            刷新
          </Button>
          <Button
            type="danger"
            theme="light"
            icon={<IconDelete />}
            disabled={!hasFinished}
            onClick={() => void handleClear()}
          >
            清理已完成
          </Button>
        </div>
      </div>

      <div className="tsk-filter-bar qvm-fade-up">
        <Select
          value={statusFilter || undefined}
          onChange={(v) => handleFilterChange((v as string) || '', typeFilter)}
          placeholder="任务状态"
          showClear
          style={{ width: 130 }}
          optionList={STATUS_OPTIONS}
        />
        <Select
          value={typeFilter || undefined}
          onChange={(v) => handleFilterChange(statusFilter, (v as string) || '')}
          placeholder="任务类型"
          showClear
          filter
          style={{ width: 180 }}
          optionList={TYPE_OPTIONS}
        />
      </div>

      <div className="tsk-table-card qvm-fade-up">
        <Table<TaskItem>
          rowKey="id"
          columns={columns}
          dataSource={list}
          loading={loading}
          pagination={false}
          size="small"
          empty="暂无任务记录"
        />
        <div className="tsk-pagination">
          <Pagination
            total={total}
            currentPage={page}
            pageSize={pageSize}
            pageSizeOpts={[10, 20, 50]}
            showSizeChanger
            showTotal
            onChange={(p, ps) => {
              setPage(p)
              setPageSize(ps)
              void fetchData(p, ps, statusFilter, typeFilter)
            }}
          />
        </div>
      </div>

      {/* 任务详情抽屉（与底部任务栏共用组件） */}
      <TaskDetailSheet
        task={detailTask}
        visible={detailVisible}
        onClose={() => setDetailVisible(false)}
      />
    </div>
  )
}
