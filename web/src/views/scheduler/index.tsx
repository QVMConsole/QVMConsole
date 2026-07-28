/**
 * 调度事件中心页（仅管理员）
 * - 调度器概览卡片：展示已注册调度器的启用状态与最近事件时间
 * - 调度事件列表：支持调度器/状态/虚拟机/时间范围筛选，服务端分页
 * - SSE 实时推送：新事件自动插入首页列表并同步概览最近事件时间
 */
import { useCallback, useEffect, useRef, useState } from 'react'
import {
  Button,
  DatePicker,
  Empty,
  Input,
  Pagination,
  Select,
  Spin,
  Table,
  Tag,
} from '@douyinfe/semi-ui'
import {
  IconClockStroked,
  IconLock,
  IconPulse,
  IconRefresh,
  IconSearch,
} from '@douyinfe/semi-icons'
import type { ColumnProps } from '@douyinfe/semi-ui/lib/es/table'
import {
  createSchedulerEventSSE,
  getSchedulerEventList,
  getSchedulerList,
  type SchedulerEventItem,
  type SchedulerEventMessage,
  type SchedulerInfo,
} from '@/api/scheduler'
import { useUserStore } from '@/stores/user'
import { ROLES } from '@/config/constants'
import { formatDateTime } from '@/utils/format'
import './scheduler.css'

type SseStatus = 'connecting' | 'connected' | 'disconnected'

/** 调度状态中文文案 */
function statusText(status: string): string {
  const map: Record<string, string> = {
    running: '正在执行',
    success: '执行完毕',
    failed: '执行失败',
  }
  return map[status] || status
}

/** 调度状态标签配色 */
function statusTagColor(status: string): 'blue' | 'green' | 'red' | 'grey' {
  const map: Record<string, 'blue' | 'green' | 'red'> = {
    running: 'blue',
    success: 'green',
    failed: 'red',
  }
  return map[status] || 'grey'
}

/** 时间参数格式化为后端支持的 "YYYY-MM-DD HH:mm:ss" */
function fmtTimeParam(d: Date): string {
  const p = (n: number) => String(n).padStart(2, '0')
  return `${d.getFullYear()}-${p(d.getMonth() + 1)}-${p(d.getDate())} ${p(d.getHours())}:${p(d.getMinutes())}:${p(d.getSeconds())}`
}

/** 筛选条件 */
interface EventFilters {
  schedulerKey: string
  status: string
  vmName: string
  range: Date[]
}

const EMPTY_FILTERS: EventFilters = { schedulerKey: '', status: '', vmName: '', range: [] }

