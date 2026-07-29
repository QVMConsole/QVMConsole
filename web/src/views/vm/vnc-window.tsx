/**
 * VNC 独立窗口页（/vm/:id/vnc-window）
 * - 在独立浏览器窗口中打开的纯 VNC 控制台
 * - 连接 / 断开 / Ctrl+Alt+Del / 常用组合键 / 粘贴密码 / 发送文本 / 全屏
 * - 打开后自动连接；窗口标题同步为「VNC - 虚拟机名」
 */
import { useCallback, useEffect, useRef, useState } from 'react'
import { useParams } from 'react-router'
import RFB from '@novnc/novnc'
import { Button, Dropdown, Modal, Tag, TextArea, Toast, Tooltip } from '@douyinfe/semi-ui'
import {
  IconDesktop,
  IconClose,
  IconChevronDown,
  IconPlayCircle,
  IconStop,
  IconSend,
  IconFullScreenStroked,
} from '@douyinfe/semi-icons'
import { getVmDetail } from '@/api/vm'
import { useUserStore } from '@/stores/user'
import { applyDocumentTitle } from '@/config/site'
import {
  buildVncWsUrl,
  refreshVncViewport,
  sendTextToVnc,
  sendVncShortcut,
  COMMON_VNC_SHORTCUTS,
  PRIMARY_VNC_SHORTCUT,
  type VncShortcut,
} from '@/utils/vnc'
import './vnc-window.css'

