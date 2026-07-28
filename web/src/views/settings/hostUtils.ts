/**
 * 宿主机设置 Tab 辅助逻辑：兜底挡位、格式化与摘要生成
 */
import type { KSMStatus, KVMUnrestrictedGuestStatus, ZRAMStatus, HostProfileOption } from '@/api/settings'

/** KSM 兜底挡位（后端未返回 profiles 时使用） */
export const FALLBACK_KSM_PROFILES: HostProfileOption[] = [
  { key: 'off', name: '关闭', description: '不扫描内存页，适合临时排障或 CPU 压力优先的宿主机。' },
  { key: 'conservative', name: '保守', description: '低频扫描，优先降低 CPU 开销，适合内存压力不高的虚拟化宿主机。' },
  { key: 'balanced', name: '均衡', description: '推荐挡位，启用零页合并，在节省内存和控制扫描开销之间取平衡。' },
  { key: 'aggressive', name: '积极', description: '提高扫描速度，适合 VM 密度较高且希望更快释放重复内存的宿主机。' },
  { key: 'extreme', name: '极致', description: '最大化去重速度，适合内存非常紧张的纯虚拟化宿主机，CPU 开销会更明显。' },
]

/** zRAM 兜底挡位 */
export const FALLBACK_ZRAM_PROFILES: HostProfileOption[] = [
  { key: 'off', name: '关闭', description: '关闭面板管理的 zRAM swap，适合排障或宿主机内存压力很低的场景。' },
  { key: 'conservative', name: '保守', description: 'zRAM 逻辑容量为宿主机内存 10%，最高 16 GiB，优先降低压缩和换页开销。' },
  { key: 'balanced', name: '均衡', description: 'zRAM 逻辑容量为宿主机内存 20%，最高 32 GiB，适合作为纯虚拟化宿主机默认挡位。' },
  { key: 'aggressive', name: '积极', description: 'zRAM 逻辑容量为宿主机内存 35%，最高 64 GiB，适合 VM 密度高且希望优先压缩内存的宿主机。' },
  { key: 'extreme', name: '极致', description: 'zRAM 逻辑容量为宿主机内存 50%，最高 128 GiB，适合内存非常紧张且能接受更多 CPU 开销的宿主机。' },
]

/** 从挡位列表中取名称（找不到时回退 key） */
export function profileName(options: HostProfileOption[], key?: string): string {
  const profile = options.find((item) => item.key === key)
  return profile?.name || key || '未配置'
}

/** 数值格式化（千分位，空值显示 -） */
export function fmtNum(value?: number | null): string {
  if (value === null || value === undefined) return '-'
  return Number(value).toLocaleString()
}

/** MB 数值格式化 */
export function fmtMB(value?: number | null): string {
  if (value === null || value === undefined) return '-'
  return `${Number(value).toLocaleString()} MB`
}

/** 布尔格式化（开启/关闭，空值显示 -） */
export function fmtBool(value?: boolean | number | null): string {
  if (value === null || value === undefined) return '-'
  return value ? '开启' : '关闭'
}

/** KSM 状态摘要 */
export function buildKsmSummary(
  status: KSMStatus | null,
  loading: boolean,
  options: HostProfileOption[],
): string {
  if (loading) return '正在读取宿主机 KSM 参数...'
  if (!status) return '进入系统设置后会自动读取当前宿主机 KSM 状态。'
  if (!status.supported) return status.message || '当前宿主机未提供 KSM sysfs 接口。'
  const current = profileName(options, status.current_profile)
  const persistent = status.persistent_configured
    ? `，重启后恢复为${profileName(options, status.persistent_profile)}`
    : '，尚未写入持久配置'
  const sharedPages = status.metrics?.pages_sharing
  const sharedText =
    sharedPages !== null && sharedPages !== undefined ? `，当前被共享页 ${fmtNum(sharedPages)}` : ''
  return `当前挡位为${current}${persistent}${sharedText}。`
}

/** zRAM 状态摘要 */
export function buildZramSummary(
  status: ZRAMStatus | null,
  loading: boolean,
  options: HostProfileOption[],
): string {
  if (loading) return '正在读取宿主机 zRAM 参数...'
  if (!status) return '进入系统设置后会自动读取当前宿主机 zRAM 状态。'
  if (!status.supported) return status.message || '当前宿主机缺少 zRAM 内核能力或 util-linux 相关工具。'
  const current = profileName(options, status.current_profile)
  const persistent = status.persistent_configured
    ? `，重启后恢复为${profileName(options, status.persistent_profile)}`
    : '，尚未写入持久配置'
  const sizeMb = status.runtime_config?.size_mb
  const sizeText = sizeMb !== null && sizeMb !== undefined ? `，当前容量 ${fmtMB(sizeMb)}` : ''
  return `当前挡位为${current}${persistent}${sizeText}。`
}

/** KVM Unrestricted Guest 状态摘要 */
export function buildKvmSummary(
  status: KVMUnrestrictedGuestStatus | null,
  loading: boolean,
): string {
  if (loading) return '正在读取宿主机 KVM 参数...'
  if (!status) return '进入系统设置后会自动读取当前宿主机运行时参数。'
  if (!status.supported) return status.message || '当前宿主机未加载 kvm_intel，或不是 Intel KVM 环境。'
  const runtimeText = status.runtime_enabled ? '运行时已启用' : '运行时已禁用'
  const persistentText = status.persistent_configured
    ? `持久配置为${status.persistent_enabled ? '启用' : '禁用'}`
    : '尚未写入持久配置'
  if (status.requires_reload) {
    if (status.message && status.message.includes('重启')) {
      return `${runtimeText}，${persistentText}；模块无法热卸载，需重启宿主机后生效。`
    }
    return `${runtimeText}，${persistentText}；需要重载 KVM 模块或重启宿主机后完全生效。`
  }
  if ((status.active_vm_count || 0) > 0) {
    return `${runtimeText}，${persistentText}；当前有 ${status.active_vm_count} 台虚拟机运行或暂停，切换后会先保存配置。`
  }
  return `${runtimeText}，${persistentText}。VMware 嵌套虚拟化出现 hardware error 0x7 时可尝试禁用。`
}
