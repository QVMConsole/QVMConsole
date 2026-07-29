/**
 * 系统设置页（仅管理员）
 * - 8 个 Tab：基础 / 存储与网络 / 宿主机 / 调度与高级 / 安全与维护 / 日志 / 诊断导出 / 存储管理
 * - 常规配置保存后立即生效并持久化到数据库；宿主机级选项写入系统配置文件
 * - 支持 ?tab=xxx 直接定位（如虚拟机表单空状态跳转到"存储与网络"）
 */
import { useCallback, useEffect, useMemo, useState } from 'react'
import { useSearchParams } from 'react-router'
import { Button, Spin, Tabs, Toast } from '@douyinfe/semi-ui'
import {
  IconArticle,
  IconClockStroked,
  IconDesktop,
  IconLock,
  IconPulse,
  IconRefresh,
  IconServer,
  IconSetting,
  IconShield,
  IconTick,
  IconFolder,
} from '@douyinfe/semi-icons'
import { getSettings, updateSettings } from '@/api/settings'
import { useUserStore } from '@/stores/user'
import { useAppStore } from '@/stores/app'
import { ROLES } from '@/config/constants'
import {
  DEFAULT_MAINTENANCE_SERVICE_UNITS,
  DEFAULT_SETTINGS_FORM,
  VALID_SETTINGS_TABS,
  buildSettingsPayload,
  validateSettingsForm,
  type SettingsForm,
  type SettingsTabKey,
} from './types'
import BasicTab from './components/BasicTab'
import StorageNetworkTab from './components/StorageNetworkTab'
import HostTab from './components/HostTab'
import AdvancedTab from './components/AdvancedTab'
import SecurityTab from './components/SecurityTab'
import LogTab from './components/LogTab'
import DiagnosticsTab from './components/DiagnosticsTab'
import StorageMaintainTab from './components/StorageMaintainTab'
import './settings.css'

