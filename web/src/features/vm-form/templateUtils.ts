/**
 * 模板默认配置解析（克隆时带出推荐值）
 * 迁移自旧前端 VmForm.vue 的 resolveTemplateDefault* 系列函数
 */
import type { TemplateItem } from '@/api/template'
import { resolveTemplateMinDiskSize } from '@/views/vm/utils'

const parsePositiveInt = (value: unknown): number => {
  const parsed = Number.parseInt(String(value ?? ''), 10)
  return Number.isFinite(parsed) && parsed > 0 ? parsed : 0
}

const resolveConfig = (tpl?: TemplateItem | null) => {
  if (!tpl || typeof tpl.default_config !== 'object' || !tpl.default_config) return null
  return tpl.default_config
}

/** 模板默认 vCPU */
export const resolveTemplateDefaultVCPU = (tpl?: TemplateItem | null): number =>
  parsePositiveInt(resolveConfig(tpl)?.vcpu)

/** 模板默认内存（GB） */
export const resolveTemplateDefaultRAM = (tpl?: TemplateItem | null): number =>
  parsePositiveInt(resolveConfig(tpl)?.ram)

/** 模板默认磁盘大小（GB，缺省回退模板磁盘虚拟大小） */
export const resolveTemplateDefaultDiskSize = (tpl?: TemplateItem | null): number => {
  const configured = parsePositiveInt(resolveConfig(tpl)?.disk_size)
  if (configured > 0) return configured
  return resolveTemplateMinDiskSize(tpl)
}

/** 模板默认系统盘总线 */
export const resolveTemplateDefaultDiskBus = (tpl?: TemplateItem | null): string => {
  const bus = String(resolveConfig(tpl)?.disk_bus || '').trim().toLowerCase()
  return ['virtio', 'scsi', 'sata', 'ide'].includes(bus) ? bus : ''
}

/** 模板默认网卡型号 */
export const resolveTemplateDefaultNicModel = (tpl?: TemplateItem | null): string => {
  const nicModel = String(resolveConfig(tpl)?.nic_model || '').trim().toLowerCase()
  return ['virtio', 'e1000e', 'rtl8139'].includes(nicModel) ? nicModel : ''
}

/** 模板默认显示设备 */
export const resolveTemplateDefaultVideoModel = (tpl?: TemplateItem | null): string => {
  const videoModel = String(resolveConfig(tpl)?.video_model || '').trim().toLowerCase()
  return ['virtio', 'vga', 'vmvga', 'cirrus', 'ramfb', 'none'].includes(videoModel) ? videoModel : ''
}

/** 模板默认 CPU 拓扑模式 */
export const resolveTemplateDefaultCPUTopologyMode = (tpl?: TemplateItem | null): string => {
  const mode = String(resolveConfig(tpl)?.cpu_topology_mode || '').trim().toLowerCase()
  return ['auto', 'single_socket', 'host_default'].includes(mode) ? mode : ''
}

/** 模板默认首次重启模式 */
export const resolveTemplateDefaultFirstBootRebootMode = (tpl?: TemplateItem | null): string => {
  const mode = String(resolveConfig(tpl)?.first_boot_reboot_mode || '').trim().toLowerCase()
  return ['normal', 'cold'].includes(mode) ? mode : ''
}

/** 模板引导方式归一化（Windows 模板 UEFI 自动升级安全引导） */
export const resolveTemplateBootType = (tpl?: TemplateItem | null): string => {
  if (!tpl) return ''
  const bootType = String(tpl.boot_type || '').trim().toLowerCase()
  if (bootType === 'uefi') return tpl.type === 'windows' ? 'uefi-secure' : 'uefi'
  if (bootType === 'bios') return 'bios'
  return ''
}
