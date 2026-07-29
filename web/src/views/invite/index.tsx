/**
 * 邀请注册页
 * 通过邮件邀请链接（/invite?token=xxx）进入，展示账号信息与配额，
 * 设置密码后完成注册并自动登录。弹性云 / 轻量云两种邀请形态。
 */
import { useCallback, useEffect, useMemo, useState, type CSSProperties } from 'react'
import { Banner, Button, Descriptions, Input, Spin, Table, Toast } from '@douyinfe/semi-ui'
import { IconLock } from '@douyinfe/semi-icons'
import { useNavigate, useSearchParams } from 'react-router'
import {
  completeInvite,
  getInviteInfo,
  type InviteDetail,
  type InviteLightweightRegistration,
} from '@/api/auth'
import { useUserStore } from '@/stores/user'
import { useAppStore } from '@/stores/app'
import { useTheme } from '@/hooks/useTheme'
import { CLOUD_TYPES, type CloudType } from '@/config/constants'
import { applyDocumentTitle } from '@/config/site'
import { formatDateTime } from '@/utils/format'
import { validatePassword, checkPasswordBreachAsync, STRONG_PASSWORD_MIN_LENGTH } from '@/utils/validate'
import loginBgDark from '@/assets/img/login-bg.png'
import loginBgLight from '@/assets/img/login-bg-light.png'
import '../login/login.css'
import './invite.css'

/** 配额展示：0 表示不限 */
const quotaText = (value: number, unit = '') => (value ? `${value}${unit}` : '不限')

/** 轻量云待确认服务器的网络配额摘要（与旧版展示口径一致） */
function formatRegistrationQuota(row: InviteLightweightRegistration): string {
  const traffic = `${row.traffic_down_gb || 0}/${row.traffic_up_gb || 0}GB`
  const bandwidth = `${row.bandwidth_down_mbps || 0}/${row.bandwidth_up_mbps || 0}Mbps`
  const runtime = row.max_runtime_hours ? `${row.max_runtime_hours}小时` : '不限'
  return `流量 ${traffic}，带宽 ${bandwidth}，端口 ${row.max_port_forwards ?? 10}，运行 ${runtime}`
}

/** 轻量云待确认服务器表格列 */
const registrationColumns = [
  { title: '名称', dataIndex: 'vm_name', width: 140 },
  { title: '模板', dataIndex: 'template', ellipsis: true },
  {
    title: '规格',
    width: 150,
    render: (_: unknown, row: InviteLightweightRegistration) =>
      `${row.vcpu}C / ${row.ram}GB / ${row.disk_size}GB`,
  },
  {
    title: '网络配额',
    width: 300,
    render: (_: unknown, row: InviteLightweightRegistration) => formatRegistrationQuota(row),
  },
]

