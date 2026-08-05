/**
 * Axios 实例封装（全局请求层）
 * 行为与旧版 request.js 对齐：
 * - 自动携带 Bearer Token
 * - NProgress 全局进度条（支持 silent 静默请求）
 * - 统一错误提示（Semi Toast）
 * - 401 自动登出并跳转登录页
 * - 428 高风险二次验证：弹窗收集验证码 → 调 /auth/high-risk/verify → 携带 X-High-Risk-Token 重试原请求
 */
import axios, { AxiosError, type AxiosResponse, type InternalAxiosRequestConfig } from 'axios'
import { createElement } from 'react'
import { Toast } from '@douyinfe/semi-ui'
import NProgress from 'nprogress'
import { API_BASE_URL } from '@/config/constants'
import TaskMessage from '@/components/business/TaskMessage'
import { useUserStore } from '@/stores/user'
import {
  useHighRiskStore,
  isHighRiskCancelledError,
  type HighRiskVerifyPayload,
} from '@/stores/highRisk'
import type { ApiResponse } from '@/types/api'

// 扩展 axios 请求配置（业务自定义字段）
declare module 'axios' {
  interface AxiosRequestConfig {
    /** 静默请求：不触发全局进度条 */
    silent?: boolean
    /** 跳过 428 高风险验证自动处理（业务自行处理时使用） */
    skipHighRiskHandler?: boolean
    /** 内部标记：高风险验证后已重试过，避免死循环 */
    _highRiskRetried?: boolean
  }
}

NProgress.configure({ showSpinner: false })

let requestCount = 0
function startLoading() {
  if (requestCount === 0) {
    NProgress.start()
  }
  requestCount++
}
function stopLoading() {
  if (requestCount > 0) {
    requestCount--
  }
  if (requestCount === 0) {
    NProgress.done()
  }
}

const service = axios.create({
  baseURL: API_BASE_URL,
  timeout: 60000,
})

/** 原始客户端：用于高风险验证等不走拦截器的请求 */
const rawClient = axios.create({
  baseURL: API_BASE_URL,
  timeout: 60000,
})

// ==================== 请求拦截器 ====================
service.interceptors.request.use(
  (config) => {
    if (!config.silent) {
      startLoading()
    }
    const { token } = useUserStore.getState()
    if (token && !config.headers.Authorization) {
      config.headers.Authorization = `Bearer ${token}`
    }
    return config
  },
  (error) => {
    stopLoading()
    return Promise.reject(error)
  },
)

// ==================== 工具函数 ====================
export function showError(message: string) {
  Toast.error({ content: createElement(TaskMessage, { message: message || '请求失败' }), duration: 5 })
}

declare global {
  interface Window {
    __qvmDebugShowError?: (message: string) => void
  }
}

if (import.meta.env.DEV) {
  window.__qvmDebugShowError = showError
}

function handleUnauthorized(message?: string) {
  const { logout } = useUserStore.getState()
  logout()
  if ((message || '').includes('登录环境发生变化')) {
    Toast.warning({ content: '登录环境发生变化，请重新登录', duration: 5 })
  }
  // 避免在登录页重复跳转
  if (window.location.pathname !== '/login') {
    const redirect = encodeURIComponent(window.location.pathname + window.location.search)
    window.location.href = `/login?redirect=${redirect}`
  }
}

// ==================== 428 高风险验证 ====================
let highRiskLock = false

async function verifyHighRiskChallenge(payload: HighRiskVerifyPayload, authHeader?: string) {
  const response = await rawClient.post<ApiResponse<{ verification_token?: string }>>(
    '/auth/high-risk/verify',
    payload,
    {
      headers: authHeader ? { Authorization: authHeader } : {},
    },
  )
  return response.data
}

async function handleHighRisk(error: AxiosError): Promise<AxiosResponse> {
  if (highRiskLock) {
    // 已有验证弹窗进行中，取消本次请求避免双弹窗
    throw new Error('高风险验证进行中，请稍后再试')
  }
  const originalConfig = error.config as InternalAxiosRequestConfig | undefined
  const responseData = (error.response?.data as ApiResponse | undefined)?.data as
    | Record<string, unknown>
    | undefined
  if (!originalConfig || originalConfig._highRiskRetried || originalConfig.skipHighRiskHandler) {
    throw error
  }

  highRiskLock = true
  try {
    const { token } = useUserStore.getState()
    const authHeader = (originalConfig.headers?.Authorization as string) || (token ? `Bearer ${token}` : '')
    const verifyPayload = await useHighRiskStore.getState().ask(responseData || {})
    const verifyRes = await verifyHighRiskChallenge(verifyPayload, authHeader)
    originalConfig._highRiskRetried = true
    const verificationToken = verifyRes?.data?.verification_token
    if (verificationToken) {
      originalConfig.headers['X-High-Risk-Token'] = verificationToken
    }
    return await service(originalConfig)
  } finally {
    highRiskLock = false
  }
}

// ==================== 响应拦截器 ====================
service.interceptors.response.use(
  (response) => {
    if (!response.config.silent) {
      stopLoading()
    }
    const res = response.data as ApiResponse
    // 二进制流（导出下载等）直接放行
    if (response.config.responseType === 'blob' || response.config.responseType === 'arraybuffer') {
      return response
    }
    if (res.code !== 200 && res.code !== 0) {
      showError(res.message || '请求失败')
      if (res.code === 401) {
        handleUnauthorized(res.message)
      }
      return Promise.reject(new Error(res.message || '请求失败'))
    }
    return res as unknown as AxiosResponse
  },
  async (error: AxiosError) => {
    if (!error.config?.silent) {
      stopLoading()
    }

    // 428：高风险二次验证
    if (error.response?.status === 428) {
      try {
        return await handleHighRisk(error)
      } catch (challengeError) {
        if (!isHighRiskCancelledError(challengeError)) {
          const err = challengeError as AxiosError
          showError(
            (err.response?.data as ApiResponse | undefined)?.message || err.message || '验证失败',
          )
        }
        return Promise.reject(challengeError)
      }
    }

    const serverMessage = (error.response?.data as ApiResponse | undefined)?.message
    const responseData = (error.response?.data as ApiResponse | undefined)?.data as
      | Record<string, unknown>
      | undefined
    // require_nvram_fix 场景由业务弹窗处理，不重复提示
    if (!responseData?.require_nvram_fix) {
      showError(serverMessage || error.message || '请求失败')
    }

    if (error.response?.status === 401) {
      handleUnauthorized(serverMessage)
    }

    return Promise.reject(error)
  },
)

export default service
