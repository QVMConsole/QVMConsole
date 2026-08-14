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
