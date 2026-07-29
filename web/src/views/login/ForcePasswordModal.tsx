/**
 * 首次登录强制修改默认密码弹窗（登录页调用）
 * 登录返回 force_password_change 时弹出：此时登录 token 已写入，仅可访问改密/登出接口。
 * 新密码先经本地弱密码快速检测，再做 HIBP 泄露检测（后端 k-匿名），最后提交。
 * 改密成功后旧 token 失效（后端更新 security_updated_at），由父组件用新密码自动重新登录。
 */
import { useEffect, useState } from 'react'
import { Banner, Button, Input, Modal, Toast } from '@douyinfe/semi-ui'
import { IconLock } from '@douyinfe/semi-icons'
import { changePassword } from '@/api/auth'
import {
  checkPasswordBreachAsync,
  validatePassword,
  STRONG_PASSWORD_MIN_LENGTH,
} from '@/utils/validate'

interface ForcePasswordModalProps {
  visible: boolean
  /** 登录时输入的密码，预填为当前密码 */
  initialPassword: string
  /** 放弃修改：父组件清除会话并回到登录表单 */
  onExit: () => void
  /** 修改成功：父组件用新密码自动重新登录 */
  onSuccess: (newPassword: string) => void
  reason?: string
}

export default function ForcePasswordModal({
  visible,
  initialPassword,
  onExit,
  onSuccess,
  reason,
}: ForcePasswordModalProps) {
  const [form, setForm] = useState({ oldPassword: '', newPassword: '', confirmPassword: '' })
  const [loading, setLoading] = useState(false)

  // 每次打开时重置表单并预填当前密码
  useEffect(() => {
    if (visible) {
      setForm({ oldPassword: initialPassword, newPassword: '', confirmPassword: '' })
    }
  }, [visible, initialPassword])

  const handleSubmit = async () => {
    if (!form.oldPassword) {
      Toast.warning({ content: '请输入当前密码', duration: 3 })
      return
    }
    if (!form.newPassword) {
      Toast.warning({ content: '请输入新密码', duration: 3 })
      return
    }
    if (form.newPassword.length < STRONG_PASSWORD_MIN_LENGTH) {
      Toast.warning({ content: `新密码长度至少 ${STRONG_PASSWORD_MIN_LENGTH} 位`, duration: 3 })
      return
    }
    const localCheck = validatePassword(form.newPassword)
    if (!localCheck.valid) {
      Toast.warning({ content: localCheck.message, duration: 3 })
      return
    }
    if (form.newPassword === form.oldPassword) {
      Toast.warning({ content: '新密码不能与当前密码相同', duration: 3 })
      return
    }
    if (form.newPassword !== form.confirmPassword) {
      Toast.warning({ content: '两次输入的密码不一致', duration: 3 })
      return
    }

    setLoading(true)
    try {
      // 泄露密码检测（HIBP k-匿名 + 本地弱密码库，由后端统一判定）
      const breach = await checkPasswordBreachAsync(form.newPassword)
      if (breach.enabled && breach.breached) {
        Toast.error({ content: '该密码已在已知泄露数据库中发现，请更换为更安全的密码', duration: 5 })
        return
      }

      await changePassword({
        old_password: form.oldPassword,
        new_password: form.newPassword,
      })
      Toast.success({ content: '密码修改成功，正在为您重新登录', duration: 3 })
      onSuccess(form.newPassword)
    } catch {
      // 错误提示由请求层统一处理
    } finally {
      setLoading(false)
    }
  }

  return (
    <Modal
      title={reason === 'password_breach' ? '当前密码已泄露，请立即修改' : '首次登录请修改默认密码'}
      visible={visible}
      width={440}
      maskClosable={false}
      closeOnEsc={false}
      onCancel={onExit}
      footer={
        <>
          <Button onClick={onExit}>退出登录</Button>
          <Button theme="solid" type="primary" loading={loading} onClick={() => void handleSubmit()}>
            修改密码并登录
          </Button>
        </>
      }
    >
      <Banner
        type="warning"
        description={
          reason === 'password_breach'
            ? '安全检测确认当前管理员密码已出现在公开泄露数据库中。完成修改前，其他面板功能将保持锁定。'
            : '检测到您正在使用默认密码，为保障账户安全，请立即修改密码。'
        }
        closeIcon={null}
        style={{ marginBottom: 14, borderRadius: 10 }}
      />
      <div className="qvm-field-label">当前密码</div>
      <Input
        size="large"
        mode="password"
        prefix={<IconLock />}
        placeholder="请输入当前密码"
        autoComplete="current-password"
        value={form.oldPassword}
        onChange={(v) => setForm((p) => ({ ...p, oldPassword: v }))}
      />
      <div className="qvm-field-label" style={{ marginTop: 14 }}>
        新密码
      </div>
      <Input
        size="large"
        mode="password"
        prefix={<IconLock />}
        placeholder={`请输入新密码（至少 ${STRONG_PASSWORD_MIN_LENGTH} 位）`}
        autoComplete="new-password"
        value={form.newPassword}
        onChange={(v) => setForm((p) => ({ ...p, newPassword: v }))}
      />
      <div className="qvm-field-label" style={{ marginTop: 14 }}>
        确认新密码
      </div>
      <Input
        size="large"
        mode="password"
        prefix={<IconLock />}
        placeholder="请再次输入新密码"
        autoComplete="new-password"
        value={form.confirmPassword}
        onChange={(v) => setForm((p) => ({ ...p, confirmPassword: v }))}
        onEnterPress={() => void handleSubmit()}
      />
      <div style={{ marginTop: 10, fontSize: 12, color: 'var(--semi-color-text-2)' }}>
        提交前会对新密码进行泄露检测（Have I Been Pwned，k-匿名模型，密码不会离开本机）。
      </div>
    </Modal>
  )
}
