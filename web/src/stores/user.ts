/**
 * 用户状态管理（zustand）
 * 与旧版行为对齐：token/username/role/cloud_type/security 持久化到 localStorage
 */
import { create } from 'zustand'
import { CLOUD_TYPES, ROLES, STORAGE_KEYS, type CloudType } from '@/config/constants'
import type { SecurityState } from '@/types/api'

interface UserState {
  token: string
  username: string
  role: string
  cloudType: CloudType
  security: SecurityState | null
  /** 是否为管理员 */
  isAdmin: () => boolean
  setToken: (token: string) => void
  setUserInfo: (
    username: string,
    role: string,
    security?: SecurityState | null,
    cloudType?: CloudType,
  ) => void
  setSecurity: (security: SecurityState | null) => void
  logout: () => void
}

function readSecurity(): SecurityState | null {
  try {
    const raw = localStorage.getItem(STORAGE_KEYS.security)
    return raw ? (JSON.parse(raw) as SecurityState) : null
  } catch {
    return null
  }
}

function normalizeCloudType(value: string | null): CloudType {
  return value === CLOUD_TYPES.lightweight ? CLOUD_TYPES.lightweight : CLOUD_TYPES.elastic
}

export const useUserStore = create<UserState>()((set, get) => ({
  token: localStorage.getItem(STORAGE_KEYS.token) || '',
  username: localStorage.getItem(STORAGE_KEYS.username) || '',
  role: localStorage.getItem(STORAGE_KEYS.role) || '',
  cloudType: normalizeCloudType(localStorage.getItem(STORAGE_KEYS.cloudType)),
  security: readSecurity(),

  isAdmin: () => get().role === ROLES.admin,

  setToken: (token) => {
    localStorage.setItem(STORAGE_KEYS.token, token)
    set({ token })
  },

  setUserInfo: (username, role, security = null, cloudType = CLOUD_TYPES.elastic) => {
    localStorage.setItem(STORAGE_KEYS.username, username)
    localStorage.setItem(STORAGE_KEYS.role, role)
    localStorage.setItem(STORAGE_KEYS.security, JSON.stringify(security ?? null))
    localStorage.setItem(STORAGE_KEYS.cloudType, cloudType)
    set({ username, role, security, cloudType })
  },

  setSecurity: (security) => {
    localStorage.setItem(STORAGE_KEYS.security, JSON.stringify(security ?? null))
    set({ security })
  },

  logout: () => {
    localStorage.removeItem(STORAGE_KEYS.token)
    localStorage.removeItem(STORAGE_KEYS.username)
    localStorage.removeItem(STORAGE_KEYS.role)
    localStorage.removeItem(STORAGE_KEYS.cloudType)
    localStorage.removeItem(STORAGE_KEYS.security)
    sessionStorage.removeItem('2fa_recommended')
    set({
      token: '',
      username: '',
      role: '',
      cloudType: CLOUD_TYPES.elastic,
      security: null,
    })
  },
}))
