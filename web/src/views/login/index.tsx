/**
 * 登录页（深空极光设计稿重构版）
 * 已实现：账号密码登录（stage=success）
 * 待后续迭代：login_verify（登录二次验证）、bootstrap_security（安全初始化）、
 *             force_password_change（强制改密）、邀请注册、找回密码
 */
import { useEffect, useState, type CSSProperties } from 'react'
import { Button, Checkbox, Form, Banner, Toast } from '@douyinfe/semi-ui'
import {
  IconUser,
  IconLock,
  IconEyeOpened,
  IconEyeClosedSolid,
  IconGithubLogo,
  IconBolt,
  IconGlobeStroke,
  IconUserGroup,
  IconActivity,
} from '@douyinfe/semi-icons'
import { useNavigate, useSearchParams } from 'react-router-dom'
import { login } from '@/api/auth'
import { useUserStore } from '@/stores/user'
import { useAppStore } from '@/stores/app'
import { useTheme } from '@/hooks/useTheme'
import { LOGIN_STAGES, CLOUD_TYPES, type CloudType } from '@/config/constants'
import { applyDocumentTitle } from '@/config/site'
import ForgotPasswordModal from './ForgotPasswordModal'
import loginBgDark from '@/assets/img/login-bg.png'
import loginBgLight from '@/assets/img/login-bg-light.png'
import './login.css'

/** 品牌区特性列表（布局参考设计稿） */
const FEATURES = [
  { icon: <IconBolt />, text: '模板克隆 秒级开机', color: '#2DD4BF', bg: 'rgba(45,212,191,.12)', bd: 'rgba(45,212,191,.24)' },
  { icon: <IconGlobeStroke />, text: 'VPC 网络 与防火墙', color: '#38BDF8', bg: 'rgba(56,189,248,.12)', bd: 'rgba(56,189,248,.24)' },
  { icon: <IconUserGroup />, text: '多租户 配额管理', color: '#8B5CF6', bg: 'rgba(139,92,246,.12)', bd: 'rgba(139,92,246,.24)' },
  { icon: <IconActivity />, text: '实时监控 SSE 推送', color: '#F59E0B', bg: 'rgba(251,191,36,.1)', bd: 'rgba(251,191,36,.22)' },
]

/** 浮动装饰虚拟机卡片（纯展示） */
const FLOAT_VMS = [
  { cls: 'qvm-fvm-1', name: 'web-server-01', meta: '4C / 8G · 运行 12 天', running: true },
  { cls: 'qvm-fvm-2', name: 'db-mysql-prod', meta: '8C / 16G · 运行 45 天', running: true },
  { cls: 'qvm-fvm-3', name: 'test-env-02', meta: '2C / 4G · 已停止', running: false },
]

