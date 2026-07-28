/**
 * 两步验证（2FA）区块
 * - 未启用：生成配置 → 扫码/手输密钥 → 输入动态验证码 → 启用（返回一次性恢复码）
 * - 已启用：关闭 2FA（密码 + 动态验证码）；恢复码管理（状态展示 + 重新生成）
 * - 关闭 2FA 为危险操作，提交前需二次确认
 */
import { useState } from 'react'
import QRCode from 'qrcode'
import { Banner, Button, Input, Toast } from '@douyinfe/semi-ui'
import { disable2FA, enable2FA, regenRecoveryCodes, setup2FA } from '@/api/auth'
import { useUserStore } from '@/stores/user'
import { confirmModal } from '@/utils/confirm'
import RecoveryCodesModal from './RecoveryCodesModal'

interface TotpSectionProps {
  /** 状态变更后刷新全局安全状态 */
  refreshSecurity: () => Promise<void>
}

export default function TotpSection({ refreshSecurity }: TotpSectionProps) {
  const security = useUserStore((s) => s.security)
  const totpEnabled = !!security?.totp_enabled

  // ==================== 启用流程 ====================
  const [setupLoading, setSetupLoading] = useState(false)
  const [enableLoading, setEnableLoading] = useState(false)
  const [secret, setSecret] = useState('')
  const [qrCodeData, setQrCodeData] = useState('')
  const [setupCode, setSetupCode] = useState('')

  // ==================== 关闭流程 ====================
  const [disableForm, setDisableForm] = useState({ password: '', code: '' })
  const [disableLoading, setDisableLoading] = useState(false)

  // ==================== 恢复码重新生成 ====================
  const [regenForm, setRegenForm] = useState({ password: '', code: '' })
  const [regenLoading, setRegenLoading] = useState(false)

  // ==================== 恢复码展示 ====================
  const [recoveryCodes, setRecoveryCodes] = useState<string[]>([])

  /** 生成 2FA 配置并渲染二维码 */
  const handleGenerate = async () => {
    setSetupLoading(true)
    try {
      const res = await setup2FA()
      const { secret: newSecret, otpauth_url: otpauthUrl } = res.data
      const qrData = await QRCode.toDataURL(otpauthUrl, { width: 180, margin: 1 })
      setSecret(newSecret)
      setQrCodeData(qrData)
      setSetupCode('')
    } catch {
      // 请求层已统一提示
    } finally {
      setSetupLoading(false)
    }
  }

  /** 提交验证码启用 2FA，成功后弹出恢复码 */
  const handleEnable = async () => {
    if (!secret) {
      Toast.warning('请先生成 2FA 配置')
      return
    }
    if (!setupCode.trim()) {
      Toast.warning('请输入 6 位验证码')
      return
    }
    setEnableLoading(true)
    try {
      const res = await enable2FA({ secret, code: setupCode.trim() })
      const codes = res.recovery?.recovery_codes
      if (codes?.length) {
        setRecoveryCodes(codes)
      }
      await refreshSecurity()
      setSecret('')
      setQrCodeData('')
      setSetupCode('')
      Toast.success(res.message || '2FA 已启用')
    } catch {
      // 请求层已统一提示
    } finally {
      setEnableLoading(false)
    }
  }

  /** 关闭 2FA（危险操作，二次确认） */
  const handleDisable = async () => {
    if (!disableForm.password || !disableForm.code.trim()) {
      Toast.warning('请输入当前密码和 2FA 验证码')
      return
    }
    const ok = await confirmModal({
      title: '关闭 2FA',
      content: '关闭后账户将不再需要两步验证，安全性会降低。确定要关闭吗？',
      okText: '确定关闭',
      danger: true,
    })
    if (!ok) return
    setDisableLoading(true)
    try {
      await disable2FA({ password: disableForm.password, code: disableForm.code.trim() })
      await refreshSecurity()
      setDisableForm({ password: '', code: '' })
      Toast.success('2FA 已关闭')
    } catch {
      // 请求层已统一提示
    } finally {
      setDisableLoading(false)
    }
  }

  /** 重新生成恢复码（旧码立即失效） */
  const handleRegen = async () => {
    if (!regenForm.password || !regenForm.code.trim()) {
      Toast.warning('请输入当前密码和 2FA 验证码')
      return
    }
    setRegenLoading(true)
    try {
      const res = await regenRecoveryCodes({ password: regenForm.password, code: regenForm.code.trim() })
      const codes = res.recovery?.recovery_codes
      if (codes?.length) {
        setRecoveryCodes(codes)
      }
      await refreshSecurity()
      setRegenForm({ password: '', code: '' })
    } catch {
      // 请求层已统一提示
    } finally {
      setRegenLoading(false)
    }
  }

  return (
    <div className="sec-tab-pane">
      <Banner
        type={totpEnabled ? 'success' : 'warning'}
        closeIcon={null}
        className="sec-banner"
        description={totpEnabled ? '已启用 2FA 验证' : '建议启用 2FA 验证增强账户安全'}
      />

      {totpEnabled ? (
        <>
          {/* 关闭 2FA */}
          <div className="sec-sub-title">关闭 2FA</div>
          <div className="sec-row">
            <div className="sec-row-label">当前密码</div>
            <div className="sec-row-main">
              <Input
                mode="password"
                value={disableForm.password}
                onChange={(v) => setDisableForm((p) => ({ ...p, password: v }))}
                placeholder="请输入当前密码"
              />
            </div>
          </div>
          <div className="sec-row">
            <div className="sec-row-label">2FA 验证码</div>
            <div className="sec-row-main">
              <Input
                value={disableForm.code}
                onChange={(v) => setDisableForm((p) => ({ ...p, code: v }))}
                maxLength={6}
                placeholder="请输入 6 位验证码"
              />
            </div>
          </div>
          <div className="sec-row">
            <div className="sec-row-label" />
            <div className="sec-row-main sec-actions">
              <Button type="danger" theme="light" loading={disableLoading} onClick={() => void handleDisable()}>
                关闭 2FA
              </Button>
            </div>
          </div>

          {/* 恢复码管理 */}
          <div className="sec-divider" />
          <Banner
            type={security?.has_recovery_codes ? 'success' : 'warning'}
            closeIcon={null}
            className="sec-banner"
            description={
              security?.has_recovery_codes
                ? '您有可用的恢复码，若 2FA 设备不可用可使用恢复码登录'
                : '暂无可用恢复码，建议生成新的恢复码以备用'
            }
          />
          <div className="sec-plain-tip">
            恢复码用于在 2FA 验证器不可用时登录。重新生成后旧恢复码将立即失效。
          </div>
          <div className="sec-row">
            <div className="sec-row-label">当前密码</div>
            <div className="sec-row-main">
              <Input
                mode="password"
                value={regenForm.password}
                onChange={(v) => setRegenForm((p) => ({ ...p, password: v }))}
                placeholder="请输入当前密码"
              />
            </div>
          </div>
          <div className="sec-row">
            <div className="sec-row-label">2FA 验证码</div>
            <div className="sec-row-main">
              <Input
                value={regenForm.code}
                onChange={(v) => setRegenForm((p) => ({ ...p, code: v }))}
                maxLength={6}
                placeholder="请输入 6 位验证码"
              />
            </div>
          </div>
          <div className="sec-row">
            <div className="sec-row-label" />
            <div className="sec-row-main sec-actions">
              <Button type="primary" theme="solid" loading={regenLoading} onClick={() => void handleRegen()}>
                重新生成恢复码
              </Button>
            </div>
          </div>
        </>
      ) : (
        <>
          <div className="sec-actions">
            <Button type="primary" theme="solid" loading={setupLoading} onClick={() => void handleGenerate()}>
              生成 2FA 配置
            </Button>
          </div>

          {secret && (
            <div className="sec-totp-panel">
              {qrCodeData && <img src={qrCodeData} alt="2FA 二维码" className="sec-qr-image" />}
              <div className="sec-totp-secret">密钥：{secret}</div>
              <div className="sec-plain-tip">
                请使用支持 TOTP 的验证器应用扫描二维码，输入 6 位动态验证码完成绑定。
              </div>
              <Input
                value={setupCode}
                onChange={setSetupCode}
                maxLength={6}
                placeholder="请输入 6 位验证码"
                style={{ maxWidth: 260 }}
              />
              <Button type="primary" theme="solid" loading={enableLoading} onClick={() => void handleEnable()}>
                启用 2FA
              </Button>
            </div>
          )}
        </>
      )}

      <RecoveryCodesModal
        visible={recoveryCodes.length > 0}
        codes={recoveryCodes}
        onClose={() => setRecoveryCodes([])}
      />
    </div>
  )
}
