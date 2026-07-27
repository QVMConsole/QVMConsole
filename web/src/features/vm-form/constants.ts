/**
 * 虚拟机表单常量选项（创建 / 编辑共用）
 */

/** 操作系统快捷选项 */
export const OS_QUICK_OPTIONS = [
  { value: 'linux', label: 'Linux', icon: '🐧', examples: 'Ubuntu / CentOS / Debian' },
  { value: 'windows', label: 'Windows', icon: '🪟', examples: 'Server 2022 / 2019 / 11' },
  { value: 'other', label: '其他', icon: '🖥️', examples: 'FreeBSD / 自定义' },
] as const

/** 磁盘总线选项 */
export const DISK_BUS_OPTIONS = [
  { value: 'virtio', label: 'VirtIO' },
  { value: 'scsi', label: 'SCSI' },
  { value: 'sata', label: 'SATA' },
  { value: 'ide', label: 'IDE' },
] as const

/** 磁盘格式选项 */
export const DISK_FORMAT_OPTIONS = [
  { value: 'qcow2', label: 'QCOW2（推荐）' },
  { value: 'raw', label: 'RAW' },
] as const

/** 网卡型号选项 */
export const NIC_MODEL_OPTIONS = [
  { value: 'virtio', label: 'VirtIO（推荐）', tag: '高性能', tagType: 'success' as const },
  { value: 'e1000e', label: 'e1000e (Intel)', tag: 'Intel', tagType: 'info' as const },
  { value: 'rtl8139', label: 'rtl8139', tag: '传统', tagType: 'warning' as const },
]

/** 显示设备选项 */
export const VIDEO_MODEL_OPTIONS = [
  { value: 'virtio', label: 'VirtIO（高性能）', tag: '推荐', tagType: 'success' as const },
  { value: 'ramfb', label: 'ramfb（ARM 兼容）', tag: 'ARM', tagType: 'danger' as const },
  { value: 'vga', label: 'VGA（兼容模式）', tag: '兼容', tagType: 'warning' as const },
  { value: 'vmvga', label: 'VMVGA（VMware 嵌套）', tag: '嵌套', tagType: 'primary' as const },
  { value: 'cirrus', label: 'Cirrus（保守排障）', tag: '排障', tagType: 'info' as const },
  { value: 'none', label: 'None（禁用虚拟显示）', tag: '无头', tagType: 'warning' as const },
]

/** 引导设备全集 */
export const ALL_BOOT_DEVICES = [
  { value: 'hd', label: '硬盘' },
  { value: 'cdrom', label: '光驱 (CD-ROM)' },
  { value: 'network', label: '网络 (PXE)' },
] as const

/** Watchdog 选项 */
export const WATCHDOG_OPTIONS = [
  { value: 'none', label: '不启用' },
  { value: 'i6300esb', label: 'i6300esb' },
  { value: 'itco', label: 'iTCO（推荐）' },
] as const

/** CPU 拓扑模式选项 */
export const CPU_TOPOLOGY_MODE_OPTIONS = [
  { value: 'auto', label: '自动（Windows 使用单插槽多核心）' },
  { value: 'single_socket', label: '单插槽多核心' },
  { value: 'host_default', label: '宿主默认拓扑' },
] as const

/** 首次重启模式选项（Windows 模板） */
export const FIRST_BOOT_REBOOT_MODE_OPTIONS = [
  { value: 'normal', label: '普通重启' },
  { value: 'cold', label: '宿主冷启动' },
] as const

/** 平台架构选项 */
export const ARCH_OPTIONS = [
  { value: 'x86_64', label: 'x86_64（默认）', tag: '默认', tagType: 'info' as const },
  { value: 'aarch64', label: 'aarch64 (ARM)', tag: 'ARM', tagType: 'success' as const },
  { value: 'riscv64', label: 'riscv64 (RISC-V)', tag: 'RISC-V', tagType: 'warning' as const },
]

/** Windows 模板固定用户名 */
export const WINDOWS_TEMPLATE_USERNAME = 'administrator'

/** 虚拟机名称 / 主机名规则 */
export const VM_NAME_PATTERN = /^[a-zA-Z0-9]([a-zA-Z0-9-]*[a-zA-Z0-9])?$/
export const HOSTNAME_PATTERN = /^[a-zA-Z0-9](?:[a-zA-Z0-9-]{0,61}[a-zA-Z0-9])?$/
export const TEMPLATE_USERNAME_PATTERN = /^[a-z_][a-z0-9_-]{0,31}$/
export const FNOS_DEVICE_ID_PATTERN = /^[0-9a-fA-F]{32}([0-9a-fA-F]{8})?$/

/** 帮助文案（高级选项 Tooltip） */
export const ADVANCED_HELP_TEXT = {
  freeze:
    '虚拟机启动时自动冻结CPU（使用监视器命令c可继续启动过程）。宿主机原生自启不会附带该行为。',
  apic: '控制虚拟机是否向来宾系统暴露 APIC（高级可编程中断控制器）。绝大多数系统都应保持启用。',
  pae: '控制虚拟机是否暴露 PAE（Physical Address Extension，物理地址扩展）。常见于 x86 老系统、32 位大内存兼容场景。',
  rtc: '控制虚拟机 RTC 硬件时钟使用 UTC 还是本地时间。Linux 通常建议使用 UTC，Windows 默认建议使用本地时间。',
  rtcStartDate:
    '默认使用 now。若填写固定时间，后端会将其转换为固定起始时间模式，支持 RFC3339、Unix 时间戳、YYYY-MM-DD HH:mm:ss 等格式。',
  guestAgent:
    '启用后会在虚拟机定义中添加 QEMU Guest Agent 通道，便于宿主机读取 VM 内部信息、做更可靠的关机与文件系统冻结协作。',
  smbiosBase64:
    '仅在需要兼容特殊字符或沿用外部配置时启用。开启后，厂商、产品、版本、序列号、SKU、家族名称、UUID 字段会先按 Base64 解码。',
  smbiosUUID:
    'libvirt 要求 SMBIOS UUID 与虚拟机 UUID 保持一致。新建时可显式指定，编辑已有虚拟机时建议保持当前值。',
} as const
