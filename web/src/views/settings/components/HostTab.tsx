/**
 * 宿主机设置 Tab：KSM 内存去重 / zRAM 压缩内存 / KVM 兼容性 / 硬件直通 / 网络等待就绪
 * 该 Tab 中 KSM / zRAM / KVM 参数为即时保存（独立于整体表单）
 */
import { useCallback, useEffect, useState } from 'react'
import { Banner, Modal, Tag, Toast } from '@douyinfe/semi-ui'
import { IconBolt, IconLink, IconServer } from '@douyinfe/semi-icons'
import TextSwitch from '@/features/vm-form/sections/TextSwitch'
import {
  getHardwarePassthroughStatus,
  getHostKSMStatus,
  getHostKVMUnrestrictedGuestStatus,
  getHostZRAMStatus,
  updateHostKSMProfile,
  updateHostKVMUnrestrictedGuest,
  updateHostZRAMProfile,
  type HardwarePassthroughStatus,
  type KSMStatus,
  type KVMUnrestrictedGuestStatus,
  type ZRAMStatus,
} from '@/api/settings'
import { confirmModal } from '@/utils/confirm'
import { SectionHead, SettingRow } from './SettingRow'
import HostProfilePanel from './HostProfilePanel'
import PassthroughSection from './PassthroughSection'
import {
  FALLBACK_KSM_PROFILES,
  FALLBACK_ZRAM_PROFILES,
  buildKsmSummary,
  buildKvmSummary,
  buildZramSummary,
  fmtBool,
  fmtMB,
  fmtNum,
  profileName,
} from '../hostUtils'
import { KSM_HELP, KVM_UNRESTRICTED_HELP, ZRAM_HELP } from '../helpContents'
import type { SettingsTabProps } from '../types'

