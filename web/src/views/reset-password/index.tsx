/**
 * 重置密码页
 * 通过找回密码流程（或邮件链接）携带重置令牌进入（/reset-password?token=xxx），
 * 设置新密码后返回登录页。
 */
import { useEffect, useState, type CSSProperties } from 'react'
import { Button, Input, Toast } from '@douyinfe/semi-ui'
import { IconLock } from '@douyinfe/semi-icons'
import { useNavigate, useSearchParams } from 'react-router'
import { resetPasswordByEmail } from '@/api/auth'
import { useAppStore } from '@/stores/app'
import { useTheme } from '@/hooks/useTheme'
import { applyDocumentTitle } from '@/config/site'
import { validatePassword, checkPasswordBreachAsync, STRONG_PASSWORD_MIN_LENGTH } from '@/utils/validate'
import loginBgDark from '@/assets/img/login-bg.png'
import loginBgLight from '@/assets/img/login-bg-light.png'
import '../login/login.css'
import '../invite/invite.css'

export default function ResetPasswordPage() {
  const navigate = useNavigate()
  const [searchParams] = useSearchParams()
  const token = searchParams.get('token') || ''
  const siteTitle = useAppStore((s) => s.siteTitle)
  const { isDark } = useTheme()

  const [password, setPassword] = useState('')
  const [confirmPassword, setConfirmPassword] = useState('')
  const [submitting, setSubmitting] = useState(false)

  useEffect(() => {
    applyDocumentTitle('重置密码')
    if (!token) {
      Toast.error({ content: '重置令牌不存在，请重新发起找回密码', duration: 3 })
      navigate('/login', { replace: true })
    }
  }, [token, navigate])

  const handleSubmit = async () => {
    if (!password || !confirmPassword) {
      Toast.warning({ content: '请完整填写密码信息', duration: 3 })
      return
    }
    if (password.length < STRONG_PASSWORD_MIN_LENGTH) {
      Toast.error({ content: `密码长度至少 ${STRONG_PASSWORD_MIN_LENGTH} 位`, duration: 3 })
      return
    }
    if (password !== confirmPassword) {
      Toast.error({ content: '两次输入的密码不一致', duration: 3 })
      return
    }
    // 本地常见弱密码检测
    const check = validatePassword(password)
    if (!check.valid) {
      Toast.error({ content: check.message, duration: 3 })
      return
    }
    setSubmitting(true)
    try {
      // 异步泄露密码检测（HIBP k-匿名）
      const breach = await checkPasswordBreachAsync(password)
      if (breach.enabled && breach.breached) {
        Toast.error({ content: '该密码已在已知泄露数据库中发现，请更换为更安全的密码', duration: 3 })
        return
      }
      await resetPasswordByEmail({
        token,
        password,
        confirm_password: confirmPassword,
      })
      Toast.success({ content: '密码已重置，请重新登录', duration: 3 })
      navigate('/login', { replace: true })
    } catch {
      // 错误提示由请求层统一处理
    } finally {
      setSubmitting(false)
    }
  }

  return (
    <div
      className="qvm-login qvm-invite"
      style={
        { '--qvm-login-bg-img': `url(${isDark ? loginBgDark : loginBgLight})` } as CSSProperties
      }
    >
      {/* 渐变背景图 + 极光氛围层（与登录页一致） */}
      <div className="qvm-login-bg" />
      <div className="qvm-aurora" />
      <div className="qvm-grid-tex" />

      <div className="qvm-invite-wrap">
        <div className="qvm-login-card qvm-g-border qvm-fade-up">
          <div className="qvm-lc-head">
            <div className="qvm-lc-logo">Q</div>
            <div className="qvm-lc-title">重置密码</div>
            <div className="qvm-lc-sub">为 {siteTitle} 账号设置新的登录密码</div>
          </div>

          <div className="qvm-field-label">新密码</div>
          <Input
            mode="password"
            size="large"
            prefix={<IconLock />}
            placeholder={`请输入密码（至少 ${STRONG_PASSWORD_MIN_LENGTH} 位）`}
            value={password}
            onChange={setPassword}
            autoComplete="new-password"
          />
          <div className="qvm-field-label" style={{ marginTop: 14 }}>
            确认密码
          </div>
          <Input
            mode="password"
            size="large"
            prefix={<IconLock />}
            placeholder="请再次输入密码"
            value={confirmPassword}
            onChange={setConfirmPassword}
            autoComplete="new-password"
            onEnterPress={handleSubmit}
          />
          <div className="qvm-invite-pwd-tip">
            密码至少 {STRONG_PASSWORD_MIN_LENGTH} 位，提交时将自动进行泄露密码检测。
          </div>

          <Button
            block
            loading={submitting}
            className="qvm-btn-grad qvm-btn-login"
            onClick={handleSubmit}
          >
            重置密码
          </Button>
          <div className="qvm-invite-back">
            想起密码了？<a onClick={() => navigate('/login')}>返回登录</a>
          </div>
        </div>
      </div>
    </div>
  )
}
