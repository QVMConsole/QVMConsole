/**
 * 提交载荷构建（创建三链路 + 编辑差异快照）
 * 创建向导与编辑表单共用，保证字段拼装规则只有一份。
 */
import type {
  AddDiskPayload,
  BatchCloneVmPayload,
  CloneVmPayload,
  CreateVmPayload,
  DiskIopsPayload,
  ExtraNicPayload,
  GuestAgentPayload,
  ImportVmPayload,
  MemoryDynamicPayload,
  Smbios1Payload,
  UpdateVmPayload,
  VmDiskItem,
} from '@/api/vm'
import { WINDOWS_TEMPLATE_USERNAME } from './constants'
import { normalizeRTCStartDate, validateCPUAffinityInput } from './recommend'
import type {
  CreateExtraNic,
  EditBootDevice,
  EditDiskIopsSnapshot,
  EditFormSnapshot,
  VmFormModel,
} from './types'

// ==================== 公共小载荷 ====================

export const buildGuestAgentPayload = (form: VmFormModel): GuestAgentPayload => ({
  enabled: !!form.guest_agent.enabled,
})

export const buildSMBIOS1Payload = (form: VmFormModel): Smbios1Payload => ({
  base64: !!form.smbios1.base64,
  family: (form.smbios1.family || '').trim(),
  manufacturer: (form.smbios1.manufacturer || '').trim(),
  product: (form.smbios1.product || '').trim(),
  serial: (form.smbios1.serial || '').trim(),
  sku: (form.smbios1.sku || '').trim(),
  uuid: (form.smbios1.uuid || '').trim(),
  version: (form.smbios1.version || '').trim(),
})

/** 动态内存载荷（创建：仅启用时；编辑：仅用户触碰过时） */
export const buildMemoryDynamicPayload = (
  form: VmFormModel,
  isEdit: boolean,
): MemoryDynamicPayload | undefined => {
  if (!isEdit && !form.memory_dynamic_enabled) return undefined
  if (isEdit && !form.memory_dynamic_touched) return undefined
  const baseMemory = isEdit ? form.memory || form.ram || 1 : form.ram || 1
  if (!form.memory_dynamic_enabled) {
    return {
      dynamic_enabled: false,
      memory_backend: form.memory_backend || 'balloon',
      memory_initial: baseMemory,
    }
  }
  return {
    dynamic_enabled: true,
    memory_backend: form.memory_backend || 'balloon',
    memory_initial: form.memory_initial,
    memory_min: form.memory_backend === 'virtio_mem' ? form.memory_initial : form.memory_min,
    memory_max: form.memory_max_dynamic,
    memory_auto_balloon: form.memory_backend === 'virtio_mem' ? false : !!form.memory_auto_balloon,
    memory_current: form.memory_current || 0,
  }
}

/** CPU 限制百分比（仅管理员；未启用返回 0） */
export const buildCPULimitPercentPayload = (
  form: VmFormModel,
  isAdmin: boolean,
): number | undefined => {
  if (!isAdmin) return undefined
  if (!form.cpu_limit_enabled) return 0
  const value = Number(form.cpu_limit_percent) || 0
  return Math.min(Math.max(value, 1), 100)
}

/** CPU 亲和性（仅管理员；非法返回 null 由调用方拦截） */
export const buildCPUAffinityPayload = (form: VmFormModel, isAdmin: boolean): string | null => {
  if (!isAdmin) return ''
  const trimmed = (form.cpu_affinity || '').trim()
  if (!trimmed) return ''
  if (!validateCPUAffinityInput(trimmed)) return null
  return trimmed
}

/** CPU 热添加上限（启用时为宿主机核心数，否则 0） */
export const buildCpuHotplugMaxVCPU = (form: VmFormModel, hostCores: number): number =>
  form.cpu_hotplug_enabled && hostCores > 0 ? hostCores : 0

/** 系统盘 IOPS（仅管理员且设置了任一项） */
export const buildSystemDiskIopsPayload = (
  form: VmFormModel,
  isAdmin: boolean,
): DiskIopsPayload | undefined => {
  if (
    !isAdmin ||
    (form.system_disk_iops_total <= 0 &&
      form.system_disk_iops_read <= 0 &&
      form.system_disk_iops_write <= 0)
  ) {
    return undefined
  }
  return {
    total_iops_sec: form.system_disk_iops_total,
    read_iops_sec: form.system_disk_iops_read,
    write_iops_sec: form.system_disk_iops_write,
  }
}