export default function LoginPage() {
  const navigate = useNavigate()
  const [searchParams] = useSearchParams()
  const setToken = useUserStore((s) => s.setToken)
  const setUserInfo = useUserStore((s) => s.setUserInfo)
  const siteTitle = useAppStore((s) => s.siteTitle)
  const { isDark } = useTheme()
  const [loading, setLoading] = useState(false)
  const [stageTip, setStageTip] = useState('')
  const [agreed, setAgreed] = useState(false)
  const [pwdVisible, setPwdVisible] = useState(false)
  const [forgotVisible, setForgotVisible] = useState(false)

  useEffect(() => {
    applyDocumentTitle('登录')
  }, [])

  const handleSubmit = async (values: { username: string; password: string }) => {
    if (!agreed) {
      Toast.warning({ content: '请先阅读并同意用户协议与公测协议', duration: 3 })
      return
    }
    setLoading(true)
    setStageTip('')
    try {
      const res = await login({ username: values.username.trim(), password: values.password })
      const data = res.data
      if (data.stage === LOGIN_STAGES.success && data.token) {
        setToken(data.token)
        setUserInfo(
          data.username,
          data.role,
          data.security,
          (data.cloud_type || CLOUD_TYPES.elastic) as CloudType,
        )
        if (data.force_password_change) {
          // TODO(重构迭代): 强制修改密码流程
          setStageTip('当前账号需先修改默认密码，该流程将在后续迭代提供')
          return
        }
        const redirect = searchParams.get('redirect')
        navigate(redirect ? decodeURIComponent(redirect) : '/', { replace: true })
        return
      }
      if (data.stage === LOGIN_STAGES.loginVerify) {
        // TODO(重构迭代): 登录二次验证（TOTP / 邮箱验证码）
        setStageTip('该账号已开启登录验证，完整验证流程将在后续迭代提供')
        return
      }
      if (data.stage === LOGIN_STAGES.bootstrapSecurity) {
        // TODO(重构迭代): 安全初始化引导（绑定邮箱 / 2FA）
        setStageTip('该账号需先完成安全初始化，引导流程将在后续迭代提供')
        return
      }
      setStageTip('未知的登录状态，请稍后再试')
    } catch {
      // 错误提示由请求层统一处理
    } finally {
      setLoading(false)
    }
  }

  return (
    <div
      className="qvm-login"
      style={
        { '--qvm-login-bg-img': `url(${isDark ? loginBgDark : loginBgLight})` } as CSSProperties
      }
    >
      {/* 渐变背景图 + 极光氛围层 */}
      <div className="qvm-login-bg" />
      <div className="qvm-aurora" />
      <div className="qvm-grid-tex" />

      {/* ============ 左侧品牌区 ============ */}
      <section className="qvm-login-brand">
        <div className="qvm-brand-logo qvm-fade-up">
          <div className="qvm-logo-mark">Q</div>
          <div>
            <div className="qvm-brand-logo-name">{siteTitle}</div>
            <div className="qvm-brand-logo-sub">OPENSOURCE KVM PANEL</div>
          </div>
        </div>

        <div className="qvm-brand-mid">
          <div className="qvm-brand-slogan qvm-fade-up" style={{ '--qvm-delay': '60ms' } as CSSProperties}>
            自托管的
            <br />
            <em>KVM 虚拟化</em>管理平台
          </div>
          <div className="qvm-brand-desc qvm-fade-up" style={{ '--qvm-delay': '120ms' } as CSSProperties}>
            开源自托管的虚拟机管理控制台，一台物理机即可构建属于自己的私有云。从模板秒级开出虚拟机，网络、存储、配额一站式管理。
          </div>
          <div className="qvm-feature-list qvm-fade-up" style={{ '--qvm-delay': '180ms' } as CSSProperties}>
            {FEATURES.map((f) => (
              <div className="qvm-feature-item" key={f.text}>
                <div
                  className="qvm-feature-ic"
                  style={{ background: f.bg, border: `1px solid ${f.bd}`, color: f.color }}
                >
                  {f.icon}
                </div>
                {f.text}
              </div>
            ))}
          </div>
        </div>

        <div className="qvm-brand-foot qvm-fade-up" style={{ '--qvm-delay': '240ms' } as CSSProperties}>
          <span className="dot-ok" />
          服务运行正常 · 开源社区驱动
        </div>

        {/* 浮动 VM 装饰卡片 */}
        {FLOAT_VMS.map((vm) => (
          <div className={`qvm-float-vm ${vm.cls}`} key={vm.name}>
            <div className="fv-name">
              <span className={`qvm-dot ${vm.running ? 'run' : 'off'}`} />
              {vm.name}
            </div>
            <div className="fv-meta">{vm.meta}</div>
          </div>
        ))}
      </section>

      {/* ============ 右侧登录卡片 ============ */}
      <section className="qvm-login-side">
        <div className="qvm-login-card qvm-g-border qvm-fade-up" style={{ '--qvm-delay': '100ms' } as CSSProperties}>
          <div className="qvm-lc-head">
            <div className="qvm-lc-logo">Q</div>
            <div className="qvm-lc-title">欢迎回来</div>
            <div className="qvm-lc-sub">登录 {siteTitle} 开源虚拟机管理控制台</div>
          </div>

          {stageTip && (
            <Banner
              type="warning"
              description={stageTip}
              className="qvm-login-stage-tip"
              closeIcon={null}
            />
          )}

          <Form<{ username: string; password: string }>
            onSubmit={handleSubmit}
            className="qvm-login-form"
          >
            <div className="qvm-field-label">用户名</div>
            <Form.Input
              field="username"
              noLabel
              size="large"
              prefix={<IconUser />}
              placeholder="请输入用户名"
              autoComplete="username"
              rules={[{ required: true, message: '请输入用户名' }]}
            />
            <div className="qvm-field-label" style={{ marginTop: 16 }}>
              密码
            </div>
            <Form.Input
              field="password"
              noLabel
              size="large"
              type={pwdVisible ? 'text' : 'password'}
              prefix={<IconLock />}
              placeholder="请输入密码"
              autoComplete="current-password"
              suffix={
                <span
                  style={{ cursor: 'pointer', display: 'flex', alignItems: 'center' }}
                  onClick={() => setPwdVisible((v) => !v)}
                  aria-label="切换密码可见性"
                >
                  {pwdVisible ? <IconEyeOpened /> : <IconEyeClosedSolid />}
                </span>
              }
              rules={[{ required: true, message: '请输入密码' }]}
            />

            <div className="qvm-agreement">
              <Checkbox
                checked={agreed}
                onChange={(e) => setAgreed(e.target.checked ?? false)}
                aria-label="同意协议"
              />
              <span>
                我已阅读并同意{' '}
                <a
                  href="https://qvmcdocs.xiaozhuhouses.asia/agreement?return=%2Fdocs%2Finstall%2F"
                  target="_blank"
                  rel="noopener noreferrer"
                >
                  《用户协议》
                </a>{' '}
                和{' '}
                <a
                  href="https://qvmcdocs.xiaozhuhouses.asia/agreement?return=%2Fdocs%2Finstall%2F"
                  target="_blank"
                  rel="noopener noreferrer"
                >
                  《公测协议》
                </a>
              </span>
            </div>

            <Button
              htmlType="submit"
              block
              loading={loading}
              disabled={!agreed}
              className="qvm-btn-grad qvm-btn-login"
            >
              登 录
            </Button>
          </Form>

          <div className="qvm-login-helper">
            <span className="reg-tip">邀请注册请联系管理员获取链接</span>
            <a onClick={() => setForgotVisible(true)}>忘记密码</a>
          </div>
        </div>

        {/* 找回密码弹窗 */}
        <ForgotPasswordModal visible={forgotVisible} onClose={() => setForgotVisible(false)} />

        <div className="qvm-login-foot">
          <a href="https://github.com/QVMConsole/QVMConsole" target="_blank" rel="noopener noreferrer">
            <IconGithubLogo />© {siteTitle} · Open source Apache 2.0
          </a>
        </div>
      </section>
    </div>
  )
}
