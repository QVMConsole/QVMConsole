/**
 * 轻量云待开通服务器面板
 * 管理员登记配置后，用户在此补全登录凭据并确认开通
 */
import { useCallback, useEffect, useState } from 'react'
import { Button, Input, Modal, Tag, Toast } from '@douyinfe/semi-ui'
import { IconRefresh, IconServer, IconTick } from '@douyinfe/semi-icons'
import {
  confirmSelfLightweightVmRegistration,
  getSelfLightweightVmRegistrations,
  type LightweightRegistration,
} from '@/api/user'
import {
  checkPasswordBreachAsync,
  generatePassword,
  validatePassword,
  STRONG_PASSWORD_MIN_LENGTH,
  PASSWORD_ALLOWED_PATTERN,
} from '@/utils/validate'

interface PendingRegistrationsProps {
  /** 确认开通成功后刷新虚拟机列表 */
  onProvisioned: () => void
}

/** 登记状态文案 */
function statusText(status: string): string {
  const map: Record<string, string> = {
    pending: '待确认',
    provisioning: '开通中',
    failed: '失败',
  }
  return map[status] || status || '待确认'
}

function statusColor(status: string): 'amber' | 'blue' | 'red' {
  const map: Record<string, 'amber' | 'blue' | 'red'> = {
    pending: 'amber',
    provisioning: 'blue',
    failed: 'red',
  }
  return map[status] || 'amber'
}

/** 网络配额摘要 */
function quotaText(item: LightweightRegistration): string {
  const traffic = `${item.traffic_down_gb || 0}/${item.traffic_up_gb || 0}GB`
  const bandwidth = `${item.bandwidth_down_mbps || 0}/${item.bandwidth_up_mbps || 0}Mbps`
  const ports = item.max_port_forwards ?? 10
  return `流量 ${traffic}，带宽 ${bandwidth}，端口 ${ports}`
}

export default function PendingRegistrations({ onProvisioned }: PendingRegistrationsProps) {
  const [list, setList] = useState<LightweightRegistration[]>([])
  const [loading, setLoading] = useState(false)
  const [current, setCurrent] = useState<LightweightRegistration | null>(null)
  const [username, setUsername] = useState('')
  const [password, setPassword] = useState('')
  const [submitting, setSubmitting] = useState(false)

  const fetchList = useCallback(async () => {
    setLoading(true)
    try {
      const res = await getSelfLightweightVmRegistrations()
      setList(res.data || [])
    } catch (err) {
      console.error('获取待开通服务器失败', err)
    } finally {
      setLoading(false)
    }
  }, [])

  useEffect(() => {
    void fetchList()
  }, [fetchList])

  if (list.length === 0) {
    return null
  }

  const isWindows = current?.template_type === 'windows'

  const openConfirm = (item: LightweightRegistration) => {
    setCurrent(item)
    setUsername(item.template_type === 'windows' ? 'administrator' : 'admin')
    setPassword(generatePassword())
  }

  const handleSubmit = async () => {
    if (!current) return
    const finalUsername = isWindows ? 'administrator' : username.trim()
    if (!finalUsername) {
      Toast.warning('请填写登录用户名')
      return
    }
    if (password.length < STRONG_PASSWORD_MIN_LENGTH || !PASSWORD_ALLOWED_PATTERN.test(password)) {
      Toast.warning(`密码至少 ${STRONG_PASSWORD_MIN_LENGTH} 位，只支持字母、数字和 !@#$%^&*_-+=? 符号`)
      return
    }
    if (!validatePassword(password).valid) {
      Toast.warning('该密码过于常见，请更换为更安全的密码')
      return
    }
    // 异步泄露密码检测（HIBP）
    const breach = await checkPasswordBreachAsync(password)
    if (breach.enabled && breach.breached) {
      Toast.error('该密码已在已知泄露数据库中发现，请更换为更安全的密码')
      return
    }
    setSubmitting(true)
    try {
      const res = await confirmSelfLightweightVmRegistration(current.id, {
        username: finalUsername,
        password,
      })
      Toast.success(res.message || '开通任务已提交，请在任务中心查看进度')
      setCurrent(null)
      void fetchList()
      onProvisioned()
    } catch (err) {
      console.error('确认开通失败', err)
    } finally {
      setSubmitting(false)
    }
  }

  return (
    <section className="qvm-pending qvm-fade-up" style={{ '--qvm-delay': '40ms' } as React.CSSProperties}>
      <div className="qvm-pending-head">
        <div>
          <div className="qvm-pending-title">待开通服务器</div>
          <div className="qvm-pending-sub">
            这些服务器已由管理员登记配置。请逐台补全登录凭据，确认后系统才会开始开通。
          </div>
        </div>
        <Button size="small" icon={<IconRefresh />} loading={loading} onClick={() => void fetchList()}>
          刷新
        </Button>
      </div>
      <div className="qvm-pending-grid">
        {list.map((item) => (
          <div className="qvm-pending-card" key={item.id}>
            <div className="qvm-pending-card-title">
              <IconServer size="small" />
              <span>{item.vm_name}</span>
              <Tag size="small" color={statusColor(item.status)}>
                {statusText(item.status)}
              </Tag>
            </div>
            <div className="qvm-pending-meta">模板：{item.template || '-'}</div>
            <div className="qvm-pending-meta">
              规格：{item.vcpu}C / {item.ram}GB / {item.disk_size}GB
            </div>
            <div className="qvm-pending-meta">网络：{quotaText(item)}</div>
            {item.error_message && <div className="qvm-pending-error">{item.error_message}</div>}
            <Button
              block
              className="qvm-btn-grad qvm-btn-new-sm"
              disabled={item.status === 'provisioning'}
              onClick={() => openConfirm(item)}
            >
              {item.status === 'failed' ? '重新确认开通' : '确认开通'}
            </Button>
          </div>
        ))}
      </div>

      <Modal
        title={`确认开通 - ${current?.vm_name || ''}`}
        visible={!!current}
        onCancel={() => setCurrent(null)}
        onOk={() => void handleSubmit()}
        okText="确认开通"
        cancelText="取消"
        confirmLoading={submitting}
        okButtonProps={{ icon: <IconTick /> }}
        width={440}
      >
        <div className="qvm-form-item">
          <div className="qvm-form-label">登录用户名</div>
          <Input
            value={isWindows ? 'administrator' : username}
            disabled={isWindows}
            onChange={setUsername}
            placeholder="请输入登录用户名"
          />
          {isWindows && <div className="qvm-form-tip">Windows 模板固定使用 administrator</div>}
        </div>
        <div className="qvm-form-item">
          <div className="qvm-form-label">登录密码</div>
          <Input
            mode="password"
            value={password}
            onChange={setPassword}
            placeholder={`至少 ${STRONG_PASSWORD_MIN_LENGTH} 位强密码`}
          />
          <div className="qvm-form-tip-row">
            <span className="qvm-form-tip">支持字母、数字和 !@#$%^&*_-+=? 符号</span>
            <Button size="small" theme="borderless" type="primary" onClick={() => setPassword(generatePassword())}>
              随机强密码
            </Button>
          </div>
        </div>
      </Modal>
    </section>
  )
}
