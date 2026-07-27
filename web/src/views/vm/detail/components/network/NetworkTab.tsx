/**
 * 网络管理 Tab 容器
 * - 子面板：端口转发 / 静态 IP / 运行状态（管理员）/ 网络诊断（管理员）
 * - 统一加载共享数据：VPC 绑定、配额、静态绑定与 DHCP 租约、运行状态、手动 IP
 * - 桥接直通交换机自动隐藏端口转发与静态 IP 面板
 */
import { useCallback, useEffect, useMemo, useState } from 'react'
import { Tabs, TabPane } from '@douyinfe/semi-ui'
import type { VmDetailInfo, VmNetworkStatus, QuotaUsage } from '@/api/vm'
import { getVMNetworkStatus, getSelfQuota } from '@/api/vm'
import {
  getPortForwardIPs,
  getStaticIPList,
  type DhcpLease,
  type PortForwardIPMapping,
  type StaticIPBinding,
} from '@/api/network'
import { getVMVPCBinding, type VpcBindingInfo } from '@/api/vpc'
import { useUserStore } from '@/stores/user'
import { CLOUD_TYPES, ROLES } from '@/config/constants'
import PortForwardPanel from './PortForwardPanel'
import StaticIpPanel from './StaticIpPanel'
import RuntimePanel from './RuntimePanel'
import DiagnosticsPanel from './DiagnosticsPanel'

interface NetworkTabProps {
  vm: VmDetailInfo | null
}

/** 共享数据（各子面板复用） */
export interface NetworkSharedData {
  vpcInfo: VpcBindingInfo | null
  selfQuota: QuotaUsage | null
  staticBindings: StaticIPBinding[]
  dhcpLeases: DhcpLease[]
  runtimeStatus: VmNetworkStatus | null
  manualIPs: PortForwardIPMapping[]
  isAdmin: boolean
  isLightweight: boolean
  /** 当前 VM 是否存在 NAT 交换机（桥接直通为 false） */
  hasNatSwitch: boolean
  /** 当前 VM 是否为轻量云 VM */
  isLightweightVM: boolean
  refreshStaticIPs: () => Promise<void>
  refreshRuntimeStatus: () => Promise<void>
  refreshVPCBinding: () => Promise<void>
  refreshSelfQuota: () => Promise<void>
  refreshManualIPs: () => Promise<void>
}

