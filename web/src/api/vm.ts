/**
 * 虚拟机相关 API
 * 对应后端 /api/vm、/api/self 路由组
 */
import service from './client'
import type { AxiosResponse } from 'axios'
import { API_BASE_URL } from '@/config/constants'
import type { ApiResponse } from '@/types/api'

/** 虚拟机列表项（管理员 /vm/list 与用户 /self/vms 结构一致） */
export interface VmListItem {
  name: string
  remark: string
  group: string
  tags: string[]
  status: string // running / shut off / paused / migrating ...
  vcpu: number
  memory: number // MB
  max_memory: number // MB
  ip: string
  disk_size: string // 如 "20 GB"
  template: string
  network: string
  autostart: boolean
  cpu_percent: number
  mem_percent: number
  locked: boolean
  in_rescue: boolean
  is_linked_clone: boolean
  continuous_runtime_seconds: number
  continuous_running_since: string
  created_at: string
}

/** 虚拟机历史监控记录 */
export interface VmStatsRecord {
  cpu_percent: number
  mem_used: number // KB
  mem_total: number // KB
  net_rx_bytes: number
  net_tx_bytes: number
  disk_rd_bytes: number
  disk_wr_bytes: number
  disk_rd_ops: number
  disk_wr_ops: number
  recorded_at: string
}

/** 用户配额使用情况（/self/quota） */
export interface QuotaUsage {
  used_cpu: number
  used_memory: number // GB
  used_disk: number // GB
  used_vm: number
  used_storage: number // Bytes
  used_storage_gb: string
  used_runtime_seconds: number
  used_runtime_display: string
  used_port_forwards: number
  used_snapshots: number
  enable_port_forward: boolean
  max_cpu: number
  max_memory: number // GB
  max_disk: number // GB
  max_vm: number
  max_storage: number // GB
  max_runtime_hours: number
  max_port_forwards: number
  max_snapshots: number
  max_bandwidth_up: number // Mbps
  max_bandwidth_down: number // Mbps
  max_traffic_down: number // GB
  max_traffic_up: number // GB
  max_public_ips: number
  used_public_ips: number
  used_traffic_down: number // Bytes
  used_traffic_up: number // Bytes
  used_traffic_down_gb: string
  used_traffic_up_gb: string
  is_limited_down: boolean
  is_limited_up: boolean
  remaining_runtime_seconds: number
  remaining_runtime_display: string
  runtime_quota_reached: boolean
}

/** 磁盘 IOPS 单项状态 */
export interface DiskIopsState {
  value: number
  is_set: boolean
}

/** 虚拟机磁盘信息（/vm/:name/disks） */
export interface VmDiskItem {
  device: string
  device_type?: string // disk / cdrom / floppy
  path?: string
  capacity_gb?: number | string
  used_gb?: number | string
  format?: string
  bus?: string
  backing_path?: string
  iops_total?: DiskIopsState
  iops_read?: DiskIopsState
  iops_write?: DiskIopsState
  is_system?: boolean
  serial?: string
  guest_device?: string
  guest_mapping_status?: 'mapped' | 'unmapped' | 'unavailable'
}

export interface GuestMountPayload {
  enabled: boolean
  filesystem?: 'ext4' | 'xfs' | 'btrfs'
  mount_point?: string
  drive_letter?: string
}

/** 删除确认用 qcow2 磁盘（/vm/:name/qcow2-disks） */
export interface VmQcow2Disk {
  device: string
  path: string
  format: string
  capacity_gb: number | string
  is_system: boolean
}

/** 虚拟机 IP 信息（/vm/:name/ip） */
export interface VmIPInfo {
  ip: string
  ip_status?: string // vlan_bridge / shut_off ...
}

/** 电源操作类型 */
export type VmPowerAction = 'start' | 'shutdown' | 'reboot' | 'destroy' | 'reset'

/** 虚拟机列表查询参数 */
export interface VmListQuery {
  include_resource_usage?: boolean
  include_ip?: boolean
}

/** 管理员：获取全部虚拟机列表 */
export function getVmList(params?: VmListQuery) {
  return service.get<unknown, ApiResponse<VmListItem[]>>('/vm/list', { params, silent: true })
}

/** 用户：获取自己的虚拟机列表 */
export function getSelfVMs(params?: VmListQuery) {
  return service.get<unknown, ApiResponse<VmListItem[]>>('/self/vms', { params, silent: true })
}

/**
 * 创建虚拟机列表 SSE 连接（实时推送）
 * 管理员使用 /vm/sse，普通用户使用 /self/vms/sse
 */
export function createVmListSSE(isAdmin: boolean, token: string): EventSource {
  const query = new URLSearchParams({
    token,
    include_resource_usage: '1',
    include_ip: '1',
  })
  const path = isAdmin ? '/vm/sse' : '/self/vms/sse'
  return new EventSource(`${API_BASE_URL}${path}?${query.toString()}`)
}

/** 用户：获取自己的配额使用情况 */
export function getSelfQuota() {
  return service.get<unknown, ApiResponse<QuotaUsage>>('/self/quota', { silent: true })
}

