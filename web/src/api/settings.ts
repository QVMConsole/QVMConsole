/**
 * 系统设置相关 API（地基阶段仅公开设置，完整设置模块后续迭代）
 */
import service from './client'
import type { ApiResponse } from '@/types/api'

/** 公开设置（无需登录即可获取） */
export interface PublicSettings {
  site_title?: string
  password_breach_check_enabled?: boolean
  spice_enabled_by_default?: boolean
  [key: string]: unknown
}

/** 获取公开设置 */
export function getPublicSettings() {
  return service.get<unknown, ApiResponse<PublicSettings>>('/public/settings', { silent: true })
}

/** 系统设置（创建表单仅需 iso_dir 等少数字段） */
export interface SystemSettings {
  iso_dir?: string
  [key: string]: unknown
}

/** 获取系统设置 */
export function getSettings() {
  return service.get<unknown, ApiResponse<SystemSettings>>('/settings', { silent: true })
}

/** 宿主机公开信息（架构 / SPICE 支持等） */
export interface PublicSystemInfo {
  arch?: string
  qemu_spice?: boolean
  [key: string]: unknown
}

/** 获取宿主机公开系统信息 */
export function getPublicSystemInfo() {
  return service.get<unknown, ApiResponse<PublicSystemInfo>>('/system-info', { silent: true })
}

/** 获取宿主机 CPU 物理核心数（CPU 热添加上限） */
export function getHostCPUCores() {
  return service.get<unknown, ApiResponse<{ cores: number }>>('/host/cpus', { silent: true })
}

/** CPU 亲和性预设 */
export interface CpuAffinityPreset {
  name: string
  value: string
}

/** 获取 CPU 亲和性预设列表 */
export function getCPUAffinityPresets() {
  return service.get<unknown, ApiResponse<CpuAffinityPreset[]>>('/cpu-affinity-presets', {
    silent: true,
  })
}
