/**
 * 安全初始化面板（登录页 stage=bootstrap_security 时替换登录卡片渲染）
 * 管理员：SMTP 配置（未配置时）→ 绑定邮箱 → 绑定 2FA，可确认风险后跳过；
 * 普通用户：仅需绑定邮箱。
 * 全程使用登录返回的 bootstrap 令牌调用接口（30 分钟有效），
 * 后端判定全部安全要求完成后返回完整登录态（stage=success + access token），交由父组件应用会话。
 */
import { useCallback, useEffect, useState, type CSSProperties } from 'react'
import QRCode from 'qrcode'
import { Banner, Button, Input, InputNumber, Select, Toast } from '@douyinfe/semi-ui'
import { IconMail, IconKey } from '@douyinfe/semi-icons'
import {
  bindEmail,
  enable2FA,
  sendEmailCode,
  setup2FA,
  skipBootstrap,
  type LoginStageResponse,
  type SecurityUpdateResult,
} from '@/api/auth'
import { getSettings, testSMTP, updateSettings } from '@/api/settings'
import { confirmModal } from '@/utils/confirm'
import { ROLES } from '@/config/constants'
import type { SecurityState } from '@/types/api'
import RecoveryCodesModal from '@/views/security/components/RecoveryCodesModal'

/** 登录页传入的 bootstrap 阶段信息 */
export interface BootstrapStageInfo {
  token: string
  username: string
  role: string
  security: SecurityState
}

interface BootstrapSecurityPanelProps {
  stage: BootstrapStageInfo
  /** 返回登录表单（放弃本次初始化） */
  onExit: () => void
  /** 初始化完成（或跳过），携带完整登录态 */
  onComplete: (data: LoginStageResponse) => void
}

/** SMTP 表单默认值（与旧前端及后端设置项对齐） */
const SMTP_FORM_DEFAULTS = {
  smtp_host: '',
  smtp_port: 587,
  smtp_username: '',
  smtp_password: '',
  smtp_from_name: 'QVMConsole',
  smtp_from_address: '',
  smtp_security: 'starttls',
  smtp_timeout_seconds: 15,
}

const SMTP_SECURITY_OPTIONS = [
  { value: 'starttls', label: 'STARTTLS' },
  { value: 'ssl', label: 'SSL/TLS' },
  { value: 'none', label: '无加密' },
]