/** 获取虚拟机历史监控数据 */
export function getVmStatsHistory(name: string, params: { start: string; end: string }) {
  return service.get<unknown, ApiResponse<VmStatsRecord[]>>(
    `/vm/${encodeURIComponent(name)}/stats/history`,
    { params, silent: true },
  )
}

/** 按需加载虚拟机 IP */
export function getVmIP(name: string) {
  return service.get<unknown, ApiResponse<VmIPInfo>>(`/vm/${encodeURIComponent(name)}/ip`, {
    silent: true,
  })
}

/** 获取虚拟机磁盘列表 */
export function getDiskList(name: string) {
  return service.get<unknown, ApiResponse<VmDiskItem[]>>(`/vm/${encodeURIComponent(name)}/disks`)
}

/** 获取虚拟机 qcow2 磁盘列表（删除确认用） */
export function getVmQcow2Disks(name: string) {
  return service.get<unknown, ApiResponse<VmQcow2Disk[]>>(
    `/vm/${encodeURIComponent(name)}/qcow2-disks`,
  )
}

/** 用户自助：获取 qcow2 磁盘列表（删除确认用） */
export function selfGetVmQcow2Disks(name: string) {
  return service.get<unknown, ApiResponse<VmQcow2Disk[]>>(
    `/self/vm/${encodeURIComponent(name)}/qcow2-disks`,
  )
}

/** 电源操作（start/shutdown/reboot/destroy/reset） */
export function operateVm(name: string, action: VmPowerAction) {
  return service.post<unknown, ApiResponse<{ task_id?: string }>>(
    `/vm/${encodeURIComponent(name)}/operate`,
    { action },
  )
}

/** 磁盘 IOPS 限制（总与读/写互斥，0 表示不限制） */
export interface DiskIopsPayload {
  total_iops_sec: number
  read_iops_sec: number
  write_iops_sec: number
}

/** QEMU Guest Agent 配置 */
export interface GuestAgentPayload {
  enabled: boolean
}

/** SMBIOS 类型 1 配置 */
export interface Smbios1Payload {
  base64: boolean
  family: string
  manufacturer: string
  product: string
  serial: string
  sku: string
  uuid: string
  version: string
}

/** 动态内存配置 */
export interface MemoryDynamicPayload {
  dynamic_enabled: boolean
  memory_backend: string // balloon / virtio_mem
  memory_initial: number // GB
  memory_min?: number // GB
  memory_max?: number // GB
  memory_auto_balloon?: boolean
  memory_current?: number // GB，0 表示不手动调整
}

/** 直接内核引导配置 */
export interface DirectBootPayload {
  enabled: boolean
  cmdline?: string
}

/** 直通设备（PCI 地址） */
export interface HostDevicePayload {
  pci_address: string
}

/** 编辑虚拟机提交载荷（仅发送变化字段，后端逐项应用） */
export interface UpdateVmPayload {
  remark?: string
  group?: string
  tags?: string[]
  vcpu?: number
  max_vcpu?: number
  memory?: number // GB
  autostart?: boolean
  freeze?: boolean
  apic?: boolean
  pae?: boolean
  kvm_hidden?: boolean
  vendor_id?: string
  nested_virt?: boolean
  rtc_offset?: string
  rtc_startdate?: string
  guest_agent?: GuestAgentPayload
  smbios1?: Smbios1Payload
  boot_order?: string[]
  device_order?: string[]
  pcie_root_ports?: number
  firmware_compat?: boolean
  direct_boot?: DirectBootPayload
  host_devices?: HostDevicePayload[]
  disk_iops?: Record<string, DiskIopsPayload>
  cpu_limit_percent?: number
  cpu_affinity?: string
  cpu_topology_mode?: string
  nic_model?: string
  boot_type?: string
  video_model?: string
  memory_dynamic?: MemoryDynamicPayload
  add_disks?: AddDiskPayload[]
}

/** 编辑模式新增磁盘 */
export interface AddDiskPayload {
  size: number // GB
  format: string
  bus: string
  storage_pool_id?: string
  guest_mount?: GuestMountPayload
}

/** 编辑虚拟机（备注 / 分组 / 硬件配置等，仅发送变化字段） */
export function updateVm(name: string, data: UpdateVmPayload) {
  return service.put<unknown, ApiResponse<null>>(`/vm/${encodeURIComponent(name)}`, data)
}

/** 删除虚拟机（管理员） */
export function deleteVm(name: string, data: { delete_disks?: string[]; transfer_disks?: string[] } = {}) {
  return service.delete<unknown, ApiResponse<null>>(`/vm/${encodeURIComponent(name)}`, { data })
}

/** 用户自助删除虚拟机 */
export function selfDeleteVm(
  name: string,
  data: { delete_disks?: string[]; transfer_disks?: string[] } = {},
) {
  return service.delete<unknown, ApiResponse<null>>(`/self/vm/${encodeURIComponent(name)}`, { data })
}

/** 锁定虚拟机 */
export function lockVm(name: string) {
  return service.post<unknown, ApiResponse<null>>(`/vm/${encodeURIComponent(name)}/lock`)
}

