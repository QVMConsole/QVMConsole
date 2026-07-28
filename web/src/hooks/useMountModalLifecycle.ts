/**
 * 为按需挂载的 Semi Modal 保留完整离场生命周期。
 *
 * 这类弹窗关闭时不能立刻通知父组件卸载，否则 Semi 的缩小动画不会执行。
 * requestClose 先切换 visible，等 afterClose 触发后再由父组件清理业务状态。
 */
import { useCallback, useEffect, useRef, useState } from 'react'

export function useMountModalLifecycle(onExited: () => void) {
  const [modalVisible, setModalVisible] = useState(true)
  const exitingRef = useRef(false)
  const onExitedRef = useRef(onExited)

  useEffect(() => {
    onExitedRef.current = onExited
  }, [onExited])

  const requestClose = useCallback(() => {
    exitingRef.current = true
    setModalVisible(false)
  }, [])

  const afterModalClose = useCallback(() => {
    if (!exitingRef.current) return
    exitingRef.current = false
    onExitedRef.current()
  }, [])

  return { modalVisible, requestClose, afterModalClose }
}
