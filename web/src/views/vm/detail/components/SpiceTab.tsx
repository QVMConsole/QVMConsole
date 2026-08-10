/**
 * SPICE 控制台 Tab
 * - 状态标签（未开启 / 仅本地 / 已对外暴露）
 * - 开启 / 关闭 / 修改密码 / 对外暴露（联动宿主防火墙，高风险确认）
 * - .vv 连接文件下载（一次性 / 可重复使用）
 */
import { useCallback, useEffect, useState } from 'react'
import { Banner, Button, Dropdown, Input, Modal, Spin, Switch, Tag, Toast } from '@douyinfe/semi-ui'
import { IconDownload, IconPlayCircle, IconStop, IconKey } from '@douyinfe/semi-icons'
import type { SpiceConnInfo, SpiceStatus, VmDetailInfo } from '@/api/vm'
import {
  changeSpicePassword,
  disableSpice,
  downloadSpiceVV,
  enableSpice,
  exposeSpice,
  getSpiceConnInfo,
  getSpiceStatus,
} from '@/api/vm'
import { confirmModal } from '@/utils/confirm'

interface SpiceTabProps {
  vm: VmDetailInfo | null
  live: boolean
  liveTick: number
}

export default function SpiceTab({ vm, live, liveTick }: SpiceTabProps) {
  const vmName = vm?.name || ''
  const connReady = vm?.status === 'running' || vm?.status === 'paused'

  const [status, setStatus] = useState<SpiceStatus | null>(null)
  const [connInfo, setConnInfo] = useState<SpiceConnInfo | null>(null)
  const [loading, setLoading] = useState(false)
  const [pwdVisible, setPwdVisible] = useState(false)
  const [newPassword, setNewPassword] = useState('')
  const [pwdLoading, setPwdLoading] = useState(false)

  const refreshStatus = useCallback(async (silent = false) => {
    if (!vmName) return
    try {
      const res = await getSpiceStatus(vmName)
      setStatus(res.data || null)
    } catch {
      if (!silent) setStatus(null)
    }
  }, [vmName])

  const refreshConnInfo = useCallback(async (silent = false) => {
    if (!vmName) return
    try {
      const res = await getSpiceConnInfo(vmName)
      setConnInfo(res.data || null)
    } catch {
      if (!silent) setConnInfo(null)
    }
  }, [vmName])

  useEffect(() => {
    if (!live) return
    void refreshStatus(liveTick > 0)
    void refreshConnInfo(liveTick > 0)
  }, [refreshStatus, refreshConnInfo, live, liveTick])

  // ============ 开启 / 关闭 ============
  const handleEnable = async () => {
    const ok = await confirmModal({
      title: '开启 SPICE',
      content: '开启 SPICE 将修改虚拟机配置。运行中的虚拟机会立即重启以使其生效，关机的虚拟机在下次启动时生效。确认继续？',
      okText: '确认开启',
    })
    if (!ok) return
    setLoading(true)
    try {
      await enableSpice(vmName)
      Toast.success('SPICE 已开启')
      await refreshStatus()
    } finally {
      setLoading(false)
    }
  }

  const handleDisable = async () => {
    const ok = await confirmModal({
      title: '关闭 SPICE',
      content: '关闭 SPICE 将修改虚拟机配置，运行中的虚拟机会立即重启以生效并断开所有外部 SPICE 客户端连接。确认？',
      okText: '确认关闭',
    })
    if (!ok) return
    setLoading(true)
    try {
      await disableSpice(vmName)
      Toast.success('SPICE 已关闭')
      await refreshStatus()
    } finally {
      setLoading(false)
    }
  }

  // ============ 对外暴露 ============
  const handleExpose = async (expose: boolean) => {
    if (expose) {
      const ok = await confirmModal({
        title: '开启 SPICE 对外暴露（高风险）',
        content: '开启后 SPICE 端口将通过公网地址对外提供连接（自动放行宿主防火墙端口）。请确保已设置访问密码，用完后及时关闭。确定要开启吗？',
        okText: '我已了解风险，确认开启',
        danger: true,
      })
      if (!ok) return
    }
    setLoading(true)
    try {
      await exposeSpice(vmName, expose)
      Toast.success(expose ? 'SPICE 已对外暴露' : 'SPICE 已关闭对外暴露')
      await refreshStatus()
      if (expose) await refreshConnInfo()
    } catch {
      // 失败时回滚状态显示
      await refreshStatus()
    } finally {
      setLoading(false)
    }
  }

  // ============ 修改密码 ============
  const handleChangePassword = async () => {
    if (!newPassword || /\s/.test(newPassword)) {
      Toast.warning('密码不能为空且不能包含空格')
      return
    }
    setPwdLoading(true)
    try {
      await changeSpicePassword(vmName, newPassword)
      Toast.success('SPICE 密码已修改')
      setPwdVisible(false)
      setNewPassword('')
      await refreshStatus()
    } catch {
      // 请求层已提示
    } finally {
      setPwdLoading(false)
    }
  }

  // ============ 下载 .vv ============
  const handleDownloadVV = async (deleteFile: boolean) => {
    if (!connReady) {
      Toast.warning('虚拟机未运行，SPICE 端口尚未分配，请先启动虚拟机后再下载连接文件')
      return
    }
    try {
      const res = await downloadSpiceVV(vmName, deleteFile)
      const blob = res.data
      const url = window.URL.createObjectURL(blob)
      const a = document.createElement('a')
      a.href = url
      a.download = `${vmName}.vv`
      document.body.appendChild(a)
      a.click()
      document.body.removeChild(a)
      window.URL.revokeObjectURL(url)
    } catch {
      Toast.error('下载 .vv 文件失败')
    }
  }

  if (!vm) {
    return (
      <div className="qvm-tab-loading">
        <Spin size="large" />
      </div>
    )
  }

  const enabled = !!status?.enabled

  return (
    <div className="qvm-spice-tab">
      {/* 状态栏 */}
      <div className="qvm-tab-toolbar">
        <div className="qvm-tab-toolbar-left qvm-vnc-status-tags">
          <span className="qvm-tab-title">SPICE 协议（外部客户端）</span>
          {enabled ? (
            <Tag size="small" color={status?.exposed ? 'red' : 'green'}>
              {status?.exposed ? '已对外暴露' : '仅本地'}
            </Tag>
          ) : (
            <Tag size="small" color="grey">未开启</Tag>
          )}
        </div>
      </div>

      {/* 操作区 */}
      <div className="qvm-spice-actions">
        {!enabled ? (
          <Button type="primary" icon={<IconPlayCircle />} loading={loading} onClick={() => void handleEnable()}>
            开启 SPICE
          </Button>
        ) : (
          <>
            <Button type="warning" theme="light" icon={<IconStop />} loading={loading} onClick={() => void handleDisable()}>
              关闭 SPICE
            </Button>
            <Button icon={<IconKey />} loading={pwdLoading} onClick={() => setPwdVisible(true)}>
              {status?.has_password ? '修改密码' : '设置密码'}
            </Button>
            <span className="qvm-spice-expose">
              <Switch
                checked={!!status?.exposed}
                loading={loading}
                onChange={(checked) => void handleExpose(checked)}
                size="small"
                checkedText="开"
                uncheckedText="关"
              />
              <span className="qvm-sub-label">对外暴露（自动放行防火墙端口）</span>
            </span>
          </>
        )}
      </div>

      {/* 连接信息 */}
      {enabled && (
        <div className="qvm-spice-info">
          <div className="qvm-info-row">
            <span className="qvm-info-label">SPICE 端口</span>
            <span className="qvm-info-value qvm-mono">{status?.port || '-'}</span>
          </div>
          {status?.exposed && connInfo?.host && (
            <div className="qvm-info-row">
              <span className="qvm-info-label">外部地址</span>
              <span className="qvm-info-value qvm-mono">
                {connInfo.host}:{connInfo.port}
              </span>
            </div>
          )}
          <Banner
            type="info"
            closeIcon={null}
            description="使用 virt-viewer / spicy 等客户端连接；下载 .vv 文件可直接双击由 virt-viewer 打开。"
          />
          <Dropdown
            trigger="click"
            position="bottomLeft"
            menu={[
              { node: 'item', name: '一次性（连接后自动删除）', onClick: () => void handleDownloadVV(true) },
              { node: 'item', name: '可重复使用（保留文件）', onClick: () => void handleDownloadVV(false) },
            ]}
          >
            <Button type="primary" theme="light" icon={<IconDownload />} disabled={!connReady}>
              下载 .vv 连接文件
            </Button>
          </Dropdown>
        </div>
      )}

      {/* 修改密码对话框 */}
      <Modal
        title="修改 SPICE 密码"
        visible={pwdVisible}
        onCancel={() => setPwdVisible(false)}
        onOk={() => void handleChangePassword()}
        okText="确认"
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
            placeholder="密码不能包含空格"
          />
        </div>
      </Modal>
    </div>
  )
}