/** 网口载荷：第一个网口作为主网口，其余为额外网口 */
export const buildAllNicsPayload = (
  extraNics: CreateExtraNic[],
): { primarySwitchId: number; primarySecurityGroupId: number; extraNics: ExtraNicPayload[] } => {
  const validNics = extraNics.filter((n) => n.switch_id)
  if (validNics.length === 0) {
    return { primarySwitchId: 0, primarySecurityGroupId: 0, extraNics: [] }
  }
  const first = validNics[0]
  const rest = validNics.slice(1).map((n) => ({
    switch_id: n.switch_id as number,
    security_group_id: n.security_group_id || 0,
    nic_model: n.nic_model || 'virtio',
  }))
  return {
    primarySwitchId: first.switch_id as number,
    primarySecurityGroupId: first.security_group_id || 0,
    extraNics: rest,
  }
}

/** 有效 SPICE 开关（禁用显示设备时强制关闭） */
export const getEffectiveSpiceEnabled = (form: VmFormModel): boolean =>
  form.video_model === 'none' ? false : form.spice_enabled

const buildExtraDisksPayload = (form: VmFormModel) =>
  form.extra_disks
    .filter((d) => d.size > 0)
    .map((d) => ({
      size: d.size,
      format: d.format,
      bus: d.bus,
      storage_pool_id: d.storage_pool_id,
      iops_total: d.iops_total || 0,
      iops_read: d.iops_read || 0,
      iops_write: d.iops_write || 0,
      guest_mount: d.guest_mount,
    }))

// ==================== 创建链路（ISO 安装） ====================

export interface CreateBuildContext {
  isAdmin: boolean
  hostCores: number
}

export const buildCreatePayload = (
  form: VmFormModel,
  ctx: CreateBuildContext,
): CreateVmPayload => {
  const nics = buildAllNicsPayload(form.extra_nics)
  const payload: CreateVmPayload = {
    name: form.name,
    remark: form.remark,
    vcpu: form.vcpu,
    max_vcpu: buildCpuHotplugMaxVCPU(form, ctx.hostCores),
    ram: form.ram,
    disk_size: form.disk_size,
    disk_format: form.disk_format,
    disk_bus: form.disk_bus,
    system_disk_iops: buildSystemDiskIopsPayload(form, ctx.isAdmin),
    os_variant: form.os_variant,
    iso_path: form.iso_path,
    iso_paths: form.iso_paths.filter(Boolean),
    floppy_image: form.floppy_image || '',
    switch_id: nics.primarySwitchId,
    security_group_id: nics.primarySecurityGroupId,
    storage_pool_id: form.storage_pool_id,
    nic_model: form.nic_model,
    autostart: form.autostart,
    freeze: form.freeze,
    apic: !!form.apic,
    pae: !!form.pae,
    rtc_offset: form.rtc_offset,
    rtc_startdate: normalizeRTCStartDate(form.rtc_startdate),
    guest_agent: buildGuestAgentPayload(form),
    smbios1: buildSMBIOS1Payload(form),
    os_type: form.os_type,
    machine_type: form.machine_type,
    boot_type: form.boot_type,
    watchdog: form.watchdog,
    boot_order: form.boot_order,
    video_model: form.video_model,
    spice_enabled: getEffectiveSpiceEnabled(form),
    cpu_topology_mode: form.cpu_topology_mode,
    virt_type: form.virt_type,
    arch: form.virt_type === 'qemu' ? form.arch : undefined,
    pcie_root_ports: form.machine_type === 'q35' ? form.pcie_root_ports : undefined,
    extra_disks: buildExtraDisksPayload(form),
    host_devices: form.host_devices,
    extra_nics: nics.extraNics,
    firmware_compat: form.arch === 'aarch64' && form.firmware_compat ? true : undefined,
    direct_boot: form.direct_boot_enabled
      ? { enabled: true, cmdline: form.direct_boot_cmdline || '' }
      : undefined,
    kvm_hidden: form.kvm_hidden || undefined,
    vendor_id: form.vendor_id || undefined,
    nested_virt: form.nested_virt !== undefined ? form.nested_virt : true,
  }
  const cpuLimitPercent = buildCPULimitPercentPayload(form, ctx.isAdmin)
  if (cpuLimitPercent !== undefined) payload.cpu_limit_percent = cpuLimitPercent
  if (ctx.isAdmin) payload.cpu_affinity = (form.cpu_affinity || '').trim()
  const memoryPayload = buildMemoryDynamicPayload(form, false)
  if (memoryPayload) payload.memory_dynamic = memoryPayload
  return payload
}

