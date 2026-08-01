/**
 * 编辑用户弹窗
 * - 支持更新邮箱与登录密码，密码留空时保持不变
 * - 管理员：仅存储配额
 * - 弹性云用户：云类型切换 + 完整用户级配额（含使用量展示）
 * - 轻量云用户：云类型切换 + 专用 VPC 选择（计算配额由单 VM 配额管理）
 */
import { useState } from 'react'
import { Banner, Button, Input, InputNumber, Modal, Select, Tag, Toast } from '@douyinfe/semi-ui'
import { IconRefresh } from '@douyinfe/semi-icons'
import {
  updateUserAccount,
  updateUserQuota,
  type UserListItem,
  type UserQuotaPayload,
} from '@/api/user'
import type { VpcSwitch } from '@/api/vpc'
import QuotaFormFields from '../components/QuotaFormFields'
import { vpcOptionLabel } from './vpcOption'
import { useMountModalLifecycle } from '@/hooks/useMountModalLifecycle'
import { checkPasswordBreachAsync, generatePassword, validatePassword } from '@/utils/validate'

interface EditQuotaDialogProps {
  row: UserListItem
  natVpcSwitches: VpcSwitch[]
  onClose: () => void
  onSaved: () => void
}

export default function EditQuotaDialog({
  row,
  natVpcSwitches,
  onClose,
  onSaved,
}: EditQuotaDialogProps) {
  const { modalVisible, requestClose, afterModalClose } = useMountModalLifecycle(onClose)
  const isAdminUser = row.role === 'admin'
  const [submitting, setSubmitting] = useState(false)
  const [email, setEmail] = useState(row.email || '')
  const [password, setPassword] = useState('')
  const [confirmPassword, setConfirmPassword] = useState('')
  const [cloudType, setCloudType] = useState(row.cloud_type || 'elastic')
  const [vpcSwitchId, setVpcSwitchId] = useState<number | null>(
    row.dedicated_vpc_switch_id || null,
  )
  const initialQuota: UserQuotaPayload = {
    max_cpu: row.max_cpu || 0,
    max_memory: row.max_memory || 0,
    max_disk: row.max_disk || 0,
    max_vm: row.max_vm || 0,
    max_storage: row.max_storage || 0,
    max_runtime_hours: row.max_runtime_hours || 0,
    enable_port_forward: row.enable_port_forward ?? true,
    max_port_forwards: row.max_port_forwards ?? 10,
    max_snapshots: row.max_snapshots ?? 5,
    max_public_ips: row.max_public_ips || 0,
    max_bandwidth_up: row.max_bandwidth_up || 0,
    max_bandwidth_down: row.max_bandwidth_down || 0,
    max_traffic_down: row.max_traffic_down || 0,
    max_traffic_up: row.max_traffic_up || 0,
  }
  const [quota, setQuota] = useState<UserQuotaPayload>(initialQuota)

  const patchQuota = (p: Partial<UserQuotaPayload>) => setQuota((q) => ({ ...q, ...p }))

  const handleSubmit = async () => {
    const normalizedEmail = email.trim()
    const accountChanged = normalizedEmail !== (row.email || '').trim() || !!password
    const resourceChanged =
      JSON.stringify(quota) !== JSON.stringify(initialQuota) ||
      (!isAdminUser &&
        (cloudType !== (row.cloud_type || 'elastic') ||
          vpcSwitchId !== (row.dedicated_vpc_switch_id || null)))

    if (password) {
      const local = validatePassword(password)
      if (!local.valid) {
        Toast.error(local.message)
        return
      }
      if (password !== confirmPassword) {
        Toast.error('两次输入的密码不一致')
        return
      }
      const breach = await checkPasswordBreachAsync(password)
      if (breach.enabled && breach.breached) {
        Toast.error('该密码已在已知泄露数据库中发现，请更换为更安全的密码')
        return
      }
    }
    if (resourceChanged && !isAdminUser && cloudType === 'lightweight' && !vpcSwitchId) {
      Toast.warning('请为轻量云用户选择专用 VPC')
      return
    }
    setSubmitting(true)
    try {
      if (accountChanged) {
        await updateUserAccount(row.username, {
          email: normalizedEmail,
          ...(password ? { password } : {}),
        })
      }
      if (resourceChanged) {
        // 管理员仅提交存储配额，不发送云类型与 VPC
        const payload = isAdminUser
          ? { ...quota }
          : { ...quota, cloud_type: cloudType, dedicated_vpc_switch_id: vpcSwitchId }
        await updateUserQuota(row.username, payload)
      }
      Toast.success(accountChanged || resourceChanged ? '用户配置更新成功' : '用户配置未发生变化')
      onSaved()
      requestClose()
    } catch {
      // 请求层已提示
    } finally {
      setSubmitting(false)
    }
  }

  return (
    <Modal
      title="编辑用户"
      visible={modalVisible}
      afterClose={afterModalClose}
      onCancel={requestClose}
      onOk={() => void handleSubmit()}
      okText="保存"
      cancelText="取消"
      confirmLoading={submitting}
      width={860}
      closeOnEsc
      bodyStyle={{ maxHeight: '68vh', overflowY: 'auto' }}
    >
      <Banner
        type="info"
        closeIcon={null}
        description={
          <span>
            用户：<strong>{row.username}</strong>{' '}
            <Tag size="small" color={isAdminUser ? 'red' : 'green'}>
              {isAdminUser ? '管理员' : '普通用户'}
            </Tag>
            <span style={{ marginLeft: 12 }}>设为 0 表示不限制</span>
          </span>
        }
        style={{ marginBottom: 16 }}
      />

      <div className="usr-section-title">账户资料</div>
      <div className="usr-quota-grid cols-2">
        <div className="qvm-form-item">
          <div className="qvm-form-label">邮箱</div>
          <Input value={email} onChange={setEmail} placeholder="选填；留空可清除邮箱" />
        </div>
        <div className="qvm-form-item">
          <div className="qvm-form-label">新密码</div>
          <div className="usr-password-row">
            <Input
              mode="password"
              value={password}
              onChange={setPassword}
              placeholder="留空则保持当前密码"
            />
            <Button
              type="primary"
              theme="light"
              icon={<IconRefresh />}
              onClick={() => {
                const generated = generatePassword()
                setPassword(generated)
                setConfirmPassword(generated)
              }}
            >
              随机
            </Button>
          </div>
        </div>
        <div className="qvm-form-item">
          <div className="qvm-form-label">确认新密码</div>
          <Input
            mode="password"
            value={confirmPassword}
            onChange={setConfirmPassword}
            placeholder={password ? '请再次输入新密码' : '设置新密码后需确认'}
          />
        </div>
      </div>

      <div className="usr-section-title">资源配置</div>

      {!isAdminUser && (
        <>
          <div className="qvm-form-item">
            <div className="qvm-form-label">用户类型</div>
            <Select
              value={cloudType}
              onChange={(v) => setCloudType(v as string)}
              style={{ width: 220 }}
              optionList={[
                { label: '弹性云', value: 'elastic' },
                { label: '轻量云', value: 'lightweight' },
              ]}
            />
          </div>
          {cloudType === 'lightweight' && (
            <div className="qvm-form-item">
              <div className="qvm-form-label required">专用 VPC</div>
              <Select
                value={vpcSwitchId ?? undefined}
                onChange={(v) => setVpcSwitchId(v as number)}
                filter
                placeholder="请选择管理员创建的 NAT VPC"
                style={{ width: '100%' }}
                optionList={natVpcSwitches.map((item) => ({
                  label: vpcOptionLabel(item),
                  value: item.id,
                }))}
              />
            </div>
          )}
        </>
      )}

      {/* 管理员仅存储配额 */}
      {isAdminUser && (
        <div className="qvm-form-item">
          <div className="qvm-form-label">存储配额 (GB)</div>
          <div style={{ display: 'flex', alignItems: 'center', gap: 12 }}>
            <InputNumber
              value={quota.max_storage}
              onNumberChange={(v) => patchQuota({ max_storage: Number(v) || 0 })}
              min={0}
              max={102400}
              style={{ width: 240 }}
            />
            {row.quota && (
              <span className="usr-muted">
                {quota.max_storage > 0
                  ? `${row.quota.used_storage_gb || '0 B'}/${quota.max_storage}GB`
                  : `已用 ${row.quota.used_storage_gb || '0 B'}（不限）`}
              </span>
            )}
          </div>
          <div className="qvm-form-tip">
            设为 0 表示不限制。管理员仅受存储配额约束，其他资源不受限制。
          </div>
        </div>
      )}

      {/* 弹性云普通用户：完整配额表单（含使用量） */}
      {!isAdminUser && cloudType !== 'lightweight' && (
        <QuotaFormFields value={quota} onChange={patchQuota} usage={row.quota} />
      )}

      {!isAdminUser && cloudType === 'lightweight' && (
        <Banner
          type="warning"
          closeIcon={null}
          description="轻量云用户的流量、带宽、端口转发等配额按单 VM 管理，请在「注册 VM」入口中调整。"
        />
      )}
    </Modal>
  )
}