export default function VncWindowPage() {
  const params = useParams<{ id: string }>()
  const vmName = params.id || ''
  const token = useUserStore((s) => s.token)

  const [connected, setConnected] = useState(false)
  const [connecting, setConnecting] = useState(false)
  const [errorMsg, setErrorMsg] = useState('')
  const [virtualDisplayDisabled, setVirtualDisplayDisabled] = useState(false)
  const [guestPassword, setGuestPassword] = useState('')
  const [pasting, setPasting] = useState(false)
  const [sendTextVisible, setSendTextVisible] = useState(false)
  const [customText, setCustomText] = useState('')
  const [sendingText, setSendingText] = useState(false)

  const containerRef = useRef<HTMLDivElement>(null)
  const rfbRef = useRef<RFB | null>(null)
  const connectTimerRef = useRef<number | null>(null)

  /** 清除容器内的残留画布（断开/重连前调用，避免旧画面残留成“图片”） */
  const clearCanvas = useCallback(() => {
    const container = containerRef.current
    if (!container) return
    container.querySelectorAll('canvas').forEach((canvas) => {
      try {
        canvas.remove()
      } catch {
        // 忽略移除异常
      }
    })
  }, [])

  // ============ 连接管理 ============
  const disconnect = useCallback(() => {
    if (connectTimerRef.current) {
      window.clearTimeout(connectTimerRef.current)
      connectTimerRef.current = null
    }
    if (rfbRef.current) {
      rfbRef.current.disconnect()
      rfbRef.current = null
    }
    clearCanvas()
    setConnected(false)
    setConnecting(false)
  }, [clearCanvas])

  const connect = useCallback(() => {
    if (!containerRef.current || !vmName || !token) return
    // 已有连接时不重复创建（防止双连接同时推流导致闪烁）
    if (rfbRef.current) return
    if (virtualDisplayDisabled) {
      setErrorMsg('该虚拟机已禁用虚拟显示设备')
      return
    }
    clearCanvas()
    setConnecting(true)
    setErrorMsg('')

    try {
      const url = buildVncWsUrl(vmName, token)
      const rfb = new RFB(containerRef.current, url)
      rfbRef.current = rfb
      rfb.viewOnly = false
      rfb.scaleViewport = true
      rfb.resizeSession = false // QEMU 不支持客户端调整分辨率

      rfb.addEventListener('connect', () => {
        // 旧实例回调不覆盖新实例状态
        if (rfbRef.current !== rfb) return
        setConnected(true)
        setConnecting(false)
        setErrorMsg('')
        applyDocumentTitle(`VNC - ${vmName}`)
        window.setTimeout(() => refreshVncViewport(rfb), 200)
      })

      rfb.addEventListener('disconnect', (e: CustomEvent) => {
        if (rfbRef.current === rfb) {
          rfbRef.current = null
        }
        setConnected(false)
        setConnecting(false)
        clearCanvas()
        if (!e.detail?.clean) {
          setErrorMsg('连接已断开（异常）')
        }
      })

      rfb.addEventListener('credentialsrequired', () => {
        const password = window.prompt('请输入 VNC 密码')
        if (password !== null && rfbRef.current === rfb) {
          rfb.sendCredentials({ password })
        } else {
          disconnect()
        }
      })

      rfb.addEventListener('securityfailure', (e: CustomEvent) => {
        setErrorMsg(`认证失败: ${e.detail?.reason || '密码错误'}`)
        setConnecting(false)
      })
    } catch (err) {
      setErrorMsg(`连接失败: ${(err as Error).message}`)
      setConnecting(false)
    }
  }, [vmName, token, virtualDisplayDisabled, disconnect, clearCanvas])

  // 加载凭据 + 自动连接（定时器在清理时取消，避免 StrictMode 双执行产生双连接）
  useEffect(() => {
    if (!vmName) return
    applyDocumentTitle(`VNC - ${vmName}`)
    let cancelled = false
    ;(async () => {
      try {
        const res = await getVmDetail(vmName)
        if (cancelled) return
        setVirtualDisplayDisabled(res.data?.video_model === 'none')
        setGuestPassword(res.data?.credential?.password || '')
      } catch {
        // 凭据获取失败不阻断连接
      }
      if (!cancelled) {
        connectTimerRef.current = window.setTimeout(connect, 300)
      }
    })()
    return () => {
      cancelled = true
      if (connectTimerRef.current) {
        window.clearTimeout(connectTimerRef.current)
        connectTimerRef.current = null
      }
      disconnect()
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [vmName])

  // 窗口尺寸变化时重新缩放
  useEffect(() => {
    const onResize = () => refreshVncViewport(rfbRef.current)
    window.addEventListener('resize', onResize)
    return () => window.removeEventListener('resize', onResize)
  }, [])

  // ============ 快捷键 ============
  const sendShortcut = useCallback(
    (shortcut: VncShortcut) => {
      if (!rfbRef.current) return
      sendVncShortcut(rfbRef.current, shortcut)
      Toast.success(`已发送 ${shortcut.label}`)
    },
    [],
  )

  // ============ 粘贴密码 ============
  const canPaste = connected && !!guestPassword
  const pasteTip = !connected
    ? '请先连接 VNC'
    : !guestPassword
      ? '当前未保存虚拟机登录密码'
      : '将已保存的登录密码输入到虚拟机当前焦点位置'

  const handlePastePassword = async () => {
    if (!connected) {
      Toast.warning('请先连接 VNC')
      return
    }
    if (!guestPassword) {
      Toast.warning('当前未保存虚拟机登录密码')
      return
    }
    setPasting(true)
    try {
      await sendTextToVnc(rfbRef.current, guestPassword)
      Toast.success('已将登录密码输入到虚拟机当前焦点位置')
    } finally {
      setPasting(false)
    }
  }

  // ============ 发送文本 ============
  const handleSendText = async () => {
    if (!connected || !rfbRef.current) {
      Toast.warning('请先连接 VNC')
      return
    }
    if (!customText) {
      Toast.warning('请输入要发送的文本')
      return
    }
    setSendingText(true)
    try {
      await sendTextToVnc(rfbRef.current, customText)
      Toast.success('文本已发送到虚拟机当前焦点位置')
      setSendTextVisible(false)
      setCustomText('')
    } finally {
      setSendingText(false)
    }
  }

  // ============ 全屏 ============
  const toggleFullscreen = () => {
    if (!containerRef.current) return
    if (!document.fullscreenElement) {
      containerRef.current.requestFullscreen().catch(() => {
        Toast.warning('浏览器不支持全屏或被阻止')
      })
    } else {
      document.exitFullscreen()
    }
  }

  const handleClose = () => {
    disconnect()
    window.close()
  }

  return (
    <div className="qvm-vnc-window">
      {/* 顶部工具栏 */}
      <div className="qvm-vnc-window-toolbar">
        <div className="qvm-vnc-window-left">
          <IconDesktop className="qvm-vnc-window-logo" />
          <span className="qvm-vnc-window-name">{vmName}</span>
          <Tag size="small" color={connected ? 'green' : 'grey'}>
            {connected ? '已连接' : '未连接'}
          </Tag>
        </div>
        <div className="qvm-vnc-window-right">
          {!connected ? (
            <Button
              size="small"
              type="primary"
              theme="solid"
              icon={<IconPlayCircle />}
              loading={connecting}
              disabled={virtualDisplayDisabled}
              onClick={connect}
            >
              连接
            </Button>
          ) : (
            <Button size="small" type="danger" icon={<IconStop />} onClick={disconnect}>
              断开
            </Button>
          )}
          <Button size="small" disabled={!connected} onClick={() => sendShortcut(PRIMARY_VNC_SHORTCUT)}>
            {PRIMARY_VNC_SHORTCUT.label}
          </Button>
          <Dropdown
            trigger="click"
            position="bottomRight"
            menu={COMMON_VNC_SHORTCUTS.map((s) => ({
              node: 'item',
              name: s.label,
              onClick: () => sendShortcut(s),
            }))}
          >
            <Button size="small" disabled={!connected}>
              常用组合键 <IconChevronDown size="small" />
            </Button>
          </Dropdown>
          <Tooltip content={pasteTip} position="bottom">
            <span>
              <Button size="small" disabled={!canPaste} loading={pasting} onClick={() => void handlePastePassword()}>
                粘贴密码
              </Button>
            </span>
          </Tooltip>
          <Tooltip content={connected ? '手动输入一段文本并发送到虚拟机当前焦点位置' : '请先连接 VNC'} position="bottom">
            <span>
              <Button size="small" icon={<IconSend />} disabled={!connected} onClick={() => setSendTextVisible(true)}>
                发送文本
              </Button>
            </span>
          </Tooltip>
          <Button size="small" icon={<IconFullScreenStroked />} disabled={!connected} onClick={toggleFullscreen} />
          <Button size="small" theme="borderless" icon={<IconClose />} onClick={handleClose}>
            关闭窗口
          </Button>
        </div>
      </div>

      {/* VNC 画布 */}
      <div ref={containerRef} className={`qvm-vnc-window-screen ${connected ? 'connected' : ''}`}>
        {!connected && (
          <div className="qvm-vnc-window-ph">
            <IconDesktop size="extra-large" />
            <p>
              {virtualDisplayDisabled
                ? '该虚拟机已禁用虚拟显示设备，VNC 控制台不可用'
                : connecting
                  ? '正在连接中…'
                  : '点击工具栏「连接」按钮开始远程控制'}
            </p>
            {errorMsg && <p className="qvm-vnc-window-error">{errorMsg}</p>}
          </div>
        )}
      </div>

      {/* 发送文本对话框 */}
      <Modal
        title="发送文本到 VNC"
        visible={sendTextVisible}
        onCancel={() => setSendTextVisible(false)}
        onOk={() => void handleSendText()}
        okText="确认发送"
        cancelText="取消"
        confirmLoading={sendingText}
        width={520}
        closeOnEsc
      >
        <div className="qvm-form-item">
          <div className="qvm-form-label">文本内容</div>
          <TextArea
            value={customText}
            onChange={setCustomText}
            rows={5}
            placeholder="请输入要发送到虚拟机当前焦点位置的文本"
          />
        </div>
        <p className="qvm-form-hint">
          文本会按字符逐个输入到虚拟机当前焦点位置，适合粘贴密码、命令、激活码或多行文本。
        </p>
      </Modal>
    </div>
  )
}