// ==================== 克隆链路（模板单台 + 批量） ====================

export interface CloneBuildContext extends CreateBuildContext {
  isWindowsTemplate: boolean
  isOpenWrtTemplate: boolean
  shouldPreserveFnosDeviceId: boolean
  customFnosDeviceId: string
}

const buildCloneSharedFields = (form: VmFormModel, ctx: CloneBuildContext) => {
  const nics = buildAllNicsPayload(form.extra_nics)
  const initUser = form.system_init_enabled
    ? ctx.isWindowsTemplate
      ? WINDOWS_TEMPLATE_USERNAME
      : form.import_user.trim()
    : ''
  return {
    nics,
    initUser,
    base: {
      template: form.template,
      template_type: form.template_type,
      clone_mode: form.clone_mode,
      vcpu: form.vcpu,
      max_vcpu: buildCpuHotplugMaxVCPU(form, ctx.hostCores),
      ram: form.ram,
      disk_size: form.disk_size,
      user: initUser,
      password: form.system_init_enabled ? form.import_password : '',
      disable_system_init: !form.system_init_enabled || undefined,
      autostart: form.autostart,
      freeze: form.freeze,
      apic: !!form.apic,
      pae: !!form.pae,
      rtc_offset: form.rtc_offset,
      rtc_startdate: normalizeRTCStartDate(form.rtc_startdate),
      guest_agent: buildGuestAgentPayload(form),
      smbios1: buildSMBIOS1Payload(form),
      uefi: form.boot_type === 'uefi' || form.boot_type === 'uefi-secure',
      disk_bus: form.disk_bus,
      nic_model: form.nic_model,
      video_model: form.video_model,
      spice_enabled: getEffectiveSpiceEnabled(form),
      storage_pool_id: form.storage_pool_id,
      cpu_topology_mode: form.cpu_topology_mode,
      first_boot_reboot_mode: form.first_boot_reboot_mode,
      switch_id: nics.primarySwitchId,
      security_group_id: nics.primarySecurityGroupId,
      extra_nics: nics.extraNics,
      static_ip: ctx.isOpenWrtTemplate ? form.static_ip : undefined,
      gateway: ctx.isOpenWrtTemplate ? form.gateway : undefined,
      dns: ctx.isOpenWrtTemplate ? form.dns : undefined,
      kvm_hidden: form.kvm_hidden || undefined,
      vendor_id: form.vendor_id || undefined,
      nested_virt: form.nested_virt !== undefined ? form.nested_virt : true,
    },
  }
}

export const buildClonePayload = (form: VmFormModel, ctx: CloneBuildContext): CloneVmPayload => {
  const { base, initUser } = buildCloneSharedFields(form, ctx)
  const payload: CloneVmPayload = {
    ...base,
    name: form.name,
    remark: form.remark,
    hostname: form.system_init_enabled ? form.hostname : '',
    system_disk_iops: buildSystemDiskIopsPayload(form, ctx.isAdmin),
    preserve_fnos_device_id: ctx.shouldPreserveFnosDeviceId,
    fnos_device_id: ctx.customFnosDeviceId,
    extra_disks: buildExtraDisksPayload(form),
    host_devices: form.host_devices,
    pcie_root_ports: form.pcie_root_ports,
  }
  // 单台克隆 user/template_user 同源
  payload.user = initUser
  const cpuLimitPercent = buildCPULimitPercentPayload(form, ctx.isAdmin)
  if (cpuLimitPercent !== undefined) payload.cpu_limit_percent = cpuLimitPercent
  if (ctx.isAdmin) payload.cpu_affinity = (form.cpu_affinity || '').trim()
  const memoryPayload = buildMemoryDynamicPayload(form, false)
  if (memoryPayload) payload.memory_dynamic = memoryPayload
  return payload
}

