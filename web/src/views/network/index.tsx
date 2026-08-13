/**
 * 网络中心页（深空极光版）
 * - 管理员 4 个 Tab：网络概览 / 交换机 / 安全组策略 / ACL
 * - 普通用户（弹性云）2 个 Tab：交换机 / 安全组策略
 * - 数据按需加载：Tab 切换时加载对应数据；管理员支持按用户筛选
 * - 旧版端口转发（建站扫描）Tab 已随功能移除，不再迁移
 */
import { useCallback, useEffect, useMemo, useState } from 'react'
import { Button, Input, Tabs, Toast } from '@douyinfe/semi-ui'
import {
  IconBranch,
  IconCheckList,
  IconGlobeStroke,
  IconLock,
  IconRefresh,
  IconSearch,
} from '@douyinfe/semi-icons'
import {
  checkOVSNetwork,
  disablePortSecurity,
  disablePortMirror,
  getPortMirrorOptions,
  getPortMirrorStatus,
  enablePortSecurity,
  getPortSecurityStatus,
  isolatePortSecurityPort,
  preflightPortSecurity,
  reconcilePortSecurity,
  releasePortSecurityPort,
  repairOVSNetwork,
  type OvsPortList,
  type OvsStatus,
  type PortSecurityPreflight,
  type PortSecurityStatus,
  type PortMirrorOptions,
  type PortMirrorStatus,
} from '@/api/ovs'
import {
  deleteNetworkBridge,
  getHostInterfaces,
  getNetworkBridges,
  type HostInterface,
  type NetworkBridge,
} from '@/api/network'
import {
  applyVPCACL,
  deleteVPCSecurityGroup,
  deleteVPCSecurityGroupRule,
  deleteVPCSwitch,
  getVPCQuota,
  getVPCSecurityGroups,
  getVPCSwitches,
  getVPCSwitchVMs,
  previewVPCACL,
  reconfigureVPCSwitch,
  resetVPCSwitchTraffic,
  type VpcQuota,
  type VpcSecurityGroup,
  type VpcSecurityGroupRule,
  type VpcSwitch,
} from '@/api/vpc'
import { useUserStore } from '@/stores/user'
import { getTaskDetail } from '@/api/task'
import { confirmModal } from '@/utils/confirm'
import { copyTextWithFallback } from '@/utils/clipboard'
import { ROLES } from '@/config/constants'
import OverviewTab from './components/OverviewTab'
import SwitchesTab from './components/SwitchesTab'
import SecurityGroupsTab from './components/SecurityGroupsTab'
import AclTab from './components/AclTab'
import SwitchDialog from './dialogs/SwitchDialog'
import SwitchVMsDialog from './dialogs/SwitchVMsDialog'
import InterfaceConfigDialog from './dialogs/InterfaceConfigDialog'
import SecurityGroupDialog from './dialogs/SecurityGroupDialog'
import RuleDialog from './dialogs/RuleDialog'
import PortMirrorDialog from './dialogs/PortMirrorDialog'
import './network.css'

/** 弹窗状态 */
type DialogState =
  | { type: 'switch'; row?: VpcSwitch }
  | { type: 'switchVMs'; row: VpcSwitch }
  | { type: 'ifaceConfig'; name: string }
  | { type: 'group'; row?: VpcSecurityGroup }
  | { type: 'rule'; group: VpcSecurityGroup }
  | { type: 'portMirror'; options: PortMirrorOptions }
  | null

