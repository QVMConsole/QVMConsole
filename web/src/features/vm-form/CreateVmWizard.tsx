/**
 * 创建虚拟机向导（全屏弹窗）
 * 左侧步骤导航 + 中间分区表单 + 底部导航按钮，最后一步确认信息并提交。
 * 表单状态 / 校验 / 联动全部来自 features/vm-form 共享模块，
 * 与详情页编辑表单（EditVmForm）共用同一套规则。
 */
import { useEffect, useMemo, useState } from 'react'
import { Button, Modal, Steps, Toast } from '@douyinfe/semi-ui'
import {
  IconBolt,
  IconBox,
  IconCheckList,
  IconServer,
  IconDisc,
  IconGlobe,
  IconInfoCircle,
  IconPuzzle,
  IconSetting,
} from '@douyinfe/semi-icons'
import { DiskIcon } from './icons'
import { useUserStore } from '@/stores/user'
import { ROLES } from '@/config/constants'
import { adminImportDisk, batchCloneVm, cloneVm, createVm } from '@/api/vm'
import { importAppliance, importVM, selfCreateVm } from '@/api/storage'
import { selfCloneVm } from '@/api/user'
import { checkPasswordBreachAsync } from '@/utils/validate'
import { resolveTemplateMinDiskSize } from '@/views/vm/utils'
import { VmFormProvider } from './scope'
import { useVmForm } from './useVmForm'
import { useVmFormOptions } from './useVmFormOptions'
import { collectMissingRequired, validateCreateStep, type ValidateContext } from './validators'
import {
  buildBatchClonePayload,
  buildClonePayload,
  buildCreatePayload,
  buildImportPayload,
  buildCPUAffinityPayload,
} from './payload'
import type { RegistrationContext, VmCreateMode } from './types'
import CreateModeSection from './sections/CreateModeSection'
import BasicInfoSection from './sections/BasicInfoSection'
import TemplateSection from './sections/TemplateSection'
import CpuMemorySection from './sections/CpuMemorySection'
import VirtEngineSection from './sections/VirtEngineSection'
import StoragePoolSection from './sections/StoragePoolSection'
import IsoStorageSection from './sections/IsoStorageSection'
import ImportStorageSection from './sections/ImportStorageSection'
import ApplianceImportSection, { ApplianceDiskSummarySection } from './sections/ApplianceImportSection'
import ExtraDiskSection from './sections/ExtraDiskSection'
import NicSection from './sections/NicSection'
import BootOrderSection from './sections/BootOrderSection'
import SystemBehaviorSection from './sections/SystemBehaviorSection'
import AdvancedSection from './sections/AdvancedSection'
import PassthroughSection from './sections/PassthroughSection'
import ConfirmSection from './sections/ConfirmSection'
import './vm-form.css'

/** 轻量云登记草稿（用户管理页登记服务器时回传） */
export interface RegistrationDraft {
  vm_name: string
  template: string
  template_type: string
  clone_mode: string
  vcpu: number
  ram: number
  disk_size: number
  hostname: string
  autostart: boolean
  freeze: boolean
  apic: boolean
  pae: boolean
  rtc_offset: string
  rtc_startdate: string
  guest_agent: unknown
  smbios1: unknown
  memory_dynamic: unknown
  disk_bus: string
  video_model: string
  cpu_topology_mode: string
  cpu_limit_percent?: number
  cpu_affinity?: string
  first_boot_reboot_mode: string
  nic_model: string
  storage_pool_id: string
  extra_disks: unknown
  preserve_fnos_device_id: boolean
  fnos_device_id: string
  traffic_down_gb: number
  traffic_up_gb: number
  bandwidth_down_mbps: number
  bandwidth_up_mbps: number
  max_port_forwards: number
  max_runtime_hours: number
}

interface CreateVmWizardProps {
  visible: boolean
  onClose: () => void
  onSuccess: () => void
  /** 初始创建方式（默认 iso） */
  initialMode?: VmCreateMode
  /** 轻量云登记模式（用户管理页使用，本轮列表页不启用） */
  registration?: RegistrationContext
  onDraft?: (draft: RegistrationDraft) => void
}

interface StepDef {
  name: string
  title: string
  icon: React.ReactNode
}

