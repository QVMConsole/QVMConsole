/**
 * 安全与维护 Tab：邮件与安全验证（SMTP）/ 安全防护 / JWT 密钥管理 / 维护模式
 */
import { useCallback, useEffect, useRef, useState } from 'react'
import { Banner, Button, Input, InputNumber, Select, Tag, TextArea, Toast } from '@douyinfe/semi-ui'
import { IconAlertTriangle, IconLock, IconMail, IconRefresh } from '@douyinfe/semi-icons'
import TextSwitch from '@/features/vm-form/sections/TextSwitch'
import { rotateJWTSecret, testSMTP } from '@/api/settings'
import { getTaskDetail } from '@/api/task'
import {
  getPasswordBreachStatus,
  startPasswordBreachScan,
  type PasswordBreachStatus,
} from '@/api/passwordBreach'
import { confirmModal } from '@/utils/confirm'
import { SectionHead, SettingRow } from './SettingRow'
import type { SettingsTabProps } from '../types'

interface SecurityTabProps extends SettingsTabProps {
  /** 测试发信前先静默保存当前配置，返回是否保存成功 */
  saveBeforeAction: () => Promise<boolean>
  /** 保存/轮换成功后刷新表单 */
  refresh: () => Promise<void>
}

export default function SecurityTab({ form, patch, saveBeforeAction, refresh }: SecurityTabProps) {
  const [testing, setTesting] = useState(false)
  const [rotating, setRotating] = useState(false)
  const [breachStatus, setBreachStatus] = useState<PasswordBreachStatus | null>(null)
  const [breachTaskRunning, setBreachTaskRunning] = useState(false)
  const [breachTaskId, setBreachTaskId] = useState<number | null>(null)
  const reportedBreachTasks = useRef(new Set<number>())

  const refreshBreachStatus = useCallback(async () => {
    try {
      const res = await getPasswordBreachStatus()
      setBreachStatus(res.data.status)
      if (res.data.active_task?.status === 'pending' || res.data.active_task?.status === 'running') {
        setBreachTaskRunning(true)
        setBreachTaskId(res.data.active_task.id)
      }
    } catch {
      // 静默刷新，错误由手动操作反馈
    }
  }, [])

  useEffect(() => {
    void refreshBreachStatus()
  }, [refreshBreachStatus])

  useEffect(() => {
    if (!breachTaskRunning || !breachTaskId) return
    const pollTask = async () => {
      try {
        const res = await getTaskDetail(breachTaskId)
        const task = res.data
        if (task.status === 'pending' || task.status === 'running') {
          await refreshBreachStatus()
          return
        }
        setBreachTaskRunning(false)
        setBreachTaskId(null)
        await refreshBreachStatus()
        if (reportedBreachTasks.current.has(task.id)) return
        reportedBreachTasks.current.add(task.id)
        if (task.status === 'success') {
          let summary = ''
          try {
            const result = JSON.parse(task.result || '{}') as {
              breached_admins?: number
              breached_users?: number
            }
            summary = `：管理员 ${result.breached_admins || 0}，普通用户 ${result.breached_users || 0}`
          } catch {
            // 结果解析失败时仍展示通用完成提示
          }
          Toast.success(`密码泄露检测已完成${summary}`)
        } else {
          Toast.error(`${task.message || '密码泄露检测任务执行失败'}，可前往任务中心查看详情`)
        }
      } catch {
        // 临时轮询失败时保留运行状态，下次继续获取
      }
    }
    void pollTask()
    const timer = window.setInterval(() => void pollTask(), 2000)
    return () => window.clearInterval(timer)
  }, [breachTaskId, breachTaskRunning, refreshBreachStatus])

  const handleRunBreachScan = async () => {
    const ok = await confirmModal({
      title: '立即执行密码泄露检测',
      content: '检测任务可能撤销管理员现有会话、限制泄露账户登录并发送安全通知，确定继续吗？',
      okText: '立即执行',
      danger: true,
    })
    if (!ok) return
    setBreachTaskRunning(true)
    try {
      const res = await startPasswordBreachScan()
      Toast.success(res.message || '密码泄露检测任务已提交')
      setBreachTaskId(res.data.task.id)
      await refreshBreachStatus()
    } catch {
      setBreachTaskRunning(false)
    }
  }

  // 保存当前配置后发送测试邮件
  const handleTestSMTP = async () => {
    if (!form.smtp_test_email) {
      Toast.warning('请输入测试收件邮箱')
      return
    }
    setTesting(true)
    try {
      const saved = await saveBeforeAction()
      if (!saved) return
      await testSMTP({ email: form.smtp_test_email })
      Toast.success('测试邮件已发送，请检查收件箱')
      await refresh()
    } catch {
      // 请求层已统一提示
    } finally {
      setTesting(false)
    }
  }

  // 手动轮换 JWT 密钥（高风险操作，428 二次验证由请求层处理）
  const handleRotateJWT = async () => {
    const ok = await confirmModal({
      title: '轮换 JWT 密钥',
      content: '轮换 JWT 密钥后所有用户 Token 将立即失效，需要重新登录。确定继续吗？',
      okText: '确定轮换',
      danger: true,
    })
    if (!ok) return
    setRotating(true)
    try {
      const res = await rotateJWTSecret()
      Toast.success(res.message || 'JWT 密钥轮换成功')
      await refresh()
    } catch {
      // 请求层已统一提示
    } finally {
      setRotating(false)
    }
  }

  return (
    <div className="stg-tab-pane">
      <SectionHead icon={<IconMail />} title="邮件与安全验证" />

      <Banner
        type={form.smtp_configured ? 'success' : 'warning'}
        closeIcon={null}
        className="stg-banner"
        description={
          form.smtp_configured
            ? 'SMTP 已配置，可用于邮箱绑定、邀请注册和密码找回。'
            : 'SMTP 尚未配置，邮箱绑定、邀请注册和密码找回将不可用。'
        }
      />

      <SettingRow
        label="启用开发环境"
        tip="启用后将绕过登录二段验证、首次强制绑定和高风险操作验证，仅建议在开发调试环境使用 | 环境变量: KVM_DEVELOPMENT_MODE"
      >
        <TextSwitch
          checked={form.development_mode}
          onChange={(v) => patch({ development_mode: v })}
        />
      </SettingRow>

      <SettingRow label="SMTP 主机" tip="环境变量: KVM_SMTP_HOST">
        <Input
          value={form.smtp_host}
          onChange={(v) => patch({ smtp_host: v })}
          placeholder="如 smtp.qq.com"
        />
      </SettingRow>

      <SettingRow label="SMTP 端口" tip="环境变量: KVM_SMTP_PORT">
        <InputNumber
          value={form.smtp_port}
          onNumberChange={(v) => patch({ smtp_port: v })}
          min={1}
          max={65535}
          style={{ width: '100%' }}
        />
      </SettingRow>

      <SettingRow label="SMTP 用户名" tip="环境变量: KVM_SMTP_USERNAME">
        <Input
          value={form.smtp_username}
          onChange={(v) => patch({ smtp_username: v })}
          placeholder="通常为发件邮箱账号"
        />
      </SettingRow>

      <SettingRow
        label="SMTP 密码"
        tip={
          form.smtp_password_configured
            ? '当前已保存 SMTP 密码，留空不会覆盖。'
            : '环境变量: KVM_SMTP_PASSWORD_ENC'
        }
      >
        <Input
          mode="password"
          value={form.smtp_password}
          onChange={(v) => patch({ smtp_password: v })}
          placeholder={
            form.smtp_password_configured ? '留空表示保持当前密码不变' : '请输入 SMTP 密码或授权码'
          }
        />
      </SettingRow>

      <SettingRow label="发件人名称" tip="环境变量: KVM_SMTP_FROM_NAME">
        <Input
          value={form.smtp_from_name}
          onChange={(v) => patch({ smtp_from_name: v })}
          placeholder="默认展示名称"
        />
      </SettingRow>

      <SettingRow label="发件邮箱" tip="环境变量: KVM_SMTP_FROM_ADDRESS">
        <Input
          value={form.smtp_from_address}
          onChange={(v) => patch({ smtp_from_address: v })}
          placeholder="如 no-reply@example.com"
        />
      </SettingRow>

      <SettingRow label="加密方式" tip="环境变量: KVM_SMTP_SECURITY">
        <Select
          value={form.smtp_security}
          onChange={(v) => patch({ smtp_security: v as string })}
          style={{ width: '100%' }}
          optionList={[
            { label: 'STARTTLS', value: 'starttls' },
            { label: 'SSL/TLS', value: 'ssl' },
            { label: '无加密', value: 'none' },
          ]}
        />
      </SettingRow>

      <SettingRow label="超时秒数" tip="环境变量: KVM_SMTP_TIMEOUT_SECONDS">
        <InputNumber
          value={form.smtp_timeout_seconds}
          onNumberChange={(v) => patch({ smtp_timeout_seconds: v })}
          min={5}
          max={120}
          style={{ width: '100%' }}
        />
      </SettingRow>

      <SettingRow label="测试收件邮箱" tip="点击右侧按钮会先保存当前配置，再发送测试邮件">
        <div className="stg-inline-group">
          <Input
            value={form.smtp_test_email}
            onChange={(v) => patch({ smtp_test_email: v })}
            placeholder="保存配置后发送测试邮件"
            style={{ flex: 1 }}
          />
          <Button loading={testing} onClick={() => void handleTestSMTP()}>
            测试发信
          </Button>
        </div>
      </SettingRow>

      <SectionHead icon={<IconLock />} title="安全防护" />

      <SettingRow
        label="会话指纹绑定"
        tip="开启后，Token 将绑定客户端特征（IP段+浏览器），被盗用后无法跨设备使用 | 环境变量: KVM_SESSION_FINGERPRINT_ENABLED"
      >
        <TextSwitch
          checked={form.session_fingerprint_enabled}
          onChange={(v) => patch({ session_fingerprint_enabled: v })}
        />
      </SettingRow>

      <SettingRow
        label="请求过滤"
        tip="开启后，自动拦截路径穿越、扫描器探测等危险请求 | 环境变量: KVM_REQUEST_FILTER_ENABLED"
      >
        <TextSwitch
          checked={form.request_filter_enabled}
          onChange={(v) => patch({ request_filter_enabled: v })}
        />
      </SettingRow>

      <SettingRow
        label="泄露密码检测"
        tip="开启后，用户设置密码时将比对 Have I Been Pwned 泄露密码数据库（110亿+条记录，采用 k-匿名性模型，密码哈希不离开本机）及内置常见弱密码列表，命中则阻止。关闭后跳过所有密码校验"
      >
        <TextSwitch
          checked={form.password_breach_check_enabled}
          onChange={(v) => patch({ password_breach_check_enabled: v })}
        />
      </SettingRow>

      <SettingRow
        label="定时泄露检测"
        tip="开启后每天按宿主机本地时间 00:00 检测已登记的账户密码；立即执行按钮不受任一泄露检测开关限制 | 环境变量: KVM_SCHEDULED_PASSWORD_BREACH_CHECK_ENABLED"
      >
        <div className="stg-inline-group">
          <TextSwitch
            checked={form.scheduled_password_breach_check_enabled}
            onChange={(v) => patch({ scheduled_password_breach_check_enabled: v })}
          />
          <Button
            icon={<IconRefresh spin={breachTaskRunning} />}
            disabled={breachTaskRunning}
            onClick={() => void handleRunBreachScan()}
          >
            {breachTaskRunning ? '检测中' : '立即执行'}
          </Button>
        </div>
      </SettingRow>

      {breachStatus && (
        <Banner
          type={breachStatus.breached_total > 0 ? 'danger' : 'info'}
          closeIcon={null}
          className="stg-banner"
          description={
            breachTaskRunning
              ? '密码泄露检测任务正在执行，可在任务中心查看进度。'
              : breachStatus.breached_total > 0
                ? `当前有 ${breachStatus.breached_total} 个泄露账户（管理员 ${breachStatus.breached_admins}，普通用户 ${breachStatus.breached_users}）。`
                : breachStatus.last_checked_at
                  ? `最近一次检测未发现泄露账户：${new Date(breachStatus.last_checked_at).toLocaleString()}`
                  : '尚未执行过定时泄露检测。'
          }
        />
      )}

      <SectionHead icon={<IconAlertTriangle />} title="JWT 密钥管理" />

      <SettingRow
        label="自动轮换间隔"
        tip="默认 24 小时自动轮换 JWT 签名密钥，设为 0 禁用自动轮换。开发模式下自动轮换会被跳过 | 环境变量: KVM_JWT_SECRET_ROTATE_HOURS"
      >
        <InputNumber
          value={form.jwt_secret_rotate_hours}
          onNumberChange={(v) => patch({ jwt_secret_rotate_hours: v })}
          min={0}
          max={720}
          disabled={form.development_mode}
          style={{ width: '100%' }}
        />
      </SettingRow>

      {form.jwt_secret_last_rotated && (
        <SettingRow label="上次轮换时间">
          <Tag size="small" color="cyan">
            {form.jwt_secret_last_rotated}
          </Tag>
        </SettingRow>
      )}

      <SettingRow
        label="手动轮换JWT密钥"
        tip={
          form.development_mode
            ? '开发模式下 JWT 密钥轮换功能已禁用'
            : '轮换后所有 Token 将立即失效，所有用户需重新登录。此操作需高风险二次验证'
        }
      >
        <Button
          type="danger"
          theme="light"
          loading={rotating}
          disabled={form.development_mode}
          onClick={() => void handleRotateJWT()}
        >
          {form.development_mode ? '开发模式不允许轮换' : '立即轮换 JWT 密钥'}
        </Button>
      </SettingRow>

      <SectionHead icon={<IconAlertTriangle />} title="维护模式" />

      <Banner
        type="warning"
        closeIcon={null}
        className="stg-banner"
        description="启用维护模式后，系统会异步关闭所有运行中的虚拟机，并停用配置中的宿主机服务。维护模式期间将阻止虚拟机启动。"
      />

      <SettingRow
        label="启用维护模式"
        tip="保存时会要求二次验证，启用后可到任务中心查看执行进度 | 环境变量: KVM_MAINTENANCE_MODE"
      >
        <TextSwitch
          checked={form.maintenance_mode}
          onChange={(v) => patch({ maintenance_mode: v })}
        />
      </SettingRow>

      <SettingRow
        label="关机等待时间"
        tip="单位秒，维护模式关闭虚拟机时先尝试优雅关机，超时后会强制断电 | 环境变量: KVM_MAINTENANCE_VM_SHUTDOWN_TIMEOUT_SECONDS"
      >
        <InputNumber
          value={form.maintenance_vm_shutdown_timeout_seconds}
          onNumberChange={(v) => patch({ maintenance_vm_shutdown_timeout_seconds: v })}
          min={5}
          max={3600}
          style={{ width: '100%' }}
        />
      </SettingRow>

      <SettingRow
        label="维护服务列表"
        tip="建议填写 libvirtd 等相关服务；kvm-console.service 即使加入也会被自动跳过，确保主机重启后面板仍自动启动 | 环境变量: KVM_MAINTENANCE_SERVICE_UNITS"
      >
        <TextArea
          value={form.maintenance_service_units}
          onChange={(v) => patch({ maintenance_service_units: v })}
          rows={5}
          placeholder="每行一个 systemd unit，也支持逗号分隔"
        />
      </SettingRow>
    </div>
  )
}