export const buildBatchClonePayload = (
  form: VmFormModel,
  ctx: CloneBuildContext,
): BatchCloneVmPayload => {
  const { base, initUser } = buildCloneSharedFields(form, ctx)
  const payload: BatchCloneVmPayload = {
    ...base,
    uefi: base.uefi ? true : undefined,
    prefix: form.name,
    start_num: 1,
    count: form.batch_count,
    hostname: '', // 批量模式每台由后端自动生成独立主机名
    template_user: initUser,
    extra_disks: buildExtraDisksPayload(form),
  }
  const cpuLimitPercent = buildCPULimitPercentPayload(form, ctx.isAdmin)
  if (cpuLimitPercent !== undefined) payload.cpu_limit_percent = cpuLimitPercent
  if (ctx.isAdmin) payload.cpu_affinity = (form.cpu_affinity || '').trim()
  return payload
}

// ==================== 导入链路 ====================

export const buildImportPayload = (form: VmFormModel, ctx: CreateBuildContext): ImportVmPayload => {
  const nics = buildAllNicsPayload(form.extra_nics)
  const payload: ImportVmPayload = {
    name: form.name,
    remark: form.remark,
    vcpu: form.vcpu,
    max_vcpu: buildCpuHotplugMaxVCPU(form, ctx.hostCores),
    ram: form.ram,
    switch_id: nics.primarySwitchId,
    security_group_id: nics.primarySecurityGroupId,
    copy_disk: form.copy_disk,
    hostname: form.system_init_enabled ? form.hostname || form.name : '',
    user: form.system_init_enabled ? form.import_user : '',
    password: form.system_init_enabled ? form.import_password : '',
    init_type: form.system_init_enabled ? form.os_type : '',
    template_root_pass: form.template_root_pass,
    template_user: form.template_user,
    autostart: form.autostart,
    freeze: form.freeze,
    start_after_import: form.start_after_import,
    apic: !!form.apic,
    pae: !!form.pae,
    rtc_offset: form.rtc_offset,
    rtc_startdate: normalizeRTCStartDate(form.rtc_startdate),
    guest_agent: buildGuestAgentPayload(form),
    smbios1: buildSMBIOS1Payload(form),
    boot_type: form.boot_type,
    machine_type: form.machine_type,
    nic_model: form.nic_model,
    video_model: form.video_model,
    spice_enabled: getEffectiveSpiceEnabled(form),
    cpu_topology_mode: form.cpu_topology_mode,
    first_boot_reboot_mode: form.first_boot_reboot_mode,
    extra_nics: nics.extraNics,
    kvm_hidden: form.kvm_hidden || undefined,
    vendor_id: form.vendor_id || undefined,
    nested_virt: form.nested_virt !== undefined ? form.nested_virt : true,
  }
  if (ctx.isAdmin && form.disk_source_type === 'path') {
    payload.disk_path = form.disk_path
    payload.disk_source_type = 'path'
    payload.storage_pool_id = form.storage_pool_id
    payload.extra_import_disks = form.extra_import_disks
      .filter((d) => d.disk_path || d.disk_file)
      .map((d) => ({
        disk_path: d.disk_path,
        disk_file: d.disk_file,
        disk_source_type: d.disk_source_type,
        storage_pool_id: d.storage_pool_id,
        copy_disk: d.copy_disk,
        bus: d.bus,
        iops_total: d.iops_total || 0,
        iops_read: d.iops_read || 0,
        iops_write: d.iops_write || 0,
      }))
    payload.system_disk_iops = buildSystemDiskIopsPayload(form, ctx.isAdmin)
  } else {
    payload.disk_file = form.disk_file
  }
  const cpuLimitPercent = buildCPULimitPercentPayload(form, ctx.isAdmin)
  if (cpuLimitPercent !== undefined) payload.cpu_limit_percent = cpuLimitPercent
  if (ctx.isAdmin) payload.cpu_affinity = (form.cpu_affinity || '').trim()
  const memoryPayload = buildMemoryDynamicPayload(form, false)
  if (memoryPayload) payload.memory_dynamic = memoryPayload
  return payload
}

// ==================== 编辑链路（差异快照提交） ====================

/** 从引导设备列表生成 boot_order（去重类型） */
export const buildBootOrderFromDevices = (devices: EditBootDevice[]): string[] => {
  const order: string[] = []
  const seen = new Set<string>()
  for (const dev of devices) {
    if (!dev.enabled) continue
    const key = dev.type === 'cdrom' ? 'cdrom' : dev.type === 'network' ? 'network' : 'hd'
    if (!seen.has(key)) {
      order.push(key)
      seen.add(key)
    }
  }
  return order.length > 0 ? order : ['hd']
}

