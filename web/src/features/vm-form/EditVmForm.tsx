/**
 * 虚拟机编辑表单（详情页「编辑」标签页）
 * 与创建向导（CreateVmWizard）共用 features/vm-form 的模型、规则与分区组件；
 * 编辑采用差异快照提交：仅发送与加载时快照相比发生变化的字段。
 */
import { useCallback, useEffect, useRef, useState } from 'react'
import { Button, Input, TabPane, Tabs, Tag, Toast } from '@douyinfe/semi-ui'
import {
  IconGlobe,
  IconInfoCircle,
  IconPuzzle,
  IconServer,
  IconSetting,
} from '@douyinfe/semi-icons'
import { DiskIcon } from './icons'
import { useUserStore } from '@/stores/user'
import { ROLES } from '@/config/constants'
import {
  disableSpice,
  enableSpice,
  getSpiceStatus,
  getVmDetail,
  getVmPassthroughDevices,
  updateVm,
  type VmDetailInfo,
} from '@/api/vm'
import { VmFormProvider } from './scope'
import { buildEditFormState, useVmForm } from './useVmForm'
import { useVmFormOptions } from './useVmFormOptions'
import { useVmEditDevices } from './useVmEditDevices'
import {
  buildCPUAffinityPayload,
  buildEditPayload,
  captureEditDiskIopsSnapshot,
  captureEditFormSnapshot,
  getEffectiveSpiceEnabled,
} from './payload'
import { createDefaultVmForm } from './defaults'
import type { EditDiskIopsSnapshot, EditFormSnapshot } from './types'
import FormField from './sections/FormField'
import SectionCard from './sections/SectionCard'
import CpuMemorySection from './sections/CpuMemorySection'
import DiskManageSection from './sections/DiskManageSection'
import NicSection from './sections/NicSection'
import NicManageSection from './sections/NicManageSection'
import VirtEngineSection from './sections/VirtEngineSection'
import BootOrderSection from './sections/BootOrderSection'
import SystemBehaviorSection from './sections/SystemBehaviorSection'
import AdvancedSection from './sections/AdvancedSection'
import PassthroughSection from './sections/PassthroughSection'
import VmXmlDialog from './dialogs/VmXmlDialog'
import './vm-form.css'

interface EditVmFormProps {
  /** 详情页 SSE 推送的最新虚拟机配置。 */
  vm: VmDetailInfo
  live: boolean
  liveTick: number
  /** 保存成功后回调（详情页用于刷新） */
  onSaved?: () => void
}

const NOOP_REGISTRATION = {
  enabled: false,
  dedicated_vpc_switch_id: 0,
  dedicated_vpc_label: '',
}

