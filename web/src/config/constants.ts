/**
 * 全局常量配置
 * 所有硬编码的常量集中在此管理，避免散落在业务代码中
 */

/** API 基础路径（开发环境通过 Vite 代理到后端） */
export const API_BASE_URL: string = import.meta.env.VITE_APP_BASE_API || '/api'

/** localStorage 存储键位（与旧版前端保持一致，便于平滑迁移） */
export const STORAGE_KEYS = {
  token: 'token',
  username: 'username',
  role: 'role',
  cloudType: 'cloud_type',
  security: 'security',
  theme: 'theme_mode',
  siteTitle: 'site_title',
  sidebarCollapsed: 'sidebar_collapsed',
  sponsorFirstVisit: 'sponsor_first_visit',
  sponsorLastClosed: 'sponsor_last_closed',
} as const

/** 外部链接（开源仓库 / 赞助相关） */
export const EXTERNAL_LINKS = {
  /** GitHub 开源仓库 */
  github: 'https://github.com/QVMConsole/QVMConsole',
  /** 爱发电赞助页 */
  sponsorPay: 'https://www.ifdian.net/item/ff67c598693811f1836452540025c377?utm_source=copylink&utm_medium=link',
  /** 赞助者权益说明文档 */
  sponsorBenefits: 'https://qvmcdocs.xiaozhuhouses.asia/docs/install/sponsorship',
} as const

/** 云类型 */
export const CLOUD_TYPES = {
  elastic: 'elastic',
  lightweight: 'lightweight',
} as const
export type CloudType = (typeof CLOUD_TYPES)[keyof typeof CLOUD_TYPES]

/** 用户角色 */
export const ROLES = {
  admin: 'admin',
  user: 'user',
} as const

/** 登录返回的阶段标识（多阶段登录流程） */
export const LOGIN_STAGES = {
  success: 'success',
  bootstrapSecurity: 'bootstrap_security',
  loginVerify: 'login_verify',
} as const
export type LoginStage = (typeof LOGIN_STAGES)[keyof typeof LOGIN_STAGES]

/** 主题模式 */
export const THEME_MODES = {
  light: 'light',
  dark: 'dark',
  system: 'system',
} as const
export type ThemeMode = (typeof THEME_MODES)[keyof typeof THEME_MODES]
