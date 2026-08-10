/**
 * VNC 管理 Tab
 * - VNC 状态展示（未开启 / 已开启 / 已对外暴露）
 * - 开启（可设密码，需重启生效）/ 关闭 / 修改密码（即时生效）/ 对外暴露（高风险确认）
 * - 打开独立 VNC 窗口（浏览器独立窗口控制台）
 */
import { useCallback, useEffect, useState } from 'react'
import { Banner, Button, Input, Modal, Tag, Toast } from '@douyinfe/semi-ui'
import {
  IconDesktop,
  IconKey,
  IconUpload,
  IconStop,
  IconPlayCircle,
  IconRefresh,
} from '@douyinfe/semi-icons'
import type { VmDetailInfo, VncStatus } from '@/api/vm'
import {
  changeVncPassword,
  disableVnc,
  enableVnc,
  exposeVnc,
  getVncStatus,
} from '@/api/vm'
import { confirmModal } from '@/utils/confirm'

interface VncTabProps {
  vm: VmDetailInfo | null
  live: boolean
  liveTick: number
  onOpenWindow: () => void
}

export default function VncTab({ vm, live, liveTick, onOpenWindow }: VncTabProps) {
  const vmName = vm?.name || ''
  const running = vm?.status === 'running'
  const paused = vm?.status === 'paused'
  const canConnect = running || paused

  const [status, setStatus] = useState<VncStatus | null>(null)
  const [loading, setLoading] = useState(false)

  // 对话框状态
  const [enableVisible, setEnableVisible] = useState(false)
  const [enablePassword, setEnablePassword] = useState('')
  const [enableLoading, setEnableLoading] = useState(false)
  const [pwdVisible, setPwdVisible] = useState(false)
  const [newPassword, setNewPassword] = useState('')
  const [pwdLoading, setPwdLoading] = useState(false)

  const refresh = useCallback(async (silent = false) => {
    if (!vmName) return
    if (!silent) setLoading(true)
    try {
      const res = await getVncStatus(vmName)
      setStatus(res.data || null)
    } catch {
      if (!silent) setStatus(null)
    } finally {
      if (!silent) setLoading(false)
    }
  }, [vmName])

  useEffect(() => {
    if (live) void refresh(liveTick > 0)
  }, [refresh, live, liveTick])

  // ============ 开启 ============
  const handleEnable = async () => {
    setEnableLoading(true)
    try {
      await enableVnc(vmName, enablePassword)
      Toast.success('VNC 已开启' + (running ? '，虚拟机正在重启' : ''))
      setEnableVisible(false)
      setEnablePassword('')
      window.setTimeout(() => void refresh(), 3000)
    } catch {
      // 请求层已提示
    } finally {
      setEnableLoading(false)
    }
  }

  // ============ 关闭 ============
  const handleDisable = async () => {
    const ok = await confirmModal({
      title: '关闭 VNC',
      content: '关闭 VNC 需要重启虚拟机才能生效，确定要关闭吗？',
      okText: '确认关闭',
    })
    if (!ok) return
    try {
      await disableVnc(vmName)
      Toast.success('VNC 已关闭')
      window.setTimeout(() => void refresh(), 3000)
    } catch {
      // 请求层已提示
    }
  }

  // ============ 对外暴露 ============
  const handleExpose = async (expose: boolean) => {
    const ok = expose
      ? await confirmModal({
          title: '开启 VNC 对外暴露（严重安全风险）',
          content:
            '开启后 VNC 端口将监听 0.0.0.0，任何人都可以通过网络直接连接虚拟机 VNC。\n\n强烈建议：务必设置 VNC 密码、使用防火墙限制访问来源、仅在必要时临时开启。\n\n此操作需要重启虚拟机才能生效，确定要开启吗？',
          okText: '我已了解风险，确认开启',
          danger: true,
        })
      : await confirmModal({
          title: '关闭 VNC 对外暴露',
          content: '关闭后 VNC 将仅监听 127.0.0.1，只能通过面板 WebSocket 代理访问。此操作需要重启虚拟机才能生效。',
          okText: '确认关闭',
        })
    if (!ok) return
    try {
      await exposeVnc(vmName, expose)
      Toast.success(expose ? 'VNC 已开启对外暴露，虚拟机正在重启' : 'VNC 已关闭对外暴露，虚拟机正在重启')
      window.setTimeout(() => void refresh(), 5000)
    } catch {
      // 请求层已提示
    }
  }

  // ============ 修改密码 ============
  const handleChangePassword = async () => {
    if (!newPassword) {
      Toast.warning('请输入新密码')
      return
    }
    setPwdLoading(true)
    try {
      await changeVncPassword(vmName, newPassword)
      Toast.success('VNC 密码已修改（即时生效）')
      setPwdVisible(false)
      setNewPassword('')
      void refresh()
    } catch {
      // 请求层已提示
    } finally {
      setPwdLoading(false)
    }
  }

  if (!vm) {
    return <div className="qvm-tab-loading">加载中…</div>
  }

  const enabled = !!status?.enabled

  return (
    <div className="qvm-vnc-tab">
      {/* 状态栏 */}
      <div className="qvm-tab-toolbar">
        <div className="qvm-tab-toolbar-left qvm-vnc-status-tags">
          <Tag size="large" color={enabled ? 'green' : 'grey'}>
            {enabled ? 'VNC 已开启' : 'VNC 未开启'}
          </Tag>
          {enabled && status?.auth && (
            <Tag size="small" color="orange">认证：{status.auth}</Tag>
          )}
          {enabled && status?.exposed && (
            <Tag size="small" color="red">已对外暴露</Tag>
          )}
          {enabled && status?.port && (
            <span className="qvm-sub-label">端口：<span className="qvm-mono">{status.port}</span></span>
          )}
        </div>
        <Button icon={<IconRefresh />} theme="borderless" loading={loading} onClick={() => void refresh()} />
      </div>

      {/* 操作区 */}
      {!enabled ? (
        <div className="qvm-vnc-off-panel">
          <IconDesktop size="extra-large" className="qvm-vnc-off-icon" />
          <div className="qvm-vnc-off-text">VNC 控制台尚未开启</div>
          <div className="qvm-sub-label">开启后可通过浏览器直接控制虚拟机画面</div>
          <Button
            type="primary"
            icon={<IconPlayCircle />}
            disabled={!running && vm.status !== 'shut off'}
            onClick={() => setEnableVisible(true)}
          >
            开启 VNC
          </Button>
        </div>
      ) : (
        <div className="qvm-vnc-on-panel">
          <div className="qvm-vnc-open-hero">
            <IconDesktop size="extra-large" className="qvm-vnc-open-icon" />
            <div className="qvm-vnc-open-title">VNC 控制台已就绪</div>
            <div className="qvm-sub-label">
              {canConnect
                ? '点击下面按钮在独立浏览器窗口中打开控制台'
                : '虚拟机未运行，启动后可连接画面'}
            </div>
            <Button
              type="primary"
              theme="solid"
              size="large"
              icon={<IconUpload />}
              disabled={!canConnect}
              onClick={onOpenWindow}
            >
              打开独立 VNC 窗口
            </Button>
          </div>

          <div className="qvm-vnc-manage-row">
            <Button icon={<IconKey />} onClick={() => setPwdVisible(true)}>
              {status?.has_password ? '修改密码' : '设置密码'}
            </Button>
            {!status?.exposed ? (
              <Button type="warning" theme="light" onClick={() => void handleExpose(true)}>
                开启对外暴露
              </Button>
            ) : (
              <Button type="warning" theme="light" onClick={() => void handleExpose(false)}>
                关闭对外暴露
              </Button>
            )}
            <Button type="danger" theme="light" icon={<IconStop />} onClick={() => void handleDisable()}>
              关闭 VNC
            </Button>
          </div>

          {status?.exposed && (
            <Banner
              type="warning"
              closeIcon={null}
              description="当前 VNC 已对外暴露，任何能访问宿主机网络的人都可能连接控制台。请确保已设置密码，并在用完后及时关闭对外暴露。"
            />
          )}
        </div>
      )}

      {/* 开启 VNC 对话框 */}
      <Modal
        title="开启 VNC"
        visible={enableVisible}
        onCancel={() => setEnableVisible(false)}
        onOk={() => void handleEnable()}
        okText="确认开启"
        cancelText="取消"
        confirmLoading={enableLoading}
        width={440}
        closeOnEsc
      >
        <div className="qvm-form-item">
          <div className="qvm-form-label">VNC 密码</div>
          <Input
            mode="password"
            value={enablePassword}
            onChange={setEnablePassword}
            placeholder="留空则无密码（最长 8 位）"
            maxLength={8}
          />
        </div>
        <Banner
          type="warning"
          closeIcon={null}
          description={`开启 VNC 需要重启虚拟机才能生效。${running ? '虚拟机将会自动重启。' : '请在开启后启动虚拟机。'}`}
        />
      </Modal>

      {/* 修改密码对话框 */}
      <Modal
        title="修改 VNC 密码"
        visible={pwdVisible}
        onCancel={() => setPwdVisible(false)}
        onOk={() => void handleChangePassword()}
        okText="确认修改"
        cancelText="取消"
        confirmLoading={pwdLoading}
        width={440}
        closeOnEsc
      >
        <div className="qvm-form-item">
          <div className="qvm-form-label">新密码</div>
          <Input
            mode="password"
            value={newPassword}
            onChange={setNewPassword}
            placeholder="请输入新密码（最长 8 位）"
            maxLength={8}
          />
        </div>
        <Banner type="info" closeIcon={null} description="密码修改即时生效，无需重启虚拟机。" />
      </Modal>
    </div>
  )
}
