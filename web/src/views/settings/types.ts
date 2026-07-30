/**
 * 系统设置页表单类型、默认值、校验与提交负载构造
 * 字段与后端 /settings 接口保持一致（迁移自旧前端 views/settings/index.vue）
 */

/** 维护服务列表默认值 */
export const DEFAULT_MAINTENANCE_SERVICE_UNITS =
  'kvm-console.service,libvirtd.service,libvirtd.socket,libvirtd-ro.socket,libvirtd-admin.socket'

/** 设置页 Tab 键位（支持 ?tab=xxx 直接定位） */
export const VALID_SETTINGS_TABS = [
  'basic',
  'network',
  'host',
  'advanced',
  'security',
  'log',
  'diagnostics',
  'storage',
] as const

export type SettingsTabKey = (typeof VALID_SETTINGS_TABS)[number]

/** 系统设置表单（常规配置，保存后立即生效并持久化） */
export interface SettingsForm {
  port: number
  template_dir: string
  template_import_dir: string
  template_export_dir: string
  appliance_temp_dir: string
  clone_dir: string
  iso_dir: string
  default_network: string
  network_backend: string
  ovs_bridge: string
  ovs_uplink: string
  ovs_dhcp_start: string
  ovs_dhcp_end: string
  subnet_prefix: string
  auto_port_start: number
  auto_port_end: number
  port_forward_dir: string
  host_ip: string
  external_nic: string
  max_burst_inbound: number
  max_burst_outbound: number
  default_disk_iops_total: number
  default_disk_iops_read: number
  default_disk_iops_write: number
  batch_clone_max_concurrency: number
  dynamic_memory_scheduler_enabled: boolean
  dynamic_memory_interval_seconds: number
  dynamic_memory_host_reserve_mb: number
  dynamic_memory_host_reserve_percent: number
  dynamic_memory_increase_threshold_percent: number
  dynamic_memory_reclaim_threshold_percent: number
  dynamic_memory_cooldown_seconds: number
  dynamic_memory_observation_hours: number
  scheduler_event_retention_hours: number
  rescue_iso: string
  public_base_url: string
  site_title: string
  development_mode: boolean
  session_fingerprint_enabled: boolean
  request_filter_enabled: boolean
  password_breach_check_enabled: boolean
  scheduled_password_breach_check_enabled: boolean
  maintenance_mode: boolean
  maintenance_service_units: string
  maintenance_vm_shutdown_timeout_seconds: number
  smtp_host: string
  smtp_port: number
  smtp_username: string
  smtp_password: string
  smtp_from_name: string
  smtp_from_address: string
  smtp_security: string
  smtp_timeout_seconds: number
  smtp_password_configured: boolean
  smtp_configured: boolean
  smtp_test_email: string
  jwt_secret_rotate_hours: number
  jwt_secret_last_rotated: string
  log_max_backups: number
  network_wait_online_disabled: boolean
  network_wait_online_summary: string
  spice_enabled_by_default: boolean
  igpu_passthrough_enabled: boolean
  hardware_passthrough_enabled: boolean
}

/** 表单默认值（后端未返回字段时兜底） */
export const DEFAULT_SETTINGS_FORM: SettingsForm = {
  port: 8080,
  template_dir: '',
  template_import_dir: '',
  template_export_dir: '',
  appliance_temp_dir: '',
  clone_dir: '',
  iso_dir: '/var/lib/libvirt/images/ISO',
  default_network: '',
  network_backend: 'ovs',
  ovs_bridge: 'br-ovs',
  ovs_uplink: '',
  ovs_dhcp_start: '',
  ovs_dhcp_end: '',
  subnet_prefix: '',
  auto_port_start: 10000,
  auto_port_end: 20000,
  port_forward_dir: '',
  host_ip: '',
  external_nic: '',
  max_burst_inbound: 0,
  max_burst_outbound: 0,
  default_disk_iops_total: 0,
  default_disk_iops_read: 0,
  default_disk_iops_write: 0,
  batch_clone_max_concurrency: 10,
  dynamic_memory_scheduler_enabled: true,
  dynamic_memory_interval_seconds: 30,
  dynamic_memory_host_reserve_mb: 2048,
  dynamic_memory_host_reserve_percent: 20,
  dynamic_memory_increase_threshold_percent: 15,
  dynamic_memory_reclaim_threshold_percent: 35,
  dynamic_memory_cooldown_seconds: 120,
  dynamic_memory_observation_hours: 24,
  scheduler_event_retention_hours: 168,
  rescue_iso: '',
  public_base_url: '',
  site_title: 'QVMConsole',
  development_mode: false,
  session_fingerprint_enabled: true,
  request_filter_enabled: true,
  password_breach_check_enabled: true,
  scheduled_password_breach_check_enabled: true,
  maintenance_mode: false,
  maintenance_service_units: DEFAULT_MAINTENANCE_SERVICE_UNITS,
  maintenance_vm_shutdown_timeout_seconds: 40,
  smtp_host: '',
  smtp_port: 587,
  smtp_username: '',
  smtp_password: '',
  smtp_from_name: 'QVMConsole',
  smtp_from_address: '',
  smtp_security: 'starttls',
  smtp_timeout_seconds: 15,
  smtp_password_configured: false,
  smtp_configured: false,
  smtp_test_email: '',
  jwt_secret_rotate_hours: 24,
  jwt_secret_last_rotated: '',
  log_max_backups: 0,
  network_wait_online_disabled: false,
  network_wait_online_summary: '',
  spice_enabled_by_default: false,
  igpu_passthrough_enabled: false,
  hardware_passthrough_enabled: false,
}

