/**
 * 高风险操作二次验证弹窗（HTTP 428）
 * 敏感操作触发后端 428 响应时自动弹出，用户完成 2FA / 邮箱验证后重试原请求。
 * 全局单例，挂载于 App 根部。
 */
import { useEffect, useState } from 'react'
import { Modal, Input, Typography } from '@douyinfe/semi-ui'
import { useHighRiskStore } from '@/stores/highRisk'

export default function HighRiskChallengeModal() {
  const pending = useHighRiskStore((s) => s.pending)
  const submit = useHighRiskStore((s) => s.submit)
  const cancel = useHighRiskStore((s) => s.cancel)
  const [challenge, setChallenge] = useState(pending)
  const [code, setCode] = useState('')
  const [error, setError] = useState('')

  // 每次打开弹窗时重置输入
  useEffect(() => {
    if (pending) {
      setChallenge(pending)
      setCode('')
      setError('')
    }
  }, [pending])

  if (!challenge) return null

  const isTotp = challenge.method === 'totp'
  const hasRecovery = !!challenge.has_recovery

  const validate = (value: string): string => {
    const trimmed = value.trim()
    if (isTotp) {
      // 6 位 TOTP 验证码或 16 位恢复码
      if (hasRecovery && trimmed.length >= 16) return ''
      if (/^\d{6}$/.test(trimmed)) return ''
      return hasRecovery ? '请输入 6 位验证码或 16 位恢复码' : '请输入 6 位验证码'
    }
    if (/^\d{6}$/.test(trimmed)) return ''
    return '请输入 6 位验证码'
  }

  const handleOk = () => {
    const trimmed = code.trim()
    const errMsg = validate(trimmed)
    if (errMsg) {
      setError(errMsg)
      return
    }
    // 自动判断是 TOTP 验证码还是恢复码
    const method = isTotp && hasRecovery && trimmed.length >= 16 ? 'recovery' : challenge.method || 'totp'
    submit({
      method,
      code: trimmed,
      challenge_id: challenge.challenge_id,
      operation: challenge.operation,
    })
  }

  const title = isTotp ? '高风险验证' : '邮箱验证'
  const tip = isTotp
    ? `请输入 2FA 验证码${hasRecovery ? '（无法使用验证器时可输入恢复码）' : ''}`
    : `验证码已发送至 ${challenge.masked_email || '您的邮箱'}，请输入邮箱验证码`

  return (
    <Modal
      title={title}
      visible={!!pending}
      afterClose={() => {
        if (!pending) setChallenge(null)
      }}
      onOk={handleOk}
      onCancel={cancel}
      okText="验证"
      cancelText="取消"
      closable={false}
      maskClosable={false}
      width={420}
    >
      <Typography.Paragraph style={{ marginBottom: 12 }}>{tip}</Typography.Paragraph>
      <Input
        autoFocus
        value={code}
        onChange={(value) => {
          setCode(value)
          setError('')
        }}
        onEnterPress={handleOk}
        placeholder={isTotp ? '6 位验证码或 16 位恢复码' : '6 位邮箱验证码'}
        validateStatus={error ? 'error' : 'default'}
      />
      {error && (
        <Typography.Text type="danger" size="small">
          {error}
        </Typography.Text>
      )}
    </Modal>
  )
}
