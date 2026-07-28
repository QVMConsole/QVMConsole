/** 只读预览弹窗，关闭后再通知父组件卸载。 */
import type { ReactNode } from 'react'
import { Button, Modal } from '@douyinfe/semi-ui'
import { useMountModalLifecycle } from '@/hooks/useMountModalLifecycle'

interface PreviewModalProps {
  title: ReactNode
  width?: number | string
  onClose: () => void
  children: ReactNode
}

export default function PreviewModal({ title, width, onClose, children }: PreviewModalProps) {
  const { modalVisible, requestClose, afterModalClose } = useMountModalLifecycle(onClose)

  return (
    <Modal
      title={title}
      visible={modalVisible}
      afterClose={afterModalClose}
      onCancel={requestClose}
      footer={
        <Button type="primary" onClick={requestClose}>
          关闭
        </Button>
      }
      width={width}
      closeOnEsc
    >
      {children}
    </Modal>
  )
}