export default function EditVmForm({ vm, live, liveTick, onSaved }: EditVmFormProps) {
  const vmName = vm.name
  const vmStatus = vm.status
  const role = useUserStore((s) => s.role)
  const isAdmin = role === ROLES.admin
  const options = useVmFormOptions({ isAdmin })
  const form = useVmForm({ isEdit: true, isAdmin, registration: NOOP_REGISTRATION, hostArch: options.hostArch })
  const devices = useVmEditDevices(vmName)

  const [activeTab, setActiveTab] = useState('basic')
  const [loading, setLoading] = useState(true)
  const [saving, setSaving] = useState(false)
  const [xmlVisible, setXmlVisible] = useState(false)
  const [currentVmUUID, setCurrentVmUUID] = useState('')

  // 编辑原始值与快照
  const snapshotRef = useRef<EditFormSnapshot | null>(null)
  const diskIopsSnapshotRef = useRef<EditDiskIopsSnapshot>({})
  const origNicModelRef = useRef('')
  const origBootTypeRef = useRef('')
  const origPcieRootPortsRef = useRef(0)
  const origVcpuRef = useRef(1)
  const origMemoryRef = useRef(1)
  const [origSpiceEnabled, setOrigSpiceEnabled] = useState(false)
  const [loadedStatus, setLoadedStatus] = useState(vmStatus)
  const [loadedGuestType, setLoadedGuestType] = useState('')
  const [guestAgentConnected, setGuestAgentConnected] = useState(false)
  /** 最近一次服务端配置签名；资源统计变化不会重置用户正在编辑的字段。 */
  const serverConfigSignatureRef = useRef('')

  // ==================== 详情加载 ====================
  const loadDetail = useCallback(async (silent = false, forceHTTP = false) => {
    if (!vmName) return
    if (!silent) setLoading(true)
    try {
      const base = await options.ensureBaseLoaded()
      const detail: Partial<VmDetailInfo> = forceHTTP
        ? (await getVmDetail(vmName)).data || {}
        : vm
      // 引导设备（启用优先，按 order 排序）
      let bootDevices: typeof devices.editBootDevices = []
      if (detail.boot_devices && detail.boot_devices.length > 0) {
        bootDevices = [...detail.boot_devices].sort((a, b) => {
          if (a.enabled && !b.enabled) return -1
          if (!a.enabled && b.enabled) return 1
          if (a.enabled && b.enabled) return a.order - b.order
          return 0
        })
      }
      // 直通设备（仅管理员，异步加载不阻塞快照）
      let hostDevices: { pci_address: string }[] = []
      if (isAdmin) {
        try {
          const passRes = await getVmPassthroughDevices(vmName)
          hostDevices = (passRes.data || [])
            .map((d) => ({ pci_address: d.pci_address }))
            .sort((a, b) => a.pci_address.localeCompare(b.pci_address))
          void options.loadPassthroughDevices()
        } catch {
          if (serverConfigSignatureRef.current) return
          hostDevices = []
        }
      }
      // SPICE 真实状态
      let spiceEnabled = false
      try {
        const spiceRes = await getSpiceStatus(vmName)
        spiceEnabled = !!spiceRes?.data?.enabled
      } catch {
        if (serverConfigSignatureRef.current) return
        spiceEnabled = false
      }
      const normalDisks = await devices.refreshEditDisks(true)
      setLoadedStatus(detail.status || vmStatus)
      setGuestAgentConnected(!!detail.guest_agent_status?.connected)

      const detailConfig = {
        name: detail.name,
        vcpu: detail.vcpu,
        memory: detail.memory,
        max_memory: detail.max_memory,
        autostart: detail.autostart,
        freeze: detail.freeze,
        apic: detail.apic,
        pae: detail.pae,
        rtc_offset: detail.rtc_offset,
        rtc_startdate: detail.rtc_startdate,
        os_type: detail.os_type,
        guest_agent: detail.guest_agent,
        smbios1: detail.smbios1,
        cpu_limit_percent: detail.cpu_limit_percent,
        cpu_affinity: detail.cpu_affinity,
        nic_model: detail.nic_model,
        arch: detail.arch,
        machine_type: detail.machine_type,
        pcie_root_ports: detail.pcie_root_ports,
        boot_type: detail.boot_type,
        firmware_compat: detail.firmware_compat,
        direct_boot: detail.direct_boot,
        kvm_hidden: detail.kvm_hidden,
        vendor_id: detail.vendor_id,
        nested_virt: detail.nested_virt,
        video_model: detail.video_model,
        cpu_topology_mode: detail.cpu_topology_mode,
        boot_order: detail.boot_order,
        boot_devices: detail.boot_devices,
      }
      const diskConfig = normalDisks.map((disk) => ({
        device: disk.device,
        device_type: disk.device_type,
        path: disk.path,
        capacity_gb: disk.capacity_gb,
        format: disk.format,
        bus: disk.bus,
        backing_path: disk.backing_path,
        iops_total: disk.iops_total,
        iops_read: disk.iops_read,
        iops_write: disk.iops_write,
        is_system: disk.is_system,
        serial: disk.serial,
      }))
      const serverSignature = JSON.stringify({ detail: detailConfig, hostDevices, spiceEnabled, diskConfig })
      if (serverConfigSignatureRef.current === serverSignature) return
      serverConfigSignatureRef.current = serverSignature

      // 由默认值 + SSE 详情同步构建完整表单（避免无变化推送覆盖本地输入）
      const initialForm = {
        ...createDefaultVmForm({ hostArch: base.hostArch }),
        name: detail.name || vmName,
        vcpu: detail.vcpu || 1,
      }
      const nextForm = buildEditFormState(initialForm, detail)
      nextForm.host_devices = hostDevices
      nextForm.host_devices_touched = false
      nextForm.spice_enabled = spiceEnabled
      form.replaceForm(nextForm)
      devices.setEditBootDevices(bootDevices)
      setOrigSpiceEnabled(spiceEnabled)
      setCurrentVmUUID(detail.uuid || '')
      setLoadedGuestType(detail.os_type || '')
      origNicModelRef.current = detail.nic_model || 'virtio'
      origBootTypeRef.current = detail.boot_type || 'bios'
      origPcieRootPortsRef.current = detail.pcie_root_ports || 6
      origVcpuRef.current = detail.vcpu || 1
      origMemoryRef.current = nextForm.memory || 1
      diskIopsSnapshotRef.current = captureEditDiskIopsSnapshot(normalDisks, isAdmin)
      // 表单快照（表单与引导设备均已就绪）
      snapshotRef.current = captureEditFormSnapshot(nextForm, base.hostCores, isAdmin, bootDevices)
      void options.loadStorageTargets()
    } catch {
      // SSE 后台同步失败时保留上一份可用配置，下一次事件会继续尝试。
    } finally {
      if (!silent) setLoading(false)
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [vmName, vm, vmStatus, isAdmin])

  useEffect(() => {
    if (live) void loadDetail(serverConfigSignatureRef.current !== '')
  }, [loadDetail, live, liveTick])

  // ==================== 保存 ====================
  const handleSave = useCallback(async () => {
    const affinity = buildCPUAffinityPayload(form.form, isAdmin)
    if (affinity === null) {
      Toast.warning('CPU 亲和性格式不正确，请使用数字、逗号、空格或连字符（如 0,2,4 或 0-3）')
      return
    }
    setSaving(true)
    try {
      const payload = buildEditPayload(form.form, {
        isAdmin,
        vmStatus: loadedStatus,
        hostCores: options.hostCores,
        snapshot: snapshotRef.current,
        diskIopsSnapshot: diskIopsSnapshotRef.current,
        editDisks: devices.editDisks,
        editBootDevices: devices.editBootDevices,
        origNicModel: origNicModelRef.current,
        origBootType: origBootTypeRef.current,
        origPcieRootPorts: origPcieRootPortsRef.current,
      })
      await updateVm(vmName, payload)
      // SPICE 联动：开关状态变化时启用/禁用（仅关机时可改，仍走二次验证）
      let spiceNote = ''
      const targetSpiceEnabled = getEffectiveSpiceEnabled(form.form)
      if (targetSpiceEnabled !== origSpiceEnabled) {
        try {
          if (targetSpiceEnabled) {
            await enableSpice(vmName, '')
          } else {
            await disableSpice(vmName)
          }
          form.setField('spice_enabled', targetSpiceEnabled)
          setOrigSpiceEnabled(targetSpiceEnabled)
        } catch {
          spiceNote = '；SPICE 状态更新失败，请在详情页手动调整'
        }
      }
      Toast.success('配置修改成功' + spiceNote)
      await loadDetail(false, true)
      onSaved?.()
    } catch {
      // 错误提示由请求层统一处理
    } finally {
      setSaving(false)
    }
  }, [form, isAdmin, loadedStatus, options.hostCores, devices, origSpiceEnabled, vmName, loadDetail, onSaved])

  const running = loadedStatus === 'running'

  return (
    <VmFormProvider
      value={{
        form,
        options,
        ctx: {
          mode: 'edit',
          isAdmin,
          vmStatus: loadedStatus,
          guestType: loadedGuestType,
          guestAgentConnected,
          hostArch: options.hostArch,
          hostCores: options.hostCores,
          spiceSupported: options.spiceSupported,
          registration: NOOP_REGISTRATION,
          editOrigVcpu: origVcpuRef.current,
          editOrigMemory: origMemoryRef.current,
        },
      }}
    >
      <div className="qvm-vf-edit-layout">
        <Tabs type="card" activeKey={activeTab} onChange={setActiveTab} lazyRender>
          <TabPane
            tab={
              <span>
                <IconInfoCircle style={{ marginRight: 6 }} />
                基础配置
              </span>
            }
            itemKey="basic"
          >
            <SectionCard icon={<IconInfoCircle />} title="虚拟机信息">
              <div className="qvm-vf-grid-2">
                <FormField label="虚拟机名称">
                  <Input value={form.form.name} disabled />
                </FormField>
                <FormField label="状态">
                  <div className="qvm-vf-switch-row">
                    <Tag color={running ? 'green' : 'grey'}>{running ? '运行中' : '已关机'}</Tag>
                  </div>
                </FormField>
              </div>
            </SectionCard>
            <CpuMemorySection />
          </TabPane>

          <TabPane
            tab={
              <span>
                <DiskIcon style={{ marginRight: 6 }} />
                磁盘与驱动器
              </span>
            }
            itemKey="disk"
          >
            <DiskManageSection devices={devices} />
          </TabPane>

          <TabPane
            tab={
              <span>
                <IconServer style={{ marginRight: 6 }} />
                启动与安全
              </span>
            }
            itemKey="security"
          >
            <NicSection />
            <VirtEngineSection />
            <BootOrderSection
              editBootDevices={devices.editBootDevices}
              onEditBootDevicesChange={devices.setEditBootDevices}
            />
            <SystemBehaviorSection showWatchdog={false} />
          </TabPane>

          <TabPane
            tab={
              <span>
                <IconGlobe style={{ marginRight: 6 }} />
                网口管理
              </span>
            }
            itemKey="nics"
          >
            <NicManageSection
              vmName={vmName}
              vmStatus={loadedStatus}
              live={live && activeTab === 'nics'}
              liveTick={liveTick}
            />
          </TabPane>

          {isAdmin && (
            <TabPane
              tab={
                <span>
                  <IconPuzzle style={{ marginRight: 6 }} />
                  硬件直通
                </span>
              }
              itemKey="passthrough"
            >
              <PassthroughSection />
            </TabPane>
          )}

          <TabPane
            tab={
              <span>
                <IconSetting style={{ marginRight: 6 }} />
                高级设置
              </span>
            }
            itemKey="advanced"
          >
            <AdvancedSection currentVmUUID={currentVmUUID} onOpenXmlEditor={() => setXmlVisible(true)} />
          </TabPane>
        </Tabs>

        <div className="qvm-vf-edit-footer">
          <Button type="primary" theme="solid" loading={saving || loading} onClick={() => void handleSave()}>
            保存修改
          </Button>
        </div>

        <VmXmlDialog
          visible={xmlVisible}
          vmName={vmName}
          vmStatus={loadedStatus}
          onClose={() => setXmlVisible(false)}
          onSaved={loadDetail}
        />
      </div>
    </VmFormProvider>
  )
}