export default function HostTab({ form, patch }: SettingsTabProps) {
  // KSM
  const [ksmStatus, setKsmStatus] = useState<KSMStatus | null>(null)
  const [ksmLoading, setKsmLoading] = useState(false)
  const [ksmSaving, setKsmSaving] = useState(false)
  const [ksmSelected, setKsmSelected] = useState('balanced')
  // zRAM
  const [zramStatus, setZramStatus] = useState<ZRAMStatus | null>(null)
  const [zramLoading, setZramLoading] = useState(false)
  const [zramSaving, setZramSaving] = useState(false)
  const [zramSelected, setZramSelected] = useState('balanced')
  // KVM Unrestricted Guest
  const [kvmStatus, setKvmStatus] = useState<KVMUnrestrictedGuestStatus | null>(null)
  const [kvmLoading, setKvmLoading] = useState(false)
  const [kvmSaving, setKvmSaving] = useState(false)
  const [kvmEnabled, setKvmEnabled] = useState(true)
  // 硬件直通
  const [hwStatus, setHwStatus] = useState<HardwarePassthroughStatus | null>(null)
  const [hwLoading, setHwLoading] = useState(false)

  const ksmOptions = ksmStatus?.profiles?.length ? ksmStatus.profiles : FALLBACK_KSM_PROFILES
  const zramOptions = zramStatus?.profiles?.length ? zramStatus.profiles : FALLBACK_ZRAM_PROFILES

  // ==================== 状态加载 ====================
  const applyKsmStatus = useCallback((status: KSMStatus | null) => {
    setKsmStatus(status)
    if (!status) return
    if (status.persistent_profile) {
      setKsmSelected(status.persistent_profile)
    } else if (status.current_profile && status.current_profile !== 'custom') {
      setKsmSelected(status.current_profile)
    }
  }, [])

  const applyZramStatus = useCallback((status: ZRAMStatus | null) => {
    setZramStatus(status)
    if (!status) return
    if (status.persistent_profile) {
      setZramSelected(status.persistent_profile)
    } else if (status.current_profile && status.current_profile !== 'custom') {
      setZramSelected(status.current_profile)
    }
  }, [])

  const applyKvmStatus = useCallback((status: KVMUnrestrictedGuestStatus | null) => {
    setKvmStatus(status)
    if (!status) return
    if (status.persistent_configured) {
      setKvmEnabled(!!status.persistent_enabled)
    } else if (status.runtime_available) {
      setKvmEnabled(!!status.runtime_enabled)
    }
  }, [])

  const loadHwStatus = useCallback(async () => {
    setHwLoading(true)
    try {
      const res = await getHardwarePassthroughStatus()
      setHwStatus(res.data || null)
    } catch {
      // 请求层已统一提示
    } finally {
      setHwLoading(false)
    }
  }, [])

  useEffect(() => {
    setKsmLoading(true)
    getHostKSMStatus()
      .then((res) => applyKsmStatus(res.data || null))
      .catch(() => {})
      .finally(() => setKsmLoading(false))
    setZramLoading(true)
    getHostZRAMStatus()
      .then((res) => applyZramStatus(res.data || null))
      .catch(() => {})
      .finally(() => setZramLoading(false))
    setKvmLoading(true)
    getHostKVMUnrestrictedGuestStatus()
      .then((res) => applyKvmStatus(res.data || null))
      .catch(() => {})
      .finally(() => setKvmLoading(false))
    void loadHwStatus()
  }, [applyKsmStatus, applyZramStatus, applyKvmStatus, loadHwStatus])

  // ==================== 挡位切换（即时保存 + 二次确认） ====================
  const handleKsmChange = async (profileKey: string) => {
    const previous = ksmStatus?.persistent_profile || ksmStatus?.current_profile || 'balanced'
    if (profileKey === ksmSelected) return
    setKsmSelected(profileKey)
    const name = profileName(ksmOptions, profileKey)
    const ok = await confirmModal({
      title: '设置宿主机 KSM',
      content: `确定要将宿主机 KSM 切换到“${name}”挡位吗？该配置会立即影响当前宿主机上的所有虚拟机。`,
      okText: '应用',
      danger: profileKey === 'off',
    })
    if (!ok) {
      setKsmSelected(previous)
      return
    }
    setKsmSaving(true)
    try {
      const res = await updateHostKSMProfile({ profile: profileKey })
      applyKsmStatus(res.data || null)
      Toast.success(res.message || 'KSM 挡位已保存')
    } catch {
      setKsmSelected(previous)
      getHostKSMStatus().then((res) => applyKsmStatus(res.data || null)).catch(() => {})
    } finally {
      setKsmSaving(false)
    }
  }

  const handleZramChange = async (profileKey: string) => {
    const previous = zramStatus?.persistent_profile || zramStatus?.current_profile || 'balanced'
    if (profileKey === zramSelected) return
    setZramSelected(profileKey)
    const name = profileName(zramOptions, profileKey)
    const ok = await confirmModal({
      title: '设置宿主机 zRAM',
      content: `确定要将宿主机 zRAM 切换到“${name}”挡位吗？该配置会立即重建面板管理的 zRAM swap，并影响当前宿主机的内存回收策略。`,
      okText: '应用',
      danger: profileKey === 'off',
    })
    if (!ok) {
      setZramSelected(previous)
      return
    }
    setZramSaving(true)
    try {
      const res = await updateHostZRAMProfile({ profile: profileKey })
      applyZramStatus(res.data || null)
      Toast.success(res.message || 'zRAM 挡位已保存')
    } catch {
      setZramSelected(previous)
      getHostZRAMStatus().then((res) => applyZramStatus(res.data || null)).catch(() => {})
    } finally {
      setZramSaving(false)
    }
  }

  const handleKvmChange = async (enabled: boolean) => {
    const previous = !enabled
    setKvmEnabled(enabled)
    const actionText = enabled ? '启用' : '禁用'
    const ok = await confirmModal({
      title: `${actionText}宿主机 KVM 参数`,
      content: enabled
        ? '确定要启用 KVM Unrestricted Guest 吗？这会恢复 Intel KVM 默认硬件辅助行为。'
        : '确定要禁用 KVM Unrestricted Guest 吗？该设置主要用于绕过 VMware 嵌套虚拟化下的 QEMU hardware error 0x7。',
      okText: actionText,
      danger: !enabled,
    })
    if (!ok) {
      setKvmEnabled(previous)
      return
    }
    setKvmSaving(true)
    try {
      const res = await updateHostKVMUnrestrictedGuest({ enabled })
      applyKvmStatus(res.data || null)
      Toast.success(res.message || 'KVM 参数已保存')
    } catch {
      setKvmEnabled(previous)
      getHostKVMUnrestrictedGuestStatus()
        .then((res) => applyKvmStatus(res.data || null))
        .catch(() => {})
    } finally {
      setKvmSaving(false)
    }
  }

  const showHelp = (title: string, paragraphs: string[]) => {
    Modal.info({
      title,
      content: (
        <div className="stg-help-content">
          {paragraphs.map((text, idx) => (
            <p key={idx}>{text}</p>
          ))}
        </div>
      ),
      okText: '我知道了',
      hasCancel: false,
      width: 560,
    })
  }

  // ==================== 渲染 ====================
  return (
    <div className="stg-tab-pane stg-tab-pane-wide">
      <SectionHead icon={<IconServer />} title="KSM 内存去重" />

      <Banner
        type="info"
        closeIcon={null}
        className="stg-banner"
        description="KSM 是宿主机级内存页去重能力，会影响当前宿主机上的所有虚拟机。挡位越高，扫描越积极，CPU 开销也越明显。"
      />

      <SettingRow label="KSM 挡位">
        <HostProfilePanel
          options={ksmOptions}
          selected={ksmSelected}
          disabled={ksmLoading || ksmSaving || !ksmStatus?.supported}
          enabled={!!ksmStatus?.enabled}
          persistentName={
            ksmStatus?.persistent_configured
              ? profileName(ksmOptions, ksmStatus.persistent_profile)
              : undefined
          }
          summary={buildKsmSummary(ksmStatus, ksmLoading, ksmOptions)}
          onChange={(key) => void handleKsmChange(key)}
          onHelp={() => showHelp('KSM 内存去重说明', KSM_HELP)}
        />
      </SettingRow>

      <SettingRow
        label="运行参数"
        tip="持久化文件: /etc/kvm-console/ksm.env，开机恢复服务: kvm-console-ksm.service"
      >
        <div className="stg-host-row">
          <Tag size="small">run: {fmtNum(ksmStatus?.runtime_config?.run)}</Tag>
          <Tag size="small">pages_to_scan: {fmtNum(ksmStatus?.runtime_config?.pages_to_scan)}</Tag>
          <Tag size="small">sleep: {fmtNum(ksmStatus?.runtime_config?.sleep_millisecs)}ms</Tag>
          <Tag size="small">
            NUMA 跨节点: {fmtBool(ksmStatus?.runtime_config?.merge_across_nodes)}
          </Tag>
          <Tag size="small">零页合并: {fmtBool(ksmStatus?.runtime_config?.use_zero_pages)}</Tag>
          <Tag size="small">智能扫描: {fmtBool(ksmStatus?.runtime_config?.smart_scan)}</Tag>
        </div>
      </SettingRow>

      <SettingRow label="去重统计">
        <div className="stg-host-row">
          <Tag size="small">共享页: {fmtNum(ksmStatus?.metrics?.pages_shared)}</Tag>
          <Tag size="small">被共享页: {fmtNum(ksmStatus?.metrics?.pages_sharing)}</Tag>
          <Tag size="small">未共享页: {fmtNum(ksmStatus?.metrics?.pages_unshared)}</Tag>
          <Tag size="small">扫描页: {fmtNum(ksmStatus?.metrics?.pages_scanned)}</Tag>
          <Tag size="small">完整扫描: {fmtNum(ksmStatus?.metrics?.full_scans)}</Tag>
        </div>
      </SettingRow>

      <SectionHead icon={<IconBolt />} title="zRAM 压缩内存" />

      <Banner
        type="info"
        closeIcon={null}
        className="stg-banner"
        description="zRAM 会在内存中创建压缩 swap，适合纯虚拟化宿主机作为内存压力缓冲。挡位越高，可用压缩空间越大，CPU 开销也越明显。"
      />

      <SettingRow label="zRAM 挡位">
        <HostProfilePanel
          options={zramOptions}
          selected={zramSelected}
          disabled={zramLoading || zramSaving || !zramStatus?.supported}
          enabled={!!zramStatus?.enabled}
          persistentName={
            zramStatus?.persistent_configured
              ? profileName(zramOptions, zramStatus.persistent_profile)
              : undefined
          }
          summary={buildZramSummary(zramStatus, zramLoading, zramOptions)}
          onChange={(key) => void handleZramChange(key)}
          onHelp={() => showHelp('zRAM 压缩内存说明', ZRAM_HELP)}
        />
      </SettingRow>

      <SettingRow
        label="zRAM 运行参数"
        tip="持久化文件: /etc/kvm-console/zram.env，开机恢复服务: kvm-console-zram.service"
      >
        <div className="stg-host-row">
          <Tag size="small">设备: {zramStatus?.runtime_config?.device || '-'}</Tag>
          <Tag size="small">容量: {fmtMB(zramStatus?.runtime_config?.size_mb)}</Tag>
          <Tag size="small">已用: {fmtMB(zramStatus?.runtime_config?.used_mb)}</Tag>
          <Tag size="small">算法: {zramStatus?.runtime_config?.algorithm || '-'}</Tag>
          <Tag size="small">优先级: {fmtNum(zramStatus?.runtime_config?.priority)}</Tag>
        </div>
      </SettingRow>

      <SectionHead icon={<IconServer />} title="虚拟化兼容性" />

      <Banner
        type="warning"
        closeIcon={null}
        className="stg-banner"
        description="这里是宿主机级 KVM 参数，会影响当前宿主机上的所有 Intel KVM 虚拟机。普通情况下请保持默认。"
      />

      <SettingRow label="KVM Unrestricted Guest" tip={buildKvmSummary(kvmStatus, kvmLoading)}>
        <div className="stg-host-row">
          <TextSwitch
            checked={kvmEnabled}
            onChange={(v) => void handleKvmChange(v)}
            checkedText="开"
            uncheckedText="关"
            disabled={kvmLoading || kvmSaving || !kvmStatus?.supported}
          />
          {kvmStatus?.runtime_available && (
            <Tag size="small" color={kvmStatus.runtime_enabled ? 'green' : 'orange'}>
              运行时：{kvmStatus.runtime_enabled ? '已启用' : '已禁用'}
            </Tag>
          )}
          {kvmStatus?.persistent_configured && (
            <Tag size="small" color="cyan">
              持久配置：{kvmStatus.persistent_enabled ? '启用' : '禁用'}
            </Tag>
          )}
          {kvmStatus?.requires_reload && (
            <Tag size="small" color="orange">
              待重载
            </Tag>
          )}
          <span
            className="stg-help-link"
            onClick={() => showHelp('KVM Unrestricted Guest 说明', KVM_UNRESTRICTED_HELP)}
          >
            说明
          </span>
        </div>
      </SettingRow>

      <PassthroughSection
        status={hwStatus}
        loading={hwLoading}
        enabled={form.hardware_passthrough_enabled}
        onEnabledChange={(v) => patch({ hardware_passthrough_enabled: v })}
        reload={loadHwStatus}
      />

      <SectionHead icon={<IconLink />} title="网络等待就绪检测" />

      <Banner
        type="info"
        closeIcon={null}
        className="stg-banner"
        description="systemd-networkd-wait-online.service 在 OVS 桥接环境中可能导致开机卡住。禁用后会执行 systemctl disable + mask，系统开机不再等待网络就绪。"
      />

      <SettingRow label="禁用网络等待就绪" tip={form.network_wait_online_summary || '加载中...'}>
        <TextSwitch
          checked={form.network_wait_online_disabled}
          onChange={(v) => patch({ network_wait_online_disabled: v })}
          checkedText="禁"
          uncheckedText="开"
        />
      </SettingRow>
    </div>
  )
}
