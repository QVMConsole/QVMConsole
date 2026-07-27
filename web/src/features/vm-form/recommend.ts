/**
 * 表单联动推荐值计算（创建 / 编辑共用）
 * 迁移自旧前端 VmForm.vue 的推荐逻辑
 */
import type { VmFormModel } from './types'

/** 推荐 RTC 时间基准：Windows 用本地时间，其余 UTC */
export const getRecommendedRTCOffset = (guestType: string): string =>
  guestType === 'windows' ? 'localtime' : 'utc'

/** 推荐显示设备：ARM 必须 ramfb；Windows 用 VGA 兼容，其余 VirtIO */
export const getRecommendedVideoModel = (osType: string, arch: string): string => {
  if (arch === 'aarch64') return 'ramfb'
  return osType === 'windows' ? 'vga' : 'virtio'
}

/** i440FX + Windows ISO 安装在当前宿主机上需强制 BIOS（避免 OVMF 卡启动画面） */
export const shouldUseBIOSForI440FXWindows = (
  form: Pick<VmFormModel, 'create_mode' | 'os_type' | 'machine_type'>,
  isEdit: boolean,
): boolean =>
  !isEdit &&
  form.create_mode === 'iso' &&
  form.os_type === 'windows' &&
  form.machine_type === 'i440fx'

/** 推荐 Windows 引导类型 */
export const getRecommendedWindowsBootType = (
  form: Pick<VmFormModel, 'create_mode' | 'os_type' | 'machine_type'>,
  isEdit: boolean,
): string => (shouldUseBIOSForI440FXWindows(form, isEdit) ? 'bios' : 'uefi')

// ==================== 动态内存推荐 ====================

/** 推荐最大内存：基础值上浮 30%（至少等于基础值） */
export const getRecommendedMemoryMax = (base: number): number =>
  Math.max(base, Math.ceil(base * 1.3))

/** 弹性内存推荐基础内存：规格的一半（最低 1GB） */
export const getRecommendedElasticMemoryInitial = (spec: number): number =>
  Math.max(1, Math.floor(spec / 2))

/** 由弹性内存配置反推规格内存 */
export const getElasticMemorySpecFromConfig = (
  initial: number,
  maxDynamic: number,
  fallback: number,
): number => {
  const initialSpec = initial > 0 ? initial * 2 : 0
  const maxSpec = maxDynamic > 0 ? Math.max(1, Math.floor((maxDynamic * 10) / 13)) : 0
  return Math.max(1, initialSpec, maxSpec, fallback || 0)
}

/** 按后端类型应用动态内存推荐值 */
export const recommendedMemoryDynamicValues = (
  backend: string,
  spec: number,
): { memory_initial: number; memory_min: number; memory_max_dynamic: number; memory_auto_balloon: boolean } => {
  if (backend === 'virtio_mem') {
    const initial = getRecommendedElasticMemoryInitial(spec)
    return {
      memory_initial: initial,
      memory_min: initial,
      memory_max_dynamic: getRecommendedMemoryMax(spec),
      memory_auto_balloon: false,
    }
  }
  return {
    memory_initial: spec,
    memory_min: Math.max(1, Math.floor(spec / 2)),
    memory_max_dynamic: getRecommendedMemoryMax(spec),
    memory_auto_balloon: true,
  }
}

// ==================== 归一化工具 ====================

export const normalizeRTCOffsetForForm = (value?: string): string =>
  value === 'localtime' ? 'localtime' : 'utc'

export const normalizeRTCStartDate = (value?: string): string => {
  const normalized = `${value || ''}`.trim()
  return normalized || 'now'
}

export const normalizeSMBIOS1Value = (value?: string): string => `${value || ''}`.trim()

export const normalizeAPICForForm = (value?: boolean): boolean => value !== false
export const normalizePAEForForm = (value?: boolean): boolean => value !== false

/** CPU 亲和性输入合法性（数字、逗号、空格、连字符） */
export const validateCPUAffinityInput = (value: string): boolean => {
  if (!value || !value.trim()) return true
  return /^[0-9,\s-]+$/.test(value.trim())
}
