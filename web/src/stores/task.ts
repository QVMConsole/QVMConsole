/**
 * 任务中心全局状态管理（Zustand）
 * - 登录后由主布局启动 SSE，全局共享任务进度
 * - 任务列表首屏走 /task/list，后续通过 SSE 增量更新
 */
import { createElement } from 'react'
import { create } from 'zustand'
import { Notification } from '@douyinfe/semi-ui'
import {
  getTaskList,
  createTaskSSE,
  type TaskItem,
  type TaskProgressEvent,
} from '@/api/task'
import { useUserStore } from '@/stores/user'
import TaskMessage from '@/components/business/TaskMessage'

export type TaskSseStatus = 'connecting' | 'connected' | 'disconnected'

/** 终态任务状态 */
export const TERMINAL_STATUSES = ['success', 'failed', 'canceled']

interface TaskState {
  tasks: TaskItem[]
  total: number
  loading: boolean
  sseStatus: TaskSseStatus
  /** 运行中/等待中的任务数 */
  activeCount: () => number
  /** 最新的一个活动任务（任务栏头部展示） */
  currentActive: () => TaskItem | undefined
  fetchTasks: (page?: number, pageSize?: number) => Promise<void>
  startSSE: () => void
  stopSSE: () => void
  reset: () => void
}

let eventSource: EventSource | null = null
let reconnectTimer: ReturnType<typeof setTimeout> | null = null
const notifiedTerminalTasks = new Set<string>()

/** 在当前登录会话内提示任务的成功或失败结果，避免 SSE 重连时重复弹出。 */
function notifyTaskResult(event: TaskProgressEvent) {
  if (event.status !== 'success' && event.status !== 'failed') return

  const notificationKey = `${event.task_id}:${event.status}`
  if (notifiedTerminalTasks.has(notificationKey)) return
  notifiedTerminalTasks.add(notificationKey)

  const taskName = taskTypeText(event.type)
  const taskLabel = `${taskName}（任务 #${event.task_id}）`
  if (event.status === 'success') {
    Notification.success({
      title: '任务执行成功',
      content: `${taskLabel}已执行成功`,
      duration: 5,
    })
    return
  }

  Notification.error({
    title: '任务执行失败',
    content: createElement(TaskMessage, {
      message: event.message
        ? `${taskLabel}执行失败：${event.message}`
        : `${taskLabel}执行失败，请前往任务中心查看详情`,
    }),
    duration: 8,
  })
}

export const useTaskStore = create<TaskState>()((set, get) => ({
  tasks: [],
  total: 0,
  loading: false,
  sseStatus: 'disconnected',

  activeCount: () =>
    get().tasks.filter((t) => t.status === 'pending' || t.status === 'running').length,

  currentActive: () =>
    get().tasks.find((t) => t.status === 'running') ||
    get().tasks.find((t) => t.status === 'pending'),

  fetchTasks: async (page = 1, pageSize = 20) => {
    set({ loading: true })
    try {
      const res = await getTaskList({ page, page_size: pageSize })
      set({ tasks: res.data?.list || [], total: res.data?.total || 0 })
    } catch {
      // 请求层已统一提示
    } finally {
      set({ loading: false })
    }
  },

  startSSE: () => {
    const { token } = useUserStore.getState()
    if (!token || eventSource) return
    set({ sseStatus: 'connecting' })
    eventSource = createTaskSSE(token)

    eventSource.addEventListener('connected', () => {
      set({ sseStatus: 'connected' })
    })

    eventSource.addEventListener('task_progress', (e) => {
      try {
        const event = JSON.parse((e as MessageEvent).data) as TaskProgressEvent
        const { tasks, fetchTasks } = get()
        notifyTaskResult(event)
        const idx = tasks.findIndex((t) => t.id === event.task_id)
        if (idx === -1) {
          // 新任务：刷新列表
          void fetchTasks()
          return
        }
        const next = tasks.slice()
        next[idx] = {
          ...next[idx],
          status: event.status,
          progress: event.progress,
          message: event.message,
        }
        set({ tasks: next })
      } catch (err) {
        console.error('解析任务 SSE 事件失败', err)
      }
    })

    eventSource.onerror = () => {
      set({ sseStatus: 'disconnected' })
      eventSource?.close()
      eventSource = null
      // 5s 后自动重连
      if (reconnectTimer) clearTimeout(reconnectTimer)
      reconnectTimer = setTimeout(() => {
        reconnectTimer = null
        get().startSSE()
      }, 5000)
    }
  },

  stopSSE: () => {
    if (reconnectTimer) {
      clearTimeout(reconnectTimer)
      reconnectTimer = null
    }
    eventSource?.close()
    eventSource = null
    set({ sseStatus: 'disconnected' })
  },

  reset: () => {
    get().stopSSE()
    notifiedTerminalTasks.clear()
    set({ tasks: [], total: 0, loading: false })
  },
}))

