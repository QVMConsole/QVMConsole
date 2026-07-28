/**
 * 邮箱绑定区块
 * - 展示当前绑定邮箱与验证状态，支持换绑/新绑
 * - 流程：输入新邮箱 → 发送验证码（10 分钟有效）→ 输入验证码 → 保存
 */
import { useState } from 'react'
import { Banner, Button, Input, Tag, Toast } from '@douyinfe/semi-ui'
import { IconMailStroked } from '@douyinfe/semi-icons'
import { bindEmail, sendEmailCode } from '@/api/auth'
import { useUserStore } from '@/stores/user'

interface EmailSectionProps {
  /** 绑定成功后刷新全局安全状态 */
  refreshSecurity: () => Promise<void>
}

const EMAIL_PATTERN = /^[^\s@]+@[^\s@]+\.[^\s@]+$/

export default function EmailSection({ refreshSecurity }: EmailSectionProps) {
  const security = useUserStore((s) => s.security)

  const [email, setEmail] = useState('')
  const [code, setCode] = useState('')
  const [challengeId, setChallengeId] = useState(0)
  const [sending, setSending] = useState(false)
  const [binding, setBinding] = useState(false)

  // 发送绑定验证码
  const handleSendCode = async () => {
    const target = email.trim()
    if (!target) {
      Toast.warning('请输入要绑定的邮箱')
      return
    }
    if (!EMAIL_PATTERN.test(target)) {
      Toast.warning('邮箱格式不正确')
      return
    }
    setSending(true)
    try {
      const res = await sendEmailCode({ email: target })
      setChallengeId(res.data?.challenge_id || 0)
      Toast.success(res.message || '验证码已发送')
    } catch {
      // 请求层已统一提示
    } finally {
      setSending(false)
    }
  }

  // 提交绑定
  const handleBind = async () => {
    const target = email.trim()
    if (!challengeId) {
      Toast.warning('请先发送邮箱验证码')
      return
    }
    if (!code.trim()) {
      Toast.warning('请输入邮箱验证码')
      return
    }
    setBinding(true)
    try {
      await bindEmail({ email: target, code: code.trim(), challenge_id: challengeId })
      await refreshSecurity()
      Toast.success('邮箱已更新')
      setEmail('')
      setCode('')
      setChallengeId(0)
    } catch {
      // 请求层已统一提示
    } finally {
      setBinding(false)
    }
  }

  return (
    <div className="sec-tab-pane">
      {security?.must_bind_email && (
        <Banner
          type="warning"
          closeIcon={null}
          className="sec-banner"
          description="当前账户尚未完成邮箱绑定，部分安全能力不可用。"
        />
      )}
      {security && !security.smtp_configured && (
        <Banner
          type="info"
          closeIcon={null}
          className="sec-banner"
          description="SMTP 尚未配置，暂时无法发送验证邮件，请联系管理员在系统设置中完成配置。"
        />
      )}

      <div className="sec-row">
        <div className="sec-row-label">当前邮箱</div>
        <div className="sec-row-main">
          <Input value={security?.email || '未绑定'} disabled />
        </div>
      </div>

      <div className="sec-row">
        <div className="sec-row-label">验证状态</div>
        <div className="sec-row-main">
          <Tag color={security?.email_verified ? 'green' : 'orange'}>
            {security?.email_verified ? '已验证' : '未验证'}
          </Tag>
        </div>
      </div>

      <div className="sec-row">
        <div className="sec-row-label">新邮箱</div>
        <div className="sec-row-main">
          <Input
            value={email}
            onChange={setEmail}
            placeholder="请输入邮箱"
            prefix={<IconMailStroked />}
            showClear
          />
        </div>
      </div>

      <div className="sec-row">
        <div className="sec-row-label">验证码</div>
        <div className="sec-row-main">
          <Input
            value={code}
            onChange={setCode}
            maxLength={6}
            placeholder="请输入 6 位邮箱验证码"
          />
          <div className="sec-row-tip">邮箱验证码 10 分钟内有效，验证通过后会立即更新账户绑定邮箱。</div>
        </div>
      </div>

      <div className="sec-row">
        <div className="sec-row-label" />
        <div className="sec-row-main sec-actions">
          <Button loading={sending} onClick={() => void handleSendCode()}>
            发送验证码
          </Button>
          <Button type="primary" theme="solid" loading={binding} onClick={() => void handleBind()}>
            保存邮箱
          </Button>
        </div>
      </div>
    </div>
  )
}