export default function BootstrapSecurityPanel({
  stage,
  onExit,
  onComplete,
}: BootstrapSecurityPanelProps) {
  const isAdmin = stage.role === ROLES.admin
  // 安全状态本地副本：各步骤完成后由接口返回值刷新，驱动区块显隐
  const [security, setSecurity] = useState<SecurityState>(stage.security)

  // ==================== SMTP 配置（仅管理员且未配置时） ====================
  const showSMTP = isAdmin && !security.smtp_configured
  const [smtpForm, setSmtpForm] = useState({ ...SMTP_FORM_DEFAULTS })
  const [smtpTestEmail, setSmtpTestEmail] = useState('')
  const [smtpTested, setSmtpTested] = useState(false)
  const [testingSMTP, setTestingSMTP] = useState(false)
  const [savingSMTP, setSavingSMTP] = useState(false)

  // ==================== 绑定邮箱 ====================
  const [emailForm, setEmailForm] = useState({ email: stage.security.email || '', code: '' })
  const [challengeId, setChallengeId] = useState(0)
  const [sendingCode, setSendingCode] = useState(false)
  const [bindingEmail, setBindingEmail] = useState(false)

  // ==================== 绑定 2FA（仅管理员） ====================
  const [totpSecret, setTotpSecret] = useState('')
  const [qrCodeData, setQrCodeData] = useState('')
  const [totpCode, setTotpCode] = useState('')
  const [generating2FA, setGenerating2FA] = useState(false)
  const [binding2FA, setBinding2FA] = useState(false)

  // ==================== 恢复码 / 跳过 ====================
  const [recoveryCodes, setRecoveryCodes] = useState<string[]>([])
  // 暂存完整登录态：恢复码确认保存后再应用会话
  const [pendingSession, setPendingSession] = useState<LoginStageResponse | null>(null)
  const [skipping, setSkipping] = useState(false)

  /** 加载已有 SMTP 设置预填表单（密码后端不回传，留空表示不修改） */
  const loadSMTPSettings = useCallback(async () => {
    try {
      const res = await getSettings(stage.token)
      const data = res.data || {}
      setSmtpForm((prev) => ({
        ...prev,
        smtp_host: (data.smtp_host as string) || '',
        smtp_port: (data.smtp_port as number) || 587,
        smtp_username: (data.smtp_username as string) || '',
        smtp_password: '',
        smtp_from_name: (data.smtp_from_name as string) || 'QVMConsole',
        smtp_from_address: (data.smtp_from_address as string) || '',
        smtp_security: (data.smtp_security as string) || 'starttls',
        smtp_timeout_seconds: (data.smtp_timeout_seconds as number) || 15,
      }))
    } catch {
      // 错误提示由请求层统一处理
    }
  }, [stage.token])

  useEffect(() => {
    if (showSMTP) {
      void loadSMTPSettings()
    }
    // 仅初次显示 SMTP 区块时加载一次
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [])

  /** 处理安全设置接口的返回：完成全部要求时结束初始化，否则刷新本地安全状态 */
  const applySecurityResult = (data: SecurityUpdateResult): boolean => {
    if (data.stage === 'success' && data.token) {
      return true
    }
    if (data.security) {
      setSecurity(data.security)
    }
    return false
  }

  /** 发送测试邮件（使用未保存的表单配置直接测试） */
  const handleTestSMTP = async () => {
    if (!smtpTestEmail.trim()) {
      Toast.warning({ content: '请先输入测试收件邮箱', duration: 3 })
      return
    }
    setTestingSMTP(true)
    try {
      await testSMTP({ email: smtpTestEmail.trim(), ...smtpForm }, stage.token)
      setSmtpTested(true)
      Toast.success({ content: '测试邮件已发送，请检查收件箱，确认无误后点击保存', duration: 5 })
    } catch {
      // 错误提示由请求层统一处理
    } finally {
      setTestingSMTP(false)
    }
  }

  /** 保存 SMTP 配置（须先通过发信测试） */
  const handleSaveSMTP = async () => {
    setSavingSMTP(true)
    try {
      await updateSettings({ ...smtpForm }, stage.token)
      setSecurity((prev) => ({ ...prev, smtp_configured: true }))
      Toast.success({ content: 'SMTP 配置已保存', duration: 3 })
    } catch {
      // 错误提示由请求层统一处理
    } finally {
      setSavingSMTP(false)
    }
  }

  /** 发送邮箱绑定验证码 */
  const handleSendEmailCode = async () => {
    if (!emailForm.email.trim()) {
      Toast.warning({ content: '请输入要绑定的邮箱', duration: 3 })
      return
    }
    setSendingCode(true)
    try {
      const res = await sendEmailCode({ email: emailForm.email.trim() }, stage.token)
      setChallengeId(res.data.challenge_id)
      Toast.success({ content: '验证码已发送，请检查邮箱', duration: 3 })
    } catch {
      // 错误提示由请求层统一处理
    } finally {
      setSendingCode(false)
    }
  }

  /** 提交邮箱绑定 */
  const handleBindEmail = async () => {
    if (!challengeId) {
      Toast.warning({ content: '请先发送邮箱验证码', duration: 3 })
      return
    }
    if (!emailForm.code.trim()) {
      Toast.warning({ content: '请输入验证码', duration: 3 })
      return
    }
    setBindingEmail(true)
    try {
      const res = await bindEmail(
        { email: emailForm.email.trim(), code: emailForm.code.trim(), challenge_id: challengeId },
        stage.token,
      )
      if (applySecurityResult(res.data)) {
        Toast.success({ content: '邮箱绑定成功', duration: 3 })
        onComplete(res.data as LoginStageResponse)
        return
      }
      setEmailForm((prev) => ({ ...prev, code: '' }))
      setChallengeId(0)
      Toast.success({ content: '邮箱绑定成功', duration: 3 })
    } catch {
      // 错误提示由请求层统一处理
    } finally {
      setBindingEmail(false)
    }
  }

  /** 生成 2FA 配置并渲染二维码 */
  const handleGenerate2FA = async () => {
    setGenerating2FA(true)
    try {
      const res = await setup2FA(stage.token)
      setTotpSecret(res.data.secret)
      setQrCodeData(await QRCode.toDataURL(res.data.otpauth_url, { width: 180, margin: 1 }))
      setTotpCode('')
    } catch {
      // 错误提示由请求层统一处理
    } finally {
      setGenerating2FA(false)
    }
  }

  /** 提交验证码启用 2FA（成功后先展示一次性恢复码，确认保存后再进入系统） */
  const handleEnable2FA = async () => {
    if (!totpSecret) {
      Toast.warning({ content: '请先生成 2FA 配置', duration: 3 })
      return
    }
    if (!totpCode.trim()) {
      Toast.warning({ content: '请输入 6 位验证码', duration: 3 })
      return
    }
    setBinding2FA(true)
    try {
      const res = await enable2FA({ secret: totpSecret, code: totpCode.trim() }, stage.token)
      const codes = res.recovery?.recovery_codes || []
      const completed = applySecurityResult(res.data)
      Toast.success({ content: '2FA 绑定成功', duration: 3 })
      if (completed) {
        if (codes.length) {
          // 先展示恢复码，用户确认保存后再应用会话
          setPendingSession(res.data as LoginStageResponse)
          setRecoveryCodes(codes)
        } else {
          onComplete(res.data as LoginStageResponse)
        }
        return
      }
      setTotpSecret('')
      setQrCodeData('')
      setTotpCode('')
      if (codes.length) {
        setRecoveryCodes(codes)
      }
    } catch {
      // 错误提示由请求层统一处理
    } finally {
      setBinding2FA(false)
    }
  }

  /** 恢复码确认保存：有暂存会话则应用并进入系统 */
  const handleRecoveryConfirm = () => {
    setRecoveryCodes([])
    if (pendingSession) {
      const session = pendingSession
      setPendingSession(null)
      onComplete(session)
    }
  }

  /** 管理员跳过安全初始化（风险确认后调用） */
  const handleSkip = async () => {
    const ok = await confirmModal({
      title: '跳过安全初始化风险提示',
      content:
        '跳过安全设置后，SMTP 邮件服务、邮箱绑定和 2FA 双因素认证均不会配置。' +
        '相关功能（邀请注册、找回密码、邮箱验证等）将不可用，敏感操作将无法进行二次验证。' +
        '请在确保当前处于安全可信的网络环境中使用，并尽快完成安全配置。',
      okText: '我已知晓风险，跳过',
      cancelText: '返回继续配置',
      danger: true,
    })
    if (!ok) return
    setSkipping(true)
    try {
      const res = await skipBootstrap(stage.token)
      onComplete(res.data)
    } catch {
      // 错误提示由请求层统一处理
    } finally {
      setSkipping(false)
    }
  }

  return (
    <div
      className="qvm-login-card qvm-bs-card qvm-g-border qvm-fade-up"
      style={{ '--qvm-delay': '60ms' } as CSSProperties}
    >
      <div className="qvm-lc-head">
        <div className="qvm-lc-logo">Q</div>
        <div className="qvm-lc-title">安全初始化</div>
        <div className="qvm-lc-sub">
          账户 {stage.username} 需先完成必要的安全配置
        </div>
      </div>

      {/* ============ SMTP 配置（管理员且未配置） ============ */}
      {showSMTP && (
        <section className="qvm-bs-section">
          <div className="qvm-bs-sec-title">SMTP 邮件服务</div>
          <Banner
            type={smtpTested ? 'success' : 'warning'}
            closeIcon={null}
            className="qvm-bs-banner"
            description={
              smtpTested
                ? '测试邮件发送成功！请确认无误后点击保存完成 SMTP 配置。'
                : '请填写 SMTP 配置与测试收件邮箱，发送测试邮件验证通过后才能保存。'
            }
          />
          <div className="qvm-bs-grid2">
            <div>
              <div className="qvm-field-label">SMTP 主机</div>
              <Input
                value={smtpForm.smtp_host}
                onChange={(v) => setSmtpForm((p) => ({ ...p, smtp_host: v }))}
                placeholder="如 smtp.example.com"
              />
            </div>
            <div>
              <div className="qvm-field-label">SMTP 端口</div>
              <InputNumber
                value={smtpForm.smtp_port}
                onNumberChange={(v) => setSmtpForm((p) => ({ ...p, smtp_port: v }))}
                min={1}
                max={65535}
                style={{ width: '100%' }}
              />
            </div>
            <div>
              <div className="qvm-field-label">用户名</div>
              <Input
                value={smtpForm.smtp_username}
                onChange={(v) => setSmtpForm((p) => ({ ...p, smtp_username: v }))}
                placeholder="SMTP 登录用户名"
              />
            </div>
            <div>
              <div className="qvm-field-label">密码</div>
              <Input
                mode="password"
                value={smtpForm.smtp_password}
                onChange={(v) => setSmtpForm((p) => ({ ...p, smtp_password: v }))}
                placeholder="留空表示不修改已有密码"
              />
            </div>
            <div>
              <div className="qvm-field-label">发件人名称</div>
              <Input
                value={smtpForm.smtp_from_name}
                onChange={(v) => setSmtpForm((p) => ({ ...p, smtp_from_name: v }))}
              />
            </div>
            <div>
              <div className="qvm-field-label">发件邮箱</div>
              <Input
                value={smtpForm.smtp_from_address}
                onChange={(v) => setSmtpForm((p) => ({ ...p, smtp_from_address: v }))}
                placeholder="如 noreply@example.com"
              />
            </div>
            <div>
              <div className="qvm-field-label">加密方式</div>
              <Select
                value={smtpForm.smtp_security}
                onChange={(v) => setSmtpForm((p) => ({ ...p, smtp_security: v as string }))}
                optionList={SMTP_SECURITY_OPTIONS}
                style={{ width: '100%' }}
              />
            </div>
            <div>
              <div className="qvm-field-label">超时秒数</div>
              <InputNumber
                value={smtpForm.smtp_timeout_seconds}
                onNumberChange={(v) => setSmtpForm((p) => ({ ...p, smtp_timeout_seconds: v }))}
                min={5}
                max={120}
                style={{ width: '100%' }}
              />
            </div>
          </div>
          <div className="qvm-field-label" style={{ marginTop: 10 }}>
            测试收件邮箱
          </div>
          <Input
            prefix={<IconMail />}
            value={smtpTestEmail}
            onChange={setSmtpTestEmail}
            disabled={smtpTested}
            placeholder="请输入用于接收测试邮件的邮箱地址"
          />
          <div className="qvm-bs-actions">
            <Button
              loading={testingSMTP}
              disabled={smtpTested}
              onClick={() => void handleTestSMTP()}
            >
              发送测试邮件
            </Button>
            {smtpTested && (
              <Button theme="solid" type="primary" loading={savingSMTP} onClick={() => void handleSaveSMTP()}>
                保存 SMTP
              </Button>
            )}
          </div>
        </section>
      )}

      {/* ============ 绑定邮箱 ============ */}
      {security.must_bind_email && (
        <section className="qvm-bs-section">
          <div className="qvm-bs-sec-title">绑定邮箱</div>
          {!security.smtp_configured && (
            <Banner
              type="warning"
              closeIcon={null}
              className="qvm-bs-banner"
              description={
                isAdmin
                  ? '请先完成上方 SMTP 配置，再进行邮箱绑定。'
                  : '当前系统尚未配置 SMTP，暂时无法完成邮箱绑定，请联系管理员处理。'
              }
            />
          )}
          <div className="qvm-field-label">邮箱</div>
          <Input
            prefix={<IconMail />}
            value={emailForm.email}
            onChange={(v) => setEmailForm((p) => ({ ...p, email: v }))}
            placeholder="请输入要绑定的邮箱"
          />
          <div className="qvm-field-label" style={{ marginTop: 10 }}>
            邮箱验证码
          </div>
          <Input
            prefix={<IconKey />}
            value={emailForm.code}
            onChange={(v) => setEmailForm((p) => ({ ...p, code: v }))}
            maxLength={6}
            placeholder="请输入 6 位验证码"
            onEnterPress={() => void handleBindEmail()}
          />
          <div className="qvm-bs-actions">
            <Button
              loading={sendingCode}
              disabled={!security.smtp_configured}
              onClick={() => void handleSendEmailCode()}
            >
              发送验证码
            </Button>
            <Button
              theme="solid"
              type="primary"
              loading={bindingEmail}
              disabled={!security.smtp_configured}
              onClick={() => void handleBindEmail()}
            >
              绑定邮箱
            </Button>
          </div>
        </section>
      )}

      {/* ============ 绑定 2FA（管理员） ============ */}
      {security.must_bind_2fa && (
        <section className="qvm-bs-section">
          <div className="qvm-bs-sec-title">绑定 2FA 两步验证</div>
          <Button loading={generating2FA} onClick={() => void handleGenerate2FA()}>
            生成 2FA 配置
          </Button>
          {totpSecret && (
            <div className="qvm-bs-totp">
              {qrCodeData && <img src={qrCodeData} alt="2FA 二维码" className="qvm-bs-qr" />}
              <div className="qvm-bs-secret">密钥：{totpSecret}</div>
              <div className="qvm-bs-tip">
                请使用支持 TOTP 的验证器应用扫描二维码或手输密钥，输入 6 位动态验证码完成绑定。
              </div>
              <Input
                value={totpCode}
                onChange={setTotpCode}
                maxLength={6}
                placeholder="请输入 6 位验证码"
                onEnterPress={() => void handleEnable2FA()}
              />
              <div className="qvm-bs-actions">
                <Button theme="solid" type="primary" loading={binding2FA} onClick={() => void handleEnable2FA()}>
                  启用 2FA
                </Button>
              </div>
            </div>
          )}
        </section>
      )}

      {/* ============ 底部操作 ============ */}
      <div className="qvm-bs-foot">
        {isAdmin ? (
          <Button type="warning" theme="borderless" loading={skipping} onClick={() => void handleSkip()}>
            跳过安全设置
          </Button>
        ) : (
          <span />
        )}
        <Button onClick={onExit}>返回登录</Button>
      </div>

      {/* 恢复码展示弹窗（启用 2FA 后仅展示一次） */}
      <RecoveryCodesModal
        visible={recoveryCodes.length > 0}
        codes={recoveryCodes}
        onClose={handleRecoveryConfirm}
      />
    </div>
  )
}