export default function SettingsPage() {
  const role = useUserStore((s) => s.role)
  const isAdmin = role === ROLES.admin

  const [searchParams, setSearchParams] = useSearchParams()
  const initialTab = useMemo<SettingsTabKey>(() => {
    const q = searchParams.get('tab') as SettingsTabKey | null
    return q && VALID_SETTINGS_TABS.includes(q) ? q : 'basic'
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [])
  const [activeTab, setActiveTab] = useState<SettingsTabKey>(initialTab)

  const [form, setForm] = useState<SettingsForm>(DEFAULT_SETTINGS_FORM)
  const [loading, setLoading] = useState(false)
  const [saving, setSaving] = useState(false)

  /** 局部更新表单字段 */
  const patch = useCallback((partial: Partial<SettingsForm>) => {
    setForm((prev) => ({ ...prev, ...partial }))
  }, [])

  // ==================== 数据加载 ====================
  const fetchData = useCallback(async () => {
    setLoading(true)
    try {
      const res = await getSettings()
      const data = (res.data || {}) as Partial<SettingsForm>
      setForm((prev) => {
        const next = { ...DEFAULT_SETTINGS_FORM, ...prev, ...data }
        // 密码字段不回显；维护服务列表空值兜底
        next.smtp_password = ''
        if (!next.maintenance_service_units?.trim()) {
          next.maintenance_service_units = DEFAULT_MAINTENANCE_SERVICE_UNITS
        }
        return next
      })
      if (data.site_title) {
        useAppStore.getState().setSiteTitle(String(data.site_title))
      }
    } catch {
      // 请求层已统一提示
    } finally {
      setLoading(false)
    }
  }, [])

  useEffect(() => {
    if (isAdmin) void fetchData()
  }, [isAdmin, fetchData])

  // ==================== 保存 ====================
  const handleSave = useCallback(async () => {
    const error = validateSettingsForm(form)
    if (error) {
      Toast.error(error)
      return
    }
    setSaving(true)
    try {
      const res = await updateSettings(buildSettingsPayload(form))
      useAppStore.getState().setSiteTitle(form.site_title)
      useAppStore.getState().setPublicFlags({
        password_breach_check_enabled: form.password_breach_check_enabled,
        spice_enabled_by_default: form.spice_enabled_by_default,
      })
      Toast.success(res.message || '设置已保存')
      await fetchData()
    } catch {
      // 请求层已统一提示（428 高风险验证由请求层自动处理）
    } finally {
      setSaving(false)
    }
  }, [form, fetchData])

  /** 保存当前配置（供测试发信前静默保存复用） */
  const saveBeforeAction = useCallback(async (): Promise<boolean> => {
    const error = validateSettingsForm(form)
    if (error) {
      Toast.error(error)
      return false
    }
    try {
      await updateSettings(buildSettingsPayload(form))
      return true
    } catch {
      return false
    }
  }, [form])

  const handleTabChange = useCallback(
    (key: string) => {
      setActiveTab(key as SettingsTabKey)
      setSearchParams({ tab: key }, { replace: true })
    },
    [setSearchParams],
  )

  // ==================== 渲染 ====================
  if (!isAdmin) {
    return (
      <div className="stg-page">
        <div className="stg-empty">
          <div className="stg-empty-icon">
            <IconLock />
          </div>
          <div>系统设置仅对管理员开放</div>
        </div>
      </div>
    )
  }

  return (
    <div className="stg-page">
      <div className="stg-page-header qvm-fade-up">
        <div>
          <h2>
            <IconSetting style={{ marginRight: 8, color: 'var(--qvm-acc-ink)' }} />
            系统设置
          </h2>
          <p className="stg-page-sub">
            常规配置保存后立即生效并持久化到数据库（重启保留）；宿主机级兼容性选项会写入系统配置文件，若配置了环境变量则环境变量优先
          </p>
        </div>
      </div>

      <div className="stg-section-card qvm-fade-up">
        <Spin spinning={loading}>
          <Tabs
            type="line"
            activeKey={activeTab}
            onChange={handleTabChange}
            lazyRender
            keepDOM
          >
            <Tabs.TabPane tab="基础设置" itemKey="basic" icon={<IconSetting />}>
              <BasicTab form={form} patch={patch} />
            </Tabs.TabPane>
            <Tabs.TabPane tab="存储与网络" itemKey="network" icon={<IconServer />}>
              <StorageNetworkTab form={form} patch={patch} />
            </Tabs.TabPane>
            <Tabs.TabPane tab="宿主机设置" itemKey="host" icon={<IconDesktop />}>
              <HostTab form={form} patch={patch} />
            </Tabs.TabPane>
            <Tabs.TabPane tab="调度与高级" itemKey="advanced" icon={<IconClockStroked />}>
              <AdvancedTab form={form} patch={patch} />
            </Tabs.TabPane>
            <Tabs.TabPane tab="安全与维护" itemKey="security" icon={<IconShield />}>
              <SecurityTab
                form={form}
                patch={patch}
                saveBeforeAction={saveBeforeAction}
                refresh={fetchData}
              />
            </Tabs.TabPane>
            <Tabs.TabPane tab="日志管理" itemKey="log" icon={<IconArticle />}>
              <LogTab form={form} patch={patch} />
            </Tabs.TabPane>
            <Tabs.TabPane tab="诊断导出" itemKey="diagnostics" icon={<IconPulse />}>
              <DiagnosticsTab />
            </Tabs.TabPane>
            <Tabs.TabPane tab="存储管理" itemKey="storage" icon={<IconFolder />}>
              <StorageMaintainTab />
            </Tabs.TabPane>
          </Tabs>

          {/* 日志/诊断/存储管理 Tab 为独立操作区，无需整体保存 */}
          {!['diagnostics', 'storage'].includes(activeTab) && (
            <div className="stg-footer">
              <Button
                type="primary"
                theme="solid"
                icon={<IconTick />}
                loading={saving}
                onClick={() => void handleSave()}
              >
                保存设置
              </Button>
              <Button icon={<IconRefresh />} onClick={() => void fetchData()}>
                重置
              </Button>
            </div>
          )}
        </Spin>
      </div>
    </div>
  )
}
