/**
 * 虚拟机表单统一数据模型
 * 创建向导（CreateVmWizard）与详情页编辑（EditVmForm）共用同一份模型、
 * 校验规则与联动逻辑，新增/调整字段只需改动本目录一处。
 */
import type { ApplianceMetadata } from '@/api/storage'
import type {
  AddDiskPayload,
  ExtraDiskPayload,
  ExtraImportDiskPayload,
  GuestAgentPayload,
  HostDevicePayload,
  Smbios1Payload,
  VmBootDevice,
} from '@/api/vm'

/** 创建方式 */
export type VmCreateMode = 'iso' | 'template' | 'import' | 'appliance'

/** 虚拟机包配置应用方式 */
export type ApplianceConfigMode = 'ovf' | 'custom'

/** 表单工作模式 */
export type VmFormMode = 'create' | 'edit'

/** 创建向导额外上下文：轻量云服务器登记模式 */
export interface RegistrationContext {
  enabled: boolean
  dedicated_vpc_switch_id: number
  dedicated_vpc_label: string
}

/** 创建模式额外磁盘（含 IOPS，随表单提交） */
export interface CreateExtraDisk extends ExtraDiskPayload {
  iops_total: number
  iops_read: number
  iops_write: number
}

/** 创建模式额外网口 */
export interface CreateExtraNic {
  nic_model: string
  switch_id: number | null
  security_group_id: number | null
}

/** 统一表单模型（创建 + 编辑字段全集） */
export interface VmFormModel {
  // ===== 基础信息 =====
  name: string
  remark: string
  vcpu: number
  memory: number // GB（编辑模式显示值）
  ram: number // GB（创建模式显示值）
  create_mode: VmCreateMode
  clone_mode: string // linked / full
  system_init_enabled: boolean
  batch_count: number

  // ===== 操作系统 =====
  os_type: string // linux / windows / other
  os_variant: string

  // ===== 磁盘与存储 =====
  disk_size: number // GB
  disk_format: string
  disk_bus: string
  system_disk_iops_total: number
  system_disk_iops_read: number
  system_disk_iops_write: number
  storage_pool_id: string
  extra_disks: CreateExtraDisk[]
  add_disks: AddDiskPayload[] // 编辑模式新增磁盘
  floppy_image: string

  // ===== ISO =====
  iso_path: string
  iso_paths: string[]

  // ===== 导入模式 =====
  disk_file: string
  disk_path: string
  disk_source_type: string // storage / path
  copy_disk: boolean
  start_after_import: boolean
  extra_import_disks: (ExtraImportDiskPayload & { iops_total: number; iops_read: number; iops_write: number })[]
  import_os_category: string
  import_user: string
  import_password: string
  hostname: string
  template_root_pass: string
  template_user: string

  // ===== 虚拟机包导入模式 =====
  appliance_file: string
  appliance_path: string
  appliance_source_type: string // storage / path
  appliance_config_mode: ApplianceConfigMode
  copy_source: boolean
  appliance_metadata: ApplianceMetadata | null

  // ===== 模板模式 =====
  template: string
  template_type: string
  preserve_fnos_device_id: boolean
  fnos_device_id_mode: string // regenerate / preserve / custom
  fnos_device_id: string
  static_ip: string
  gateway: string
  dns: string

  // ===== 网络 =====
  nic_model: string
  switch_id: number | null
  security_group_id: number | null
  /** 创建模式网口列表（第一个为主网口，空列表表示不配置网卡） */
  extra_nics: CreateExtraNic[]

  // ===== 虚拟化引擎 =====
  virt_type: string // kvm / qemu
  arch: string // x86_64 / aarch64 / riscv64
  machine_type: string // q35 / i440fx / virt
  boot_type: string // bios / uefi / uefi-secure
  firmware_compat: boolean
  direct_boot_enabled: boolean
  direct_boot_cmdline: string

  // ===== 系统行为 =====
  watchdog: string
  autostart: boolean
  boot_order: string[]

  // ===== 显示 =====
  video_model: string
  spice_enabled: boolean

  // ===== CPU / 内存高级 =====
  cpu_hotplug_enabled: boolean
  cpu_limit_enabled: boolean
  cpu_limit_percent: number
  cpu_affinity: string
  cpu_topology_mode: string
  first_boot_reboot_mode: string
  memory_dynamic_enabled: boolean
  memory_backend: string // balloon / virtio_mem
  memory_initial: number // GB
  memory_min: number // GB
  memory_max_dynamic: number // GB
  memory_auto_balloon: boolean
  memory_current: number // GB
  memory_virtio_mem_current: number // GB
  memory_dynamic_touched: boolean
  memory_pending_apply: boolean
  memory_compat_mode: string
  memory_balloon_supported: boolean
  memory_balloon_status: string

  // ===== 开发者选项 =====
  freeze: boolean
  apic: boolean
  pae: boolean
  kvm_hidden: boolean
  vendor_id: string
  nested_virt: boolean
  rtc_offset: string
  rtc_startdate: string
  guest_agent: GuestAgentPayload
  smbios1: Smbios1Payload

  // ===== 硬件 =====
  pcie_root_ports: number
  host_devices: HostDevicePayload[]
  host_devices_touched: boolean

  // ===== 轻量云配额（登记模式） =====
  traffic_down_gb: number
  traffic_up_gb: number
  bandwidth_down_mbps: number
  bandwidth_up_mbps: number
  max_port_forwards: number
  max_runtime_hours: number
}

/** 表单运行上下文（由壳层提供，Section 按此控制显隐与禁用） */
export interface VmFormContext {
  mode: VmFormMode
  isAdmin: boolean
  /** 编辑模式的虚拟机运行状态（running / shut off / paused） */
  vmStatus: string
  guestType: string
  guestAgentConnected?: boolean
  hostArch: string
  hostCores: number
  spiceSupported: boolean
  registration: RegistrationContext
  /** 编辑模式原始 vCPU（运行态下禁止减少） */
  editOrigVcpu?: number
  /** 编辑模式原始内存 GB（运行态下禁止减少） */
  editOrigMemory?: number
}

/** 编辑模式引导设备（Cockpit 风格，由详情 boot_devices 回填） */
export type EditBootDevice = VmBootDevice

/** 编辑模式快照（提交时仅发送变化字段） */
export interface EditFormSnapshot {
  vcpu: number
  max_vcpu: number
  memory: number
  autostart: boolean
  freeze: boolean
  apic: boolean
  pae: boolean
  rtc_offset: string
  rtc_startdate: string
  guest_agent: string
  smbios1: string
  boot_order: string
  device_order: string
  cpu_topology_mode: string
  video_model: string
  cpu_limit_percent: number | undefined
  cpu_affinity: string | null
  kvm_hidden: boolean
  vendor_id: string
  nested_virt: boolean
}

/** 编辑模式磁盘 IOPS 快照（按设备名索引） */
export type EditDiskIopsSnapshot = Record<
  string,
  { total_iops_sec: number; read_iops_sec: number; write_iops_sec: number }
>