export default function SchedulerPage() {
  const role = useUserStore((s) => s.role)
  const isAdmin = role === ROLES.admin

  // 概览
  const [overview, setOverview] = useState<SchedulerInfo[]>([])
  const [overviewLoading, setOverviewLoading] = useState(false)

  // 事件列表（服务端分页）
  const [events, setEvents] = useState<SchedulerEventItem[]>([])
  const [total, setTotal] = useState(0)
  const [loading, setLoading] = useState(false)
  const [filters, setFilters] = useState<EventFilters>(EMPTY_FILTERS)
  const [page, setPage] = useState(1)
  const [pageSize, setPageSize] = useState(20)

  // SSE 实时连接
  const [sseStatus, setSseStatus] = useState<SseStatus>('connecting')
  const esRef = useRef<EventSource | null>(null)
  const reconnectRef = useRef<number | null>(null)

  // SSE 回调中读取最新筛选/分页状态（避免闭包过期）
  const filtersRef = useRef(filters)
  filtersRef.current = filters
  const pageRef = useRef(page)
  pageRef.current = page
  const pageSizeRef = useRef(pageSize)
  pageSizeRef.current = pageSize

  // ==================== 数据加载 ====================
  const fetchOverview = useCallback(async () => {
    setOverviewLoading(true)
    try {
      const res = await getSchedulerList()
      setOverview(res.data || [])
    } catch {
      // 请求层已统一提示
    } finally {
      setOverviewLoading(false)
    }
  }, [])

  const fetchEvents = useCallback(
    async (targetPage: number, targetPageSize: number, f: EventFilters) => {
      setLoading(true)
      try {
        const res = await getSchedulerEventList({
          page: targetPage,
          page_size: targetPageSize,
          scheduler_key: f.schedulerKey || undefined,
          status: f.status || undefined,
          vm_name: f.vmName || undefined,
          start: f.range[0] ? fmtTimeParam(f.range[0]) : undefined,
          end: f.range[1] ? fmtTimeParam(f.range[1]) : undefined,
        })
        setEvents(res.data?.list || [])
        setTotal(res.data?.total || 0)
      } catch {
        // 请求层已统一提示
      } finally {
        setLoading(false)
      }
    },
    [],
  )

  const handleSearch = useCallback(() => {
    setPage(1)
    void fetchEvents(1, pageSizeRef.current, filtersRef.current)
  }, [fetchEvents])

  const handleReset = useCallback(() => {
    setFilters(EMPTY_FILTERS)
    setPage(1)
    void fetchEvents(1, pageSizeRef.current, EMPTY_FILTERS)
  }, [fetchEvents])

  // ==================== SSE 实时事件 ====================
  /** 判断事件是否命中当前筛选条件 */
  const matchesFilters = useCallback((event: SchedulerEventItem): boolean => {
    const f = filtersRef.current
    if (f.schedulerKey && event.scheduler_key !== f.schedulerKey) return false
    if (f.status && event.status !== f.status) return false
    if (f.vmName && !(event.vm_name || '').includes(f.vmName)) return false
    if (f.range[0] && f.range[1]) {
      const createdAt = new Date(event.created_at).getTime()
      if (
        !Number.isNaN(createdAt) &&
        (createdAt < f.range[0].getTime() || createdAt > f.range[1].getTime())
      ) {
        return false
      }
    }
    return true
  }, [])

  /** 插入或更新事件行，并同步概览最近事件时间 */
  const upsertEvent = useCallback(
    (event: SchedulerEventItem) => {
      setOverview((prev) =>
        prev.map((item) =>
          item.key === event.scheduler_key ? { ...item, last_event_at: event.created_at } : item,
        ),
      )
      setEvents((prev) => {
        const idx = prev.findIndex((row) => row.id === event.id)
        if (idx !== -1) {
          const next = prev.slice()
          next[idx] = event
          return next
        }
        // 仅第一页且命中筛选时头部插入新事件
        if (pageRef.current !== 1 || !matchesFilters(event)) return prev
        const next = [event, ...prev]
        if (next.length > pageSizeRef.current) next.pop()
        setTotal((t) => t + 1)
        return next
      })
    },
    [matchesFilters],
  )

  const connectSSE = useCallback(() => {
    const { token } = useUserStore.getState()
    if (!token) return
    setSseStatus('connecting')
    const es = createSchedulerEventSSE(token)
    esRef.current = es

    es.addEventListener('connected', () => setSseStatus('connected'))
    es.addEventListener('scheduler_event', (e) => {
      try {
        const payload = JSON.parse((e as MessageEvent).data) as SchedulerEventMessage
        if (payload?.event) upsertEvent(payload.event)
      } catch (err) {
        console.error('解析调度事件 SSE 失败', err)
      }
    })
    es.onerror = () => {
      setSseStatus('disconnected')
      es.close()
      esRef.current = null
      // 5s 后自动重连
      if (reconnectRef.current) window.clearTimeout(reconnectRef.current)
      reconnectRef.current = window.setTimeout(() => {
        reconnectRef.current = null
        connectSSE()
      }, 5000)
    }
  }, [upsertEvent])

  // ==================== 生命周期 ====================
  useEffect(() => {
    if (!isAdmin) return
    void fetchOverview()
    void fetchEvents(1, 20, EMPTY_FILTERS)
    connectSSE()
    return () => {
      if (reconnectRef.current) window.clearTimeout(reconnectRef.current)
      esRef.current?.close()
      esRef.current = null
    }
  }, [isAdmin, fetchOverview, fetchEvents, connectSSE])

  // ==================== 表格列 ====================
  const columns: ColumnProps<SchedulerEventItem>[] = [
    {
      title: '触发时间',
      dataIndex: 'created_at',
      width: 170,
      render: (text) => <span className="qvm-mono">{formatDateTime(text)}</span>,
    },
    { title: '虚拟机', dataIndex: 'vm_name', width: 160 },
    { title: '调度器类型', dataIndex: 'scheduler_name', width: 180 },
    {
      title: '调度状态',
      dataIndex: 'status',
      width: 110,
      align: 'center',
      render: (text) => (
        <Tag size="small" color={statusTagColor(text)}>
          {statusText(text)}
        </Tag>
      ),
    },
    {
      title: '调度原因',
      dataIndex: 'trigger_reason',
      ellipsis: { showTitle: true },
      render: (text) => text || '-',
    },
    {
      title: '执行结果 / 失败原因',
      dataIndex: 'result_message',
      ellipsis: { showTitle: true },
      render: (_text, row) => row.error_message || row.result_message || '-',
    },
    {
      title: '完成时间',
      dataIndex: 'finished_at',
      width: 170,
      render: (text) => <span className="qvm-mono">{formatDateTime(text)}</span>,
    },
  ]

  // ==================== 渲染 ====================
  if (!isAdmin) {
    return (
      <div className="sch-page">
        <div className="sch-empty">
          <div className="sch-empty-icon">
            <IconLock />
          </div>
          <div>调度事件中心仅对管理员开放</div>
        </div>
      </div>
    )
  }

  return (
    <div className="sch-page">
      <div className="sch-page-header qvm-fade-up">
        <div>
          <h2>
            <IconClockStroked style={{ marginRight: 8, color: 'var(--qvm-acc-ink)' }} />
            调度事件
          </h2>
          <p className="sch-page-sub">仅记录实际发生调度动作尝试的事件，不记录普通轮询扫描</p>
        </div>
        <div className="sch-header-actions">
          <span className={`sch-sse-tag ${sseStatus}`}>
            <IconPulse size="small" />
            {sseStatus === 'connected' ? '实时连接' : sseStatus === 'connecting' ? '连接中…' : '连接断开'}
          </span>
          <Button
            icon={<IconRefresh />}
            loading={overviewLoading || loading}
            onClick={() => {
              void fetchOverview()
              void fetchEvents(page, pageSize, filters)
            }}
          >
            刷新
          </Button>
        </div>
      </div>

      {/* ==================== 调度器概览 ==================== */}
      <div className="sch-section-card qvm-fade-up">
        <div className="sch-section-head">
          <div>
            <div className="sch-section-title">调度器概览</div>
            <div className="sch-section-sub">当前先接入动态内存体系下的调度器，后续可继续扩展</div>
          </div>
        </div>
        <Spin spinning={overviewLoading}>
          {overview.length > 0 ? (
            <div className="sch-overview-grid">
              {overview.map((item) => (
                <div className="sch-overview-item" key={item.key}>
                  <div className="sch-overview-top">
                    <div>
                      <div className="sch-overview-title">{item.name}</div>
                      <div className="sch-overview-key qvm-mono">{item.key}</div>
                    </div>
                    <Tag size="small" color={item.enabled ? 'green' : 'grey'}>
                      {item.enabled ? '已启用' : '已停用'}
                    </Tag>
                  </div>
                  <div className="sch-overview-line">分组：{item.group || '-'}</div>
                  <div className="sch-overview-line">{item.description || '暂无说明'}</div>
                  <div className="sch-overview-line muted">
                    最近事件：{formatDateTime(item.last_event_at)}
                  </div>
                </div>
              ))}
            </div>
          ) : (
            <Empty description="暂无已注册调度器" style={{ padding: '24px 0' }} />
          )}
        </Spin>
      </div>

      {/* ==================== 调度事件列表 ==================== */}
      <div className="sch-section-card qvm-fade-up">
        <div className="sch-filter-bar">
          <Select
            value={filters.schedulerKey || undefined}
            onChange={(v) => setFilters((f) => ({ ...f, schedulerKey: (v as string) || '' }))}
            placeholder="调度器"
            showClear
            style={{ width: 200 }}
            optionList={overview.map((item) => ({ label: item.name, value: item.key }))}
          />
          <Select
            value={filters.status || undefined}
            onChange={(v) => setFilters((f) => ({ ...f, status: (v as string) || '' }))}
            placeholder="状态"
            showClear
            style={{ width: 130 }}
            optionList={[
              { label: '正在执行', value: 'running' },
              { label: '执行完毕', value: 'success' },
              { label: '执行失败', value: 'failed' },
            ]}
          />
          <Input
            prefix={<IconSearch />}
            placeholder="虚拟机名称"
            value={filters.vmName}
            onChange={(v) => setFilters((f) => ({ ...f, vmName: v }))}
            showClear
            style={{ width: 180 }}
          />
          <DatePicker
            type="dateTimeRange"
            value={filters.range}
            onChange={(v) => setFilters((f) => ({ ...f, range: Array.isArray(v) ? (v as Date[]) : [] }))}
            style={{ width: 380 }}
          />
          <Button type="primary" theme="light" onClick={handleSearch}>
            查询
          </Button>
          <Button onClick={handleReset}>重置</Button>
        </div>

        <Table<SchedulerEventItem>
          rowKey="id"
          columns={columns}
          dataSource={events}
          loading={loading}
          pagination={false}
          size="small"
          empty="暂无调度事件"
        />
        <div className="sch-pagination">
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
              void fetchEvents(p, ps, filtersRef.current)
            }}
          />
        </div>
      </div>
    </div>
  )
}
