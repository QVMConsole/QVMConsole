/**
 * 高风险操作二次验证状态管理（HTTP 428 挑战流程）
 * 后端对敏感操作（删除虚拟机、重置密码等）返回 428，
 * 前端弹出验证框（2FA / 邮箱验证码），验证通过后携带 X-High-Risk-Token 重试原请求。
 */
import { create } from 'zustand'

/** 挑战数据（后端 428 响应 data 字段） */
export interface HighRiskChallengeData {
  method?: string
  masked_email?: string
  challenge_id?: number
  operation?: string
  has_recovery?: boolean
}

/** 提交给 /auth/high-risk/verify 的载荷 */
export interface HighRiskVerifyPayload {
  method: string
  code: string
  challenge_id?: number
  operation?: string
}

interface HighRiskState {
  /** 当前待处理的挑战（null 表示无弹窗） */
  pending: HighRiskChallengeData | null
  /** 发起一次挑战，返回用户输入的验证载荷；用户取消时 reject */
  ask: (data: HighRiskChallengeData) => Promise<HighRiskVerifyPayload>
  /** 用户提交验证码 */
  submit: (payload: HighRiskVerifyPayload) => void
  /** 用户取消验证 */
  cancel: () => void
}

/** 取消错误标记，业务侧可据此静默处理 */
export class HighRiskCancelledError extends Error {
  readonly isHighRiskCancelled = true
  constructor() {
    super('已取消高风险验证')
    this.name = 'HighRiskCancelledError'
  }
}

export function isHighRiskCancelledError(error: unknown): boolean {
  return error instanceof HighRiskCancelledError || !!(error as { isHighRiskCancelled?: boolean })?.isHighRiskCancelled
}

// Promise 决议句柄保存在模块内，不进入状态（避免序列化问题）
let resolver: ((payload: HighRiskVerifyPayload) => void) | null = null
let rejecter: ((error: Error) => void) | null = null

export const useHighRiskStore = create<HighRiskState>()((set) => ({
  pending: null,

  ask: (data) => {
    return new Promise<HighRiskVerifyPayload>((resolve, reject) => {
      resolver = resolve
      rejecter = reject
      set({ pending: data })
    })
  },

  submit: (payload) => {
    resolver?.(payload)
    resolver = null
    rejecter = null
    set({ pending: null })
  },

  cancel: () => {
    rejecter?.(new HighRiskCancelledError())
    resolver = null
    rejecter = null
    set({ pending: null })
  },
}))