export default function NetworkPage() {
  const role = useUserStore((s) => s.role)
  const isAdmin = role === ROLES.admin

  const [activeTab, setActiveTab] = useState(isAdmin ? 'overview' : 'switches')
  const [usernameFilter, setUsernameFilter] = useState('')
  const [loading, setLoading] = useState(false)
  const [dialog, setDialog] = useState<DialogState>(null)
  const [operatingSwitchIds, setOperatingSwitchIds] = useState<Set<number>>(() => new Set())

  // 交换机 / 安全组
  const [switches, setSwitches] = useState<VpcSwitch[]>([])
  const [quota, setQuota] = useState<VpcQuota | null>(null)
  const [securityGroups, setSecurityGroups] = useState<VpcSecurityGroup[]>([])
  // 概览
  const [ovsStatus, setOvsStatus] = useState<OvsStatus | null>(null)
  const [ovsPorts, setOvsPorts] = useState<OvsPortList | null>(null)
  const [bridges, setBridges] = useState<NetworkBridge[]>([])
  const [hostInterfaces, setHostInterfaces] = useState<HostInterface[]>([])
  const [checking, setChecking] = useState(false)
  const [repairing, setRepairing] = useState(false)
  const [portSecurity, setPortSecurity] = useState<PortSecurityStatus | null>(null)
  const [portSecurityPreflight, setPortSecurityPreflight] = useState<PortSecurityPreflight | null>(null)
  const [portSecurityAction, setPortSecurityAction] = useState('')
  const [portMirror, setPortMirror] = useState<PortMirrorStatus | null>(null)
  const [portMirrorAction, setPortMirrorAction] = useState('')
  const [portMirrorLoading, setPortMirrorLoading] = useState(true)
  // ACL
  const [aclPreview, setAclPreview] = useState('')
  const [aclLoading, setAclLoading] = useState(false)
  const [aclApplying, setAclApplying] = useState(false)

  /** 管理员按用户筛选的查询参数 */
  const queryParams = useMemo(
    () => (isAdmin && usernameFilter ? { username: usernameFilter } : undefined),
    [isAdmin, usernameFilter],
  )

  // ==================== 数据加载 ====================
  const loadSwitches = useCallback(async () => {
    const [switchRes, quotaRes, bridgeRes, ifaceRes] = await Promise.all([
      getVPCSwitches(queryParams),
      getVPCQuota(queryParams),
      isAdmin ? getNetworkBridges() : Promise.resolve(null),
      isAdmin ? getHostInterfaces() : Promise.resolve(null),
    ])
    setSwitches(switchRes.data || [])
    setQuota(quotaRes.data || null)
    if (isAdmin && bridgeRes) setBridges(bridgeRes.data || [])
    if (isAdmin && ifaceRes) setHostInterfaces(ifaceRes.data || [])
  }, [queryParams, isAdmin])

  const loadSecurityGroups = useCallback(async () => {
    const res = await getVPCSecurityGroups(queryParams)
    setSecurityGroups(res.data || [])
  }, [queryParams])

  const loadPortMirror = useCallback(async () => {
    setPortMirrorLoading(true)
    setPortMirror(null)
    try {
      const response = await getPortMirrorStatus()
      setPortMirror(response.data || null)
    } catch {
      // 请求层已提示，保留空状态以阻止在运行态未知时提交操作。
    } finally {
      setPortMirrorLoading(false)
    }
  }, [])

  const loadOverview = useCallback(async () => {
    const [checkRes, bridgeRes, ifaceRes, portSecurityRes] = await Promise.all([
      checkOVSNetwork(),
      getNetworkBridges(),
      getHostInterfaces(),
      getPortSecurityStatus(),
      loadPortMirror(),
    ])
    setOvsStatus(checkRes.data?.status || null)
    setOvsPorts(checkRes.data?.ports || null)
    setBridges(bridgeRes.data || [])
    setHostInterfaces(ifaceRes.data || [])
    setPortSecurity(portSecurityRes.data || null)
  }, [loadPortMirror])

  const loadACLPreview = useCallback(async () => {
    if (!isAdmin) return
    setAclLoading(true)
    try {
      const res = await previewVPCACL()
      setAclPreview(res.data || '')
    } catch {
      setAclPreview('')
    } finally {
      setAclLoading(false)
    }
  }, [isAdmin])

  const loadAll = useCallback(async () => {
    setLoading(true)
    try {
      const jobs: Promise<void>[] = [loadSwitches(), loadSecurityGroups()]
      if (isAdmin) {
        if (activeTab === 'overview') jobs.push(loadOverview())
        if (activeTab === 'acl') jobs.push(loadACLPreview())
      }
      await Promise.all(jobs)
    } finally {
      setLoading(false)
    }
  }, [isAdmin, activeTab, loadSwitches, loadSecurityGroups, loadOverview, loadACLPreview])

  useEffect(() => {
    void loadAll()
    // 仅在挂载与用户筛选变化时全量加载
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [queryParams])

  const switchTab = useCallback(
    (key: string) => {
      setActiveTab(key)
      if (key === 'acl') void loadACLPreview()
      if (key === 'overview') void loadOverview()
    },
    [loadACLPreview, loadOverview],
  )

  // ==================== 概览操作 ====================
  const handleCheck = useCallback(async () => {
    setChecking(true)
    try {
      const res = await checkOVSNetwork()
      setOvsStatus(res.data?.status || null)
      setOvsPorts(res.data?.ports || null)
      Toast.success(res.data?.healthy ? '网络检测通过' : '网络检测完成')
    } catch {
      // 请求层已提示
    } finally {
      setChecking(false)
    }
  }, [])

  const handleRepair = useCallback(async () => {
    const ok = await confirmModal({
      title: '高风险操作',
      content: '修复会补齐 OVS 网桥、dnsmasq、ip_forward、NAT 和 FORWARD 规则，确认继续？',
      okText: '确认修复',
      danger: true,
    })
    if (!ok) return
    setRepairing(true)
    try {
      const res = await repairOVSNetwork()
      Toast.success(res.message || '修复任务已提交')
    } catch {
      // 请求层已提示
    } finally {
      setRepairing(false)
    }
  }, [])

  const handleDeleteBridge = useCallback(
    async (row: NetworkBridge) => {
      const ok = await confirmModal({
        title: '删除网桥',
        content: `确定删除网桥 ${row.name}？`,
        okText: '删除',
        danger: true,
      })
      if (!ok) return
      try {
        await deleteNetworkBridge(row.id, row.name)
        Toast.success('网桥已删除')
        void loadOverview()
      } catch {
        // 请求层已提示
      }
    },
    [loadOverview],
  )

  const handlePortSecurityPreflight = useCallback(async () => {
    setPortSecurityAction('preflight')
    try {
      const res = await preflightPortSecurity()
      const data = res.data
      setPortSecurityPreflight(data ? {
        ...data,
        capabilities: data.capabilities ?? [],
        ports: data.ports ?? [],
        issues: data.issues ?? [],
      } : null)
      if (res.data?.ready) Toast.success('端口安全预检通过')
      else Toast.warning('预检完成，请先处理阻断项')
      return !!res.data?.ready
    } catch {
      return false
    } finally {
      setPortSecurityAction('')
    }
  }, [])

  const waitPortSecurityTask = useCallback(
    async (taskId?: number) => {
      if (!taskId) return
      for (;;) {
        await new Promise((resolve) => window.setTimeout(resolve, 1200))
        const res = await getTaskDetail(taskId)
        const task = res.data
        if (!task || !['success', 'failed', 'canceled'].includes(task.status)) continue
        if (task.status === 'success') Toast.success(task.message || '端口安全任务执行完成')
        else Toast.error(task.message || '端口安全任务执行失败')
        await loadOverview()
        return
      }
    },
    [loadOverview],
  )

  const handlePortSecurityToggle = useCallback(
    async (enabled: boolean) => {
      if (enabled) {
        const ready = await handlePortSecurityPreflight()
        if (!ready) return
      }
      const ok = await confirmModal({
        title: enabled ? '启用端口安全防护' : '停用端口安全防护',
        content: enabled
          ? '系统将先隔离活动端口，再安装并校验身份、ARP/ND 与速率策略。确认提交启用任务？'
          : '系统将移除本模块流表并恢复原有转发；异常端口会继续隔离。确认提交停用任务？',
        okText: enabled ? '启用' : '停用',
        danger: true,
      })
      if (!ok) return
      setPortSecurityAction(enabled ? 'enable' : 'disable')
      try {
        const res = enabled ? await enablePortSecurity() : await disablePortSecurity()
        Toast.success(res.message || '端口安全任务已提交')
        await waitPortSecurityTask(res.data?.task_id)
      } finally {
        setPortSecurityAction('')
      }
    },
    [handlePortSecurityPreflight, waitPortSecurityTask],
  )

  const handlePortSecurityReconcile = useCallback(async () => {
    setPortSecurityAction('reconcile')
    try {
      const res = await reconcilePortSecurity()
      Toast.success(res.message || '协调任务已提交')
      await waitPortSecurityTask(res.data?.task_id)
    } finally {
      setPortSecurityAction('')
    }
  }, [waitPortSecurityTask])

  const handlePortSecurityPortAction = useCallback(async (port: string, release: boolean) => {
    const ok = await confirmModal({
      title: release ? '释放端口' : '隔离端口',
      content: `确定${release ? '释放' : '隔离'} OVS 端口 ${port}？`,
      okText: release ? '释放' : '隔离',
      danger: true,
    })
    if (!ok) return
    setPortSecurityAction(`${release ? 'release' : 'isolate'}:${port}`)
    try {
      const res = release
        ? await releasePortSecurityPort(port)
        : await isolatePortSecurityPort(port)
      Toast.success(res.message || '端口任务已提交')
      await waitPortSecurityTask(res.data?.task_id)
    } finally {
      setPortSecurityAction('')
    }
  }, [waitPortSecurityTask])

  const waitPortMirrorTask = useCallback(
    async (taskId: number) => {
      try {
        for (;;) {
          await new Promise((resolve) => window.setTimeout(resolve, 1200))
          const response = await getTaskDetail(taskId)
          const task = response.data
          if (!task || !['success', 'failed', 'canceled'].includes(task.status)) continue
          if (task.status === 'success') Toast.success(task.message || '端口镜像任务执行完成')
          else Toast.error(task.message || '端口镜像任务执行失败，运行态已回滚')
          await loadOverview()
          return
        }
      } finally {
        setPortMirrorAction('')
      }
    },
    [loadOverview],
  )

  const handlePortMirrorConfig = useCallback(async () => {
    try {
      const response = await getPortMirrorOptions()
      setDialog({
        type: 'portMirror',
        options: { sources: response.data?.sources || [], targets: response.data?.targets || [] },
      })
    } catch {
      // 请求层已提示
    }
  }, [])

  const handlePortMirrorSubmitted = useCallback(
    (taskId: number) => {
      setPortMirrorAction('enable')
      void waitPortMirrorTask(taskId)
    },
    [waitPortMirrorTask],
  )

  const handlePortMirrorDisable = useCallback(async () => {
    const ok = await confirmModal({
      title: '停用端口镜像',
      content: '将删除本功能创建的 tc 规则、veth、OVS 端口和专用流表，确认继续？',
      okText: '停用',
      danger: true,
    })
    if (!ok) return
    setPortMirrorAction('disable')
    try {
      const response = await disablePortMirror()
      Toast.success(response.message || '端口镜像停用任务已提交')
      if (response.data?.task_id) await waitPortMirrorTask(response.data.task_id)
    } catch {
      setPortMirrorAction('')
    }
  }, [waitPortMirrorTask])

  // ==================== 交换机操作 ====================
  const monitorSwitchTask = useCallback(
    async (switchId: number, taskId: number) => {
      setOperatingSwitchIds((current) => new Set(current).add(switchId))
      try {
        for (;;) {
          await new Promise((resolve) => window.setTimeout(resolve, 1200))
          const response = await getTaskDetail(taskId)
          const task = response.data
          if (!task || !['success', 'failed', 'canceled'].includes(task.status)) continue
          if (task.status === 'success') Toast.success(task.message || '交换机重配置完成')
          else Toast.error(task.message || '交换机重配置失败，旧网络已恢复')
          await loadSwitches()
          return
        }
      } finally {
        setOperatingSwitchIds((current) => {
          const next = new Set(current)
          next.delete(switchId)
          return next
        })
      }
    },
    [loadSwitches],
  )

  const handleMigrateLegacySwitch = useCallback(
    async (row: VpcSwitch) => {
      let vmNames: string[] = []
      try {
        const response = await getVPCSwitchVMs(row.id)
        vmNames = (response.data || []).map((item) => item.vm_name)
      } catch {
        // 预览失败时仍由后端任务重新读取绑定清单。
      }
      const ok = await confirmModal({
        title: '迁移为空交换机',
        content: vmNames.length > 0
          ? `将停止旧 DHCP/NAT，并把 ${vmNames.length} 台虚拟机的全部关联网口迁移到独立二层交换机：${vmNames.join('、')}。确认提交？`
          : '将停止此历史交换机的 DHCP/NAT，并迁移为独立纯二层交换机。确认提交？',
        okText: '提交迁移',
        danger: true,
      })
      if (!ok) return
      try {
        const response = await reconfigureVPCSwitch(row.id, {
          name: row.name,
          uplink_mode: 'none',
          uplink_if: '',
          dhcp_enabled: false,
          migrate_host_ip: false,
          bridge_vlan_id: 0,
        })
        const taskId = response.data?.task_id
        if (taskId) void monitorSwitchTask(row.id, taskId)
        Toast.success(response.message || '交换机迁移任务已提交')
      } catch {
        // 请求层已提示
      }
    },
    [monitorSwitchTask],
  )

  const handleDeleteSwitch = useCallback(
    async (row: VpcSwitch) => {
      // 先查询该交换机下是否仍有虚拟机绑定
      let vms: { vm_name: string }[] = []
      try {
        const res = await getVPCSwitchVMs(row.id)
        vms = res.data || []
      } catch {
        // 查询失败也允许继续尝试删除
      }
      if (vms.length > 0) {
        const vmNames = vms.map((v) => v.vm_name).join('、')
        const ok = await confirmModal({
          title: '强制删除交换机',
          content: `交换机「${row.name}」下仍有 ${vms.length} 台虚拟机绑定：${vmNames}。强制删除将会移除这些虚拟机的网卡，确定继续？`,
          okText: '强制删除',
          danger: true,
        })
        if (!ok) return
        try {
          await deleteVPCSwitch(row.id, true)
        } catch {
          return
        }
      } else {
        const ok = await confirmModal({
          title: '删除交换机',
          content: `确定删除交换机 ${row.name}？`,
          okText: '删除',
          danger: true,
        })
        if (!ok) return
        try {
          await deleteVPCSwitch(row.id, false)
        } catch {
          return
        }
      }
      Toast.success('交换机已删除')
      void loadSwitches()
    },
    [loadSwitches],
  )

  const handleResetSwitchTraffic = useCallback(
    async (row: VpcSwitch) => {
      const ok = await confirmModal({
        title: '重置流量计数器',
        content: `确定将交换机 ${row.name} 的本月流量计数值重置？若当前因超限被强制限速，会立即解除。`,
        okText: '重置',
      })
      if (!ok) return
      try {
        await resetVPCSwitchTraffic(row.id)
        Toast.success('交换机流量计数器已重置')
        void loadSwitches()
      } catch {
        // 请求层已提示
      }
    },
    [loadSwitches],
  )

  // ==================== 安全组操作 ====================
  const handleDeleteGroup = useCallback(
    async (row: VpcSecurityGroup) => {
      const ok = await confirmModal({
        title: '删除安全组',
        content: `确定删除安全组 ${row.name}？`,
        okText: '删除',
        danger: true,
      })
      if (!ok) return
      try {
        await deleteVPCSecurityGroup(row.id)
        Toast.success('安全组已删除')
        void loadSecurityGroups()
      } catch {
        // 请求层已提示
      }
    },
    [loadSecurityGroups],
  )

  const handleDeleteRule = useCallback(
    async (rule: VpcSecurityGroupRule) => {
      try {
        await deleteVPCSecurityGroupRule(rule.id)
        Toast.success('规则已删除')
        void loadSecurityGroups()
      } catch {
        // 请求层已提示
      }
    },
    [loadSecurityGroups],
  )

  // ==================== ACL 操作 ====================
  const handleApplyACL = useCallback(async () => {
    const ok = await confirmModal({
      title: '高风险操作',
      content: '应用 ACL 会重建 VPC 防火墙规则，确认继续？',
      okText: '确认应用',
      danger: true,
    })
    if (!ok) return
    setAclApplying(true)
    try {
      const res = await applyVPCACL()
      Toast.success(res.message || 'ACL 已应用')
      void loadACLPreview()
    } catch {
      // 请求层已提示
    } finally {
      setAclApplying(false)
    }
  }, [loadACLPreview])

  const handleCopyACL = useCallback(async () => {
    if (!aclPreview) return
    try {
      await copyTextWithFallback(aclPreview)
      Toast.success('ACL 规则已复制到剪贴板')
    } catch {
      Toast.warning('复制失败，请手动复制')
    }
  }, [aclPreview])

  // ==================== 渲染 ====================
  return (
    <div className="net-page">
      <div className="net-page-header qvm-fade-up">
        <div>
          <h2>
            <IconBranch style={{ marginRight: 8, color: 'var(--qvm-acc-ink)' }} />
            {isAdmin ? '网络中心' : 'VPC 网络'}
          </h2>
          <p className="net-page-sub">
            {isAdmin
              ? '管理 OVS 基础网络、交换机、安全组策略与 ACL'
              : '管理交换机、安全组策略与月流量配额'}
          </p>
        </div>
        <div className="net-header-actions">
          {isAdmin && (
            <Input
              prefix={<IconSearch />}
              placeholder="按用户筛选"
              value={usernameFilter}
              onChange={(v) => setUsernameFilter(v)}
              showClear
              style={{ width: 200 }}
            />
          )}
          <Button icon={<IconRefresh />} loading={loading} onClick={() => void loadAll()}>
            刷新
          </Button>
        </div>
      </div>

      <Tabs activeKey={activeTab} onChange={switchTab} type="line" className="qvm-fade-up">
        {isAdmin && (
          <Tabs.TabPane
            tab="网络概览"
            itemKey="overview"
            icon={<IconGlobeStroke />}
          >
            <OverviewTab
              status={ovsStatus}
              ports={ovsPorts}
              bridges={bridges}
              hostInterfaces={hostInterfaces}
              checking={checking}
              repairing={repairing}
              portSecurity={portSecurity}
              portSecurityPreflight={portSecurityPreflight}
              portSecurityAction={portSecurityAction}
              portMirror={portMirror}
              portMirrorAction={portMirrorAction}
              portMirrorLoading={portMirrorLoading}
              onCheck={() => void handleCheck()}
              onRepair={() => void handleRepair()}
              onDeleteBridge={(row) => void handleDeleteBridge(row)}
              onConfigInterface={(name) => setDialog({ type: 'ifaceConfig', name })}
              onPortSecurityPreflight={() => void handlePortSecurityPreflight()}
              onPortSecurityToggle={(enabled) => void handlePortSecurityToggle(enabled)}
              onPortSecurityReconcile={() => void handlePortSecurityReconcile()}
              onPortSecurityPortAction={(port, release) => void handlePortSecurityPortAction(port, release)}
              onPortMirrorConfig={() => void handlePortMirrorConfig()}
              onPortMirrorDisable={() => void handlePortMirrorDisable()}
            />
          </Tabs.TabPane>
        )}
        <Tabs.TabPane tab="交换机" itemKey="switches" icon={<IconBranch />}>
          <SwitchesTab
            isAdmin={isAdmin}
            switches={switches}
            quota={quota}
            loading={loading}
            onCreate={() => setDialog({ type: 'switch' })}
            onEdit={(row) => setDialog({ type: 'switch', row })}
            onDelete={(row) => void handleDeleteSwitch(row)}
            onResetTraffic={(row) => void handleResetSwitchTraffic(row)}
            onViewVMs={(row) => setDialog({ type: 'switchVMs', row })}
            onMigrate={(row) => void handleMigrateLegacySwitch(row)}
            operatingSwitchIds={operatingSwitchIds}
          />
        </Tabs.TabPane>
        <Tabs.TabPane tab="安全组策略" itemKey="securityGroups" icon={<IconLock />}>
          <SecurityGroupsTab
            isAdmin={isAdmin}
            groups={securityGroups}
            loading={loading}
            onCreate={() => setDialog({ type: 'group' })}
            onEdit={(row) => setDialog({ type: 'group', row })}
            onDelete={(row) => void handleDeleteGroup(row)}
            onAddRule={(group) => setDialog({ type: 'rule', group })}
            onDeleteRule={(rule) => void handleDeleteRule(rule)}
          />
        </Tabs.TabPane>
        {isAdmin && (
          <Tabs.TabPane tab="ACL" itemKey="acl" icon={<IconCheckList />}>
            <AclTab
              preview={aclPreview}
              loading={aclLoading}
              applying={aclApplying}
              onRefresh={() => void loadACLPreview()}
              onApply={() => void handleApplyACL()}
              onCopy={() => void handleCopyACL()}
            />
          </Tabs.TabPane>
        )}
      </Tabs>

      {/* ==================== 弹窗 ==================== */}
      {dialog?.type === 'switch' && (
        <SwitchDialog
          row={dialog.row}
          isAdmin={isAdmin}
          hostInterfaces={hostInterfaces}
          quota={quota}
          defaultUsername={usernameFilter}
          portSecurityEnabled={!!portSecurity?.enabled}
          onClose={() => setDialog(null)}
          onSaved={() => {
            void loadSwitches()
          }}
          onTaskSubmitted={(switchId, taskId) => void monitorSwitchTask(switchId, taskId)}
        />
      )}
      {dialog?.type === 'switchVMs' && (
        <SwitchVMsDialog row={dialog.row} onClose={() => setDialog(null)} />
      )}
      {dialog?.type === 'ifaceConfig' && (
        <InterfaceConfigDialog
          name={dialog.name}
          onClose={() => setDialog(null)}
          onSaved={() => {
            void loadOverview()
          }}
        />
      )}
      {dialog?.type === 'group' && (
        <SecurityGroupDialog
          row={dialog.row}
          isAdmin={isAdmin}
          defaultUsername={usernameFilter}
          onClose={() => setDialog(null)}
          onSaved={() => {
            void loadSecurityGroups()
          }}
        />
      )}
      {dialog?.type === 'rule' && (
        <RuleDialog
          group={dialog.group}
          switches={switches}
          securityGroups={securityGroups}
          onClose={() => setDialog(null)}
          onSaved={() => {
            void loadSecurityGroups()
          }}
        />
      )}
      {dialog?.type === 'portMirror' && (
        <PortMirrorDialog
          options={dialog.options}
          status={portMirror}
          onClose={() => setDialog(null)}
          onSubmitted={handlePortMirrorSubmitted}
        />
      )}
    </div>
  )
}