/** 从引导设备列表生成设备级排序（多光驱先后） */
export const buildDeviceOrderFromDevices = (devices: EditBootDevice[]): string[] => {
  const order: string[] = []
  for (const dev of devices) {
    if (!dev.enabled) continue
    if (dev.device) order.push(dev.device)
  }
  return order
}

export interface EditBuildContext {
  isAdmin: boolean
  vmStatus: string
  hostCores: number
  snapshot: EditFormSnapshot | null
  diskIopsSnapshot: EditDiskIopsSnapshot
  editDisks: VmDiskItem[]
  editBootDevices: EditBootDevice[]
  origNicModel: string
  origBootType: string
  origPcieRootPorts: number
}

/**
 * 构建编辑提交载荷：仅发送与快照相比发生变化的字段，
 * 避免后端对未变化字段执行无谓的 virsh XML 读写操作。
 */
export const buildEditPayload = (form: VmFormModel, ctx: EditBuildContext): UpdateVmPayload => {
  const payload: UpdateVmPayload = {
    add_disks: form.add_disks.filter((d: AddDiskPayload) => d.size > 0),
  }
  const snap = ctx.snapshot || ({} as Partial<EditFormSnapshot>)
  const running = ctx.vmStatus === 'running'
  const runningOrPaused = running || ctx.vmStatus === 'paused'

  if (form.vcpu !== snap.vcpu) payload.vcpu = form.vcpu
  const maxVcpu = buildCpuHotplugMaxVCPU(form, ctx.hostCores)
  if (maxVcpu !== snap.max_vcpu) payload.max_vcpu = maxVcpu
  if (form.memory !== snap.memory) payload.memory = form.memory
  if (form.autostart !== snap.autostart) payload.autostart = form.autostart
  if (form.freeze !== snap.freeze) payload.freeze = form.freeze
  if (!!form.apic !== snap.apic) payload.apic = !!form.apic
  if (!!form.pae !== snap.pae) payload.pae = !!form.pae
  if (form.kvm_hidden !== snap.kvm_hidden) payload.kvm_hidden = form.kvm_hidden
  const curVendorID = form.vendor_id || ''
  if (curVendorID !== snap.vendor_id) payload.vendor_id = curVendorID
  if (!!form.nested_virt !== snap.nested_virt) payload.nested_virt = !!form.nested_virt

  const curRtcStartdate = normalizeRTCStartDate(form.rtc_startdate)
  if (form.rtc_offset !== snap.rtc_offset || curRtcStartdate !== snap.rtc_startdate) {
    payload.rtc_offset = form.rtc_offset
    payload.rtc_startdate = curRtcStartdate
  }

  const curGuestAgent = buildGuestAgentPayload(form)
  if (JSON.stringify(curGuestAgent) !== snap.guest_agent) payload.guest_agent = curGuestAgent
  const curSmbios1 = buildSMBIOS1Payload(form)
  if (JSON.stringify(curSmbios1) !== snap.smbios1) payload.smbios1 = curSmbios1

  const computedBootOrder =
    ctx.editBootDevices.length > 0 ? buildBootOrderFromDevices(ctx.editBootDevices) : form.boot_order
  const computedDeviceOrder =
    ctx.editBootDevices.length > 0 ? buildDeviceOrderFromDevices(ctx.editBootDevices) : []
  if (JSON.stringify(computedBootOrder) !== snap.boot_order) payload.boot_order = computedBootOrder
  if (JSON.stringify(computedDeviceOrder) !== snap.device_order) {
    payload.device_order = computedDeviceOrder
  }

  if (form.machine_type === 'q35' && form.pcie_root_ports !== ctx.origPcieRootPorts) {
    payload.pcie_root_ports = form.pcie_root_ports
  }
  if (form.arch === 'aarch64') payload.firmware_compat = !!form.firmware_compat
  payload.direct_boot = form.direct_boot_enabled
    ? { enabled: true, cmdline: form.direct_boot_cmdline || '' }
    : { enabled: false }

  if (ctx.isAdmin && form.host_devices_touched) payload.host_devices = form.host_devices

  // 磁盘 IOPS：仅发送发生变化的磁盘（仅管理员）
  if (ctx.isAdmin) {
    const diskIops: Record<string, DiskIopsPayload> = {}
    ctx.editDisks.forEach((disk) => {
      if (!disk.device) return
      const pending = disk as VmDiskItem & {
        _iops_total?: number
        _iops_read?: number
        _iops_write?: number
      }
      const total = pending._iops_total !== undefined ? pending._iops_total : disk.iops_total?.value || 0
      const read = pending._iops_read !== undefined ? pending._iops_read : disk.iops_read?.value || 0
      const write = pending._iops_write !== undefined ? pending._iops_write : disk.iops_write?.value || 0
      const orig = ctx.diskIopsSnapshot[disk.device] || {
        total_iops_sec: 0,
        read_iops_sec: 0,
        write_iops_sec: 0,
      }
      if (
        total !== orig.total_iops_sec ||
        read !== orig.read_iops_sec ||
        write !== orig.write_iops_sec
      ) {
        diskIops[disk.device] = {
          total_iops_sec: total || 0,
          read_iops_sec: read || 0,
          write_iops_sec: write || 0,
        }
      }
    })
    if (Object.keys(diskIops).length > 0) payload.disk_iops = diskIops
  }

  const cpuLimitPercent = buildCPULimitPercentPayload(form, ctx.isAdmin)
  if (ctx.isAdmin && cpuLimitPercent !== snap.cpu_limit_percent) {
    payload.cpu_limit_percent = cpuLimitPercent
  }
  if (ctx.isAdmin) {
    const curAffinity = (form.cpu_affinity || '').trim()
    if (curAffinity !== snap.cpu_affinity) payload.cpu_affinity = curAffinity
  }
  if (form.cpu_topology_mode && !runningOrPaused && form.cpu_topology_mode !== snap.cpu_topology_mode) {
    payload.cpu_topology_mode = form.cpu_topology_mode
  }
  if (form.nic_model && form.nic_model !== ctx.origNicModel) payload.nic_model = form.nic_model
  if (form.boot_type && form.boot_type !== ctx.origBootType) payload.boot_type = form.boot_type
  if (form.video_model && !runningOrPaused && form.video_model !== snap.video_model) {
    payload.video_model = form.video_model
  }
  const memoryPayload = buildMemoryDynamicPayload(form, true)
  if (memoryPayload) payload.memory_dynamic = memoryPayload
  return payload
}

