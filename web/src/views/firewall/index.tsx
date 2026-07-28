/**
 * 防火墙管理页（仅管理员）
 * - 三个 Tab：宿主机防火墙（UFW）/ KVM 网络防火墙（nftables）/ 连接管理
 * - 宿主机规则与 KVM 策略变更均为高风险操作（428 二次验证由请求层处理）
 * - 应用/禁用/回滚/启用等耗时操作走任务队列，提交后延迟刷新状态
 */
import { useCallback, useEffect, useMemo, useState } from 'react'
import { Button, Tabs, Toast } from '@douyinfe/semi-ui'
import {
  IconBranch,
  IconDesktop,
  IconLink,
  IconLock,
  IconRefresh,
  IconShield,
} from '@douyinfe/semi-icons'
import {
  addHostFirewallVNCDefaultRule,
  applyFirewallPolicy,
  closeHostFirewallConnections,
  deleteHostFirewallRule,
  disableFirewall,
  disableHostFirewall,
  getFirewallStatus,
  getHostFirewallStatus,
  previewEnableHostFirewall,
  previewFirewallPolicy,
  previewHostFirewallConnections,
  rollbackFirewall,
  saveFirewallPolicy,
  updateFirewallGeoIP,
  type FirewallPolicy,
  type FirewallStatus,
  type FirewallVmOverride,
  type HostFirewallConnectionPreview,
  type HostFirewallRule,
  type HostFirewallStatus,
} from '@/api/firewall'
import { useUserStore } from '@/stores/user'
import { confirmModal } from '@/utils/confirm'
import { ROLES } from '@/config/constants'
import {
  createDefaultPolicy,
  extractVmNames,
  formatRulePort,
  normalizeVmOverrides,
} from './utils'
import HostFirewallTab from './components/HostFirewallTab'
import KvmFirewallTab from './components/KvmFirewallTab'
import ConnectionsTab from './components/ConnectionsTab'
import PreviewModal from '@/components/common/PreviewModal'
import EnableHostFirewallDialog from './dialogs/EnableHostFirewallDialog'
import HostRuleDialog from './dialogs/HostRuleDialog'
import ImportRegionDialog from './dialogs/ImportRegionDialog'
import './firewall.css'

/** 弹窗状态 */
type DialogState =
  | { type: 'enableHost'; rules: HostFirewallRule[] }
  | { type: 'rule'; row?: HostFirewallRule }
  | { type: 'import' }
  | { type: 'preview'; rules: string }
  | null

/** 任务队列类操作提交后的延迟刷新（ms） */
const TASK_REFRESH_DELAY = 1200

