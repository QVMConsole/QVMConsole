/**
 * 虚拟机表单作用域 Provider
 * 创建向导（CreateVmWizard）与编辑表单（EditVmForm）各自搭建 Provider，
 * Section 组件无需关心宿主差异，实现一处改动两处生效。
 */
import type { ReactNode } from 'react'
import { VmFormScope, type VmFormScopeValue } from './scopeContext'

export function VmFormProvider({ value, children }: { value: VmFormScopeValue; children: ReactNode }) {
  return <VmFormScope.Provider value={value}>{children}</VmFormScope.Provider>
}