/** 解锁虚拟机（高风险操作，428 二次验证由请求层自动处理） */
export function unlockVm(name: string) {
  return service.post<unknown, ApiResponse<null>>(`/vm/${encodeURIComponent(name)}/unlock`)
}

/** 启动/关闭救援系统 */
export function rescueVm(name: string, action: 'start' | 'stop') {
  return service.post<unknown, ApiResponse<{ task_id?: string }>>(
    `/vm/${encodeURIComponent(name)}/rescue`,
    { action },
  )
}

/** 转为独立虚拟机（脱离链式克隆 backing chain，仅管理员） */
export function makeVMIndependent(name: string) {
  return service.post<unknown, ApiResponse<{ task_id?: string }>>(
    `/vm/${encodeURIComponent(name)}/make-independent`,
  )
}

/** 重装系统请求参数 */
export interface ReinstallVmPayload {
  template: string
  disk_size: number
  hostname: string
  user: string
  password: string
  preserve_fnos_device_id?: boolean
  fnos_device_id?: string
}

/** 重装系统 */
export function reinstallVm(name: string, data: ReinstallVmPayload) {
  return service.post<unknown, ApiResponse<{ task_id?: string }>>(
    `/vm/${encodeURIComponent(name)}/reinstall`,
    data,
  )
}

// ==================== 虚拟机详情（SSE 推送） ====================

/** 实时资源统计（SSE 推送，前端计算速率） */
export interface VmStats {
  cpu_percent: number
  mem_used: number // KB
  mem_total: number // KB
  net_rx_bytes: number
  net_tx_bytes: number
  disk_rd_bytes: number
  disk_wr_bytes: number
  disk_rd_ops: number
  disk_wr_ops: number
  // 以下字段由前端增量计算
  net_rx_rate?: number
  net_tx_rate?: number
  disk_rd_rate?: number
  disk_wr_rate?: number
  disk_rd_iops?: number
  disk_wr_iops?: number
}

/** 登录凭据 */
export interface VmCredential {
  username: string
  password: string
}

/** Guest Agent 运行状态 */
export interface GuestAgentStatus {
  connected: boolean
  configured: boolean
  version?: string
}

/** 公网 IP 绑定信息 */
export interface PublicIPAttachment {
  public_ip: string
  mode: string
  mode_label?: string
}

/** PCIe 热插槽用量 */
export interface VmPCIEInfo {
  total_ports: number
  used_ports: number
  free_ports: number
}

/** 虚拟机详情（/vm/:name 与详情 SSE 推送结构一致） */
export interface VmDetailInfo {
  name: string
  remark: string
  group: string
  tags: string[]
  status: string
  vcpu: number
  memory: number // MB
  max_memory: number
  ip: string
  ip_status?: string
  disk_size: string
  disk_healthy?: boolean | null
  template: string
  network: string
  nic_model?: string
  autostart: boolean
  mac_address?: string
  vnc_port: string
  video_model: string
  cpu_limit_percent: number
  cpu_affinity: string
  cpu_percent?: number
  mem_percent?: number
  memory_dynamic_enabled: boolean
  memory_backend: string // balloon / virtio_mem
  created_at: string
  bandwidth_in?: number
  bandwidth_out?: number
  public_ips?: PublicIPAttachment[]
  in_rescue: boolean
  locked: boolean
  is_linked_clone: boolean
  continuous_runtime_seconds: number
  continuous_running_since: string
  uuid?: string
  os_type: string // linux / windows / fnos ...
  boot_type?: string
  arch?: string
  machine_type: string // q35 / i440fx / virt
  stats?: VmStats | null
  credential?: VmCredential | null
  freeze: boolean
  apic: boolean
  pae: boolean
  rtc_offset?: string
  guest_agent_status?: GuestAgentStatus | null
  pcie_root_ports: number
  pcie_info?: VmPCIEInfo | null
  kvm_hidden?: boolean
  nested_virt?: boolean
  // ===== 以下字段供编辑表单回填 =====
  vendor_id?: string
  cpu_topology_mode?: string
  rtc_startdate?: string
  firmware_compat?: boolean
  direct_boot?: DirectBootPayload | null
  guest_agent?: GuestAgentPayload | null
  smbios1?: Partial<Smbios1Payload> | null
  memory_initial?: number // MB
  memory_min?: number // MB
  memory_max_dynamic?: number // MB
  memory_auto_balloon?: boolean
  memory_pending_apply?: boolean
  memory_compat_mode?: string // legacy_static / dynamic / pending_apply
  memory_balloon_supported?: boolean
  memory_balloon_status?: string // ok / no_stats / not_running / missing_balloon / pending_apply
  memory_virtio_mem_current?: number // MB
  boot_order?: string[]
  boot_devices?: VmBootDevice[]
}

/** 可引导设备（编辑模式 Cockpit 风格引导顺序） */
export interface VmBootDevice {
  type: string // disk / cdrom / network
  device?: string
  bus?: string
  file?: string
  enabled: boolean
  order: number
}

