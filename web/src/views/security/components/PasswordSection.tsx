/**
 * 修改密码区块
 * - 新密码先经本地弱密码快速检测，再做 HIBP 泄露检测（后端 k-匿名），最后提交
 * - 修改成功后会话失效，需重新登录
 */
import { useState } from 'react'
import { useNavigate } from 'react-router'
import { Button, Input, Toast } from '@douyinfe/semi-ui'
import { changePassword } from '@/api/auth'
import { useUserStore } from '@/stores/user'
import { checkPasswordBreachAsync, validatePassword, STRONG_PASSWORD_MIN_LENGTH } from '@/utils/validate'

export default function PasswordSection() {
  const navigate = useNavigate()
  const logout = useUserStore((s) => s.logout)

  const [form, setForm] = useState({ oldPassword: '', newPassword: '', confirmPassword: '' })
  const [loading, setLoading] = useState(false)

  const handleSubmit = async () => {
    if (!form.oldPassword) {
      Toast.warning('请输入当前密码')
      return
    }
    if (!form.newPassword) {
      Toast.warning('请输入新密码')
      return
    }
    if (form.newPassword.length < STRONG_PASSWORD_MIN_LENGTH) {
      Toast.warning(`新密码长度至少 ${STRONG_PASSWORD_MIN_LENGTH} 位`)
      return
    }
    const localCheck = validatePassword(form.newPassword)
    if (!localCheck.valid) {
      Toast.warning(localCheck.message)
      return
    }
    if (form.newPassword !== form.confirmPassword) {
      Toast.warning('两次输入的密码不一致')
      return
    }

    setLoading(true)
    try {
      // 泄露密码检测（HIBP k-匿名 + 本地弱密码库，由后端统一判定）
      const breach = await checkPasswordBreachAsync(form.newPassword)
      if (breach.enabled && breach.breached) {
        Toast.error('该密码已在已知泄露数据库中发现，请更换为更安全的密码')
        return
      }

      const res = await changePassword({
        old_password: form.oldPassword,
        new_password: form.newPassword,
      })
      Toast.success(res.message || '密码修改成功，请重新登录')
      // 密码修改后所有会话失效，重新登录
      logout()
      navigate('/login', { replace: true })
    } catch {
      // 请求层已统一提示（428 高风险验证由请求层自动处理）
    } finally {
      setLoading(false)
    }
  }

  return (
    <div className="sec-tab-pane">
      <div className="sec-row">
        <div className="sec-row-label">当前密码</div>
        <div className="sec-row-main">
          <Input
            mode="password"
            value={form.oldPassword}
            onChange={(v) => setForm((p) => ({ ...p, oldPassword: v }))}
            placeholder="请输入当前密码"
          />
        </div>
      </div>
      <div className="sec-row">
        <div className="sec-row-label">新密码</div>
        <div className="sec-row-main">
          <Input
            mode="password"
            value={form.newPassword}
            onChange={(v) => setForm((p) => ({ ...p, newPassword: v }))}
            placeholder={`请输入新密码（至少 ${STRONG_PASSWORD_MIN_LENGTH} 位）`}
          />
          <div className="sec-row-tip">
            提交前会对新密码进行泄露检测（Have I Been Pwned，k-匿名模型，密码不会离开本机）。
          </div>
        </div>
      </div>
      <div className="sec-row">
        <div className="sec-row-label">确认密码</div>
        <div className="sec-row-main">
          <Input
            mode="password"
            value={form.confirmPassword}
            onChange={(v) => setForm((p) => ({ ...p, confirmPassword: v }))}
            placeholder="请再次输入新密码"
          />
        </div>
      </div>
      <div className="sec-row">
        <div className="sec-row-label" />
        <div className="sec-row-main sec-actions">
          <Button type="primary" theme="solid" loading={loading} onClick={() => void handleSubmit()}>
            确认修改
          </Button>
        </div>
      </div>
    </div>
  )
}