/** 捕获编辑表单快照（详情回填完成后调用） */
export const captureEditFormSnapshot = (
  form: VmFormModel,
  hostCores: number,
  isAdmin: boolean,
  editBootDevices: EditBootDevice[],
): EditFormSnapshot => ({
  vcpu: form.vcpu,
  max_vcpu: buildCpuHotplugMaxVCPU(form, hostCores),
  memory: form.memory,
  autostart: form.autostart,
  freeze: form.freeze,
  apic: !!form.apic,
  pae: !!form.pae,
  rtc_offset: form.rtc_offset,
  rtc_startdate: normalizeRTCStartDate(form.rtc_startdate),
  guest_agent: JSON.stringify(buildGuestAgentPayload(form)),
  smbios1: JSON.stringify(buildSMBIOS1Payload(form)),
  boot_order: JSON.stringify(
    editBootDevices.length > 0 ? buildBootOrderFromDevices(editBootDevices) : form.boot_order,
  ),
  device_order: JSON.stringify(
    editBootDevices.length > 0 ? buildDeviceOrderFromDevices(editBootDevices) : [],
  ),
  cpu_topology_mode: form.cpu_topology_mode || '',
  video_model: form.video_model || '',
  cpu_limit_percent: buildCPULimitPercentPayload(form, isAdmin),
  cpu_affinity: isAdmin ? (form.cpu_affinity || '').trim() : null,
  kvm_hidden: form.kvm_hidden,
  vendor_id: form.vendor_id || '',
  nested_virt: !!form.nested_virt,
})

/** 捕获磁盘 IOPS 快照（磁盘列表加载完成后调用，仅管理员） */
export const captureEditDiskIopsSnapshot = (
  disks: VmDiskItem[],
  isAdmin: boolean,
): EditDiskIopsSnapshot => {
  const snap: EditDiskIopsSnapshot = {}
  if (!isAdmin) return snap
  disks.forEach((disk) => {
    if (!disk.device) return
    snap[disk.device] = {
      total_iops_sec: disk.iops_total?.value || 0,
      read_iops_sec: disk.iops_read?.value || 0,
      write_iops_sec: disk.iops_write?.value || 0,
    }
  })
  return snap
}
