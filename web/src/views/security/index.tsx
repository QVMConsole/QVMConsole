/**
 * 个人安全中心页
 * - 迁移自旧前端 layout/index.vue 的"安全设置"对话框（用户头像下拉入口）
 * - 5 个 Tab：邮箱绑定 / 两步验证（2FA + 恢复码）/ API 凭证 / 修改密码 / 修改用户名
 * - 支持 ?tab=xxx 直接定位（email / totp / api / password / username）
 */
import { useCallback, useEffect, useMemo, useState } from 'react'
import { useSearchParams } from 'react-router'
import { Tabs } from '@douyinfe/semi-ui'
import { IconKeyStroked, IconLockStroked, IconMailStroked, IconSafeStroked, IconUser } from '@douyinfe/semi-icons'
import { getUserInfo } from '@/api/auth'
import { useUserStore } from '@/stores/user'
import { CLOUD_TYPES, type CloudType } from '@/config/constants'
import EmailSection from './components/EmailSection'
import TotpSection from './components/TotpSection'
import ApiKeySection from './components/ApiKeySection'
import PasswordSection from './components/PasswordSection'
import UsernameSection from './components/UsernameSection'
import './security.css'

const VALID_TABS = ['email', 'totp', 'api', 'password', 'username'] as const
type SecurityTabKey = (typeof VALID_TABS)[number]

export default function SecurityPage() {
  const [searchParams, setSearchParams] = useSearchParams()
  const initialTab = useMemo<SecurityTabKey>(() => {
    const q = searchParams.get('tab') as SecurityTabKey | null
    return q && (VALID_TABS as readonly string[]).includes(q) ? q : 'email'
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [])
  const [activeTab, setActiveTab] = useState<SecurityTabKey>(initialTab)

  /** 刷新当前用户安全状态（绑定邮箱 / 开关 2FA / 重新生成恢复码后调用） */
  const refreshSecurity = useCallback(async () => {
    try {
      const res = await getUserInfo()
      const { username, role, security, cloud_type } = res.data
      useUserStore
        .getState()
        .setUserInfo(username, role, security || null, (cloud_type || CLOUD_TYPES.elastic) as CloudType)
    } catch {
      // 请求层已统一提示
    }
  }, [])

  useEffect(() => {
    void refreshSecurity()
  }, [refreshSecurity])

  const handleTabChange = useCallback(
    (key: string) => {
      setActiveTab(key as SecurityTabKey)
      setSearchParams({ tab: key }, { replace: true })
    },
    [setSearchParams],
  )

  return (
    <div className="sec-page">
      <div className="sec-page-header qvm-fade-up">
        <div>
          <h2>
            <IconSafeStroked style={{ marginRight: 8, color: 'var(--qvm-acc-ink)' }} />
            安全中心
          </h2>
          <p className="sec-page-sub">
            管理账户的登录邮箱、两步验证（2FA）、登录密码与用户名。涉及敏感操作时会要求二次验证，请确保验证器可用。
          </p>
        </div>
      </div>

      <div className="sec-section-card qvm-fade-up">
        <Tabs type="line" activeKey={activeTab} onChange={handleTabChange} lazyRender keepDOM>
          <Tabs.TabPane tab="邮箱绑定" itemKey="email" icon={<IconMailStroked />}>
            <EmailSection refreshSecurity={refreshSecurity} />
          </Tabs.TabPane>
          <Tabs.TabPane tab="两步验证" itemKey="totp" icon={<IconKeyStroked />}>
            <TotpSection refreshSecurity={refreshSecurity} />
          </Tabs.TabPane>
          <Tabs.TabPane tab="API 凭证" itemKey="api" icon={<IconKeyStroked />}>
            <ApiKeySection />
          </Tabs.TabPane>
          <Tabs.TabPane tab="修改密码" itemKey="password" icon={<IconLockStroked />}>
            <PasswordSection />
          </Tabs.TabPane>
          <Tabs.TabPane tab="修改用户名" itemKey="username" icon={<IconUser />}>
            <UsernameSection />
          </Tabs.TabPane>
        </Tabs>
      </div>
    </div>
  )
}