// ==================== 展示辅助 ====================

/** 任务类型中文文案映射（任务中心筛选项与展示共用） */
export const TASK_TYPE_TEXT: Record<string, string> = {
  clone: '链式克隆',
  batch: '批量克隆',
  reinstall: '重装系统',
  prepare: '制作模板',
  template_export: '导出模板',
  template_import: '导入模板',
  template_linux_prepare: 'Linux 模板预处理',
  delete_template: '删除模板',
  create: '普通创建',
  lightweight_vm_provision: '轻量云开通',
  lightweight_runtime_quota_shutdown: '轻量云时长关机',
  delete: '删除虚拟机',
  snapshot: '快照操作',
  export: '导出虚拟机',
  import: '导入虚拟机',
  import_appliance: '导入虚拟机包',
  vm_migrate: '迁移虚拟机',
  vm_disk_migrate: '迁移硬盘',
  storage_format: '格式化存储',
  storage_create_partition: '创建分区',
  storage_delete_partitions: '删除分区',
  ovs_repair: 'OVS 修复',
  port_mirror: '端口镜像',
  network_capture: '网络抓包',
  vm_schedule_action: '虚拟机定时任务',
  password_breach_scan: '泄露密码扫描',
  password_breach_notify: '泄露密码通知',
  power: '电源操作',
}

/** 任务类型中文文案 */
export function taskTypeText(type: string): string {
  return TASK_TYPE_TEXT[type] || type
}

/** 任务类型标签配色（对应设计稿 type-tag） */
export function taskTypeColor(type: string): { color: string; bg: string; border: string } {
  const palette: Record<string, { color: string; bg: string; border: string }> = {
    clone: { color: '#B7A2F7', bg: 'rgba(139,92,246,.12)', border: 'rgba(139,92,246,.25)' },
    batch: { color: '#B7A2F7', bg: 'rgba(139,92,246,.12)', border: 'rgba(139,92,246,.25)' },
    snapshot: { color: '#7DD3FC', bg: 'rgba(56,189,248,.12)', border: 'rgba(56,189,248,.25)' },
    prepare: { color: '#5EEAD4', bg: 'rgba(45,212,191,.12)', border: 'rgba(45,212,191,.25)' },
    template_export: { color: '#5EEAD4', bg: 'rgba(45,212,191,.12)', border: 'rgba(45,212,191,.25)' },
    template_import: { color: '#5EEAD4', bg: 'rgba(45,212,191,.12)', border: 'rgba(45,212,191,.25)' },
    import_appliance: { color: '#7DD3FC', bg: 'rgba(56,189,248,.12)', border: 'rgba(56,189,248,.25)' },
    create: { color: '#6EE7B7', bg: 'rgba(52,211,153,.12)', border: 'rgba(52,211,153,.25)' },
    power: { color: '#6EE7B7', bg: 'rgba(52,211,153,.12)', border: 'rgba(52,211,153,.25)' },
    delete: { color: '#FDA4AF', bg: 'rgba(251,113,133,.12)', border: 'rgba(251,113,133,.25)' },
    delete_template: { color: '#FDA4AF', bg: 'rgba(251,113,133,.12)', border: 'rgba(251,113,133,.25)' },
    network_capture: { color: '#FCD34D', bg: 'rgba(251,191,36,.12)', border: 'rgba(251,191,36,.25)' },
    port_mirror: { color: '#67E8F9', bg: 'rgba(34,211,238,.12)', border: 'rgba(34,211,238,.25)' },
  }
  return (
    palette[type] || {
      color: '#B7A2F7',
      bg: 'rgba(139,92,246,.12)',
      border: 'rgba(139,92,246,.25)',
    }
  )
}

/** 任务状态中文文案 */
export function taskStatusText(status: string): string {
  const map: Record<string, string> = {
    pending: '等待中',
    running: '执行中',
    success: '成功',
    failed: '失败',
    canceled: '已取消',
  }
  return map[status] || status
}
