/**
 * 登录二次验证面板（登录页 stage=login_verify 时替换登录卡片渲染）
 * 管理员：仅 2FA 动态码 / 恢复码；普通用户：邮箱验证码，绑定 2FA 后可选动态码 / 恢复码。
 * 全程使用登录返回的 login 令牌调用接口（15 分钟有效），
 * 验证通过后后端返回完整登录态（stage=success + access token），交由父组件应用会话。
 */
import { useState, type CSSProperties } from 'react'
import { Banner, Button, Input, RadioGroup, Radio, Toast } from '@douyinfe/semi-ui'
import { IconKey } from '@douyinfe/semi-icons'
import {
  sendLoginEmailCode,
  verifyLoginStage,
  type LoginStageResponse,
} from '@/api/auth'
import type { SecurityState } from '@/types/api'

/** 登录页传入的 login_verify 阶段信息 */
export interface LoginVerifyStageInfo {
  token: string
  username: string
  role: string
  security: SecurityState
  allowedMethods: string[]
  password: string
}

interface LoginVerifyPanelProps {
  stage: LoginVerifyStageInfo
  /** 返回登录表单（放弃本次验证） */
  onExit: () => void
  /** 验证通过，携带完整登录态 */
  onComplete: (data: LoginStageResponse) => void
}

/** 验证方式文案 */
const METHOD_LABELS: Record<string, string> = {
  totp: '2FA 动态码',
  recovery: '恢复码',
  email: '邮箱验证码',
}

export default function LoginVerifyPanel({ stage, onExit, onComplete }: LoginVerifyPanelProps) {
  const methods = stage.allowedMethods.filter((m) => METHOD_LABELS[m])
  const [method, setMethod] = useState(methods.includes('totp') ? 'totp' : methods[0] || 'email')
  const [code, setCode] = useState('')
  const [challengeId, setChallengeId] = useState(0)
  const [maskedEmail, setMaskedEmail] = useState(stage.security.masked_email || '')
  const [sending, setSending] = useState(false)
  const [loading, setLoading] = useState(false)

  /** 切换验证方式时清空输入 */
  const handleMethodChange = (next: string) => {
    setMethod(next)
    setCode('')
  }

  /** 发送邮箱验证码（发送到已绑定邮箱） */
  const handleSendCode = async () => {
    setSending(true)
    try {
      const res = await sendLoginEmailCode(stage.token)
      setChallengeId(res.data.challenge_id || 0)
      if (res.data.masked_email) {
        setMaskedEmail(res.data.masked_email)
      }
      Toast.success({ content: '验证码已发送，请检查邮箱', duration: 3 })
    } catch {
      // 错误提示由请求层统一处理
    } finally {
      setSending(false)
    }
  }

  /** 提交验证 */
  const handleSubmit = async () => {
    if (!code.trim()) {
      Toast.warning({
        content: method === 'recovery' ? '请输入 16 位恢复码' : '请输入验证码',
        duration: 3,
      })
      return
    }
    if (method === 'email' && !challengeId) {
      Toast.warning({ content: '请先发送邮箱验证码', duration: 3 })
      return
    }
    setLoading(true)
    try {
      const res = await verifyLoginStage(
        { method, code: code.trim(), challenge_id: challengeId || undefined },
        stage.token,
      )
      onComplete(res.data)
    } catch {
      // 错误提示由请求层统一处理
    } finally {
      setLoading(false)
    }
  }

  return (
    <div
      className="qvm-login-card qvm-g-border qvm-fade-up"
      style={{ '--qvm-delay': '60ms' } as CSSProperties}
    >
      <div className="qvm-lc-head">
        <div className="qvm-lc-logo">Q</div>
        <div className="qvm-lc-title">登录验证</div>
        <div className="qvm-lc-sub">账户 {stage.username} 已开启登录保护，请完成验证</div>
      </div>

      {methods.length > 1 && (
        <RadioGroup
          type="button"
          value={method}
          onChange={(e) => handleMethodChange(e.target.value as string)}
          className="qvm-lv-methods"
        >
          {methods.map((m) => (
            <Radio key={m} value={m}>
              {METHOD_LABELS[m]}
            </Radio>
          ))}
        </RadioGroup>
      )}

      {method === 'email' && (
        <Banner
          type="info"
          closeIcon={null}
          className="qvm-login-stage-tip"
          description={`验证码将发送至 ${maskedEmail || '已绑定邮箱'}`}
        />
      )}
      {method === 'recovery' && (
        <Banner
          type="warning"
          closeIcon={null}
          className="qvm-login-stage-tip"
          description="每个恢复码只能使用一次，使用后即失效"
        />
      )}

      <div className="qvm-field-label">
        {method === 'recovery' ? '恢复码' : method === 'email' ? '邮箱验证码' : '2FA 动态验证码'}
      </div>
      <Input
        size="large"
        prefix={<IconKey />}
        value={code}
        onChange={setCode}
        maxLength={method === 'recovery' ? 16 : 6}
        placeholder={method === 'recovery' ? '请输入 16 位恢复码' : '请输入 6 位验证码'}
        onEnterPress={() => void handleSubmit()}
      />

      <div className="qvm-lv-actions">
        {method === 'email' && (
          <Button loading={sending} onClick={() => void handleSendCode()}>
            发送邮箱验证码
          </Button>
        )}
        <Button
          theme="solid"
          type="primary"
          loading={loading}
          className="qvm-lv-submit"
          onClick={() => void handleSubmit()}
        >
          完成验证
        </Button>
      </div>

      <div className="qvm-lv-foot">
        <Button theme="borderless" onClick={onExit}>
          返回登录
        </Button>
      </div>
    </div>
  )
}
