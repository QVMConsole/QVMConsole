/**
 * 虚拟机详情页（深空极光版）
 * - SSE 实时推送全量详情（vm_detail 事件 + 速率增量计算）
 * - Hero 三卡布局：状态卡（电源/锁定/救援/重装）/ 资源卡（实时用量）/ VNC 预览截帧
 * - 功能标签页：系统信息 / 快照管理 / 网络管理 / 定时任务 / VNC / SPICE / 编辑（占位）
 * - 底部监控图表：实时监控 + 历史查询（近 24 小时），磁盘 IO 双单位
 */
import { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import { useLocation, useNavigate, useParams } from 'react-router'
import { Button, Tabs, TabPane, Tag, Toast, Tooltip } from '@douyinfe/semi-ui'
import {
  IconArrowLeft,
  IconChevronUp,
  IconInfoCircle,
  IconCamera,
  IconGlobeStroke,
  IconClock,
  IconDesktop,
  IconCameraStroked,
  IconEditStroked,
} from '@douyinfe/semi-icons'
import type { VmPowerAction, SnapshotQuota } from '@/api/vm'
import { lockVm, operateVm, rescueVm, unlockVm } from '@/api/vm'
import { ROLES, CLOUD_TYPES } from '@/config/constants'
import { useUserStore } from '@/stores/user'
import { usePageTabsStore } from '@/stores/pageTabs'
import { useVmStore } from '@/stores/vm'
import { useVmDetailSSE } from '@/hooks/useVmDetailSSE'
import { canResetVmPassword, openVncWindow, vmStatusDot, detailToListItem } from './utils'
import HeroStatusCard from './components/HeroStatusCard'
import HeroResourceCard from './components/HeroResourceCard'
import VncPreviewCard from './components/VncPreviewCard'
import InfoTab from './components/InfoTab'
import SnapshotTab from './components/SnapshotTab'
import ScheduleTab from './components/ScheduleTab'
import VncTab from './components/VncTab'
import SpiceTab from './components/SpiceTab'
import EditTab from './components/EditTab'
import MonitorCharts from './components/MonitorCharts'
import NetworkTab from './components/network/NetworkTab'
import ResetPasswordDialog from './dialogs/ResetPasswordDialog'
import VmRemarkDialog from '../dialogs/VmRemarkDialog'
import VmReinstallDialog from '../dialogs/VmReinstallDialog'
import './detail.css'

type DialogState = 'remark' | 'reinstall' | 'resetPassword' | null

export default function VmDetailPage() {
  const navigate = useNavigate()
  const location = useLocation()
  const params = useParams<{ id: string }>()
  const vmName = useMemo(() => params.id || '', [params.id])
  const role = useUserStore((s) => s.role)
  const cloudType = useUserStore((s) => s.cloudType)
  const isAdmin = role === ROLES.admin
  const isLightweight = !isAdmin && cloudType === CLOUD_TYPES.lightweight

  const { vmData, sseStatus, statusTick } = useVmDetailSSE(vmName)
  const [operating, setOperating] = useState(false)
  const [activeTab, setActiveTab] = useState('info')
  const [diskIoMode, setDiskIoMode] = useState<'iops' | 'throughput'>('throughput')
  const [dialog, setDialog] = useState<DialogState>(null)
  const [snapshotQuota, setSnapshotQuota] = useState<SnapshotQuota | null>(null)
  const [showBackToTop, setShowBackToTop] = useState(false)
  const topRef = useRef<HTMLDivElement>(null)

  // ============ 页面标签与最近访问 ============
  const openTab = usePageTabsStore((s) => s.openTab)
  const addVisitedVm = useVmStore((s) => s.addVisitedVm)
  useEffect(() => {
    if (vmData?.name) {
      openTab({ key: location.pathname, title: vmData.name, dot: vmStatusDot(vmData.status) })
      addVisitedVm({ id: vmData.name, name: vmData.name })
    }
  }, [vmData?.name, vmData?.status, location.pathname, openTab, addVisitedVm])

  // 状态变化时复位操作按钮 loading
  useEffect(() => {
    if (statusTick > 0) setOperating(false)
  }, [statusTick])

  // 视频设备被禁用时自动离开 VNC/SPICE 标签
  useEffect(() => {
    if (vmData?.video_model === 'none' && (activeTab === 'vnc' || activeTab === 'spice')) {
      setActiveTab('info')
    }
  }, [vmData?.video_model, activeTab])

  // 滚动监听
  useEffect(() => {
    const onScroll = () => setShowBackToTop(window.scrollY > 400)
    window.addEventListener('scroll', onScroll, { passive: true })
    return () => window.removeEventListener('scroll', onScroll)
  }, [])

  // ============ 电源操作 ============
  const handlePower = useCallback(
    async (action: VmPowerAction) => {
      if (!vmData || vmData.status === 'migrating') {
        Toast.warning('虚拟机正在迁移中，暂不能执行操作')
        return
      }
      setOperating(true)
      try {
        const res = await operateVm(vmName, action)
        const msgMap: Record<string, string> = {
          start: '开机',
          reboot: '重启',
          shutdown: '关机',
          destroy: '断电',
          reset: '重置',
        }
        const resStatus = (res as unknown as { status?: string }).status
        if (resStatus === 'no_change') {
          Toast.info(res.message || `${msgMap[action]}操作已执行，状态未改变`)
        } else {
          Toast.success(res.message || `${msgMap[action]}操作成功`)
        }
      } catch {
        setOperating(false)
      }
    },
    [vmData, vmName],
  )

  // ============ 锁定 / 解锁 ============
  const handleLockAction = useCallback(
    async (action: 'lock' | 'unlock') => {
      setOperating(true)
      try {
        if (action === 'lock') {
          await lockVm(vmName)
          Toast.success('虚拟机已锁定')
        } else {
          await unlockVm(vmName)
          Toast.success('虚拟机已解锁')
        }
      } catch {
        // 请求层已提示
      } finally {
        setOperating(false)
      }
    },
    [vmName],
  )

  // ============ 救援模式 ============
  const handleRescue = useCallback(async () => {
    if (!vmData) return
    setOperating(true)
    try {
      await rescueVm(vmName, vmData.in_rescue ? 'stop' : 'start')
      Toast.success(vmData.in_rescue ? '救援系统关闭任务已提交' : '救援系统启动任务已提交')
    } catch {
      setOperating(false)
    }
  }, [vmData, vmName])

  // ============ 弹窗 ============
  const handleReinstall = useCallback(() => {
    if (vmData?.status === 'migrating') {
      Toast.warning('虚拟机正在迁移中，暂不能执行重装')
      return
    }
    setDialog('reinstall')
  }, [vmData?.status])

  const handleRemark = useCallback(() => {
    if (vmData?.status === 'migrating') {
      Toast.warning('虚拟机正在迁移中，暂不能编辑备注')
      return
    }
    setDialog('remark')
  }, [vmData?.status])

  const handleResetPassword = useCallback(() => {
    if (!vmData) return
    if (!['linux', 'windows', 'fnos'].includes((vmData.os_type || '').toLowerCase())) {
      Toast.warning('当前仅支持 Linux、Windows 或 fnOS 虚拟机重置密码')
      return
    }
    if (!canResetVmPassword(vmData)) {
      const status = (vmData.status || '').trim().toLowerCase()
      if (status === 'running') {
        Toast.warning('QEMU Guest Agent 未连接，在线密码重置暂未就绪')
      } else {
        Toast.warning('当前状态不适合执行密码重置')
      }
      return
    }
    setDialog('resetPassword')
  }, [vmData])

  // ============ 其他 ============
  const toggleDiskIoMode = useCallback(
    () => setDiskIoMode((m) => (m === 'iops' ? 'throughput' : 'iops')),
    [],
  )
  const handleOpenVncWindow = useCallback(() => openVncWindow(vmName), [vmName])
  const handleGoBack = useCallback(() => navigate('/vm'), [navigate])
  const handleGotoVncTab = useCallback(() => setActiveTab('vnc'), [])
  const handleSnapshotQuotaChange = useCallback((q: SnapshotQuota | null) => setSnapshotQuota(q), [])

  const snapshotQuotaText = useMemo(() => {
    if (!snapshotQuota) return ''
    const used = snapshotQuota.used_snapshots || 0
    const max = snapshotQuota.max_snapshots || 0
    return max > 0 ? `${used}/${max}` : `${used}/不限`
  }, [snapshotQuota])

  const virtualDisplayEnabled = vmData?.video_model !== 'none'

  return (
    <div className="qvm-detail" ref={topRef}>
      {/* 顶部导航 */}
      <div className="qvm-detail-topline">
        <button type="button" className="qvm-back-link" onClick={handleGoBack}>
          <IconArrowLeft size="small" />
          返回虚拟机列表
        </button>
        <span className="qvm-detail-page-title">虚拟机详情</span>
        <div className="qvm-detail-topline-right">
          <Tooltip
            content={sseStatus === 'connected' ? '实时推送已连接' : '实时推送连接中…'}
            position="bottom"
          >
            <span className={`qvm-live-dot ${sseStatus === 'connected' ? 'on' : ''}`} />
          </Tooltip>
        </div>
      </div>

      {/* Hero 三卡 */}
      <section className="qvm-hero-grid">
        <div className="qvm-fade-up">
          <HeroStatusCard
            vm={vmData}
            operating={operating}
            isLightweight={isLightweight}
            onPower={(a) => void handlePower(a)}
            onLock={(a) => void handleLockAction(a)}
            onRescue={() => void handleRescue()}
            onReinstall={handleReinstall}
            onRemark={handleRemark}
          />
        </div>
        <div className="qvm-fade-up" style={{ '--qvm-delay': '60ms' } as React.CSSProperties}>
          <HeroResourceCard vm={vmData} diskIoMode={diskIoMode} onToggleDiskIoMode={toggleDiskIoMode} />
        </div>
        <div className="qvm-fade-up" style={{ '--qvm-delay': '120ms' } as React.CSSProperties}>
          <VncPreviewCard vm={vmData} onOpenWindow={handleOpenVncWindow} onGotoVncTab={handleGotoVncTab} />
        </div>
      </section>

      {/* 功能标签页 */}
      <section className="qvm-detail-tabs qvm-panel qvm-g-border qvm-fade-up" style={{ '--qvm-delay': '180ms' } as React.CSSProperties}>
        <Tabs activeKey={activeTab} onChange={setActiveTab} lazyRender type="card" collapsible="auto">
          <TabPane
            itemKey="info"
            tab={
              <span className="qvm-tab-label">
                <IconInfoCircle size="default" /> 系统信息
              </span>
            }
          >
            <InfoTab
              vm={vmData}
              isLightweight={isLightweight}
              onResetPassword={handleResetPassword}
              onReinstall={handleReinstall}
              onRemark={handleRemark}
            />
          </TabPane>
          <TabPane
            itemKey="snapshot"
            tab={
              <span className="qvm-tab-label">
                <IconCamera size="default" /> 快照管理
                {snapshotQuotaText && (
                  <Tag size="small" color="violet" className="qvm-tab-badge">
                    {snapshotQuotaText}
                  </Tag>
                )}
              </span>
            }
          >
            <SnapshotTab vm={vmData} onQuotaChange={handleSnapshotQuotaChange} />
          </TabPane>
          <TabPane
            itemKey="network"
            tab={
              <span className="qvm-tab-label">
                <IconGlobeStroke size="default" /> 网络管理
              </span>
            }
          >
            <NetworkTab vm={vmData} />
          </TabPane>
          <TabPane
            itemKey="schedule"
            tab={
              <span className="qvm-tab-label">
                <IconClock size="default" /> 定时任务
              </span>
            }
          >
            <ScheduleTab vm={vmData} />
          </TabPane>
          {virtualDisplayEnabled && (
            <TabPane
              itemKey="vnc"
              tab={
                <span className="qvm-tab-label">
                  <IconDesktop size="default" /> VNC 控制台
                </span>
              }
            >
              <VncTab vm={vmData} onOpenWindow={handleOpenVncWindow} />
            </TabPane>
          )}
          {virtualDisplayEnabled && (
            <TabPane
              itemKey="spice"
              tab={
                <span className="qvm-tab-label">
                  <IconCameraStroked size="default" /> SPICE 控制台
                </span>
              }
            >
              <SpiceTab vm={vmData} />
            </TabPane>
          )}
          {!isLightweight && vmData && (
            <TabPane
              itemKey="edit"
              tab={
                <span className="qvm-tab-label">
                  <IconEditStroked size="default" /> 编辑
                </span>
              }
            >
              <EditTab vm={vmData} />
            </TabPane>
          )}
        </Tabs>
      </section>

      {/* 监控图表 */}
      <section className="qvm-monitor-section qvm-fade-up" style={{ '--qvm-delay': '240ms' } as React.CSSProperties}>
        <MonitorCharts
          vmName={vmName}
          status={vmData?.status || ''}
          externalStats={vmData?.stats}
          diskIoMode={diskIoMode}
          onToggleDiskIoMode={toggleDiskIoMode}
        />
      </section>

      {/* 返回顶部 */}
      {showBackToTop && (
        <Button
          className="qvm-back-top"
          icon={<IconChevronUp />}
          theme="solid"
          type="primary"
          onClick={() => topRef.current?.scrollIntoView({ behavior: 'smooth' })}
        />
      )}

      {/* 弹窗 */}
      {dialog === 'remark' && vmData && (
        <VmRemarkDialog
          vm={detailToListItem(vmData)}
          onClose={() => setDialog(null)}
          onSuccess={() => undefined}
        />
      )}
      {dialog === 'reinstall' && vmData && (
        <VmReinstallDialog
          vm={detailToListItem(vmData)}
          onClose={() => setDialog(null)}
          onSuccess={() => undefined}
        />
      )}
      {dialog === 'resetPassword' && vmData && (
        <ResetPasswordDialog vm={vmData} onClose={() => setDialog(null)} />
      )}
    </div>
  )
}
