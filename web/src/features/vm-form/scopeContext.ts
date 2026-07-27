/**
 * 虚拟机表单作用域（context 与 hook）
 * Section / dialogs 通过 useVmFormScope 获取表单状态、选项数据与运行上下文。
 */
import { createContext, useContext } from 'react'
import type { VmForm } from './useVmForm'
import type { VmFormOptions } from './useVmFormOptions'
import type { VmFormContext } from './types'

export interface VmFormScopeValue {
  form: VmForm
  options: VmFormOptions
  ctx: VmFormContext
}

export const VmFormScope = createContext<VmFormScopeValue | null>(null)

/** 获取表单作用域（Section / dialogs 内部使用） */
export function useVmFormScope(): VmFormScopeValue {
  const value = useContext(VmFormScope)
  if (!value) {
    throw new Error('useVmFormScope 必须在 VmFormProvider 内使用')
  }
  return value
}