/** 获取虚拟机详情（一次性） */
export function getVmDetail(name: string) {
  return service.get<unknown, ApiResponse<VmDetailInfo>>(`/vm/${encodeURIComponent(name)}`, {
    silent: true,
  })
}

/** 创建虚拟机详情 SSE 连接（vm_detail 事件推送 VmDetailInfo） */
export function createVmDetailSSE(name: string, token: string): EventSource {
  return new EventSource(
    `${API_BASE_URL}/vm/${encodeURIComponent(name)}/sse?token=${encodeURIComponent(token)}`,
  )
}

/** 获取 PCIe 热插槽用量 */
export function getVmPCIEInfo(name: string) {
  return service.get<unknown, ApiResponse<VmPCIEInfo>>(
    `/vm/${encodeURIComponent(name)}/pcie-info`,
    { silent: true },
  )
}

/** 获取虚拟机实时统计（一次性，非 SSE） */
export function getVmStats(name: string) {
  return service.get<unknown, ApiResponse<VmStats>>(`/vm/${encodeURIComponent(name)}/stats`, {
    silent: true,
  })
}

/** 重置系统登录密码（运行态 QGA 在线处理，关机态离线处理） */
export function resetVmLinuxPassword(
  name: string,
  data: { username: string; password: string; mode?: 'auto' | 'online' | 'offline' },
) {
  return service.post<unknown, ApiResponse<{ task_id?: string }>>(
    `/vm/${encodeURIComponent(name)}/password/reset`,
    data,
  )
}

// ==================== 网口 IP 状态 ====================

/** 网口运行状态 */
export interface VmNetworkInterface {
  target: string
  mac: string
  ip: string
  ip_source?: string
  bridge?: string
  source_bridge?: string
  virtualport_type?: string
  ofport?: string
  model?: string
  issues?: string[]
}

export interface VmNetworkStatus {
  vm_name?: string
  state?: string
  bridge?: string
  interfaces?: VmNetworkInterface[]
  issues?: string[]
  bandwidth?: {
    enabled?: boolean
    cookie?: string
    flow_exists?: boolean
    checked_port?: string
    down_qos?: boolean
    bridge_qos?: boolean
    queue?: string
    tc_root?: boolean
    tc_upload_police?: boolean
    tc_ingress?: boolean
  } | null
}

/** 获取虚拟机 OVS 网络运行状态 */
export function getVMNetworkStatus(name: string) {
  return service.get<unknown, ApiResponse<VmNetworkStatus>>(
    `/vm/${encodeURIComponent(name)}/network/status`,
    { silent: true },
  )
}

// ==================== 快照 ====================

/** 快照信息 */
export interface SnapshotItem {
  name: string
  description: string
  created_at: string
  state: string // running / shutoff / disk-snapshot / paused
  location?: string // internal / external
  is_current?: boolean
  children?: number
}

/** 快照配额 */
export interface SnapshotQuota {
  used_snapshots: number
  max_snapshots: number
}

/** 获取快照列表（响应含 quota 字段） */
export function getSnapshots(name: string) {
  return service.get<unknown, ApiResponse<SnapshotItem[]> & { quota?: SnapshotQuota }>(
    `/vm/${encodeURIComponent(name)}/snapshots`,
  )
}

/** 创建快照请求 */
export interface CreateSnapshotPayload {
  description: string
  include_memory: boolean
  pause_for_memory_snapshot: boolean
  auto_fix_nvram?: boolean
}

/** 创建快照 */
export function createSnapshot(name: string, data: CreateSnapshotPayload) {
  return service.post<unknown, ApiResponse<{ task_id?: string }>>(
    `/vm/${encodeURIComponent(name)}/snapshot`,
    data,
  )
}

/** 恢复快照 */
export function revertSnapshot(vmName: string, snapName: string) {
  return service.post<unknown, ApiResponse<{ task_id?: string }>>(
    `/vm/${encodeURIComponent(vmName)}/snapshot/${encodeURIComponent(snapName)}/revert`,
  )
}

/** 删除快照 */
export function deleteSnapshot(vmName: string, snapName: string) {
  return service.delete<unknown, ApiResponse<{ task_id?: string }>>(
    `/vm/${encodeURIComponent(vmName)}/snapshot/${encodeURIComponent(snapName)}`,
  )
}

/** 删除全部快照 */
export function deleteAllSnapshots(vmName: string) {
  return service.delete<unknown, ApiResponse<{ task_id?: string }>>(
    `/vm/${encodeURIComponent(vmName)}/snapshots`,
  )
}

// ==================== 定时任务 ====================

/** 虚拟机定时任务 */
export interface VmScheduleItem {
  id: number
  event_type: string // power / vm
  action: string // start / shutdown / delete
  schedule_type: string // once / daily / weekly
  run_at?: string
  time_of_day?: string
  weekdays?: number[]
  timezone?: string
  enabled: boolean
  next_run_at?: string
  last_triggered_at?: string
  last_status?: string // pending / running / success / failed
  last_message?: string
  last_task_id?: number
}

