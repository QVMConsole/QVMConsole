/**
 * 网络诊断面板（仅管理员）
 * - 诊断信息（默认接口 / 默认 IP / 异常提示）
 * - 抓包表单（BPF 过滤条件构建 + 协议模板一键填充）
 * - 抓包会话管理（启动 / 取消 / 下载 pcap / 删除 pcap + 实时摘要轮询）
 */
import { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import {
  Banner,
  Button,
  Descriptions,
  Input,
  InputNumber,
  Select,
  Tag,
  Toast,
} from '@douyinfe/semi-ui'
import { IconRefresh } from '@douyinfe/semi-icons'
import type {
  NetworkCaptureRequest,
  NetworkDiagnosticTemplate,
  VmNetworkDiagnostics,
} from '@/api/vm'
import { getVMNetworkDiagnostics, startVMNetworkCapture } from '@/api/vm'
import {
  deleteNetworkCapture,
  getNetworkCaptureDownloadUrl,
  getNetworkCaptureSession,
  type NetworkCaptureSession,
} from '@/api/network'
import { cancelTask } from '@/api/task'
import { confirmModal } from '@/utils/confirm'
import type { NetworkSharedData } from './NetworkTab'

interface DiagnosticsPanelProps {
  vmName: string
  shared: NetworkSharedData
}

interface CaptureFormState {
  interface_name: string
  filter: {
    protocol: string
    source_ip: string
    dest_ip: string
    port: number
    source_port: number
    dest_port: number
  }
  duration_seconds: number
  max_mb: number
  max_packets: number
}

const INITIAL_CAPTURE_FORM: CaptureFormState = {
  interface_name: '',
  filter: { protocol: 'any', source_ip: '', dest_ip: '', port: 0, source_port: 0, dest_port: 0 },
  duration_seconds: 30,
  max_mb: 64,
  max_packets: 5000,
}

const PROTOCOL_OPTIONS = [
  { value: 'any', label: '全部' },
  { value: 'tcp', label: 'TCP' },
  { value: 'udp', label: 'UDP' },
  { value: 'icmp', label: 'ICMP' },
  { value: 'arp', label: 'ARP' },
  { value: 'dhcp', label: 'DHCP' },
  { value: 'dns', label: 'DNS' },
]

function captureStatusText(status?: string): string {
  const map: Record<string, string> = {
    pending: '等待中',
    running: '运行中',
    success: '已完成',
    failed: '失败',
    canceled: '已取消',
  }
  return map[status || ''] || status || '-'
}

function captureStatusColor(status?: string): 'grey' | 'orange' | 'green' | 'red' {
  const map: Record<string, 'grey' | 'orange' | 'green' | 'red'> = {
    pending: 'grey',
    running: 'orange',
    success: 'green',
    failed: 'red',
    canceled: 'grey',
  }
  return map[status || ''] || 'grey'
}

function formatFileSize(value?: number): string {
  const size = Number(value || 0)
  if (size <= 0) return '0 B'
  if (size < 1024) return `${size} B`
  if (size < 1024 * 1024) return `${(size / 1024).toFixed(1)} KB`
  return `${(size / 1024 / 1024).toFixed(2)} MB`
}

export default function DiagnosticsPanel({ vmName }: DiagnosticsPanelProps) {
  const [diagnostics, setDiagnostics] = useState<VmNetworkDiagnostics | null>(null)
  const [loading, setLoading] = useState(false)
  const [form, setForm] = useState<CaptureFormState>(INITIAL_CAPTURE_FORM)
  const [submitting, setSubmitting] = useState(false)
  const [taskId, setTaskId] = useState<number | null>(null)
  const [session, setSession] = useState<NetworkCaptureSession | null>(null)
  const pollTimerRef = useRef<number | null>(null)

  // ============ 诊断信息 ============
  const fetchDiagnostics = useCallback(async () => {
    setLoading(true)
    try {
      const res = await getVMNetworkDiagnostics(vmName)
      setDiagnostics(res.data || null)
      setForm((f) =>
        f.interface_name ? f : { ...f, interface_name: res.data?.default_interface || '' },
      )
    } catch {
      setDiagnostics(null)
    } finally {
      setLoading(false)
    }
  }, [vmName])

  useEffect(() => {
    void fetchDiagnostics()
  }, [fetchDiagnostics])

  // ============ 抓包会话轮询 ============
  const stopPolling = useCallback(() => {
    if (pollTimerRef.current) {
      window.clearInterval(pollTimerRef.current)
      pollTimerRef.current = null
    }
  }, [])

  const fetchSession = useCallback(async () => {
    if (!taskId) return
    try {
      const res = await getNetworkCaptureSession(taskId)
      setSession(res.data || null)
      if (['success', 'failed', 'canceled'].includes(res.data?.status || '')) {
        stopPolling()
      }
    } catch {
      // 静默失败
    }
  }, [taskId, stopPolling])

  const startPolling = useCallback(() => {
    stopPolling()
    if (!taskId) return
    void fetchSession()
    pollTimerRef.current = window.setInterval(() => void fetchSession(), 2000)
  }, [taskId, fetchSession, stopPolling])

  useEffect(() => {
    if (taskId) startPolling()
    return () => stopPolling()
  }, [taskId, startPolling, stopPolling])

  useEffect(() => {
    return () => stopPolling()
  }, [stopPolling])

  // ============ 模板 ============
  const applyTemplate = (tpl: NetworkDiagnosticTemplate) => {
    setForm((f) => ({
      ...f,
      filter: {
        protocol: tpl.filter?.protocol || 'any',
        source_ip: tpl.filter?.source_ip || '',
        dest_ip: tpl.filter?.dest_ip || '',
        port: tpl.filter?.port || 0,
        source_port: tpl.filter?.source_port || 0,
        dest_port: tpl.filter?.dest_port || 0,
      },
    }))
  }

  // ============ 抓包操作 ============
  const handleStartCapture = async () => {
    if (!form.interface_name) {
      Toast.warning('请选择抓包接口')
      return
    }
    const ok = await confirmModal({
      title: '高风险操作',
      content: '抓包会临时读取该 VM 的网络流量并生成 pcap 文件，确认继续？',
      okText: '开始抓包',
    })
    if (!ok) return
    setSubmitting(true)
    try {
      const payload: NetworkCaptureRequest = {
        interface_name: form.interface_name,
        filter: { ...form.filter },
        duration_seconds: form.duration_seconds,
        max_mb: form.max_mb,
        max_packets: form.max_packets,
      }
      const res = await startVMNetworkCapture(vmName, payload)
      setTaskId(res.data?.task_id || null)
      setSession(null)
      Toast.success(res.message || '抓包任务已提交')
    } catch {
      // 请求层已提示
    } finally {
      setSubmitting(false)
    }
  }

  const handleCancelCapture = async () => {
    if (!taskId) return
    const ok = await confirmModal({ title: '取消抓包', content: '确定取消当前抓包任务？' })
    if (!ok) return
    try {
      await cancelTask(taskId)
      Toast.success('已请求取消抓包')
      await fetchSession()
    } catch {
      // 请求层已提示
    }
  }

  const handleDownload = () => {
    if (!taskId) return
    window.open(getNetworkCaptureDownloadUrl(taskId), '_blank')
  }

  const handleDeleteFile = async () => {
    if (!taskId) return
    const ok = await confirmModal({
      title: '删除 pcap',
      content: '确定删除当前 pcap 文件？删除后不能再下载该文件。',
      okText: '删除',
      danger: true,
    })
    if (!ok) return
    try {
      await deleteNetworkCapture(taskId)
      Toast.success('pcap 文件已删除')
      await fetchSession()
    } catch {
      // 请求层已提示
    }
  }

  const captureInterfaces = useMemo(() => {
    return (diagnostics?.interfaces || []).filter(
      (i) => i.target && i.target !== '-' && i.ofport && i.ofport !== '-1',
    )
  }, [diagnostics])

  const issues = diagnostics?.issues || []

  return (
    <div className="qvm-diag-panel">
      <div className="qvm-tab-toolbar">
        <div className="qvm-tab-toolbar-left">
          <Button size="small" icon={<IconRefresh />} loading={loading} onClick={() => void fetchDiagnostics()}>
            刷新诊断
          </Button>
          <Tag size="small" color={issues.length ? 'orange' : 'green'}>
            {issues.length ? '需要关注' : '可抓包'}
          </Tag>
        </div>
      </div>

      {issues.length > 0 && (
        <Banner type="warning" closeIcon={null} description={issues.join('；')} style={{ marginBottom: 12 }} />
      )}

      <Descriptions align="left" className="qvm-runtime-summary">
        <Descriptions.Item itemKey="默认接口">{diagnostics?.default_interface || '-'}</Descriptions.Item>
        <Descriptions.Item itemKey="默认 IP">{diagnostics?.default_ip || '-'}</Descriptions.Item>
        <Descriptions.Item itemKey="状态">{diagnostics?.state || '-'}</Descriptions.Item>
        <Descriptions.Item itemKey="端口转发">
          {diagnostics?.port_forwards?.length || 0} 条
        </Descriptions.Item>
      </Descriptions>

      {/* 抓包表单 */}
      <div className="qvm-diag-form">
        <div className="qvm-diag-form-row">
          <div className="qvm-form-item">
            <div className="qvm-form-label">抓包接口</div>
            <Select
              style={{ width: '100%' }}
              placeholder="选择运行态接口"
              value={form.interface_name}
              onChange={(v) => setForm((f) => ({ ...f, interface_name: String(v || '') }))}
              optionList={captureInterfaces.map((i) => ({
                value: i.target,
                label: `${i.target} / ${i.ip || '无 IP'} / ofport ${i.ofport || '-'}`,
              }))}
            />
          </div>
          <div className="qvm-form-item">
            <div className="qvm-form-label">协议模板</div>
            <Select
              style={{ width: '100%' }}
              value={form.filter.protocol}
              onChange={(v) =>
                setForm((f) => ({ ...f, filter: { ...f.filter, protocol: String(v) } }))
              }
              optionList={PROTOCOL_OPTIONS}
            />
          </div>
        </div>
        <div className="qvm-diag-form-row">
          <div className="qvm-form-item">
            <div className="qvm-form-label">源 IP</div>
            <Input
              value={form.filter.source_ip}
              onChange={(v) => setForm((f) => ({ ...f, filter: { ...f.filter, source_ip: v } }))}
              placeholder="可留空"
            />
          </div>
          <div className="qvm-form-item">
            <div className="qvm-form-label">目标 IP</div>
            <Input
              value={form.filter.dest_ip}
              onChange={(v) => setForm((f) => ({ ...f, filter: { ...f.filter, dest_ip: v } }))}
              placeholder="可留空"
            />
          </div>
        </div>
        <div className="qvm-diag-form-row three">
          <div className="qvm-form-item">
            <div className="qvm-form-label">任意端口</div>
            <InputNumber
              style={{ width: '100%' }}
              min={0}
              max={65535}
              value={form.filter.port}
              onChange={(v) => setForm((f) => ({ ...f, filter: { ...f.filter, port: Number(v) || 0 } }))}
            />
          </div>
          <div className="qvm-form-item">
            <div className="qvm-form-label">源端口</div>
            <InputNumber
              style={{ width: '100%' }}
              min={0}
              max={65535}
              value={form.filter.source_port}
              onChange={(v) =>
                setForm((f) => ({ ...f, filter: { ...f.filter, source_port: Number(v) || 0 } }))
              }
            />
          </div>
          <div className="qvm-form-item">
            <div className="qvm-form-label">目标端口</div>
            <InputNumber
              style={{ width: '100%' }}
              min={0}
              max={65535}
              value={form.filter.dest_port}
              onChange={(v) =>
                setForm((f) => ({ ...f, filter: { ...f.filter, dest_port: Number(v) || 0 } }))
              }
            />
          </div>
        </div>
        <div className="qvm-diag-form-row three">
          <div className="qvm-form-item">
            <div className="qvm-form-label">时长（秒）</div>
            <InputNumber
              style={{ width: '100%' }}
              min={1}
              max={120}
              value={form.duration_seconds}
              onChange={(v) => setForm((f) => ({ ...f, duration_seconds: Number(v) || 30 }))}
            />
          </div>
          <div className="qvm-form-item">
            <div className="qvm-form-label">大小（MB）</div>
            <InputNumber
              style={{ width: '100%' }}
              min={1}
              max={256}
              value={form.max_mb}
              onChange={(v) => setForm((f) => ({ ...f, max_mb: Number(v) || 64 }))}
            />
          </div>
          <div className="qvm-form-item">
            <div className="qvm-form-label">包数</div>
            <InputNumber
              style={{ width: '100%' }}
              min={1}
              max={100000}
              value={form.max_packets}
              onChange={(v) => setForm((f) => ({ ...f, max_packets: Number(v) || 5000 }))}
            />
          </div>
        </div>
      </div>

      {/* 模板快捷按钮 */}
      {(diagnostics?.templates || []).length > 0 && (
        <div className="qvm-diag-templates">
          {(diagnostics?.templates || []).map((tpl) => (
            <Button key={tpl.key} size="small" theme="light" onClick={() => applyTemplate(tpl)}>
              {tpl.name}
            </Button>
          ))}
        </div>
      )}

      {/* 操作按钮 */}
      <div className="qvm-diag-actions">
        <Button
          type="primary"
          loading={submitting}
          disabled={!form.interface_name}
          onClick={() => void handleStartCapture()}
        >
          开始抓包
        </Button>
        {taskId && session?.status === 'running' && (
          <Button type="warning" theme="light" onClick={() => void handleCancelCapture()}>
            取消抓包
          </Button>
        )}
        {session?.download_path && session?.file_size > 0 && (
          <>
            <Button type="primary" theme="light" onClick={handleDownload}>
              下载 pcap
            </Button>
            <Button type="danger" theme="light" onClick={() => void handleDeleteFile()}>
              删除 pcap
            </Button>
          </>
        )}
      </div>

      {/* 会话状态 */}
      {session && (
        <Descriptions align="left" className="qvm-runtime-summary">
          <Descriptions.Item itemKey="任务 ID">{session.task_id}</Descriptions.Item>
          <Descriptions.Item itemKey="状态">
            <Tag size="small" color={captureStatusColor(session.status)}>
              {captureStatusText(session.status)}
            </Tag>
          </Descriptions.Item>
          <Descriptions.Item itemKey="文件大小">{formatFileSize(session.file_size)}</Descriptions.Item>
          <Descriptions.Item itemKey="接口">{session.interface_name || '-'}</Descriptions.Item>
          <Descriptions.Item itemKey="BPF">{session.bpf || '全部流量'}</Descriptions.Item>
          <Descriptions.Item itemKey="消息">{session.message || '-'}</Descriptions.Item>
        </Descriptions>
      )}

      {/* 实时摘要 */}
      {session?.summary_lines && session.summary_lines.length > 0 && (
        <div className="qvm-diag-output">
          <div className="qvm-sub-title">实时摘要</div>
          <pre className="qvm-diag-pre">{session.summary_lines.join('\n')}</pre>
        </div>
      )}

      {/* 邻居表 */}
      {diagnostics?.neighbors && diagnostics.neighbors.length > 0 && (
        <div className="qvm-diag-output">
          <div className="qvm-sub-title">邻居表</div>
          <pre className="qvm-diag-pre">{diagnostics.neighbors.join('\n')}</pre>
        </div>
      )}
    </div>
  )
}
