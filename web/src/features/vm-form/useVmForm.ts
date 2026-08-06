/**
 * 虚拟机表单核心 hook
 * 集中管理表单状态与全部联动规则（OS/ISO/模板/架构/机型/引导切换、
 * 动态内存推荐、编辑回填），创建向导与编辑表单共用同一份逻辑。
 */
import { useCallback, useMemo, useRef, useState } from 'react'
import type { TemplateItem } from '@/api/template'
import type { VmDetailInfo, VmListItem } from '@/api/vm'
import { resolveTemplateMinDiskSize } from '@/views/vm/utils'
import { WINDOWS_TEMPLATE_USERNAME, FNOS_DEVICE_ID_PATTERN } from './constants'
import {
  createDefaultVmForm,
  createEmptyGuestAgentConfig,
  createEmptySMBIOS1Config,
  generateRandomHostname,
  type CreateDefaultFormOptions,
} from './defaults'
import {
  getElasticMemorySpecFromConfig,
  getRecommendedRTCOffset,
  getRecommendedVideoModel,
  normalizeAPICForForm,
  normalizePAEForForm,
  normalizeRTCOffsetForForm,
  recommendedMemoryDynamicValues,
  shouldUseBIOSForI440FXWindows,
} from './recommend'
import {
  resolveTemplateBootType,
  resolveTemplateDefaultCPUTopologyMode,
  resolveTemplateDefaultDiskBus,
  resolveTemplateDefaultDiskSize,
  resolveTemplateDefaultFirstBootRebootMode,
  resolveTemplateDefaultNicModel,
  resolveTemplateDefaultRAM,
  resolveTemplateDefaultVCPU,
  resolveTemplateDefaultVideoModel,
} from './templateUtils'
import type { RegistrationContext, VmCreateMode, VmFormModel } from './types'

export interface UseVmFormParams {
  isEdit: boolean
  /** 预留给壳层语义一致性（表单内部权限判断统一走 ctx） */
  isAdmin: boolean
  registration: RegistrationContext
  hostArch: string
}

/** 由虚拟机详情构建编辑表单状态（纯函数，供回填与快照捕获同步使用） */
export function buildEditFormState(
  prev: VmFormModel,
  detail: Partial<VmDetailInfo>,
  row: Partial<VmListItem> & { os_type?: string } = {},
): VmFormModel {
  const next = { ...prev }
  next.autostart = detail.autostart || false
  next.freeze = detail.freeze || false
  next.apic = normalizeAPICForForm(detail.apic)
  next.pae = normalizePAEForForm(detail.pae)
  next.rtc_offset = normalizeRTCOffsetForForm(detail.rtc_offset)
  next.rtc_startdate = detail.rtc_startdate || 'now'
  next.os_type = detail.os_type || row.os_type || 'linux'
  next.guest_agent = {
    ...createEmptyGuestAgentConfig(),
    ...(detail.guest_agent || {}),
  }
  next.smbios1 = {
    ...createEmptySMBIOS1Config(),
    ...(detail.smbios1 || {}),
  } as VmFormModel['smbios1']
  next.memory_dynamic_enabled = !!detail.memory_dynamic_enabled
  next.memory_backend = detail.memory_backend || 'balloon'
  next.memory_initial = Math.max(1, Math.round((detail.memory_initial || detail.memory || 1024) / 1024))
  next.memory_min = Math.max(1, Math.round((detail.memory_min || 1024) / 1024))
  next.memory_max_dynamic = Math.max(
    1,
    Math.round((detail.memory_max_dynamic || detail.max_memory || detail.memory || 1024) / 1024),
  )
  next.cpu_limit_enabled = Number(detail.cpu_limit_percent || 0) > 0
  next.cpu_limit_percent = next.cpu_limit_enabled ? Number(detail.cpu_limit_percent) : 100
  next.cpu_affinity = detail.cpu_affinity || ''
  const rowMemoryGB = Math.max(1, Math.round((detail.memory || row.memory || 1024) / 1024))
  if (next.memory_dynamic_enabled && next.memory_backend === 'virtio_mem') {
    next.memory = getElasticMemorySpecFromConfig(
      next.memory_initial,
      next.memory_max_dynamic,
      rowMemoryGB,
    )
  } else {
    next.memory = rowMemoryGB
  }
  next.memory_auto_balloon = !!detail.memory_auto_balloon
  next.memory_current = 0
  next.memory_virtio_mem_current = Math.max(
    0,
    Math.round((detail.memory_virtio_mem_current || 0) / 1024),
  )
  next.memory_dynamic_touched = false
  next.memory_pending_apply = !!detail.memory_pending_apply
  next.memory_compat_mode = detail.memory_compat_mode || 'legacy_static'
  next.memory_balloon_supported = !!detail.memory_balloon_supported
  next.memory_balloon_status = detail.memory_balloon_status || 'not_running'
  if (detail.nic_model) next.nic_model = detail.nic_model
  next.arch = detail.arch || prev.arch || 'x86_64'
  next.machine_type = detail.machine_type || prev.machine_type || 'q35'
  next.pcie_root_ports = detail.pcie_root_ports || 6
  next.boot_type = detail.boot_type || prev.boot_type || 'bios'
  next.firmware_compat = !!detail.firmware_compat
  next.direct_boot_enabled = !!detail.direct_boot?.enabled
  next.direct_boot_cmdline = detail.direct_boot?.cmdline || ''
  next.kvm_hidden = !!detail.kvm_hidden
  next.vendor_id = detail.vendor_id || ''
  next.nested_virt = detail.nested_virt !== undefined ? !!detail.nested_virt : true
  next.video_model = detail.video_model || getRecommendedVideoModel(detail.os_type || 'linux', next.arch)
  next.cpu_topology_mode = detail.cpu_topology_mode || 'auto'
  next.boot_order = detail.boot_order && detail.boot_order.length > 0 ? [...detail.boot_order] : ['hd']
  return next
}