/** 定时任务提交载荷 */
export interface VmSchedulePayload {
  event_type: string
  action: string
  schedule_type: string
  run_at: string
  timezone: string
  time_of_day: string
  weekdays: number[]
  enabled: boolean
}

/** 获取定时任务列表 */
export function getVmSchedules(name: string) {
  return service.get<unknown, ApiResponse<VmScheduleItem[]>>(
    `/vm/${encodeURIComponent(name)}/schedules`,
  )
}

/** 创建定时任务 */
export function createVmSchedule(name: string, data: VmSchedulePayload) {
  return service.post<unknown, ApiResponse<null>>(
    `/vm/${encodeURIComponent(name)}/schedules`,
    data,
  )
}

/** 更新定时任务 */
export function updateVmSchedule(name: string, id: number, data: VmSchedulePayload) {
  return service.put<unknown, ApiResponse<null>>(
    `/vm/${encodeURIComponent(name)}/schedules/${id}`,
    data,
  )
}

/** 删除定时任务 */
export function deleteVmSchedule(name: string, id: number) {
  return service.delete<unknown, ApiResponse<null>>(
    `/vm/${encodeURIComponent(name)}/schedules/${id}`,
  )
}

// ==================== VNC 管理 ====================

/** VNC 状态 */
export interface VncStatus {
  enabled: boolean
  port: string
  auth: string
  has_password: boolean
  exposed: boolean
}

/** 获取 VNC 状态 */
export function getVncStatus(name: string) {
  return service.get<unknown, ApiResponse<VncStatus>>(
    `/vm/${encodeURIComponent(name)}/vnc/status`,
    { silent: true },
  )
}

/** 开启 VNC（password 最长 8 位，留空无密码） */
export function enableVnc(name: string, password = '') {
  return service.post<unknown, ApiResponse<{ task_id?: string }>>(
    `/vm/${encodeURIComponent(name)}/vnc/enable`,
    { password },
  )
}

/** 关闭 VNC */
export function disableVnc(name: string) {
  return service.post<unknown, ApiResponse<{ task_id?: string }>>(
    `/vm/${encodeURIComponent(name)}/vnc/disable`,
  )
}

/** 修改 VNC 密码（即时生效） */
export function changeVncPassword(name: string, password: string) {
  return service.post<unknown, ApiResponse<null>>(
    `/vm/${encodeURIComponent(name)}/vnc/passwd`,
    { password },
  )
}

/** 切换 VNC 对外暴露 */
export function exposeVnc(name: string, expose: boolean) {
  return service.post<unknown, ApiResponse<{ task_id?: string }>>(
    `/vm/${encodeURIComponent(name)}/vnc/expose`,
    { expose },
  )
}

// ==================== SPICE 管理 ====================

/** SPICE 状态 */
export interface SpiceStatus {
  enabled: boolean
  port: string
  auth?: string
  has_password: boolean
  exposed: boolean
}

/** SPICE 连接信息 */
export interface SpiceConnInfo {
  host: string
  port: string
  password: string
  exposed: boolean
}

/** 获取 SPICE 状态 */
export function getSpiceStatus(name: string) {
  return service.get<unknown, ApiResponse<SpiceStatus>>(
    `/vm/${encodeURIComponent(name)}/spice/status`,
    { silent: true },
  )
}

/** 获取 SPICE 连接信息 */
export function getSpiceConnInfo(name: string) {
  return service.get<unknown, ApiResponse<SpiceConnInfo>>(
    `/vm/${encodeURIComponent(name)}/spice/info`,
    { silent: true },
  )
}

/** 开启 SPICE */
export function enableSpice(name: string, password = '') {
  return service.post<unknown, ApiResponse<{ task_id?: string }>>(
    `/vm/${encodeURIComponent(name)}/spice/enable`,
    { password },
  )
}

/** 关闭 SPICE */
export function disableSpice(name: string) {
  return service.post<unknown, ApiResponse<{ task_id?: string }>>(
    `/vm/${encodeURIComponent(name)}/spice/disable`,
  )
}

/** 修改 SPICE 密码 */
export function changeSpicePassword(name: string, password: string) {
  return service.post<unknown, ApiResponse<null>>(
    `/vm/${encodeURIComponent(name)}/spice/passwd`,
    { password },
  )
}

/** 切换 SPICE 对外暴露（联动宿主防火墙） */
export function exposeSpice(name: string, expose: boolean) {
  return service.post<unknown, ApiResponse<{ task_id?: string }>>(
    `/vm/${encodeURIComponent(name)}/spice/expose`,
    { expose },
  )
}

/** 下载 SPICE .vv 连接文件（deleteFile=true 连接后自动删除）
 * 注意：请求层对 blob 响应直接放行 AxiosResponse，需取 .data 使用 */
export function downloadSpiceVV(name: string, deleteFile = true) {
  return service.get<unknown, AxiosResponse<Blob>>(
    `/vm/${encodeURIComponent(name)}/spice/vv`,
    { params: { delete: deleteFile ? 1 : 0 }, responseType: 'blob' },
  )
}