export default function FirewallPage() {
  const role = useUserStore((s) => s.role)
  const isAdmin = role === ROLES.admin

  const [activeTab, setActiveTab] = useState('host')
  const [loading, setLoading] = useState(false)
  const [saving, setSaving] = useState(false)
  const [applying, setApplying] = useState(false)
  const [enablePreviewLoading, setEnablePreviewLoading] = useState(false)
  const [dialog, setDialog] = useState<DialogState>(null)

  // 宿主机防火墙
  const [hostStatus, setHostStatus] = useState<HostFirewallStatus | null>(null)
  // KVM 网络防火墙
  const [kvmStatus, setKvmStatus] = useState<FirewallStatus | null>(null)
  const [policy, setPolicy] = useState<FirewallPolicy>(createDefaultPolicy())
  const [whitelistText, setWhitelistText] = useState('')
  const [geoCodesText, setGeoCodesText] = useState('cn')
  // 连接管理
  const [connectionPreview, setConnectionPreview] =
    useState<HostFirewallConnectionPreview | null>(null)
  const [previewingMode, setPreviewingMode] = useState<string | null>(null)

  // ==================== 数据加载 ====================
  const loadKvmStatus = useCallback(async () => {
    const res = await getFirewallStatus()
    const status = res.data || null
    setKvmStatus(status)
    if (status?.policy) {
      const vmNames = extractVmNames(status)
      const merged = { ...createDefaultPolicy(), ...status.policy }
      merged.vm_overrides = normalizeVmOverrides(status.policy.vm_overrides, vmNames)
      setPolicy(merged)
      setWhitelistText((status.policy.whitelist_cidrs || []).join('\n'))
      setGeoCodesText('')
    }
  }, [])

  const loadAll = useCallback(
    async (tab: string = activeTab) => {
      setLoading(true)
      try {
        const jobs: Promise<unknown>[] = [
          getHostFirewallStatus().then((res) => setHostStatus(res.data || null)),
        ]
        if (tab === 'kvm') {
          jobs.push(loadKvmStatus())
        }
        await Promise.all(jobs)
      } catch {
        // 请求层已提示
      } finally {
        setLoading(false)
      }
    },
    [activeTab, loadKvmStatus],
  )

  useEffect(() => {
    if (isAdmin) void loadAll()
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [isAdmin])

  /** 切换 Tab：首次进入 KVM Tab 时加载策略与状态（显式传 tab，避免闭包读到旧的 activeTab） */
  const switchTab = useCallback(
    (key: string) => {
      setActiveTab(key)
      if (key === 'kvm' && !kvmStatus) {
        void loadAll('kvm')
      }
    },
    [kvmStatus, loadAll],
  )

  /** 任务提交后延迟刷新（等待任务队列应用） */
  const refreshAfterTask = useCallback(() => {
    window.setTimeout(() => void loadAll(), TASK_REFRESH_DELAY)
  }, [loadAll])

  // ==================== 宿主机防火墙操作 ====================
  const handleOpenEnable = useCallback(async () => {
    setEnablePreviewLoading(true)
    try {
      const res = await previewEnableHostFirewall()
      // 推荐规则（SSH/面板保护 + 端口转发放通）优先，兜底当前规则
      const rules = res.data?.recommended_rules?.length
        ? res.data.recommended_rules
        : res.data?.rules || []
      setDialog({ type: 'enableHost', rules })
    } catch {
      // 请求层已提示
    } finally {
      setEnablePreviewLoading(false)
    }
  }, [])

  const handleDisableHost = useCallback(async () => {
    const ok = await confirmModal({
      title: '高风险操作',
      content: '确认关闭宿主机防火墙？关闭后端口转发不会再自动写入 UFW 放通。',
      okText: '确认关闭',
      danger: true,
    })
    if (!ok) return
    try {
      const res = await disableHostFirewall()
      Toast.success(res.message || '宿主机防火墙关闭任务已提交')
      refreshAfterTask()
    } catch {
      // 请求层已提示
    }
  }, [refreshAfterTask])

  const handleDeleteHostRule = useCallback(
    async (row: HostFirewallRule) => {
      const ok = await confirmModal({
        title: '高风险操作',
        content: `确认删除 ${formatRulePort(row)}/${row.protocol} 规则？`,
        okText: '确认删除',
        danger: true,
      })
      if (!ok) return
      try {
        await deleteHostFirewallRule(row.id)
        Toast.success('规则已删除')
        void loadAll()
      } catch {
        // 请求层已提示
      }
    },
    [loadAll],
  )

  const handleAddVncDefault = useCallback(async () => {
    const ok = await confirmModal({
      title: '确认操作',
      content: '确认添加 VNC 默认端口范围 5900-5999/tcp？该规则不是保护规则，后续可编辑或删除。',
      okText: '确认添加',
    })
    if (!ok) return
    try {
      await addHostFirewallVNCDefaultRule()
      Toast.success('VNC 默认端口范围已添加')
      void loadAll()
    } catch {
      // 请求层已提示
    }
  }, [loadAll])

  // ==================== KVM 网络防火墙操作 ====================
  const handlePolicyChange = useCallback((patch: Partial<FirewallPolicy>) => {
    setPolicy((p) => ({ ...p, ...patch }))
  }, [])

  const handleVmOverrideChange = useCallback(
    (name: string, patch: Partial<FirewallVmOverride>) => {
      setPolicy((p) => {
        const current: FirewallVmOverride = p.vm_overrides[name] || {
          mode: 'inherit',
          regions: [],
        }
        return {
          ...p,
          vm_overrides: { ...p.vm_overrides, [name]: { ...current, ...patch } },
        }
      })
    },
    [],
  )

  /** 组装提交载荷：白名单文本拆分为数组 */
  const buildPayload = useCallback((): FirewallPolicy => {
    return {
      ...policy,
      whitelist_cidrs: whitelistText
        .split('\n')
        .map((line) => line.trim())
        .filter(Boolean),
    }
  }, [policy, whitelistText])

  const handleSave = useCallback(async () => {
    setSaving(true)
    try {
      await saveFirewallPolicy(buildPayload())
      Toast.success('KVM 网络防火墙策略已保存')
      await loadKvmStatus()
    } catch {
      // 请求层已提示
    } finally {
      setSaving(false)
    }
  }, [buildPayload, loadKvmStatus])

  const handlePreview = useCallback(async () => {
    try {
      const res = await previewFirewallPolicy(buildPayload())
      setDialog({ type: 'preview', rules: res.data?.rules || '' })
    } catch {
      // 请求层已提示
    }
  }, [buildPayload])

  const handleApply = useCallback(async () => {
    const ok = await confirmModal({
      title: '高风险操作',
      content: '应用后会立即影响 KVM 虚拟机入站/出站转发流量，确认继续？',
      okText: '确认应用',
      danger: true,
    })
    if (!ok) return
    setApplying(true)
    try {
      const res = await applyFirewallPolicy(buildPayload())
      Toast.success(res.message || 'KVM 网络防火墙应用任务已提交')
      refreshAfterTask()
    } catch {
      // 请求层已提示
    } finally {
      setApplying(false)
    }
  }, [buildPayload, refreshAfterTask])

  const handleDisable = useCallback(async () => {
    const ok = await confirmModal({
      title: '高风险操作',
      content: '确认禁用 KVM 网络防火墙并删除独立 nft 表？',
      okText: '确认禁用',
      danger: true,
    })
    if (!ok) return
    try {
      const res = await disableFirewall()
      Toast.success(res.message || 'KVM 网络防火墙禁用任务已提交')
      refreshAfterTask()
    } catch {
      // 请求层已提示
    }
  }, [refreshAfterTask])

  const handleRollback = useCallback(async () => {
    const ok = await confirmModal({
      title: '高风险操作',
      content: '回滚会删除独立 nft 表，恢复到未管控状态，确认继续？',
      okText: '确认回滚',
      danger: true,
    })
    if (!ok) return
    try {
      const res = await rollbackFirewall()
      Toast.success(res.message || 'KVM 网络防火墙回滚任务已提交')
      refreshAfterTask()
    } catch {
      // 请求层已提示
    }
  }, [refreshAfterTask])

  const handleGeoUpdate = useCallback(async () => {
    const codes = geoCodesText
      .split(/[,\s]+/)
      .map((v) => v.trim())
      .filter(Boolean)
    if (codes.length === 0) {
      Toast.warning('请输入需要更新的区域代码')
      return
    }
    try {
      const res = await updateFirewallGeoIP({ codes, base_url: policy.geoip_base_url })
      Toast.success(res.message || 'GeoIP 更新任务已提交')
      refreshAfterTask()
    } catch {
      // 请求层已提示
    }
  }, [geoCodesText, policy.geoip_base_url, refreshAfterTask])

  // ==================== 连接管理操作 ====================
  const handlePreviewConnections = useCallback(async (mode: 'non_firewall' | 'all') => {
    setPreviewingMode(mode)
    try {
      const res = await previewHostFirewallConnections(mode)
      setConnectionPreview(res.data || null)
    } catch {
      // 请求层已提示
    } finally {
      setPreviewingMode(null)
    }
  }, [])

  const handleCloseConnections = useCallback(
    async (mode: 'non_firewall' | 'all') => {
      let count = 0
      try {
        const res = await previewHostFirewallConnections(mode)
        setConnectionPreview(res.data || null)
        count = res.data?.count || 0
      } catch {
        return // 请求层已提示
      }
      const message =
        mode === 'all'
          ? `将关闭全部 ${count} 个连接，包括 SSH 和面板连接，当前会话可能立即断开。确认继续？`
          : `将关闭 ${count} 个非防火墙端口连接，确认继续？`
      const ok = await confirmModal({
        title: '高风险操作',
        content: message,
        okText: '确认关闭',
        danger: true,
      })
      if (!ok) return
      try {
        await closeHostFirewallConnections({ mode })
        Toast.success('连接关闭命令已执行')
      } catch {
        // 请求层已提示
      }
    },
    [],
  )

  // ==================== 渲染 ====================
  const vmNames = useMemo(() => extractVmNames(kvmStatus), [kvmStatus])

  if (!isAdmin) {
    return (
      <div className="fw-page">
        <div className="fw-empty">
          <div className="fw-empty-icon">
            <IconLock />
          </div>
          <div>防火墙管理仅对管理员开放</div>
        </div>
      </div>
    )
  }

  return (
    <div className="fw-page">
      <div className="fw-page-header qvm-fade-up">
        <div>
          <h2>
            <IconShield style={{ marginRight: 8, color: 'var(--qvm-acc-ink)' }} />
            防火墙
          </h2>
          <p className="fw-page-sub">管理宿主机入站防火墙、KVM 转发流量防火墙与当前连接清理</p>
        </div>
        <div className="fw-header-actions">
          <Button icon={<IconRefresh />} loading={loading} onClick={() => void loadAll()}>
            刷新
          </Button>
        </div>
      </div>

      <Tabs activeKey={activeTab} onChange={switchTab} type="line" className="qvm-fade-up">
        <Tabs.TabPane tab="宿主机防火墙" itemKey="host" icon={<IconDesktop />}>
          <HostFirewallTab
            hostStatus={hostStatus}
            loading={loading}
            enableLoading={enablePreviewLoading}
            onEnable={() => void handleOpenEnable()}
            onDisable={() => void handleDisableHost()}
            onAddVncDefault={() => void handleAddVncDefault()}
            onEditRule={(row) => setDialog({ type: 'rule', row })}
            onDeleteRule={(row) => void handleDeleteHostRule(row)}
          />
        </Tabs.TabPane>
        <Tabs.TabPane tab="KVM 网络防火墙" itemKey="kvm" icon={<IconBranch />}>
          <KvmFirewallTab
            status={kvmStatus}
            policy={policy}
            onPolicyChange={handlePolicyChange}
            whitelistText={whitelistText}
            onWhitelistTextChange={setWhitelistText}
            geoCodesText={geoCodesText}
            onGeoCodesTextChange={setGeoCodesText}
            vmNames={vmNames}
            onVmOverrideChange={handleVmOverrideChange}
            saving={saving}
            applying={applying}
            onPreview={() => void handlePreview()}
            onSave={() => void handleSave()}
            onApply={() => void handleApply()}
            onDisable={() => void handleDisable()}
            onRollback={() => void handleRollback()}
            onImport={() => setDialog({ type: 'import' })}
            onGeoUpdate={() => void handleGeoUpdate()}
          />
        </Tabs.TabPane>
        <Tabs.TabPane tab="连接管理" itemKey="connections" icon={<IconLink />}>
          <ConnectionsTab
            preview={connectionPreview}
            previewing={previewingMode}
            onPreview={(mode) => void handlePreviewConnections(mode)}
            onCloseConnections={(mode) => void handleCloseConnections(mode)}
          />
        </Tabs.TabPane>
      </Tabs>

      {/* ==================== 弹窗 ==================== */}
      {dialog?.type === 'enableHost' && (
        <EnableHostFirewallDialog
          rules={dialog.rules}
          onClose={() => setDialog(null)}
          onEnabled={() => {
            refreshAfterTask()
          }}
        />
      )}
      {dialog?.type === 'rule' && (
        <HostRuleDialog
          row={dialog.row}
          onClose={() => setDialog(null)}
          onSaved={() => {
            void loadAll()
          }}
        />
      )}
      {dialog?.type === 'import' && (
        <ImportRegionDialog
          onClose={() => setDialog(null)}
          onSaved={() => {
            void loadKvmStatus()
          }}
        />
      )}
      {dialog?.type === 'preview' && (
        <PreviewModal
          title="nftables 规则预览"
          onClose={() => setDialog(null)}
          width={820}
        >
          <pre className="fw-preview-code">{dialog.rules || '（无规则内容）'}</pre>
        </PreviewModal>
      )}
    </div>
  )
}