/** 保存前校验，返回第一条错误信息；通过时返回 null */
export function validateSettingsForm(form: SettingsForm): string | null {
  if (form.auto_port_start >= form.auto_port_end) return '端口起始值必须小于结束值'
  if (form.auto_port_start < 1024 || form.auto_port_end > 65535) return '端口范围: 1024 - 65535'
  if (form.smtp_port < 1 || form.smtp_port > 65535) return 'SMTP 端口范围: 1 - 65535'
  if (form.smtp_timeout_seconds < 5) return 'SMTP 超时时间不能小于 5 秒'
  if (form.dynamic_memory_interval_seconds < 10) return '动态内存调度间隔不能小于 10 秒'
  if (form.dynamic_memory_host_reserve_mb < 512) return '宿主机保留内存不能小于 512MB'
  if (form.dynamic_memory_host_reserve_percent < 5 || form.dynamic_memory_host_reserve_percent > 80)
    return '宿主机保留比例需在 5% - 80% 之间'
  if (
    form.dynamic_memory_increase_threshold_percent < 5 ||
    form.dynamic_memory_increase_threshold_percent > 50
  )
    return '增长触发阈值需在 5% - 50% 之间'
  if (
    form.dynamic_memory_reclaim_threshold_percent < 10 ||
    form.dynamic_memory_reclaim_threshold_percent > 90
  )
    return '回收触发阈值需在 10% - 90% 之间'
  if (form.dynamic_memory_cooldown_seconds < 30) return '动态内存冷却时间不能小于 30 秒'
  if (form.dynamic_memory_observation_hours < 0 || form.dynamic_memory_observation_hours > 168)
    return '观察期需在 0 - 168 小时之间'
  if (form.scheduler_event_retention_hours < 1 || form.scheduler_event_retention_hours > 2160)
    return '调度事件保留时长需在 1 - 2160 小时之间'
  if (
    form.maintenance_vm_shutdown_timeout_seconds < 5 ||
    form.maintenance_vm_shutdown_timeout_seconds > 3600
  )
    return '维护模式虚拟机关机等待时间需在 5 - 3600 秒之间'
  return null
}

/** 构造保存负载（只提交可写字段） */
export function buildSettingsPayload(form: SettingsForm): Record<string, unknown> {
  return {
    template_dir: form.template_dir,
    template_import_dir: form.template_import_dir,
    template_export_dir: form.template_export_dir,
    appliance_temp_dir: form.appliance_temp_dir,
    clone_dir: form.clone_dir,
    iso_dir: form.iso_dir,
    default_network: form.default_network,
    network_backend: form.network_backend || 'ovs',
    ovs_bridge: form.ovs_bridge,
    ovs_uplink: form.ovs_uplink,
    ovs_dhcp_start: form.ovs_dhcp_start,
    ovs_dhcp_end: form.ovs_dhcp_end,
    subnet_prefix: form.subnet_prefix,
    auto_port_start: form.auto_port_start,
    auto_port_end: form.auto_port_end,
    host_ip: form.host_ip,
    external_nic: form.external_nic,
    max_burst_inbound: form.max_burst_inbound,
    max_burst_outbound: form.max_burst_outbound,
    default_disk_iops_total: form.default_disk_iops_total,
    default_disk_iops_read: form.default_disk_iops_read,
    default_disk_iops_write: form.default_disk_iops_write,
    dynamic_memory_scheduler_enabled: form.dynamic_memory_scheduler_enabled,
    dynamic_memory_interval_seconds: form.dynamic_memory_interval_seconds,
    dynamic_memory_host_reserve_mb: form.dynamic_memory_host_reserve_mb,
    dynamic_memory_host_reserve_percent: form.dynamic_memory_host_reserve_percent,
    dynamic_memory_increase_threshold_percent: form.dynamic_memory_increase_threshold_percent,
    dynamic_memory_reclaim_threshold_percent: form.dynamic_memory_reclaim_threshold_percent,
    dynamic_memory_cooldown_seconds: form.dynamic_memory_cooldown_seconds,
    dynamic_memory_observation_hours: form.dynamic_memory_observation_hours,
    scheduler_event_retention_hours: form.scheduler_event_retention_hours,
    rescue_iso: form.rescue_iso,
    public_base_url: form.public_base_url,
    site_title: form.site_title?.trim() || 'QVMConsole',
    development_mode: form.development_mode,
    session_fingerprint_enabled: form.session_fingerprint_enabled,
    request_filter_enabled: form.request_filter_enabled,
    password_breach_check_enabled: form.password_breach_check_enabled,
    scheduled_password_breach_check_enabled: form.scheduled_password_breach_check_enabled,
    maintenance_mode: form.maintenance_mode,
    maintenance_service_units:
      form.maintenance_service_units?.trim() || DEFAULT_MAINTENANCE_SERVICE_UNITS,
    maintenance_vm_shutdown_timeout_seconds: form.maintenance_vm_shutdown_timeout_seconds,
    smtp_host: form.smtp_host,
    smtp_port: form.smtp_port,
    smtp_username: form.smtp_username,
    smtp_password: form.smtp_password,
    smtp_from_name: form.smtp_from_name,
    smtp_from_address: form.smtp_from_address,
    smtp_security: form.smtp_security,
    smtp_timeout_seconds: form.smtp_timeout_seconds,
    jwt_secret_rotate_hours: form.jwt_secret_rotate_hours,
    log_max_backups: form.log_max_backups,
    network_wait_online_disabled: form.network_wait_online_disabled,
    spice_enabled_by_default: form.spice_enabled_by_default,
    igpu_passthrough_enabled: form.igpu_passthrough_enabled,
    hardware_passthrough_enabled: form.hardware_passthrough_enabled,
  }
}

/** Tab 子组件通用 props：表单值 + 局部更新函数 */
export interface SettingsTabProps {
  form: SettingsForm
  patch: (partial: Partial<SettingsForm>) => void
}