// ==================== 网络诊断与抓包（仅管理员） ====================

/** 抓包过滤条件 */
export interface NetworkDiagnosticFilter {
  protocol: string
  source_ip: string
  dest_ip: string
  port: number
  source_port: number
  dest_port: number
}

/** 抓包请求 */
export interface NetworkCaptureRequest {
  interface_name: string
  filter: NetworkDiagnosticFilter
  duration_seconds: number
  max_mb: number
  max_packets: number
}

/** 诊断模板 */
export interface NetworkDiagnosticTemplate {
  key: string
  name: string
  description: string
  filter: NetworkDiagnosticFilter
}

/** 网络诊断结果 */
export interface VmNetworkDiagnostics {
  vm_name: string
  state: string
  interfaces?: VmNetworkInterface[]
  neighbors?: string[]
  templates?: NetworkDiagnosticTemplate[]
  port_forwards?: { protocol: string; host_port: string; dest_ip: string; dest_port: string }[]
  default_interface: string
  default_ip: string
  issues?: string[]
}

/** 获取虚拟机网络诊断信息 */
export function getVMNetworkDiagnostics(name: string) {
  return service.get<unknown, ApiResponse<VmNetworkDiagnostics>>(
    `/vm/${encodeURIComponent(name)}/network/diagnostics`,
    { silent: true },
  )
}

/** 发起抓包任务 */
export function startVMNetworkCapture(name: string, data: NetworkCaptureRequest) {
  return service.post<unknown, ApiResponse<{ task_id: number }>>(
    `/vm/${encodeURIComponent(name)}/network/capture`,
    data,
  )
}

// ==================== 创建 / 克隆 / 导入 ====================

/** 额外磁盘（创建/克隆时随虚拟机一并创建） */
export interface ExtraDiskPayload {
  size: number // GB
  format: string
  bus: string
  storage_pool_id?: string
  iops_total?: number
  iops_read?: number
  iops_write?: number
  guest_mount?: GuestMountPayload
}

/** 额外网口 */
export interface ExtraNicPayload {
  switch_id: number
  security_group_id: number
  nic_model: string
}

/** 创建虚拟机（ISO 安装）提交载荷 */
export interface CreateVmPayload {
  name: string
  remark?: string
  vcpu: number
  max_vcpu?: number
  ram: number // GB
  disk_size: number // GB
  disk_format?: string
  disk_bus?: string
  system_disk_iops?: DiskIopsPayload
  os_type?: string
  os_variant?: string
  iso_path?: string
  iso_paths?: string[]
  floppy_image?: string
  switch_id?: number | null
  security_group_id?: number | null
  storage_pool_id?: string
  nic_model?: string
  autostart?: boolean
  freeze?: boolean
  apic?: boolean
  pae?: boolean
  rtc_offset?: string
  rtc_startdate?: string
  guest_agent?: GuestAgentPayload
  smbios1?: Smbios1Payload
  machine_type?: string
  boot_type?: string
  watchdog?: string
  boot_order?: string[]
  video_model?: string
  spice_enabled?: boolean
  cpu_topology_mode?: string
  virt_type?: string
  arch?: string
  pcie_root_ports?: number
  extra_disks?: ExtraDiskPayload[]
  host_devices?: HostDevicePayload[]
  extra_nics?: ExtraNicPayload[]
  firmware_compat?: boolean
  direct_boot?: DirectBootPayload
  kvm_hidden?: boolean
  vendor_id?: string
  nested_virt?: boolean
  cpu_limit_percent?: number
  cpu_affinity?: string
  memory_dynamic?: MemoryDynamicPayload
}

/** 管理员：创建虚拟机（ISO 安装） */
export function createVm(data: CreateVmPayload) {
  return service.post<unknown, ApiResponse<{ task_id?: string }>>('/vm/create', data)
}

/** 模板克隆提交载荷（单台） */
export interface CloneVmPayload {
  name: string
  remark?: string
  template: string
  template_type?: string
  clone_mode: string // linked / full
  vcpu: number
  max_vcpu?: number
  ram: number // GB
  disk_size: number // GB
  hostname?: string
  user?: string
  password?: string
  disable_system_init?: boolean
  switch_id?: number | null
  security_group_id?: number | null
  storage_pool_id?: string
  autostart?: boolean
  freeze?: boolean
  apic?: boolean
  pae?: boolean
  rtc_offset?: string
  rtc_startdate?: string
  guest_agent?: GuestAgentPayload
  smbios1?: Smbios1Payload
  uefi?: boolean
  disk_bus?: string
  system_disk_iops?: DiskIopsPayload
  nic_model?: string
  video_model?: string
  spice_enabled?: boolean
  cpu_topology_mode?: string
  first_boot_reboot_mode?: string
  extra_nics?: ExtraNicPayload[]
  preserve_fnos_device_id?: boolean
  fnos_device_id?: string
  extra_disks?: ExtraDiskPayload[]
  host_devices?: HostDevicePayload[]
  pcie_root_ports?: number
  static_ip?: string
  gateway?: string
  dns?: string
  kvm_hidden?: boolean
  vendor_id?: string
  nested_virt?: boolean
  cpu_limit_percent?: number
  cpu_affinity?: string
  memory_dynamic?: MemoryDynamicPayload
}

