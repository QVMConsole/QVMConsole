/**
 * 底部常驻任务栏（PVE 式抽屉面板）
 * - 点击头部展开/收起，顶部拖拽调整高度（状态持久化）
 * - 任务进度来自全局任务 Store（SSE 实时推送）
 * - 支持任务详情抽屉与取消任务（二次确认）
 */
import { useCallback, useEffect, useRef, useState } from 'react'
import { useNavigate } from 'react-router'
import { Modal, Toast } from '@douyinfe/semi-ui'
import { IconCheckList, IconChevronUp, IconDesktop, IconPulse } from '@douyinfe/semi-icons'
import { useTaskStore, taskTypeText, taskTypeColor, taskStatusText } from '@/stores/task'
import { cancelTask, getTaskDetail, type TaskItem } from '@/api/task'
import TaskDetailSheet from '@/components/business/TaskDetailSheet'
import TaskMessage from '@/components/business/TaskMessage'

const COLLAPSED_HEIGHT = 46
const DEFAULT_HEIGHT = 320
const MAX_HEIGHT_RATIO = 0.7
const LS_HEIGHT = 'qvm_taskbar_height'
const LS_OPEN = 'qvm_taskbar_open'

/** 从任务参数构建可读描述 */
function buildTaskDesc(task: TaskItem): string {
  try {
    const params = JSON.parse(task.params || '{}') as Record<string, unknown>
    const pick = (...keys: string[]) =>
      keys.map((k) => params[k]).find((v) => typeof v === 'string' && v) as string | undefined
    const name = pick('name', 'vm_name', 'new_name', 'template', 'target', 'pool')
    if (name) return `${taskTypeText(task.type)} · ${name}`
    const names = params.names || params.vm_names
    if (Array.isArray(names) && names.length > 0) {
      return `${taskTypeText(task.type)} · ${names.slice(0, 2).join('、')}${names.length > 2 ? ` 等 ${names.length} 项` : ''}`
    }
  } catch {
    // params 非 JSON 时忽略
  }
  return taskTypeText(task.type)
}

/** 时间格式化：今天显示时分秒，否则显示月日时分 */
function formatTime(time: string): string {
  if (!time) return '-'
  const d = new Date(time)
  const now = new Date()
  const sameDay = d.toDateString() === now.toDateString()
  const hm = d.toLocaleTimeString('zh-CN', { hour: '2-digit', minute: '2-digit', second: '2-digit', hour12: false })
  if (sameDay) return hm
  return `${String(d.getMonth() + 1).padStart(2, '0')}-${String(d.getDate()).padStart(2, '0')} ${hm.slice(0, 5)}`
}

/** 进度条配色 */
function progressColor(status: string): string {
  if (status === 'success') return '#2DD4BF'
  if (status === 'failed') return '#FB7185'
  if (status === 'canceled') return '#F59E0B'
  return 'linear-gradient(90deg,#2DD4BF,#38BDF8)'
}

