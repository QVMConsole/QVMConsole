/**
 * 定时任务 Tab
 * - 一次性 / 每天 / 每周定时任务（开机 / 关机 / 删除虚拟机）
 * - 删除虚拟机属于高风险动作：仅支持一次性任务，触发 428 二次验证
 * - 支持启用/停用开关、编辑、删除
 */
import { useCallback, useEffect, useMemo, useState } from 'react'
import {
  Banner,
  Button,
  Checkbox,
  DatePicker,
  Modal,
  Radio,
  Select,
  Spin,
  Switch,
  Table,
  Tag,
  TimePicker,
  Toast,
} from '@douyinfe/semi-ui'
import { IconPlus } from '@douyinfe/semi-icons'
import type { ColumnProps } from '@douyinfe/semi-ui/lib/es/table'
import type { VmDetailInfo, VmScheduleItem, VmSchedulePayload } from '@/api/vm'
import {
  createVmSchedule,
  deleteVmSchedule,
  getVmSchedules,
  updateVmSchedule,
} from '@/api/vm'
import { confirmModal } from '@/utils/confirm'
import TextSwitch from '@/features/vm-form/sections/TextSwitch'

interface ScheduleTabProps {
  vm: VmDetailInfo | null
}

const WEEKDAY_OPTIONS = [
  { value: 1, label: '周一' },
  { value: 2, label: '周二' },
  { value: 3, label: '周三' },
  { value: 4, label: '周四' },
  { value: 5, label: '周五' },
  { value: 6, label: '周六' },
  { value: 7, label: '周日' },
]

interface ScheduleFormState {
  eventType: string
  action: string
  scheduleType: string
  runAt: Date | null
  timeOfDay: string
  weekdays: number[]
  enabled: boolean
}

const INITIAL_FORM: ScheduleFormState = {
  eventType: 'power',
  action: 'start',
  scheduleType: 'once',
  runAt: null,
  timeOfDay: '08:00',
  weekdays: [1, 2, 3, 4, 5],
  enabled: true,
}

function actionText(value: string): string {
  const map: Record<string, string> = { start: '开机', shutdown: '关机', delete: '删除虚拟机' }
  return map[value] || value
}

function lastStatusText(value?: string): string {
  const map: Record<string, string> = {
    pending: '等待中',
    running: '执行中',
    success: '执行成功',
    failed: '执行失败',
  }
  return map[value || ''] || '未执行'
}

function lastStatusColor(value?: string): 'grey' | 'blue' | 'green' | 'red' {
  const map: Record<string, 'grey' | 'blue' | 'green' | 'red'> = {
    pending: 'grey',
    running: 'blue',
    success: 'green',
    failed: 'red',
  }
  return map[value || ''] || 'grey'
}

function formatDateTime(value?: string, tz?: string): string {
  if (!value) return '-'
  const date = new Date(value)
  if (Number.isNaN(date.getTime())) return '-'
  const options: Intl.DateTimeFormatOptions = {
    year: 'numeric',
    month: '2-digit',
    day: '2-digit',
    hour: '2-digit',
    minute: '2-digit',
    second: '2-digit',
    hour12: false,
  }
  if (tz) {
    try {
      return new Intl.DateTimeFormat('zh-CN', { ...options, timeZone: tz }).format(date)
    } catch {
      // 时区不可用时回退
    }
  }
  return date.toLocaleString('zh-CN', options)
}