export default function CreateVmWizard({
  visible,
  onClose,
  onSuccess,
  initialMode = 'iso',
  registration,
  onDraft,
}: CreateVmWizardProps) {
  const role = useUserStore((s) => s.role)
  const isAdmin = role === ROLES.admin
  const reg: RegistrationContext = registration || {
    enabled: false,
    dedicated_vpc_switch_id: 0,
    dedicated_vpc_label: '',
  }
  const options = useVmFormOptions({ isAdmin })
  const form = useVmForm({ isEdit: false, isAdmin, registration: reg, hostArch: options.hostArch })
  const [step, setStep] = useState(0)
  const [submitting, setSubmitting] = useState(false)

  // ==================== 打开初始化 ====================
  useEffect(() => {
    if (!visible) return
    setStep(0)
    void (async () => {
      const base = await options.ensureBaseLoaded()
      form.resetForCreate({
        createMode: reg.enabled ? 'template' : initialMode,
        hostArch: base.hostArch,
        spiceDefault: base.spiceDefault,
        registration: reg.enabled,
      })
      const targets = await options.loadStorageTargets()
      const defaultTarget = targets.find((t) => t.is_default)
      if (defaultTarget) form.setField('storage_pool_id', defaultTarget.id)
      if (reg.enabled) {
        form.setField('switch_id', reg.dedicated_vpc_switch_id || null)
        await options.loadTemplates(true)
      } else {
        void options.loadVPCOptions().then(({ groups }) => {
          // 默认安全组兜底
          const defaultGroup = groups.find((g) => g.is_default)
          form.setField('security_group_id', defaultGroup?.id || groups[0]?.id || null)
        })
      }
      if ((initialMode === 'import' || initialMode === 'appliance') && !reg.enabled) void options.loadDiskFiles()
      if (initialMode === 'template' && !reg.enabled) await options.loadTemplates(true)
    })()
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [visible])

  // ==================== 步骤定义 ====================
  const steps = useMemo<StepDef[]>(() => {
    if (reg.enabled) {
      return [
        { name: 'basic', title: '基础信息', icon: <IconInfoCircle /> },
        { name: 'hardware', title: '硬件规格', icon: <IconServer /> },
        { name: 'storage', title: '存储介质', icon: <DiskIcon /> },
        { name: 'network', title: '网络配额', icon: <IconGlobe /> },
        { name: 'security', title: '系统配置', icon: <IconSetting /> },
        { name: 'advanced', title: '高级选项', icon: <IconBolt /> },
        { name: 'confirm', title: '确认信息', icon: <IconCheckList /> },
      ]
    }
    const applianceSteps: StepDef[] = [
      { name: 'mode', title: '创建方式', icon: <IconDisc /> },
      ...(form.form.create_mode === 'appliance'
        ? [{ name: 'appliance', title: '虚拟机包', icon: <IconBox /> }]
        : []),
    ]
    if (
      form.form.create_mode === 'appliance' &&
      form.form.appliance_config_mode === 'ovf'
    ) {
      return applianceSteps
    }
    return [
      ...applianceSteps,
      { name: 'basic', title: '基础信息', icon: <IconInfoCircle /> },
      { name: 'hardware', title: '硬件规格', icon: <IconServer /> },
      { name: 'storage', title: '存储介质', icon: <DiskIcon /> },
      { name: 'network', title: '网络设置', icon: <IconGlobe /> },
      { name: 'security', title: '系统配置', icon: <IconSetting /> },
      { name: 'advanced', title: '高级选项', icon: <IconBolt /> },
      ...(isAdmin ? [{ name: 'passthrough', title: '硬件直通', icon: <IconPuzzle /> }] : []),
      { name: 'confirm', title: '确认信息', icon: <IconCheckList /> },
    ]
  }, [reg.enabled, isAdmin, form.form.create_mode, form.form.appliance_config_mode])

  const maxStep = steps.length - 1
  const currentStepName = steps[step]?.name
  const followsOvfConfig =
    form.form.create_mode === 'appliance' && form.form.appliance_config_mode === 'ovf'
  const showSubmitButton =
    form.form.create_mode !== 'appliance' ||
    followsOvfConfig ||
    currentStepName === 'confirm'

  // ==================== 校验上下文 ====================
  const validateCtx: ValidateContext = useMemo(() => {
    const selectedTemplate = options.templates.find((tpl) => tpl.name === form.form.template) || null
    return {
      isAdmin,
      hostArch: options.hostArch,
      isTemplateMode: form.isTemplateSourceMode,
      isWindowsTemplate: form.isWindowsTemplate,
      isFnOSTemplate: form.isFnOSTemplate,
      isOpenWrtTemplate: form.isOpenWrtTemplate,
      isNoInitTemplate: selectedTemplate?.cloud_init_mode === 'none',
      disableSystemInit: form.disableSystemInit,
      registrationMode: reg.enabled,
      templateMinDiskSize: resolveTemplateMinDiskSize(selectedTemplate),
    }
  }, [isAdmin, form, options.templates, options.hostArch, reg.enabled])

  const missing = useMemo(() => collectMissingRequired(form.form, validateCtx), [form.form, validateCtx])
  const submitDisabledReason =
    missing.length > 0 ? `以下必填项未完成：${missing.join('、')}` : ''

  // ==================== 步骤导航 ====================
  const goNext = () => {
    const err = validateCreateStep(currentStepName, form.form, validateCtx)
    if (err) {
      Toast.warning({ content: err, duration: 3 })
      return
    }
    setStep((s) => Math.min(s + 1, maxStep))
  }

  const goPrev = () => setStep((s) => Math.max(s - 1, 0))

  const handleSelectMode = (mode: VmCreateMode) => {
    const selectedTemplate = options.templates.find((t) => t.name === form.form.template) || null
    form.onCreateModeChange(mode, selectedTemplate)
    if (mode === 'import' || mode === 'appliance') void options.loadDiskFiles()
    if (mode === 'template') void options.loadTemplates(true)
    setStep((s) => Math.min(s + 1, maxStep))
  }

  // ==================== 提交 ====================
  const handleSubmit = async () => {
    if (missing.length > 0) {
      Toast.warning({ content: submitDisabledReason, duration: 3 })
      return
    }
    // CPU 亲和性格式校验
    const affinity = buildCPUAffinityPayload(form.form, isAdmin)
    if (affinity === null) {
      Toast.warning('CPU 亲和性格式不正确，请使用数字、逗号、空格或连字符（如 0,2,4 或 0-3）')
      return
    }
    // 密码泄露检测（HIBP）
    if (form.form.import_password) {
      const breach = await checkPasswordBreachAsync(form.form.import_password)
      if (breach.enabled && breach.breached) {
        Toast.error('该密码已在已知泄露数据库中发现，请更换为更安全的密码')
        return
      }
    }

    const f = form.form
    setSubmitting(true)
    try {
      if (f.create_mode === 'appliance') {
        const base = buildImportPayload(f, { isAdmin, hostCores: options.hostCores })
        await importAppliance(
          {
            ...base,
            appliance_file: f.appliance_file || undefined,
            appliance_path: f.appliance_path || undefined,
            source_type: isAdmin && f.appliance_source_type === 'path' ? 'path' : 'storage',
            config_mode: f.appliance_config_mode,
            copy_source: f.copy_source,
            storage_pool_id: f.storage_pool_id,
          },
          isAdmin,
        )
        Toast.success('虚拟机包导入任务已提交，请在任务中查看进度')
      } else if (f.create_mode === 'import') {
        // 导入模式
        const payload = buildImportPayload(f, { isAdmin, hostCores: options.hostCores })
        if (isAdmin && f.disk_source_type === 'path') {
          await adminImportDisk(payload)
          Toast.success('导入磁盘任务已提交，请在任务中查看进度')
        } else {
          await importVM(payload)
          Toast.success('导入任务已提交，请在任务中查看进度')
        }
      } else if (form.isTemplateSourceMode) {
        // 模板克隆模式
        form.ensureTemplateDefaults(options.templates.find((t) => t.name === f.template) || null)
        const cloneCtx = {
          isAdmin,
          hostCores: options.hostCores,
          isWindowsTemplate: form.isWindowsTemplate,
          isOpenWrtTemplate: form.isOpenWrtTemplate,
          shouldPreserveFnosDeviceId: form.shouldPreserveFnosDeviceId,
          customFnosDeviceId: form.hasCustomFnosDeviceId ? form.normalizedFnosDeviceId : '',
        }
        if (f.batch_count > 1) {
          if (reg.enabled) {
            Toast.warning('批量创建暂不支持服务器登记模式')
            setSubmitting(false)
            return
          }
          if (f.host_devices.length > 0) {
            Toast.warning('批量克隆不能复用同一组物理直通设备，请改为单台克隆')
            setSubmitting(false)
            return
          }
          await batchCloneVm(buildBatchClonePayload(f, cloneCtx))
          Toast.success(`批量克隆任务已提交（${f.batch_count} 台），请在任务中查看进度`)
        } else {
          const payload = buildClonePayload(f, cloneCtx)
          if (reg.enabled) {
            const draft: RegistrationDraft = {
              vm_name: payload.name,
              template: payload.template,
              template_type: payload.template_type || '',
              clone_mode: payload.clone_mode,
              vcpu: payload.vcpu,
              ram: payload.ram,
              disk_size: payload.disk_size,
              hostname: payload.hostname || '',
              autostart: !!payload.autostart,
              freeze: !!payload.freeze,
              apic: !!payload.apic,
              pae: !!payload.pae,
              rtc_offset: payload.rtc_offset || 'utc',
              rtc_startdate: payload.rtc_startdate || 'now',
              guest_agent: payload.guest_agent,
              smbios1: payload.smbios1,
              memory_dynamic: payload.memory_dynamic,
              disk_bus: payload.disk_bus || 'virtio',
              video_model: payload.video_model || 'virtio',
              cpu_topology_mode: payload.cpu_topology_mode || 'auto',
              cpu_limit_percent: payload.cpu_limit_percent,
              cpu_affinity: payload.cpu_affinity,
              first_boot_reboot_mode: payload.first_boot_reboot_mode || 'normal',
              nic_model: payload.nic_model || 'virtio',
              storage_pool_id: payload.storage_pool_id || '',
              extra_disks: payload.extra_disks,
              preserve_fnos_device_id: !!payload.preserve_fnos_device_id,
              fnos_device_id: payload.fnos_device_id || '',
              traffic_down_gb: f.traffic_down_gb || 0,
              traffic_up_gb: f.traffic_up_gb || 0,
              bandwidth_down_mbps: f.bandwidth_down_mbps || 0,
              bandwidth_up_mbps: f.bandwidth_up_mbps || 0,
              max_port_forwards: f.max_port_forwards ?? 10,
              max_runtime_hours: f.max_runtime_hours || 0,
            }
            onDraft?.(draft)
            Toast.success('已加入注册列表，请在列表中确认后保存')
            onClose()
            return
          }
          if (isAdmin) {
            await cloneVm(payload)
          } else {
            await selfCloneVm(payload)
          }
          Toast.success('克隆任务已提交，请在任务中查看进度')
        }
      } else {
        // ISO 安装模式
        const payload = buildCreatePayload(f, { isAdmin, hostCores: options.hostCores })
        if (isAdmin) {
          await createVm(payload)
        } else {
          await selfCreateVm(payload)
        }
        Toast.success('创建任务已提交，请在任务中查看进度')
      }
      onClose()
      onSuccess()
    } catch {
      // 错误提示由请求层统一处理
    } finally {
      setSubmitting(false)
    }
  }

  // ==================== 步骤内容 ====================
  const renderStepContent = () => {
    switch (currentStepName) {
      case 'mode':
        return <CreateModeSection onSelect={handleSelectMode} />
      case 'appliance':
        return <ApplianceImportSection />
      case 'basic':
        return (
          <>
            <BasicInfoSection />
            {form.isTemplateSourceMode && <TemplateSection />}
          </>
        )
      case 'hardware':
        return (
          <>
            <CpuMemorySection />
            <VirtEngineSection />
          </>
        )
      case 'storage':
        if (form.form.create_mode === 'iso') {
          return (
            <>
              <StoragePoolSection />
              <IsoStorageSection />
            </>
          )
        }
        if (form.form.create_mode === 'import') {
          return (
            <>
              <StoragePoolSection />
              <ImportStorageSection />
            </>
          )
        }
        if (form.form.create_mode === 'appliance') {
          return (
            <>
              <StoragePoolSection />
              <ApplianceDiskSummarySection />
            </>
          )
        }
        // 模板模式：提示 + 额外数据盘
        return (
          <>
            <StoragePoolSection />
            <ExtraDiskSection tip="额外磁盘会在模板克隆完成后自动挂载，并计入普通用户硬盘配额" />
          </>
        )
      case 'network':
        return <NicSection />
      case 'security':
        return (
          <>
            <BootOrderSection />
            <SystemBehaviorSection />
          </>
        )
      case 'advanced':
        return <AdvancedSection />
      case 'passthrough':
        return <PassthroughSection />
      case 'confirm':
        return <ConfirmSection />
      default:
        return null
    }
  }

  const stepHeader = useMemo(() => {
    const meta: Record<string, { title: string; desc: string }> = {
      mode: { title: '选择创建方式', desc: '根据需求选择最适合的虚拟机创建途径' },
      appliance: {
        title: '导入虚拟机',
        desc: followsOvfConfig
          ? '检查 OVF 或 OVA 后，按包内配置直接创建虚拟机'
          : '检查 OVF 或 OVA，并在后续步骤自定义最终配置',
      },
      basic: { title: '基础信息', desc: '设置虚拟机名称、用途和操作系统类型' },
      hardware: { title: '硬件规格', desc: '配置 CPU、内存和虚拟化引擎参数' },
      storage: {
        title: '存储介质',
        desc:
          form.form.create_mode === 'iso'
            ? '选择 ISO 镜像并配置系统磁盘'
            : form.isTemplateSourceMode
              ? '配置存储位置并追加数据盘'
              : form.form.create_mode === 'appliance'
                ? '选择全部包内磁盘的目标存储位置'
                : '选择要导入的磁盘文件',
      },
      network: { title: '网络设置', desc: '配置网卡类型和网口' },
      security: { title: '系统配置', desc: '设置引导顺序、守护服务和开机自启' },
      advanced: { title: '高级选项', desc: '开发者选项和底层参数，一般保持默认即可' },
      passthrough: { title: '硬件直通', desc: '将宿主机 PCI 设备直接分配给虚拟机，获得接近原生的性能' },
      confirm: { title: '确认信息', desc: '核对全部配置后提交创建任务' },
    }
    return meta[currentStepName] || { title: '', desc: '' }
  }, [currentStepName, form.form.create_mode, form.isTemplateSourceMode, followsOvfConfig])

  return (
    <Modal
      title={reg.enabled ? '登记轻量云服务器' : '新建虚拟机'}
      visible={visible}
      onCancel={onClose}
      fullScreen
      maskClosable={false}
      className="qvm-vf-wizard"
      footer={
        <div className="qvm-vf-wizard-footer">
          <Button onClick={onClose}>取消</Button>
          <div className="qvm-vf-wizard-footer-right">
            {step > 0 && <Button onClick={goPrev}>上一步</Button>}
            {step < maxStep && (
              <Button type="primary" theme="solid" onClick={goNext}>
                下一步
              </Button>
            )}
            {showSubmitButton && (
              <Button
                type="warning"
                theme="solid"
                loading={submitting}
                disabled={missing.length > 0}
                onClick={() => void handleSubmit()}
              >
                {reg.enabled
                  ? '加入注册列表'
                  : followsOvfConfig
                    ? '按 OVF 配置创建'
                    : '创建虚拟机'}
              </Button>
            )}
          </div>
        </div>
      }
    >
      <VmFormProvider
        value={{
          form,
          options,
          ctx: {
            mode: 'create',
            isAdmin,
            vmStatus: 'shut off',
            guestType: form.form.os_type,
            guestAgentConnected: false,
            hostArch: options.hostArch,
            hostCores: options.hostCores,
            spiceSupported: options.spiceSupported,
            registration: reg,
          },
        }}
      >
        <div className="qvm-vf-wizard-layout">
          <aside className="qvm-vf-wizard-aside">
            <Steps type="basic" direction="vertical" current={step} onChange={(i) => setStep(i)}>
              {steps.map((s) => (
                <Steps.Step key={s.name} title={s.title} icon={s.icon} />
              ))}
            </Steps>
          </aside>
          <main className="qvm-vf-wizard-main">
            <div className="qvm-vf-wizard-main-inner">
              <div className="qvm-vf-step-header">
                <div className="qvm-vf-step-title">{stepHeader.title}</div>
                <div className="qvm-vf-step-desc">{stepHeader.desc}</div>
              </div>
              {submitDisabledReason && currentStepName === 'confirm' && (
                <div className="qvm-vf-tip warn" style={{ marginBottom: 12 }}>
                  {submitDisabledReason}
                </div>
              )}
              {renderStepContent()}
            </div>
          </main>
        </div>
      </VmFormProvider>
    </Modal>
  )
}