export default function TaskBar() {
  const navigate = useNavigate()
  const tasks = useTaskStore((s) => s.tasks)
  const sseStatus = useTaskStore((s) => s.sseStatus)
  const fetchTasks = useTaskStore((s) => s.fetchTasks)

  const [open, setOpen] = useState(() => localStorage.getItem(LS_OPEN) === 'true')
  const [height, setHeight] = useState(() => {
    const saved = Number(localStorage.getItem(LS_HEIGHT))
    if (localStorage.getItem(LS_OPEN) === 'true') {
      return saved > COLLAPSED_HEIGHT ? saved : DEFAULT_HEIGHT
    }
    return COLLAPSED_HEIGHT
  })
  const [dragging, setDragging] = useState(false)
  const heightRef = useRef(height)
  heightRef.current = height
  const openRef = useRef(open)
  openRef.current = open

  const [detailVisible, setDetailVisible] = useState(false)
  const [currentTask, setCurrentTask] = useState<TaskItem | null>(null)

  // 首次挂载拉取一次任务列表
  useEffect(() => {
    void fetchTasks()
  }, [fetchTasks])

  const activeCount = tasks.filter((t) => t.status === 'pending' || t.status === 'running').length
  const current = tasks.find((t) => t.status === 'running') || tasks.find((t) => t.status === 'pending')

  const persist = useCallback((h: number, isOpen: boolean) => {
    localStorage.setItem(LS_HEIGHT, String(h))
    localStorage.setItem(LS_OPEN, String(isOpen))
  }, [])

  const toggle = () => {
    if (!open) {
      // 展开：恢复上次高度（至少默认高度）
      const h = Math.min(Math.max(height, DEFAULT_HEIGHT), window.innerHeight * MAX_HEIGHT_RATIO)
      setOpen(true)
      setHeight(h)
      persist(h, true)
    } else {
      // 收起：记住展开时的高度，下次恢复
      setOpen(false)
      setHeight(COLLAPSED_HEIGHT)
      persist(height, false)
    }
  }

  // 拖拽调整高度
  const startResize = (e: React.MouseEvent) => {
    e.preventDefault()
    e.stopPropagation()
    const startY = e.clientY
    const startH = height
    setDragging(true)
    const onMove = (ev: MouseEvent) => {
      const maxH = window.innerHeight * MAX_HEIGHT_RATIO
      const h = Math.min(Math.max(startH + (startY - ev.clientY), COLLAPSED_HEIGHT), maxH)
      setHeight(h)
      setOpen(h > COLLAPSED_HEIGHT + 60)
    }
    const onUp = () => {
      setDragging(false)
      window.removeEventListener('mousemove', onMove)
      window.removeEventListener('mouseup', onUp)
      persist(heightRef.current, openRef.current)
    }
    window.addEventListener('mousemove', onMove)
    window.addEventListener('mouseup', onUp)
  }

  const openDetail = async (task: TaskItem, e: React.MouseEvent) => {
    e.stopPropagation()
    setCurrentTask(task)
    setDetailVisible(true)
    try {
      const res = await getTaskDetail(task.id)
      setCurrentTask(res.data || task)
    } catch {
      // 请求层已提示
    }
  }

  const handleCancel = (task: TaskItem, e: React.MouseEvent) => {
    e.stopPropagation()
    const isRunning = task.status === 'running'
    Modal.confirm({
      title: '取消任务',
      content: isRunning
        ? `任务 #${task.id} 正在执行中，取消后已创建的资源将被自动清理。确定要取消吗？`
        : `确定要取消任务 #${task.id} 吗？`,
      okText: '确定取消',
      cancelText: '再想想',
      onOk: async () => {
        try {
          await cancelTask(task.id)
          Toast.success(isRunning ? '取消信号已发送，任务将尽快停止' : '任务已取消')
          void fetchTasks()
        } catch {
          // 请求层已提示
        }
      },
    })
  }

  return (
    <>
      <div
        className={`qvm-taskbar ${open ? 'open' : ''} ${dragging ? 'dragging' : ''}`}
        style={{ height }}
      >
        <div className="qvm-tb-resize" onMouseDown={startResize} />
        <div className="qvm-tb-head" onClick={toggle}>
          <div className="qvm-tb-title">
            <IconCheckList />
            异步任务
            {activeCount > 0 && <span className="qvm-tb-badge">{activeCount} 运行中</span>}
          </div>
          <div className="qvm-tb-sep" />
          <div className="qvm-tb-current">
            {current ? (
              <>
                <span className="name">{buildTaskDesc(current)}</span>
                <span className="msg">{current.message || '等待执行…'}</span>
              </>
            ) : (
              <span className="idle">暂无进行中的任务</span>
            )}
          </div>
          {current && (
            <div className="qvm-tb-prog">
              <div className="qvm-tb-prog-track">
                <div className="qvm-tb-prog-fill" style={{ width: `${current.progress || 0}%` }} />
              </div>
              <span className="qvm-tb-prog-num">{current.progress || 0}%</span>
            </div>
          )}
          <div className="qvm-tb-sep" />
          <div className={`qvm-sse-st ${sseStatus === 'connected' ? 'ok' : ''}`}>
            <IconPulse size="small" />
            {sseStatus === 'connected' ? '实时推送已连接' : sseStatus === 'connecting' ? '连接中…' : '已断开'}
          </div>
          <div
            className="qvm-tb-full"
            onClick={(e) => {
              e.stopPropagation()
              navigate('/task')
            }}
          >
            <IconDesktop size="small" />
            完整任务中心
          </div>
          <div className="qvm-tb-toggle">
            <IconChevronUp />
          </div>
        </div>

        <div className="qvm-tb-body">
          {tasks.length === 0 ? (
            <div className="qvm-tb-empty">暂无任务记录</div>
          ) : (
            <table className="qvm-table">
              <thead>
                <tr>
                  <th>类型</th>
                  <th>任务描述</th>
                  <th>状态</th>
                  <th>进度</th>
                  <th>状态消息</th>
                  <th>创建人</th>
                  <th>时间</th>
                  <th>操作</th>
                </tr>
              </thead>
              <tbody>
                {tasks.map((task) => {
                  const tc = taskTypeColor(task.type)
                  return (
                    <tr key={task.id}>
                      <td>
                        <span
                          className="qvm-type-tag"
                          style={{ color: tc.color, background: tc.bg, border: `1px solid ${tc.border}` }}
                        >
                          {taskTypeText(task.type)}
                        </span>
                      </td>
                      <td>{buildTaskDesc(task)}</td>
                      <td>
                        <span className={`qvm-st-tag ${task.status}`}>
                          <i />
                          {taskStatusText(task.status)}
                        </span>
                      </td>
                      <td>
                        <div className="qvm-cell-prog">
                          <div className="qvm-cell-prog-track">
                            <div
                              className="qvm-cell-prog-fill"
                              style={{ width: `${task.progress || 0}%`, background: progressColor(task.status) }}
                            />
                          </div>
                          <span className="qvm-cell-prog-num">{task.progress || 0}%</span>
                        </div>
                      </td>
                      <td className="qvm-cell-msg">
                        <TaskMessage message={task.message} truncate />
                      </td>
                      <td className="qvm-mono">{task.created_by || '-'}</td>
                      <td className="qvm-mono">{formatTime(task.created_at)}</td>
                      <td>
                        <span className="qvm-act-btn" onClick={(e) => openDetail(task, e)}>
                          详情
                        </span>
                        {(task.status === 'pending' || task.status === 'running') && (
                          <span className="qvm-act-btn cancel" onClick={(e) => handleCancel(task, e)}>
                            取消
                          </span>
                        )}
                      </td>
                    </tr>
                  )
                })}
              </tbody>
            </table>
          )}
        </div>
      </div>

      {/* 任务详情抽屉（与任务中心页共用组件） */}
      <TaskDetailSheet
        task={currentTask}
        visible={detailVisible}
        onClose={() => setDetailVisible(false)}
      />
    </>
  )
}