/** 管理员：模板克隆虚拟机 */
export function cloneVm(data: CloneVmPayload) {
  return service.post<unknown, ApiResponse<{ task_id?: string }>>('/vm/clone', data)
}

/** 批量克隆提交载荷 */
export interface BatchCloneVmPayload {
  prefix: string
  start_num: number
  count: number
  template: string
  template_type?: string
  clone_mode: string
  vcpu: number
  max_vcpu?: number
  ram: number
  disk_size: number
  hostname?: string
  user?: string
  password?: string
  disable_system_init?: boolean
  autostart?: boolean
  freeze?: boolean
  apic?: boolean
  pae?: boolean
  rtc_offset?: string
  rtc_startdate?: string
  guest_agent?: GuestAgentPayload
  smbios1?: Smbios1Payload
  uefi?: boolean
  template_user?: string
  video_model?: string
  spice_enabled?: boolean
  disk_bus?: string
  nic_model?: string
  storage_pool_id?: string
  cpu_topology_mode?: string
  first_boot_reboot_mode?: string
  switch_id?: number | null
  security_group_id?: number | null
  extra_nics?: ExtraNicPayload[]
  extra_disks?: ExtraDiskPayload[]
  static_ip?: string
  gateway?: string
  dns?: string
  kvm_hidden?: boolean
  vendor_id?: string
  nested_virt?: boolean
  cpu_limit_percent?: number
  cpu_affinity?: string
}

/** 管理员：批量克隆虚拟机 */
export function batchCloneVm(data: BatchCloneVmPayload) {
  return service.post<unknown, ApiResponse<{ task_id?: string }>>('/vm/batch-clone', data)
}

/** 额外导入磁盘项 */
export interface ExtraImportDiskPayload {
  disk_path?: string
  disk_file?: string
  disk_source_type?: string
  storage_pool_id?: string
  copy_disk?: boolean
  bus?: string
  iops_total?: number
  iops_read?: number
  iops_write?: number
}

/** 导入磁盘创建虚拟机提交载荷 */
export interface ImportVmPayload {
  name: string
  remark?: string
  disk_file?: string
  disk_path?: string
  disk_source_type?: string
  storage_pool_id?: string
  vcpu: number
  max_vcpu?: number
  ram: number
  switch_id?: number | null
  security_group_id?: number | null
  copy_disk?: boolean
  hostname?: string
  user?: string
  password?: string
  init_type?: string
  template_root_pass?: string
  template_user?: string
  autostart?: boolean
  freeze?: boolean
  start_after_import?: boolean
  apic?: boolean
  pae?: boolean
  rtc_offset?: string
  rtc_startdate?: string
  guest_agent?: GuestAgentPayload
  smbios1?: Smbios1Payload
  boot_type?: string
  machine_type?: string
  nic_model?: string
  video_model?: string
  spice_enabled?: boolean
  cpu_topology_mode?: string
  first_boot_reboot_mode?: string
  extra_nics?: ExtraNicPayload[]
  extra_import_disks?: ExtraImportDiskPayload[]
  system_disk_iops?: DiskIopsPayload
  kvm_hidden?: boolean
  vendor_id?: string
  nested_virt?: boolean
  cpu_limit_percent?: number
  cpu_affinity?: string
  memory_dynamic?: MemoryDynamicPayload
}

/** 管理员：绝对路径导入磁盘创建虚拟机 */
export function adminImportDisk(data: ImportVmPayload) {
  return service.post<unknown, ApiResponse<{ task_id?: string }>>('/vm/import-disk', data)
}

/** 操作系统变体（libvirt osinfo，供 ISO 安装选择） */
export interface OsVariantItem {
  id: string
  name: string
  category: string // Linux / Windows
}

/** 获取操作系统变体列表 */
export function getOSVariants() {
  return service.get<unknown, ApiResponse<OsVariantItem[]>>('/vm/os-variants', { silent: true })
}

// ==================== 虚拟机 XML ====================

/** 获取虚拟机持久化 XML */
export function getVmXML(name: string) {
  return service.get<unknown, ApiResponse<{ xml: string }>>(
    `/vm/${encodeURIComponent(name)}/xml`,
    { silent: true },
  )
}

/** 保存虚拟机持久化 XML */
export function updateVmXML(name: string, data: { xml: string }) {
  return service.put<unknown, ApiResponse<null>>(`/vm/${encodeURIComponent(name)}/xml`, data)
}

// ==================== 磁盘管理（编辑模式） ====================

/** 磁盘扩容（仅扩大） */
export function resizeDisk(name: string, dev: string, sizeGB: number, autoGrowPartition = false) {
  return service.post<unknown, ApiResponse<{ task_id?: number }>>(
    `/vm/${encodeURIComponent(name)}/disk/${encodeURIComponent(dev)}/resize`,
    { size_gb: sizeGB, auto_grow_partition: autoGrowPartition },
  )
}

