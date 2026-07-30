/**
 * 虚拟机表单默认值工厂与随机生成工具
 */
import type { GuestAgentPayload, Smbios1Payload } from '@/api/vm'
import type { VmCreateMode, VmFormModel } from './types'

/** 空 Guest Agent 配置 */
export function createEmptyGuestAgentConfig(): GuestAgentPayload {
  return { enabled: true }
}

/** 空 SMBIOS1 配置 */
export function createEmptySMBIOS1Config(): Smbios1Payload {
  return {
    base64: false,
    family: '',
    manufacturer: '',
    product: '',
    serial: '',
    sku: '',
    uuid: '',
    version: '',
  }
}

// ==================== 安全随机生成 ====================

const VM_NAME_CHARSET = 'abcdefghijklmnopqrstuvwxyz0123456789'

/** 浏览器安全随机整数 [0, max) */
export function getRandomInt(max: number): number {
  if (max <= 0) return 0
  const cryptoApi = globalThis.crypto
  if (cryptoApi && typeof cryptoApi.getRandomValues === 'function') {
    const randomValues = new Uint32Array(1)
    cryptoApi.getRandomValues(randomValues)
    return randomValues[0] % max
  }
  return Math.floor(Math.random() * max)
}

function randomStringFrom(charset: string, length: number): string {
  return Array.from({ length }, () => charset[getRandomInt(charset.length)]).join('')
}

/** 随机虚拟机名称：vm + 8 位随机字符 */
export function generateRandomVmName(): string {
  return `vm${randomStringFrom(VM_NAME_CHARSET, 8)}`
}

/** 随机主机名：vm- + 8 位随机字符 */
export function generateRandomHostname(): string {
  return `vm-${randomStringFrom(VM_NAME_CHARSET, 8)}`
}

// ==================== 默认值工厂 ====================

export interface CreateDefaultFormOptions {
  createMode?: VmCreateMode
  hostArch?: string
  spiceDefault?: boolean
  registration?: boolean
}

/** 创建模式表单默认值 */
export function createDefaultVmForm(options: CreateDefaultFormOptions = {}): VmFormModel {
  const {
    createMode = 'iso',
    hostArch = 'x86_64',
    spiceDefault = false,
    registration = false,
  } = options
  const isTemplateLike = createMode === 'template' || registration
  return {
    name: generateRandomVmName(),
    remark: '',
    vcpu: 2,
    memory: 2,
    ram: 2,
    create_mode: createMode,
    clone_mode: 'linked',
    system_init_enabled: true,
    batch_count: 1,
    os_type: 'linux',
    os_variant: '',
    disk_size: isTemplateLike ? 0 : 20,
    disk_format: 'qcow2',
    disk_bus: 'virtio',
    system_disk_iops_total: 0,
    system_disk_iops_read: 0,
    system_disk_iops_write: 0,
    storage_pool_id: '',
    extra_disks: [],
    add_disks: [],
    floppy_image: '',
    iso_path: '',
    iso_paths: [],
    disk_file: '',
    disk_path: '',
    disk_source_type: 'storage',
    copy_disk: false,
    start_after_import: true,
    extra_import_disks: [],
    import_os_category: '',
    import_user: '',
    import_password: '',
    hostname: isTemplateLike ? generateRandomHostname() : '',
    template_root_pass: '',
    template_user: '',
    appliance_file: '',
    appliance_path: '',
    appliance_source_type: 'storage',
    appliance_config_mode: 'ovf',
    copy_source: true,
    appliance_metadata: null,
    template: '',
    template_type: '',
    preserve_fnos_device_id: false,
    fnos_device_id_mode: 'regenerate',
    fnos_device_id: '',
    static_ip: '',
    gateway: '',
    dns: '',
    nic_model: 'virtio',
    switch_id: null,
    security_group_id: null,
    extra_nics: [],
    virt_type: 'kvm',
    arch: hostArch,
    machine_type: hostArch === 'aarch64' ? 'virt' : 'q35',
    boot_type: hostArch === 'aarch64' ? 'uefi' : 'bios',
    firmware_compat: false,
    direct_boot_enabled: false,
    direct_boot_cmdline: '',
    watchdog: 'none',
    autostart: false,
    boot_order: ['hd'],
    video_model: 'virtio',
    spice_enabled: spiceDefault,
    cpu_hotplug_enabled: false,
    cpu_limit_enabled: false,
    cpu_limit_percent: 100,
    cpu_affinity: '',
    cpu_topology_mode: 'auto',
    first_boot_reboot_mode: 'normal',
    memory_dynamic_enabled: false,
    memory_backend: 'balloon',
    memory_initial: 2,
    memory_min: 1,
    memory_max_dynamic: 3,
    memory_auto_balloon: true,
    memory_current: 0,
    memory_virtio_mem_current: 0,
    memory_dynamic_touched: false,
    memory_pending_apply: false,
    memory_compat_mode: 'legacy_static',
    memory_balloon_supported: false,
    memory_balloon_status: 'not_running',
    freeze: false,
    apic: true,
    pae: true,
    kvm_hidden: false,
    vendor_id: '',
    nested_virt: true,
    rtc_offset: 'utc',
    rtc_startdate: 'now',
    guest_agent: createEmptyGuestAgentConfig(),
    smbios1: createEmptySMBIOS1Config(),
    pcie_root_ports: 4,
    host_devices: [],
    host_devices_touched: false,
    traffic_down_gb: 0,
    traffic_up_gb: 0,
    bandwidth_down_mbps: 0,
    bandwidth_up_mbps: 0,
    max_port_forwards: 10,
    max_runtime_hours: 0,
  }
}
