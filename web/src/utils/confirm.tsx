/**
 * Promise 化的确认弹窗（基于 Semi Modal.confirm）
 * 用于电源操作、批量操作等需要二次确认的场景
 */
import type { ReactNode } from 'react'
import { Modal } from '@douyinfe/semi-ui'

export interface ConfirmOptions {
  title: string
  content: ReactNode
  okText?: string
  cancelText?: string
  /** 危险操作：确认按钮使用危险色 */
  danger?: boolean
}

/** 弹出确认框，resolve(true)=确认，resolve(false)=取消/关闭 */
export function confirmModal(options: ConfirmOptions): Promise<boolean> {
  return new Promise((resolve) => {
    Modal.confirm({
      title: options.title,
      content: options.content,
      okText: options.okText ?? '确定',
      cancelText: options.cancelText ?? '取消',
      okButtonProps: options.danger ? { type: 'danger' } : undefined,
      onOk: () => resolve(true),
      onCancel: () => resolve(false),
    })
  })
}