export default function NetworkTab({ vm }: NetworkTabProps) {
  const role = useUserStore((s) => s.role)
  const cloudType = useUserStore((s) => s.cloudType)
  const isAdmin = role === ROLES.admin
  const isLightweight = !isAdmin && cloudType === CLOUD_TYPES.lightweight

  const vmName = vm?.name || ''
  const [vpcInfo, setVpcInfo] = useState<VpcBindingInfo | null>(null)
  const [selfQuota, setSelfQuota] = useState<QuotaUsage | null>(null)
  const [staticBindings, setStaticBindings] = useState<StaticIPBinding[]>([])
  const [dhcpLeases, setDhcpLeases] = useState<DhcpLease[]>([])
  const [runtimeStatus, setRuntimeStatus] = useState<VmNetworkStatus | null>(null)
  const [manualIPs, setManualIPs] = useState<PortForwardIPMapping[]>([])

  // ============ 数据加载 ============
  const refreshVPCBinding = useCallback(async () => {
    if (!vmName) return
    try {
      const res = await getVMVPCBinding(vmName)
      setVpcInfo(res.data || null)
    } catch {
      setVpcInfo(null)
    }
  }, [vmName])

  const refreshSelfQuota = useCallback(async () => {
    if (isAdmin || isLightweight) return
    try {
      const res = await getSelfQuota()
      setSelfQuota(res.data || null)
    } catch {
      setSelfQuota(null)
    }
  }, [isAdmin, isLightweight])

  const refreshStaticIPs = useCallback(async () => {
    try {
      const res = await getStaticIPList()
      setStaticBindings(res.data?.static_bindings || [])
      setDhcpLeases(res.data?.dhcp_leases || [])
    } catch {
      // 静默失败
    }
  }, [])

  const refreshRuntimeStatus = useCallback(async () => {
    if (!vmName) return
    try {
      const res = await getVMNetworkStatus(vmName)
      setRuntimeStatus(res.data || null)
    } catch {
      setRuntimeStatus(null)
    }
  }, [vmName])

  const refreshManualIPs = useCallback(async () => {
    if (!vmName) return
    try {
      const res = await getPortForwardIPs(vmName)
      setManualIPs(res.data || [])
    } catch {
      setManualIPs([])
    }
  }, [vmName])

  useEffect(() => {
    void refreshVPCBinding()
    void refreshSelfQuota()
    void refreshStaticIPs()
    void refreshRuntimeStatus()
    void refreshManualIPs()
  }, [
    refreshVPCBinding,
    refreshSelfQuota,
    refreshStaticIPs,
    refreshRuntimeStatus,
    refreshManualIPs,
  ])

  // ============ 可见性计算 ============
  const currentSwitchIsBridge = vpcInfo?.switch?.bridge_mode === 'bridge'
  const hasNatSwitch = !currentSwitchIsBridge
  const isLightweightVM = !!vpcInfo?.lightweight_quota

  const portForwardTabVisible =
    hasNatSwitch &&
    (isAdmin || isLightweight || isLightweightVM || !!selfQuota?.enable_port_forward)
  const staticIpTabVisible = !isLightweight && hasNatSwitch

  const defaultTab = portForwardTabVisible
    ? 'forward'
    : staticIpTabVisible
      ? 'staticip'
      : isAdmin
        ? 'runtime'
        : 'forward'

  const [activeTab, setActiveTab] = useState(defaultTab)
  // 数据到达后校正默认面板
  useEffect(() => {
    setActiveTab((current) => {
      if (current === 'forward' && !portForwardTabVisible && defaultTab !== 'forward') return defaultTab
      if (current === 'staticip' && !staticIpTabVisible && defaultTab !== 'staticip') return defaultTab
      return current
    })
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [portForwardTabVisible, staticIpTabVisible, defaultTab])

  const shared: NetworkSharedData = useMemo(
    () => ({
      vpcInfo,
      selfQuota,
      staticBindings,
      dhcpLeases,
      runtimeStatus,
      manualIPs,
      isAdmin,
      isLightweight,
      hasNatSwitch,
      isLightweightVM,
      refreshStaticIPs,
      refreshRuntimeStatus,
      refreshVPCBinding,
      refreshSelfQuota,
      refreshManualIPs,
    }),
    [
      vpcInfo,
      selfQuota,
      staticBindings,
      dhcpLeases,
      runtimeStatus,
      manualIPs,
      isAdmin,
      isLightweight,
      hasNatSwitch,
      isLightweightVM,
      refreshStaticIPs,
      refreshRuntimeStatus,
      refreshVPCBinding,
      refreshSelfQuota,
      refreshManualIPs,
    ],
  )

  if (!vm) return <div className="qvm-tab-loading">加载中…</div>

  return (
    <div className="qvm-network-tab">
      <Tabs activeKey={activeTab} onChange={setActiveTab} type="button" size="small" lazyRender>
        {portForwardTabVisible && (
          <TabPane itemKey="forward" tab="端口转发">
            <PortForwardPanel vmName={vmName} shared={shared} />
          </TabPane>
        )}
        {staticIpTabVisible && (
          <TabPane itemKey="staticip" tab="静态 IP">
            <StaticIpPanel vmName={vmName} shared={shared} />
          </TabPane>
        )}
        {isAdmin && (
          <TabPane itemKey="runtime" tab="运行状态">
            <RuntimePanel shared={shared} />
          </TabPane>
        )}
        {isAdmin && (
          <TabPane itemKey="diagnostics" tab="网络诊断">
            <DiagnosticsPanel vmName={vmName} shared={shared} />
          </TabPane>
        )}
      </Tabs>
    </div>
  )
}