/** 删除/卸载磁盘（transfer=true 转移到我的存储） */
export function removeDisk(name: string, dev: string, deleteFile = false, transfer = false) {
  return service.delete<unknown, ApiResponse<null>>(
    `/vm/${encodeURIComponent(name)}/disk/${encodeURIComponent(dev)}`,
    { data: { delete_file: deleteFile, transfer } },
  )
}

/** 修改磁盘驱动类型 */
export function changeDiskBus(name: string, dev: string, bus: string) {
  return service.put<unknown, ApiResponse<null>>(
    `/vm/${encodeURIComponent(name)}/disk/${encodeURIComponent(dev)}/bus`,
    { bus },
  )
}

/** 挂载已有磁盘文件（我的存储中的磁盘） */
export function attachDisk(name: string, path: string, bus = 'virtio', guestMount?: GuestMountPayload) {
  return service.post<unknown, ApiResponse<{ task_id?: number }>>(`/vm/${encodeURIComponent(name)}/disk/attach`, {
    path,
    bus,
    guest_mount: guestMount,
  })
}

/** 管理员：绝对路径导入磁盘到指定虚拟机（异步任务） */
export function adminImportDiskForVM(
  name: string,
  data: {
    disk_path: string
    disk_source_type: string
    storage_pool_id?: string
    copy_disk?: boolean
    bus?: string
    guest_mount?: GuestMountPayload
  },
) {
  return service.post<unknown, ApiResponse<{ task_id?: string }>>(
    `/vm/${encodeURIComponent(name)}/disk/import`,
    data,
  )
}

export function mountGuestDisk(
  name: string,
  dev: string,
  data: { guest_mount: GuestMountPayload; existing_disk?: boolean },
) {
  return service.post<unknown, ApiResponse<{ task_id?: number }>>(
    `/vm/${encodeURIComponent(name)}/disk/${encodeURIComponent(dev)}/guest-mount`,
    data,
  )
}

export function retryGuestDiskGrow(name: string, dev: string) {
  return service.post<unknown, ApiResponse<{ task_id?: number }>>(
    `/vm/${encodeURIComponent(name)}/disk/${encodeURIComponent(dev)}/guest-grow`,
  )
}

// ==================== 光驱 / 软盘管理（编辑模式） ====================

/** 插入/更换光驱 ISO（force_new=true 新增光驱设备） */
export function changeCDROM(name: string, data: { iso_path: string; device?: string; force_new?: boolean }) {
  return service.post<unknown, ApiResponse<null>>(`/vm/${encodeURIComponent(name)}/cdrom`, data)
}

/** 弹出光驱 */
export function ejectCDROM(name: string, device = '') {
  return service.post<unknown, ApiResponse<null>>(
    `/vm/${encodeURIComponent(name)}/cdrom/eject`,
    null,
    { params: device ? { device } : {} },
  )
}

/** 移除光驱设备 */
export function removeCDROM(name: string, device = '') {
  return service.delete<unknown, ApiResponse<null>>(`/vm/${encodeURIComponent(name)}/cdrom`, {
    params: device ? { device } : {},
  })
}

/** 插入/更换软盘镜像（force_new=true 新增软盘设备） */
export function changeFloppy(name: string, data: { image_path: string; device?: string; force_new?: boolean }) {
  return service.post<unknown, ApiResponse<null>>(`/vm/${encodeURIComponent(name)}/floppy`, data)
}

/** 弹出软盘 */
export function ejectFloppy(name: string, device = '') {
  return service.post<unknown, ApiResponse<null>>(
    `/vm/${encodeURIComponent(name)}/floppy/eject`,
    null,
    { params: device ? { device } : {} },
  )
}

/** 移除软盘设备 */
export function removeFloppy(name: string, device = '') {
  return service.delete<unknown, ApiResponse<null>>(`/vm/${encodeURIComponent(name)}/floppy`, {
    params: device ? { device } : {},
  })
}

// ==================== 硬件直通（仅管理员） ====================

/** 宿主机 PCI 直通设备 */
export interface PassthroughDevice {
  pci_address: string
  vendor_name?: string
  product_name?: string
  class_name?: string
  vendor_id?: string
  product_id?: string
  is_vfio_bound?: boolean
  driver_in_use?: string
  is_used_by_vm?: boolean
  used_by_vm_name?: string
}

/** 获取宿主机可直通 PCI 设备列表 */
export function getPassthroughDevices() {
  return service.get<unknown, ApiResponse<PassthroughDevice[]>>('/host/passthrough', {
    silent: true,
  })
}

/** 获取虚拟机已配置的直通设备 */
export function getVmPassthroughDevices(name: string) {
  return service.get<unknown, ApiResponse<{ pci_address: string }[]>>(
    `/vm/${encodeURIComponent(name)}/passthrough`,
    { silent: true },
  )
}

/** 绑定 PCI 设备到 vfio-pci 驱动 */
export function bindPCIDevice(pciAddress: string) {
  return service.post<unknown, ApiResponse<null>>('/host/passthrough/bind', {
    pci_address: pciAddress,
  })
}
