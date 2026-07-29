/**
 * 重置系统登录密码对话框（运行态在线处理，关机态离线处理）
 * - Linux / Windows / fnOS 差异化文案与用户名规则
 * - 密码强度校验 + HIBP 泄露检测 + 一键生成强密码
 */
import { useMemo, useState } from 'react'
import { Banner, Button, Input, Modal, Select, Toast } from '@douyinfe/semi-ui'
import type { VmDetailInfo } from '@/api/vm'
import { resetVmLinuxPassword } from '@/api/vm'
import {
  checkPasswordBreachAsync,
  generatePassword,
  validatePassword,
  STRONG_PASSWORD_MIN_LENGTH,
} from '@/utils/validate'
import { useMountModalLifecycle } from '@/hooks/useMountModalLifecycle'

interface ResetPasswordDialogProps {
  vm: VmDetailInfo
  onClose: () => void
}

const LINUX_USERNAME_PATTERN = /^[a-z_][a-z0-9_-]{0,31}$/
const WINDOWS_USERNAME_INVALID = /["/\\[\]:;|=,+*?<>]/

export default function ResetPasswordDialog({ vm, onClose }: ResetPasswordDialogProps) {
  const { modalVisible, requestClose, afterModalClose } = useMountModalLifecycle(onClose)
  const osType = vm.os_type || ''
  const isWindows = osType === 'windows'

  const [username, setUsername] = useState(
    vm.credential?.username || (isWindows ? 'administrator' : ''),
  )
  const [password, setPassword] = useState(generatePassword())
  const [mode, setMode] = useState<'auto' | 'online' | 'offline'>('auto')
  const [submitting, setSubmitting] = useState(false)

  const title = useMemo(() => {
    if (osType === 'fnos') return '重置 fnOS 登录密码'
    if (isWindows) return '重置 Windows 登录密码'
    return '重置虚拟机登录密码'
  }, [osType, isWindows])

  const alertText = useMemo(() => {
    if (vm.status === 'running') {
      return '虚拟机运行中，将通过 QEMU Guest Agent 在线更新密码，无需关机。提交后请在任务中心查看进度。'
    }
    if (isWindows) {
      return '虚拟机已关机，将注入 Windows 一次性重置脚本。任务完成后请手动开机一次，系统会自动处理并关机。'
    }
    return '该操作会在虚拟机关机状态下离线修改登录密码，不需要旧密码。提交后请在任务中心查看进度。'
  }, [isWindows, vm.status])

  const validate = (): boolean => {
    const trimmed = username.trim()
    if (!trimmed) {
      Toast.warning('请输入要重置的用户名')
      return false
    }
    if (isWindows) {
      if (trimmed.length > 64) {
        Toast.warning('Windows 用户名长度不能超过 64 个字符')
        return false
      }
      if (WINDOWS_USERNAME_INVALID.test(trimmed)) {
        Toast.warning('Windows 用户名包含不支持的字符')
        return false
      }
    } else if (!LINUX_USERNAME_PATTERN.test(trimmed)) {
      Toast.warning('用户名只能以小写字母或下划线开头，且只能包含小写字母、数字、下划线和短横线')
      return false
    }
    if (!password) {
      Toast.warning('请输入新密码')
      return false
    }
    if (password.length < STRONG_PASSWORD_MIN_LENGTH) {
      Toast.warning(`密码长度至少 ${STRONG_PASSWORD_MIN_LENGTH} 位`)
      return false
    }
    const localCheck = validatePassword(password)
    if (!localCheck.valid) {
      Toast.warning(localCheck.message)
      return false
    }
    return true
  }

  const handleSubmit = async () => {
    if (!validate()) return
    // HIBP 泄露密码检测
    const breach = await checkPasswordBreachAsync(password)
    if (breach.enabled && breach.breached) {
      Toast.error('该密码已在已知泄露数据库中发现，请更换为更安全的密码')
      return
    }
    setSubmitting(true)
    try {
      const res = await resetVmLinuxPassword(vm.name, { username: username.trim(), password, mode })
      const defaultMessage =
        isWindows && vm.status === 'shut off'
          ? 'Windows 重置任务已提交，任务完成后请手动开机一次等待系统自动处理并关机'
          : '重置密码任务已提交'
      Toast.success(res.message || defaultMessage)
      requestClose()
    } catch {
      // 请求层已提示
    } finally {
      setSubmitting(false)
    }
  }

  return (
    <Modal
      title={title}
      visible={modalVisible}
      afterClose={afterModalClose}
      onCancel={requestClose}
      onOk={() => void handleSubmit()}
      okText="提交任务"
      cancelText="取消"
      confirmLoading={submitting}
      width={520}
      closeOnEsc
    >
      <Banner type="warning" closeIcon={null} description={alertText} style={{ marginBottom: 16 }} />

      <div className="qvm-form-item">
        <div className="qvm-form-label">执行模式</div>
        <Select
          style={{ width: '100%' }}
          value={mode}
          onChange={(value) => setMode(value as 'auto' | 'online' | 'offline')}
          optionList={[
            { value: 'auto', label: '自动选择（推荐）' },
            { value: 'online', label: '在线重置', disabled: vm.status !== 'running' },
            { value: 'offline', label: '离线重置', disabled: vm.status !== 'shut off' },
          ]}
        />
      </div>

      <div className="qvm-form-item">
        <div className="qvm-form-label">用户名</div>
        <Input value={username} onChange={setUsername} placeholder="请输入要重置的用户名" />
        {isWindows && (
          <div className="qvm-form-hint">
            Windows 默认账号为 administrator；Windows Server 修改此项通常无效，建议保持默认。
          </div>
        )}
      </div>
      <div className="qvm-form-item">
        <div className="qvm-form-label">新密码</div>
        <Input
          mode="password"
          value={password}
          onChange={setPassword}
          placeholder={`请输入强密码（至少 ${STRONG_PASSWORD_MIN_LENGTH} 位）`}
        />
        <div className="qvm-form-hint">
          <Button size="small" theme="borderless" type="primary" onClick={() => setPassword(generatePassword())}>
            生成强密码
          </Button>
        </div>
      </div>
    </Modal>
  )
}