export default function ScheduleTab({ vm }: ScheduleTabProps) {
  const vmName = vm?.name || ''
  const browserTimezone = Intl.DateTimeFormat().resolvedOptions().timeZone || 'UTC'

  const [loading, setLoading] = useState(false)
  const [list, setList] = useState<VmScheduleItem[]>([])
  const [dialogVisible, setDialogVisible] = useState(false)
  const [editingId, setEditingId] = useState(0)
  const [submitting, setSubmitting] = useState(false)
  const [form, setForm] = useState<ScheduleFormState>(INITIAL_FORM)
  const [switchLoading, setSwitchLoading] = useState<Record<number, boolean>>({})

  const fetchList = useCallback(async () => {
    if (!vmName) return
    setLoading(true)
    try {
      const res = await getVmSchedules(vmName)
      setList(Array.isArray(res.data) ? res.data : [])
    } finally {
      setLoading(false)
    }
  }, [vmName])

  useEffect(() => {
    void fetchList()
  }, [fetchList])

  // 动作选项联动
  const actionOptions = useMemo(() => {
    if (form.eventType === 'vm') return [{ value: 'delete', label: '删除此虚拟机' }]
    return [
      { value: 'start', label: '定时开机' },
      { value: 'shutdown', label: '定时关机' },
    ]
  }, [form.eventType])

  const scheduleTypeOptions = useMemo(() => {
    if (form.action === 'delete') return [{ value: 'once', label: '一次性' }]
    return [
      { value: 'once', label: '一次性' },
      { value: 'daily', label: '每天' },
      { value: 'weekly', label: '每周' },
    ]
  }, [form.action])

  const setField = <K extends keyof ScheduleFormState>(key: K, value: ScheduleFormState[K]) => {
    setForm((f) => {
      const next = { ...f, [key]: value }
      // 事件类型联动
      if (key === 'eventType') {
        if (value === 'vm') {
          next.action = 'delete'
          next.scheduleType = 'once'
        } else if (!['start', 'shutdown'].includes(next.action)) {
          next.action = 'start'
        }
      }
      if (key === 'action') {
        if (value === 'delete') {
          next.eventType = 'vm'
          next.scheduleType = 'once'
        } else {
          next.eventType = 'power'
        }
      }
      if (key === 'scheduleType' && value !== 'weekly') {
        next.weekdays = [1, 2, 3, 4, 5]
      }
      return next
    })
  }

  const schedulePlanText = (row: VmScheduleItem): string => {
    const tz = row.timezone || browserTimezone
    if (row.schedule_type === 'once') return `一次性：${formatDateTime(row.run_at, tz)}`
    if (row.schedule_type === 'daily') return `每天 ${row.time_of_day || '--:--'}`
    const labels = WEEKDAY_OPTIONS.filter((d) => (row.weekdays || []).includes(d.value))
      .map((d) => d.label)
      .join('、')
    return `每周 ${labels || '--'} ${row.time_of_day || '--:--'}`
  }

  const openCreate = () => {
    setEditingId(0)
    setForm(INITIAL_FORM)
    setDialogVisible(true)
  }

  const openEdit = (row: VmScheduleItem) => {
    setEditingId(row.id)
    setForm({
      eventType: row.event_type || 'power',
      action: row.action || 'start',
      scheduleType: row.schedule_type || 'once',
      runAt: row.run_at ? new Date(row.run_at) : null,
      timeOfDay: row.time_of_day || '08:00',
      weekdays: Array.isArray(row.weekdays) && row.weekdays.length ? [...row.weekdays] : [1, 2, 3, 4, 5],
      enabled: row.enabled !== false,
    })
    setDialogVisible(true)
  }

  const buildPayload = (override: Partial<VmSchedulePayload> = {}): VmSchedulePayload => ({
    event_type: override.event_type || form.eventType,
    action: override.action || form.action,
    schedule_type: override.schedule_type || form.scheduleType,
    run_at:
      override.run_at !== undefined
        ? override.run_at
        : form.runAt
          ? form.runAt.toISOString()
          : '',
    timezone: override.timezone || browserTimezone,
    time_of_day: override.time_of_day || form.timeOfDay || '',
    weekdays:
      override.weekdays !== undefined
        ? override.weekdays
        : form.scheduleType === 'weekly'
          ? form.weekdays
          : [],
    enabled: override.enabled !== undefined ? override.enabled : form.enabled,
  })

  const validateForm = (): boolean => {
    if (form.scheduleType === 'once' && !form.runAt) {
      Toast.warning('请选择执行时间')
      return false
    }
    if (form.scheduleType !== 'once' && !form.timeOfDay) {
      Toast.warning('请选择执行时刻')
      return false
    }
    if (form.scheduleType === 'weekly' && form.weekdays.length === 0) {
      Toast.warning('请选择每周执行日期')
      return false
    }
    return true
  }

  const handleSubmit = async () => {
    if (!validateForm()) return
    setSubmitting(true)
    try {
      if (editingId) {
        await updateVmSchedule(vmName, editingId, buildPayload())
        Toast.success('定时任务已更新')
      } else {
        await createVmSchedule(vmName, buildPayload())
        Toast.success('定时任务已创建')
      }
      setDialogVisible(false)
      await fetchList()
    } finally {
      setSubmitting(false)
    }
  }

  const handleToggle = async (row: VmScheduleItem, enabled: boolean) => {
    setSwitchLoading((m) => ({ ...m, [row.id]: true }))
    try {
      await updateVmSchedule(vmName, row.id, {
        event_type: row.event_type,
        action: row.action,
        schedule_type: row.schedule_type,
        run_at: row.run_at || '',
        timezone: row.timezone || browserTimezone,
        time_of_day: row.time_of_day || '',
        weekdays: Array.isArray(row.weekdays) ? row.weekdays : [],
        enabled,
      })
      Toast.success(enabled ? '定时任务已启用' : '定时任务已停用')
      await fetchList()
    } finally {
      setSwitchLoading((m) => ({ ...m, [row.id]: false }))
    }
  }

  const handleDelete = async (row: VmScheduleItem) => {
    const ok = await confirmModal({
      title: '删除定时任务',
      content: `确定删除“${schedulePlanText(row)} / ${actionText(row.action)}”这条定时任务吗？`,
    })
    if (!ok) return
    try {
      await deleteVmSchedule(vmName, row.id)
      Toast.success('定时任务已删除')
      await fetchList()
    } catch {
      // 请求层已提示
    }
  }

  const columns: ColumnProps<VmScheduleItem>[] = [
    {
      title: '事件类型',
      dataIndex: 'event_type',
      width: 110,
      render: (text) => (
        <Tag size="small" color={text === 'vm' ? 'orange' : 'blue'}>
          {text === 'vm' ? '虚拟机事件' : '电源事件'}
        </Tag>
      ),
    },
    {
      title: '执行动作',
      dataIndex: 'action',
      width: 110,
      render: (text) => (
        <Tag size="small" color={text === 'delete' ? 'red' : text === 'start' ? 'green' : 'grey'}>
          {actionText(String(text || ''))}
        </Tag>
      ),
    },
    {
      title: '执行计划',
      dataIndex: 'schedule_type',
      render: (_text, row) => (
        <div>
          <div className="qvm-schedule-plan">{schedulePlanText(row)}</div>
          <div className="qvm-sub-label">时区：{row.timezone || browserTimezone}</div>
        </div>
      ),
    },
    {
      title: '下次执行',
      dataIndex: 'next_run_at',
      width: 170,
      render: (_text, row) =>
        row.enabled ? formatDateTime(row.next_run_at, row.timezone || browserTimezone) : '已停用',
    },
    {
      title: '最近执行',
      dataIndex: 'last_triggered_at',
      width: 170,
      render: (_text, row) => formatDateTime(row.last_triggered_at, row.timezone || browserTimezone),
    },
    {
      title: '最近结果',
      dataIndex: 'last_status',
      render: (_text, row) => (
        <div>
          <div className="qvm-schedule-result">
            <Tag size="small" color={lastStatusColor(row.last_status)}>
              {lastStatusText(row.last_status)}
            </Tag>
            {row.last_task_id ? <span className="qvm-sub-label">任务 #{row.last_task_id}</span> : null}
          </div>
          <div className="qvm-sub-label qvm-ellipsis">{row.last_message || '-'}</div>
        </div>
      ),
    },
    {
      title: '启用',
      dataIndex: 'enabled',
      width: 80,
      render: (_text, row) => (
        <Switch
          checked={row.enabled}
          loading={!!switchLoading[row.id]}
          onChange={(checked) => void handleToggle(row, checked)}
          size="small"
          checkedText="开"
          uncheckedText="关"
        />
      ),
    },
    {
      title: '操作',
      dataIndex: 'actions',
      width: 130,
      render: (_text, row) => (
        <div className="qvm-snap-actions">
          <Button size="small" theme="borderless" type="primary" onClick={() => openEdit(row)}>
            编辑
          </Button>
          <Button size="small" theme="borderless" type="danger" onClick={() => void handleDelete(row)}>
            删除
          </Button>
        </div>
      ),
    },
  ]

  if (!vm) {
    return (
      <div className="qvm-tab-loading">
        <Spin size="large" />
      </div>
    )
  }

  return (
    <div className="qvm-schedule-tab">
      <div className="qvm-tab-toolbar">
        <Banner
          type="info"
          closeIcon={null}
          className="qvm-schedule-banner"
          description="可为当前虚拟机设置一次性、每日或每周定时任务。删除虚拟机属于高风险动作，仅支持一次性任务，并会触发二次验证。"
        />
        <Button type="primary" icon={<IconPlus />} onClick={openCreate}>
          新增定时任务
        </Button>
      </div>

      <Table<VmScheduleItem>
        rowKey="id"
        columns={columns}
        dataSource={list}
        loading={loading}
        pagination={false}
        size="middle"
        empty="暂无定时任务"
      />

      {/* 新增/编辑对话框 */}
      <Modal
        title={editingId ? '编辑定时任务' : '新增定时任务'}
        visible={dialogVisible}
        onCancel={() => setDialogVisible(false)}
        onOk={() => void handleSubmit()}
        okText="保存"
        cancelText="取消"
        confirmLoading={submitting}
        width={620}
        closeOnEsc
      >
        <div className="qvm-form-item">
          <div className="qvm-form-label">事件类型</div>
          <Radio.Group
            type="button"
            value={form.eventType}
            onChange={(e) => setField('eventType', String(e.target.value))}
          >
            <Radio value="power">电源事件</Radio>
            <Radio value="vm">虚拟机事件</Radio>
          </Radio.Group>
        </div>

        <div className="qvm-form-item">
          <div className="qvm-form-label">执行动作</div>
          <Select
            style={{ width: '100%' }}
            value={form.action}
            optionList={actionOptions}
            onChange={(v) => setField('action', String(v))}
          />
        </div>

        <div className="qvm-form-item">
          <div className="qvm-form-label">执行计划</div>
          <Radio.Group
            type="button"
            value={form.scheduleType}
            onChange={(e) => setField('scheduleType', String(e.target.value))}
          >
            {scheduleTypeOptions.map((item) => (
              <Radio key={item.value} value={item.value}>
                {item.label}
              </Radio>
            ))}
          </Radio.Group>
        </div>

        {form.scheduleType === 'once' ? (
          <div className="qvm-form-item">
            <div className="qvm-form-label">执行时间</div>
            <DatePicker
              type="dateTime"
              style={{ width: '100%' }}
              placeholder="请选择执行时间"
              value={form.runAt ?? undefined}
              onChange={(v) => setField('runAt', (v as Date) || null)}
            />
          </div>
        ) : (
          <>
            <div className="qvm-form-item">
              <div className="qvm-form-label">执行时刻</div>
              <TimePicker
                style={{ width: '100%' }}
                format="HH:mm"
                placeholder="请选择时间"
                value={form.timeOfDay}
                onChange={(v) => setField('timeOfDay', typeof v === 'string' ? v : '')}
              />
            </div>
            {form.scheduleType === 'weekly' && (
              <div className="qvm-form-item">
                <div className="qvm-form-label">执行日期</div>
                <Checkbox.Group
                  options={WEEKDAY_OPTIONS}
                  value={form.weekdays}
                  onChange={(v) => setField('weekdays', v as number[])}
                />
              </div>
            )}
          </>
        )}

        <div className="qvm-form-item">
          <div className="qvm-form-label">浏览器时区</div>
          <span className="qvm-sub-label">{browserTimezone}</span>
        </div>

        <div className="qvm-form-item">
          <div className="qvm-form-label">状态</div>
          <TextSwitch
            checked={form.enabled}
            onChange={(checked) => setField('enabled', checked)}
            checkedText="开"
            uncheckedText="停"
          />
        </div>

        {form.action === 'delete' && (
          <Banner
            type="warning"
            closeIcon={null}
            description="删除虚拟机会走异步任务队列执行。任务触发后会自动清理该虚拟机关联的定时任务。"
          />
        )}
      </Modal>
    </div>
  )
}