export function useVmForm({ isEdit, registration, hostArch }: UseVmFormParams) {
  const [form, setForm] = useState<VmFormModel>(() =>
    createDefaultVmForm({ hostArch, registration: registration.enabled }),
  )
  /** 用户手动改过引导类型后不再自动推荐 */
  const bootTypeTouchedRef = useRef(false)

  // ==================== 基础操作 ====================

  const setField = useCallback(
    <K extends keyof VmFormModel>(key: K, value: VmFormModel[K]) => {
      setForm((prev) => ({ ...prev, [key]: value }))
    },
    [],
  )

  const patch = useCallback((partial: Partial<VmFormModel>) => {
    setForm((prev) => ({ ...prev, ...partial }))
  }, [])

  /** 创建模式重置（切换创建方式/重新打开时） */
  const resetForCreate = useCallback(
    (options: CreateDefaultFormOptions = {}) => {
      bootTypeTouchedRef.current = false
      setForm(
        createDefaultVmForm({
          hostArch,
          registration: registration.enabled,
          ...options,
        }),
      )
    },
    [hostArch, registration.enabled],
  )

  // ==================== 派生状态 ====================

  const isTemplateSourceMode = form.create_mode === 'template'
  const disableSystemInit = isTemplateSourceMode && !form.system_init_enabled
  const isWindowsTemplate = isTemplateSourceMode && form.template_type === 'windows'
  const isFnOSTemplate = isTemplateSourceMode && form.template_type === 'fnos'
  const isOpenWrtTemplate = isTemplateSourceMode && form.template_type === 'openwrt'

  const normalizedFnosDeviceId = useMemo(
    () => `${form.fnos_device_id || ''}`.trim(),
    [form.fnos_device_id],
  )
  const hasCustomFnosDeviceId =
    isFnOSTemplate && !disableSystemInit && FNOS_DEVICE_ID_PATTERN.test(normalizedFnosDeviceId)
  const shouldPreserveFnosDeviceId = useMemo(() => {
    if (!isFnOSTemplate || disableSystemInit) return false
    return (
      form.fnos_device_id_mode === 'preserve' ||
      form.fnos_device_id_mode === 'custom' ||
      hasCustomFnosDeviceId
    )
  }, [isFnOSTemplate, disableSystemInit, form.fnos_device_id_mode, hasCustomFnosDeviceId])

  /** 是否为 Windows 内存目标（决定是否可用 virtio_mem 弹性内存） */
  const isWindowsMemoryTarget = useMemo(() => {
    if (isEdit) return form.os_type === 'windows' || form.memory_backend === 'virtio_mem'
    if (isTemplateSourceMode) return form.template_type === 'windows'
    if (form.create_mode === 'import') return false
    return form.os_type === 'windows'
  }, [isEdit, form.os_type, form.memory_backend, isTemplateSourceMode, form.template_type, form.create_mode])

  const windowsElasticMemoryDisabled = !isWindowsMemoryTarget
  const i440fxWindowsBios = shouldUseBIOSForI440FXWindows(form, isEdit)

  // ==================== 模板联动 ====================

  /** 应用选中模板的默认配置（applyProfile=false 时仅同步类型/引导） */
  const applySelectedTemplateSettings = useCallback(
    (tpl: TemplateItem | null, applyProfile = true) => {
      if (!tpl) return
      setForm((prev) => {
        const next = { ...prev }
        next.template_type = tpl.type || ''
        if (next.template_type !== 'fnos') {
          next.preserve_fnos_device_id = false
          next.fnos_device_id_mode = 'regenerate'
          next.fnos_device_id = ''
        }
        // 芯片组由用户在高级设置中选择，模板切换不覆盖
        const templateBoot = resolveTemplateBootType(tpl)
        if (templateBoot && !bootTypeTouchedRef.current) {
          next.boot_type = templateBoot
        }
        if (!applyProfile) return next
        const defaultVCPU = resolveTemplateDefaultVCPU(tpl)
        if (defaultVCPU > 0) next.vcpu = defaultVCPU
        const defaultRAM = resolveTemplateDefaultRAM(tpl)
        if (defaultRAM > 0) next.ram = defaultRAM
        const defaultDiskSize = resolveTemplateDefaultDiskSize(tpl)
        if (defaultDiskSize > 0) next.disk_size = defaultDiskSize
        const defaultDiskBus = resolveTemplateDefaultDiskBus(tpl)
        if (defaultDiskBus) next.disk_bus = defaultDiskBus
        const defaultNicModel = resolveTemplateDefaultNicModel(tpl)
        if (defaultNicModel) next.nic_model = defaultNicModel
        const defaultVideoModel = resolveTemplateDefaultVideoModel(tpl)
        if (defaultVideoModel) {
          next.video_model = defaultVideoModel
        } else if (next.arch === 'aarch64') {
          next.video_model = 'ramfb'
        }
        const defaultTopology = resolveTemplateDefaultCPUTopologyMode(tpl)
        if (defaultTopology) next.cpu_topology_mode = defaultTopology
        const defaultRebootMode = resolveTemplateDefaultFirstBootRebootMode(tpl)
        if (defaultRebootMode) next.first_boot_reboot_mode = defaultRebootMode
        return next
      })
    },
    [],
  )

  /** 模板模式默认值兜底（主机名 / Windows 用户名 / 最小磁盘） */
  const ensureTemplateDefaults = useCallback(
    (selectedTemplate: TemplateItem | null) => {
      setForm((prev) => {
        const next = { ...prev }
        if (prev.create_mode === 'template' && !prev.hostname) {
          next.hostname = generateRandomHostname()
        }
        if (prev.create_mode === 'template' && prev.template_type === 'windows') {
          next.import_user = WINDOWS_TEMPLATE_USERNAME
        } else if (
          prev.create_mode === 'template' &&
          selectedTemplate?.template_user &&
          !prev.import_user
        ) {
          next.import_user = selectedTemplate.template_user
        }
        const minDisk = resolveTemplateMinDiskSize(selectedTemplate)
        if (prev.create_mode === 'template' && minDisk > 0 && next.disk_size < minDisk) {
          next.disk_size = minDisk
        }
        return next
      })
    },
    [],
  )

  /** 切换模板（由壳层调用，先刷新模板列表再回传选中的模板对象） */
  const onTemplateChange = useCallback(
    (tpl: TemplateItem | null) => {
      if (!tpl) return
      setForm((prev) => {
        const next = { ...prev }
        const guestType = tpl.type === 'windows' ? 'windows' : 'linux'
        next.rtc_offset = getRecommendedRTCOffset(guestType)
        next.video_model = getRecommendedVideoModel(guestType, next.arch)
        // 非 Windows 目标不允许 virtio_mem，回退 balloon
        if (guestType !== 'windows' && next.memory_backend === 'virtio_mem') {
          next.memory_backend = 'balloon'
          const spec = next.ram || 1
          Object.assign(next, recommendedMemoryDynamicValues('balloon', spec))
        }
        return next
      })
      applySelectedTemplateSettings(tpl, true)
      setForm((prev) => {
        const next = { ...prev }
        const minDisk = resolveTemplateMinDiskSize(tpl)
        if (minDisk > 0 && next.disk_size < minDisk) next.disk_size = minDisk
        if (tpl.type === 'windows') {
          next.import_user = WINDOWS_TEMPLATE_USERNAME
        } else if (tpl.template_user) {
          next.import_user = tpl.template_user
        }
        if (next.create_mode === 'template' && !next.hostname) {
          next.hostname = generateRandomHostname()
        }
        return next
      })
    },
    [applySelectedTemplateSettings],
  )

  // ==================== 系统 / ISO 联动 ====================

  /** 切换系统类型（ISO 模式） */
  const onOsTypeChange = useCallback(
    (osType: string) => {
      setForm((prev) => {
        const next = { ...prev, os_type: osType, os_variant: '' }
        if (osType === 'windows') {
          if (shouldUseBIOSForI440FXWindows(next, isEdit)) {
            next.boot_type = 'bios'
          } else if (!bootTypeTouchedRef.current) {
            next.boot_type = 'uefi'
          }
          // Windows 默认 SATA 磁盘 + e1000e 网卡（兼容性更好）
          next.disk_bus = 'sata'
          next.nic_model = 'e1000e'
        } else {
          next.disk_bus = 'virtio'
          next.nic_model = 'virtio'
        }
        next.rtc_offset = getRecommendedRTCOffset(osType)
        next.video_model =
          next.arch === 'aarch64' ? 'ramfb' : getRecommendedVideoModel(osType, next.arch)
        if (osType !== 'windows' && next.memory_backend === 'virtio_mem') {
          next.memory_backend = 'balloon'
          Object.assign(next, recommendedMemoryDynamicValues('balloon', next.ram || 1))
        }
        return next
      })
    },
    [isEdit],
  )

  /** 选择 ISO 后自动补全系统信息（首个 ISO 为主安装盘） */
  const onISOChange = useCallback(
    (paths: string[], isoList: { path: string; os_type?: string; os_variant?: string; min_disk?: number }[]) => {
      setForm((prev) => {
        const selectedPaths = (paths || []).filter(Boolean)
        const next = { ...prev, iso_paths: selectedPaths, iso_path: selectedPaths[0] || '' }
        if (!next.iso_path) return next
        const iso = isoList.find((i) => i.path === next.iso_path)
        if (!iso) return next
        if (iso.os_type) {
          next.os_type = iso.os_type
          next.rtc_offset = getRecommendedRTCOffset(iso.os_type)
          next.video_model = getRecommendedVideoModel(iso.os_type, next.arch)
          if (iso.os_type !== 'windows' && next.memory_backend === 'virtio_mem') {
            next.memory_backend = 'balloon'
            Object.assign(next, recommendedMemoryDynamicValues('balloon', next.ram || 1))
          }
        }
        if (iso.os_variant) next.os_variant = iso.os_variant
        const minDisk = iso.min_disk || (iso.os_type === 'windows' ? 20 : 10)
        if (next.disk_size < minDisk) next.disk_size = minDisk
        if (iso.os_type === 'windows') {
          if (shouldUseBIOSForI440FXWindows(next, isEdit)) {
            next.boot_type = 'bios'
          } else if (!bootTypeTouchedRef.current) {
            next.boot_type = 'uefi'
          }
          next.disk_bus = 'sata'
          next.nic_model = 'e1000e'
        }
        // 有 ISO 时启动顺序 cdrom 优先
        if (!next.boot_order.includes('cdrom')) {
          next.boot_order = ['cdrom', 'hd']
        }
        return next
      })
    },
    [isEdit],
  )

  /** 切换创建方式 */
  const onCreateModeChange = useCallback(
    (mode: VmCreateMode, selectedTemplate: TemplateItem | null) => {
      setForm((prev) => {
        const next = { ...prev, create_mode: mode }
        if (mode === 'import') {
          next.boot_order = ['hd']
          next.disk_source_type = 'storage'
          next.disk_path = ''
          next.extra_import_disks = []
        } else if (mode === 'appliance') {
          next.boot_order = ['hd']
          next.appliance_source_type = 'storage'
          next.appliance_config_mode = 'ovf'
          next.appliance_path = ''
          next.appliance_metadata = null
        } else if (mode === 'template') {
          next.boot_order = ['hd']
          next.clone_mode = 'linked'
          next.system_init_enabled = true
        } else {
          next.disk_file = ''
          next.disk_path = ''
        }
        const guestType =
          mode === 'template'
            ? next.template_type === 'windows'
              ? 'windows'
              : 'linux'
            : mode === 'import' || mode === 'appliance'
              ? 'linux'
              : next.os_type
        next.rtc_offset = getRecommendedRTCOffset(guestType)
        next.video_model = getRecommendedVideoModel(guestType, next.arch)
        if (guestType !== 'windows' && next.memory_backend === 'virtio_mem') {
          next.memory_backend = 'balloon'
          Object.assign(next, recommendedMemoryDynamicValues('balloon', next.ram || 1))
        }
        return next
      })
      if (mode === 'template') ensureTemplateDefaults(selectedTemplate)
    },
    [ensureTemplateDefaults],
  )

  // ==================== 虚拟化引擎联动 ====================

  /** 切换引导类型 */
  const onBootTypeChange = useCallback((value: string) => {
    bootTypeTouchedRef.current = true
    setForm((prev) => {
      const next = { ...prev, boot_type: value }
      // 安全引导需要 Q35
      if (value === 'uefi-secure') next.machine_type = 'q35'
      return next
    })
  }, [])

  /** 切换机型（i440FX + Windows ISO 强制 BIOS） */
  const onMachineTypeChange = useCallback(
    (value: string) => {
      setForm((prev) => {
        const next = { ...prev, machine_type: value }
        if (shouldUseBIOSForI440FXWindows(next, isEdit)) next.boot_type = 'bios'
        return next
      })
    },
    [isEdit],
  )

  /** 切换虚拟化方案（KVM 回宿主机架构；QEMU 默认 x86_64） */
  const onVirtTypeChange = useCallback(
    (value: string) => {
      setForm((prev) => {
        const next = { ...prev, virt_type: value }
        if (value === 'kvm') {
          next.arch = hostArch
          if (hostArch === 'aarch64') {
            next.machine_type = 'virt'
            next.boot_type = 'uefi'
            next.video_model = 'ramfb'
          } else {
            next.machine_type = 'q35'
            next.boot_type = 'bios'
            next.video_model = getRecommendedVideoModel(next.os_type, next.arch)
          }
        } else {
          next.arch = 'x86_64'
          next.video_model = getRecommendedVideoModel(next.os_type, next.arch)
        }
        return next
      })
    },
    [hostArch],
  )

  /** 切换平台架构 */
  const onArchChange = useCallback((value: string) => {
    setForm((prev) => {
      const next = { ...prev, arch: value }
      if (value === 'aarch64') {
        next.machine_type = 'virt'
        next.boot_type = 'uefi'
        next.video_model = 'ramfb'
      } else if (value === 'riscv64') {
        next.machine_type = 'virt'
        next.boot_type = 'bios'
        next.video_model = getRecommendedVideoModel(next.os_type, next.arch)
      } else {
        next.machine_type = 'q35'
        next.video_model = getRecommendedVideoModel(next.os_type, next.arch)
      }
      return next
    })
  }, [])

  // ==================== 动态内存联动 ====================

  /** 应用动态内存推荐值（按当前后端类型） */
  const applyRecommendedMemoryDynamicValues = useCallback((spec?: number) => {
    setForm((prev) => {
      const base = spec ?? (isEdit ? prev.memory || prev.ram || 1 : prev.ram || 1)
      return { ...prev, ...recommendedMemoryDynamicValues(prev.memory_backend, base) }
    })
  }, [isEdit])

  const handleDynamicMemoryEnabledChange = useCallback(
    (enabled: boolean) => {
      setForm((prev) => {
        const next = { ...prev, memory_dynamic_enabled: enabled, memory_dynamic_touched: true }
        if (enabled) {
          const base = isEdit ? prev.memory || prev.ram || 1 : prev.ram || 1
          Object.assign(next, recommendedMemoryDynamicValues(prev.memory_backend, base))
        }
        return next
      })
    },
    [isEdit],
  )

  const handleMemoryBackendChange = useCallback(
    (backend: string) => {
      setForm((prev) => {
        const base = isEdit ? prev.memory || prev.ram || 1 : prev.ram || 1
        return {
          ...prev,
          memory_backend: backend,
          memory_dynamic_touched: true,
          ...recommendedMemoryDynamicValues(backend, base),
        }
      })
    },
    [isEdit],
  )

  /** 基础内存变化时，若已启用动态内存则同步推荐值 */
  const handleBaseMemoryChange = useCallback(() => {
    setForm((prev) => {
      if (!prev.memory_dynamic_enabled) return prev
      const base = isEdit ? prev.memory || 1 : prev.ram || 1
      return {
        ...prev,
        memory_dynamic_touched: true,
        ...recommendedMemoryDynamicValues(prev.memory_backend, base),
      }
    })
  }, [isEdit])

  /** 打开动态内存配置弹窗前兜底默认值 */
  const ensureMemoryDynamicDefaults = useCallback(() => {
    setForm((prev) => {
      const next = { ...prev }
      const base = isEdit ? prev.memory || prev.ram || 1 : prev.ram || 1
      if (!next.memory_initial || next.memory_initial < 1) next.memory_initial = base
      if (next.memory_backend === 'virtio_mem') {
        const initial = Math.max(1, Math.floor(base / 2))
        next.memory_initial = initial
        next.memory_min = initial
      } else if (!next.memory_min || next.memory_min < 1) {
        next.memory_min = Math.max(1, Math.floor(next.memory_initial / 2))
      }
      const recommendedMax = Math.max(base, Math.ceil(base * 1.3))
      if (!next.memory_max_dynamic || next.memory_max_dynamic < next.memory_initial) {
        next.memory_max_dynamic = recommendedMax
      }
      return next
    })
  }, [isEdit])

  // ==================== 编辑模式回填 ====================

  /** 应用虚拟机详情到表单（编辑模式打开/刷新时调用）
   * 返回回填后的完整表单（同步计算，调用方可直接用于快照捕获） */
  const applyEditVmDetail = useCallback(
    (
      detail: Partial<VmDetailInfo>,
      base?: Partial<VmFormModel>,
      row: Partial<VmListItem> & { os_type?: string } = {},
    ): VmFormModel => {
      const next = buildEditFormState(
        { ...form, ...(base || {}) } as VmFormModel,
        detail,
        row,
      )
      setForm(next)
      return next
    },
    [form],
  )

  /** 直接替换整个表单（快照捕获 / 重置场景） */
  const replaceForm = useCallback((next: VmFormModel) => {
    setForm(next)
  }, [])

  return {
    form,
    setField,
    patch,
    replaceForm,
    resetForCreate,
    bootTypeTouchedRef,
    // 派生
    isTemplateSourceMode,
    disableSystemInit,
    isWindowsTemplate,
    isFnOSTemplate,
    isOpenWrtTemplate,
    normalizedFnosDeviceId,
    hasCustomFnosDeviceId,
    shouldPreserveFnosDeviceId,
    isWindowsMemoryTarget,
    windowsElasticMemoryDisabled,
    i440fxWindowsBios,
    // 联动
    applySelectedTemplateSettings,
    ensureTemplateDefaults,
    onTemplateChange,
    onOsTypeChange,
    onISOChange,
    onCreateModeChange,
    onBootTypeChange,
    onMachineTypeChange,
    onVirtTypeChange,
    onArchChange,
    // 动态内存
    applyRecommendedMemoryDynamicValues,
    handleDynamicMemoryEnabledChange,
    handleMemoryBackendChange,
    handleBaseMemoryChange,
    ensureMemoryDynamicDefaults,
    // 编辑
    applyEditVmDetail,
  }
}

export type VmForm = ReturnType<typeof useVmForm>
