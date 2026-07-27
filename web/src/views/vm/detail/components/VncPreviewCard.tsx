/**
 * Hero VNC 预览卡片（详情页右栏）
 * - 显示当前一帧 VNC 画面（临时 viewOnly 连接截帧，20s 自动刷新）
 * - 悬停显示「打开独立 VNC 页面」按钮，点击打开独立浏览器窗口
 * - 覆盖状态：禁用虚拟显示 / VNC 未开启 / 虚拟机未运行
 */
import { useCallback, useEffect, useRef, useState } from 'react'
import { Button, Spin, Tooltip } from '@douyinfe/semi-ui'
import { IconDesktop, IconRefresh, IconUpload } from '@douyinfe/semi-icons'
import type { VmDetailInfo } from '@/api/vm'
import { getVncStatus } from '@/api/vm'
import { captureVncFrame } from '@/utils/vnc'
import { useUserStore } from '@/stores/user'

interface VncPreviewCardProps {
  vm: VmDetailInfo | null
  onOpenWindow: () => void
  onGotoVncTab: () => void
}

const CAPTURE_INTERVAL = 20000

export default function VncPreviewCard({ vm, onOpenWindow, onGotoVncTab }: VncPreviewCardProps) {
  const token = useUserStore((s) => s.token)
  const [frame, setFrame] = useState<string>('')
  const [capturing, setCapturing] = useState(false)
  const [vncEnabled, setVncEnabled] = useState<boolean | null>(null)
  const timerRef = useRef<number | null>(null)

  const noDisplay = vm?.video_model === 'none'
  const running = vm?.status === 'running' || vm?.status === 'paused'
  const vmName = vm?.name || ''

  // 查询 VNC 开启状态
  useEffect(() => {
    if (!vmName || noDisplay) {
      setVncEnabled(null)
      return
    }
    let cancelled = false
    getVncStatus(vmName)
      .then((res) => {
        if (!cancelled) setVncEnabled(!!res.data?.enabled)
      })
      .catch(() => {
        if (!cancelled) setVncEnabled(null)
      })
    return () => {
      cancelled = true
    }
  }, [vmName, noDisplay])

  // 截帧（capturing 用 ref 防止闭包依赖导致定时器反复重建）
  const capturingRef = useRef(false)
  const capture = useCallback(async () => {
    if (!vmName || !token || capturingRef.current) return
    capturingRef.current = true
    setCapturing(true)
    try {
      const dataUrl = await captureVncFrame(vmName, token)
      setFrame(dataUrl)
    } catch {
      // 截帧失败保持原画面/占位
    } finally {
      capturingRef.current = false
      setCapturing(false)
    }
  }, [vmName, token])

  // 定时截帧（仅 VNC 开启且虚拟机运行时）
  useEffect(() => {
    if (timerRef.current) {
      window.clearInterval(timerRef.current)
      timerRef.current = null
    }
    if (vncEnabled && running && !noDisplay) {
      void capture()
      timerRef.current = window.setInterval(() => void capture(), CAPTURE_INTERVAL)
    } else {
      setFrame('')
    }
    return () => {
      if (timerRef.current) {
        window.clearInterval(timerRef.current)
        timerRef.current = null
      }
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [vncEnabled, running, noDisplay, vmName])

  // ============ 占位状态 ============
  const renderPlaceholder = (text: string, hint?: string, action?: React.ReactNode) => (
    <div className="qvm-vnc-ph">
      <IconDesktop size="extra-large" className="qvm-vnc-ph-icon" />
      <div className="qvm-vnc-ph-text">{text}</div>
      {hint && <div className="qvm-vnc-ph-hint">{hint}</div>}
      {action}
    </div>
  )

  let body: React.ReactNode
  if (!vm) {
    body = (
      <div className="qvm-vnc-ph">
        <Spin size="large" />
      </div>
    )
  } else if (noDisplay) {
    body = renderPlaceholder('虚拟显示设备已禁用', '该虚拟机未配置显示设备，VNC 控制台不可用')
  } else if (vncEnabled === false) {
    body = renderPlaceholder('VNC 未开启', '开启 VNC 后才能查看画面', (
      <Button size="small" type="primary" theme="light" onClick={onGotoVncTab}>
        前往开启
      </Button>
    ))
  } else if (!running) {
    body = renderPlaceholder('虚拟机未运行', '启动虚拟机后可查看实时画面')
  } else if (frame) {
    body = (
      <>
        <img
          src={frame}
          alt={`${vmName} VNC 画面`}
          className="qvm-vnc-frame"
          draggable={false}
          onClick={onOpenWindow}
        />
        <div className="qvm-vnc-overlay">
          <span className="qvm-vnc-live-tip">VNC 预览（每 20 秒自动截帧，点击打开控制台）</span>
          <Tooltip content="手动刷新画面" position="left">
            <span
              className={`qvm-vnc-refresh ${capturing ? 'spinning' : ''}`}
              onClick={(e) => {
                e.stopPropagation()
                void capture()
              }}
            >
              <IconRefresh spin={capturing} size="small" />
            </span>
          </Tooltip>
        </div>
        <button className="qvm-vnc-open-btn" onClick={onOpenWindow}>
          <IconUpload size="small" />
          打开独立 VNC 页面
        </button>
      </>
    )
  } else {
    body = (
      <div className="qvm-vnc-ph">
        <Spin size="large" />
        <div className="qvm-vnc-ph-hint">正在获取 VNC 画面…</div>
      </div>
    )
  }

  return (
    <div
      className={`qvm-hero-card qvm-vnc-card ${frame ? 'has-frame' : ''}`}
      onClick={frame ? onOpenWindow : undefined}
      title={frame ? '点击打开独立 VNC 控制台' : undefined}
    >
      {body}
    </div>
  )
}
