/**
 * 找回密码弹窗（登录页调用）
 * 三步流程：输入邮箱发验证码 → 校验验证码 → 选择账号，
 * 成功后携带重置令牌跳转 /reset-password 设置新密码。
 */
import { useState } from 'react'
import { Banner, Button, Input, Modal, Select, Toast } from '@douyinfe/semi-ui'
import { IconMail, IconKey } from '@douyinfe/semi-icons'
import { useNavigate } from 'react-router'
import {
  sendForgotPasswordCode,
  verifyForgotPasswordCode,
  selectForgotPasswordAccount,
  type ForgotPasswordAccount,
} from '@/api/auth'

type ForgotStep = 'email' | 'verify' | 'select'

interface ForgotPasswordModalProps {
  visible: boolean
  onClose: () => void
}

export default function ForgotPasswordModal({ visible, onClose }: ForgotPasswordModalProps) {
  const navigate = useNavigate()
  const [step, setStep] = useState<ForgotStep>('email')
  const [loading, setLoading] = useState(false)
  const [email, setEmail] = useState('')
  const [code, setCode] = useState('')
  const [challengeId, setChallengeId] = useState(0)
  const [maskedEmail, setMaskedEmail] = useState('')
  const [selectionToken, setSelectionToken] = useState('')
  const [accounts, setAccounts] = useState<ForgotPasswordAccount[]>([])
  const [username, setUsername] = useState('')

  /** 重置全部状态（关闭或流程重来时调用） */
  const resetState = () => {
    setStep('email')
    setEmail('')
    setCode('')
    setChallengeId(0)
    setMaskedEmail('')
    setSelectionToken('')
    setAccounts([])
    setUsername('')
  }

  const handleClose = () => {
    resetState()
    onClose()
  }

  /** 次按钮：email 步为取消，verify 回到 email，select 回到 verify */
  const handleCancel = () => {
    if (step === 'email') {
      handleClose()
      return
    }
    if (step === 'select') {
      setStep('verify')
      setUsername('')
      return
    }
    setStep('email')
    setCode('')
    setChallengeId(0)
    setMaskedEmail('')
  }

  const handleSubmit = async () => {
    if (step === 'email') {
      if (!email.trim()) {
        Toast.warning({ content: '请输入邮箱', duration: 3 })
        return
      }
      setLoading(true)
      try {
        const res = await sendForgotPasswordCode(email.trim())
        setChallengeId(res.data.challenge_id)
        setMaskedEmail(res.data.masked_email || '')
        setStep('verify')
        Toast.success({ content: '验证码已发送，请检查邮箱', duration: 3 })
      } catch {
        // 错误提示由请求层统一处理
      } finally {
        setLoading(false)
      }
      return
    }

    if (step === 'verify') {
      if (!challengeId) {
        Toast.warning({ content: '请先发送验证码', duration: 3 })
        return
      }
      if (!code.trim()) {
        Toast.warning({ content: '请输入验证码', duration: 3 })
        return
      }
      setLoading(true)
      try {
        const res = await verifyForgotPasswordCode({
          email: email.trim(),
          code: code.trim(),
          challenge_id: challengeId,
        })
        const list = res.data.accounts || []
        if (!list.length) {
          Toast.warning({ content: '该邮箱下暂无可重置的已激活账号', duration: 3 })
          resetState()
          return
        }
        setAccounts(list)
        setSelectionToken(res.data.selection_token || '')
        setUsername(list.length === 1 ? list[0].username : '')
        setStep('select')
        Toast.success({ content: '邮箱验证成功，请选择账号', duration: 3 })
      } catch {
        // 错误提示由请求层统一处理
      } finally {
        setLoading(false)
      }
      return
    }

    // select 步：确认账号并跳转重置页
    if (!selectionToken) {
      Toast.warning({ content: '账号选择状态已失效，请重新验证邮箱', duration: 3 })
      resetState()
      return
    }
    if (!username) {
      Toast.warning({ content: '请选择要重置的账号', duration: 3 })
      return
    }
    setLoading(true)
    try {
      const res = await selectForgotPasswordAccount({
        selection_token: selectionToken,
        username,
      })
      Toast.success({ content: `已确认账号 ${res.data.username}，请设置新密码`, duration: 3 })
      handleClose()
      navigate(`/reset-password?token=${encodeURIComponent(res.data.reset_token)}`)
    } catch {
      // 错误提示由请求层统一处理
    } finally {
      setLoading(false)
    }
  }

  const primaryText = step === 'email' ? '发送验证码' : step === 'verify' ? '验证邮箱' : '继续重置'

  return (
    <Modal
      title="找回密码"
      visible={visible}
      width={420}
      onCancel={handleClose}
      maskClosable={false}
      footer={
        <>
          <Button onClick={handleCancel}>{step === 'email' ? '取消' : '返回'}</Button>
          <Button theme="solid" type="primary" loading={loading} onClick={handleSubmit}>
            {primaryText}
          </Button>
        </>
      }
    >
      {step === 'email' && (
        <>
          <div className="qvm-field-label">绑定邮箱</div>
          <Input
            size="large"
            prefix={<IconMail />}
            placeholder="请输入账号绑定邮箱"
            value={email}
            onChange={setEmail}
            onEnterPress={handleSubmit}
          />
        </>
      )}

      {step === 'verify' && (
        <>
          <Banner
            type="info"
            description={`验证码已发送至 ${maskedEmail || '目标邮箱'}，10 分钟内有效`}
            closeIcon={null}
            style={{ marginBottom: 14, borderRadius: 10 }}
          />
          <div className="qvm-field-label">邮箱验证码</div>
          <Input
            size="large"
            prefix={<IconKey />}
            placeholder="请输入 6 位验证码"
            maxLength={6}
            value={code}
            onChange={setCode}
            onEnterPress={handleSubmit}
          />
        </>
      )}

      {step === 'select' && (
        <>
          <Banner
            type="success"
            description={`请选择 ${maskedEmail || '该邮箱'} 下要重置的账号`}
            closeIcon={null}
            style={{ marginBottom: 14, borderRadius: 10 }}
          />
          <div className="qvm-field-label">选择账号</div>
          <Select
            size="large"
            style={{ width: '100%' }}
            placeholder="请选择账号"
            value={username || undefined}
            onChange={(v) => setUsername(v as string)}
            optionList={accounts.map((a) => ({
              value: a.username,
              label: `${a.username}（${a.role === 'admin' ? '管理员' : '普通用户'}）`,
            }))}
          />
        </>
      )}
    </Modal>
  )
}