export default function InviteRegisterPage() {
  const navigate = useNavigate()
  const [searchParams] = useSearchParams()
  const token = searchParams.get('token') || ''
  const setToken = useUserStore((s) => s.setToken)
  const setUserInfo = useUserStore((s) => s.setUserInfo)
  const siteTitle = useAppStore((s) => s.siteTitle)
  const { isDark } = useTheme()

  const [loading, setLoading] = useState(true)
  const [detail, setDetail] = useState<InviteDetail | null>(null)
  const [password, setPassword] = useState('')
  const [confirmPassword, setConfirmPassword] = useState('')
  const [submitting, setSubmitting] = useState(false)

  const isLightweight = detail?.cloud_type === CLOUD_TYPES.lightweight
  const registrations = detail?.lightweight_vm_registrations || []

  const fetchDetail = useCallback(async () => {
    if (!token) {
      Toast.error({ content: '邀请令牌不存在，请通过邮件中的完整链接访问', duration: 3 })
      navigate('/login', { replace: true })
      return
    }
    setLoading(true)
    try {
      const res = await getInviteInfo(token)
      setDetail(res.data)
    } catch {
      // 错误提示由请求层统一处理，页面保留失效态
    } finally {
      setLoading(false)
    }
  }, [token, navigate])

  useEffect(() => {
    applyDocumentTitle('邀请注册')
    fetchDetail()
  }, [fetchDetail])

  /** 账号信息（Descriptions 数据源） */
  const accountRows = useMemo(() => {
    if (!detail) return []
    const rows: { key: string; value: string }[] = [
      { key: '用户名', value: detail.username },
      { key: '邮箱', value: detail.email },
      { key: '角色', value: detail.role === 'admin' ? '管理员' : '普通用户' },
    ]
    if (detail.role !== 'admin') {
      rows.push({ key: '用户类型', value: isLightweight ? '轻量云' : '弹性云' })
    }
    rows.push({ key: '有效期', value: formatDateTime(detail.expires_at) })
    if (isLightweight) {
      rows.push(
        { key: '资源模式', value: '管理员分配云服务器' },
        { key: '配额模式', value: '流量、带宽、运行时长和端口转发按单台服务器配置' },
        {
          key: '专用 VPC',
          value: detail.dedicated_vpc_switch_id
            ? `#${detail.dedicated_vpc_switch_id}`
            : '管理员已配置',
        },
      )
    }
    return rows
  }, [detail, isLightweight])

  /** 弹性云配额（网格展示） */
  const quotaRows = useMemo(() => {
    if (!detail || isLightweight) return []
    const rows = [
      { key: 'CPU 配额', value: quotaText(detail.max_cpu, ' 核') },
      { key: '内存配额', value: quotaText(detail.max_memory, ' GB') },
      { key: '磁盘配额', value: quotaText(detail.max_disk, ' GB') },
      { key: '虚拟机数量', value: quotaText(detail.max_vm) },
      { key: '存储配额', value: quotaText(detail.max_storage, ' GB') },
      { key: '运行时长', value: quotaText(detail.max_runtime_hours, ' 小时') },
      { key: '端口转发', value: detail.enable_port_forward ? '已开通' : '未开通' },
    ]
    if (detail.enable_port_forward) {
      rows.push({ key: '端口转发配额', value: quotaText(detail.max_port_forwards) })
    }
    return rows
  }, [detail, isLightweight])

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
      const res = await completeInvite({
        token,
        password,
        confirm_password: confirmPassword,
      })
      const data = res.data
      if (data.token) {
        setToken(data.token)
        setUserInfo(
          data.username,
          data.role,
          data.security,
          (data.cloud_type || CLOUD_TYPES.elastic) as CloudType,
        )
        Toast.success({ content: '注册完成，已自动登录', duration: 3 })
        navigate('/', { replace: true })
      }
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
        <div className="qvm-login-card qvm-invite-card qvm-g-border qvm-fade-up">
          <div className="qvm-lc-head">
            <div className="qvm-lc-logo">Q</div>
            <div className="qvm-lc-title">邀请注册</div>
            <div className="qvm-lc-sub">欢迎加入 {siteTitle}，请确认信息并设置登录密码</div>
          </div>

          {loading ? (
            <div className="qvm-invite-loading">
              <Spin size="large" />
            </div>
          ) : !detail ? (
            <div className="qvm-invite-invalid">
              <Banner
                type="danger"
                description="邀请链接无效或已过期，请联系管理员重新发送邀请邮件。"
                closeIcon={null}
              />
              <Button
                block
                className="qvm-btn-grad qvm-btn-login"
                style={{ marginTop: 20 }}
                onClick={() => navigate('/login')}
              >
                返回登录
              </Button>
            </div>
          ) : (
            <>
              {/* 账号信息 */}
              <div className="qvm-invite-sec-title">账号信息</div>
              <Descriptions data={accountRows} align="left" className="qvm-invite-desc" />

              {/* 弹性云配额 */}
              {quotaRows.length > 0 && (
                <>
                  <div className="qvm-invite-sec-title">资源配额</div>
                  <div className="qvm-invite-quota-grid">
                    {quotaRows.map((q) => (
                      <div className="qvm-invite-quota-item" key={q.key}>
                        <span className="qk">{q.key}</span>
                        <span className="qv">{q.value}</span>
                      </div>
                    ))}
                  </div>
                </>
              )}

              {/* 轻量云说明 + 待确认服务器 */}
              {isLightweight && (
                <Banner
                  type="info"
                  className="qvm-invite-banner"
                  description="轻量云账号注册后不会显示用户级额度。管理员会直接分配服务器，并为每台服务器设置流量、带宽和端口转发上限。"
                  closeIcon={null}
                />
              )}
              {isLightweight && registrations.length > 0 && (
                <>
                  <div className="qvm-invite-sec-title">待确认开通服务器</div>
                  <Table
                    columns={registrationColumns}
                    dataSource={registrations}
                    rowKey="id"
                    size="small"
                    pagination={false}
                    className="qvm-invite-table"
                  />
                  <div className="qvm-invite-note">
                    注册完成并登录面板后，请在虚拟机列表中逐台补全登录凭据并确认开通。
                  </div>
                </>
              )}

              {/* 设置密码 */}
              <div className="qvm-invite-sec-title">设置登录密码</div>
              <div className="qvm-field-label">密码</div>
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
                完成注册
              </Button>
              <div className="qvm-invite-back">
                已有账号？<a onClick={() => navigate('/login')}>返回登录</a>
              </div>
            </>
          )}
        </div>
      </div>
    </div>
  )
}
