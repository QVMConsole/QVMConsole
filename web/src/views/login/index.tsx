/**
 * 登录页（地基版）
 * 已实现：账号密码登录（stage=success）
 * 待后续迭代：login_verify（登录二次验证）、bootstrap_security（安全初始化）、
 *             force_password_change（强制改密）、邀请注册、找回密码
 */
import { useEffect, useState } from 'react'
import { Button, Card, Form, Typography, Banner } from '@douyinfe/semi-ui'
import { useNavigate, useSearchParams } from 'react-router-dom'
import { login } from '@/api/auth'
import { useUserStore } from '@/stores/user'
import { useAppStore } from '@/stores/app'
import { LOGIN_STAGES, CLOUD_TYPES, type CloudType } from '@/config/constants'
import { applyDocumentTitle } from '@/config/site'

export default function LoginPage() {
  const navigate = useNavigate()
  const [searchParams] = useSearchParams()
  const setToken = useUserStore((s) => s.setToken)
  const setUserInfo = useUserStore((s) => s.setUserInfo)
  const siteTitle = useAppStore((s) => s.siteTitle)
  const [loading, setLoading] = useState(false)
  const [stageTip, setStageTip] = useState('')

  useEffect(() => {
    applyDocumentTitle('登录')
  }, [])

  const handleSubmit = async (values: { username: string; password: string }) => {
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
      style={{
        minHeight: '100vh',
        display: 'flex',
        alignItems: 'center',
        justifyContent: 'center',
        background: 'var(--semi-color-fill-0)',
      }}
    >
      <Card style={{ width: 400 }} shadows="always">
        <div style={{ textAlign: 'center', marginBottom: 24 }}>
          <Typography.Title heading={3} style={{ margin: 0 }}>
            {siteTitle}
          </Typography.Title>
          <Typography.Text type="tertiary">开源虚拟机管理控制台</Typography.Text>
        </div>
        {stageTip && (
          <Banner type="warning" description={stageTip} style={{ marginBottom: 16 }} closeIcon={null} />
        )}
        <Form<{ username: string; password: string }> onSubmit={handleSubmit} labelPosition="inset">
          <Form.Input
            field="username"
            label="用户名"
            placeholder="请输入用户名"
            rules={[{ required: true, message: '请输入用户名' }]}
          />
          <Form.Input
            field="password"
            label="密码"
            type="password"
            placeholder="请输入密码"
            rules={[{ required: true, message: '请输入密码' }]}
          />
          <Button htmlType="submit" type="primary" theme="solid" block loading={loading} style={{ marginTop: 8 }}>
            登录
          </Button>
        </Form>
      </Card>
    </div>
  )
}
