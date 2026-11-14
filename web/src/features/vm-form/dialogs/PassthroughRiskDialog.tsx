/** 查看全量 PCI 设备前的风险确认，每次打开都必须完整等待。 */
import { useEffect, useRef, useState } from 'react'
import { Banner, Modal } from '@douyinfe/semi-ui'

const RISK_CONFIRM_DELAY_MS = 5000

interface PassthroughRiskDialogProps {
  visible: boolean
  onClose: () => void
  onConfirm: () => void
}

export default function PassthroughRiskDialog({ visible, onClose, onConfirm }: PassthroughRiskDialogProps) {
  const [remainingSeconds, setRemainingSeconds] = useState(RISK_CONFIRM_DELAY_MS / 1000)
  const deadlineRef = useRef<number | null>(null)

  useEffect(() => {
    if (!visible) {
      deadlineRef.current = null
      return
    }
    const deadline = performance.now() + RISK_CONFIRM_DELAY_MS
    deadlineRef.current = deadline
    const timer = window.setInterval(() => {
      setRemainingSeconds(Math.max(0, Math.ceil((deadline - performance.now()) / 1000)))
    }, 100)
    return () => {
      window.clearInterval(timer)
      deadlineRef.current = null
    }
  }, [visible])

  const handleConfirm = () => {
    // 除禁用按钮外，再检查实际截止时间，防止提前触发确认回调。
    if (!visible || deadlineRef.current === null || performance.now() < deadlineRef.current) return
    setRemainingSeconds(RISK_CONFIRM_DELAY_MS / 1000)
    onConfirm()
  }

  const handleClose = () => {
    setRemainingSeconds(RISK_CONFIRM_DELAY_MS / 1000)
    onClose()
  }

  return (
    <Modal
      title="查看全部直通设备的风险"
      visible={visible}
      afterClose={() => setRemainingSeconds(RISK_CONFIRM_DELAY_MS / 1000)}
      onCancel={handleClose}
      onOk={handleConfirm}
      okText={remainingSeconds > 0 ? `请阅读风险（${remainingSeconds} 秒）` : '我已了解风险，继续查看'}
      okButtonProps={{ type: 'danger', disabled: remainingSeconds > 0 }}
      cancelText="取消"
      maskClosable={false}
      width={560}
    >
      <Banner
        type="danger"
        closeIcon={null}
        description="全部设备列表不经过白名单或准入规则过滤，会包含宿主机正在使用的系统关键设备。"
      />
      <div style={{ color: 'var(--qvm-text-1)', lineHeight: 1.8, marginTop: 12 }}>
        <p>误将磁盘控制器、网卡、USB 控制器、桥接设备或 CPU 系统设备用于直通，可能导致宿主机断网、键鼠失效、存储中断、数据丢失或系统崩溃。</p>
        <p>USB 控制器直通会影响连接在该控制器上的所有 USB 设备。请核实 PCI 地址、宿主机占用情况和 IOMMU 分组，并准备本地或带外恢复方式。</p>
        <p>继续后可以选择原先被过滤的设备并实际直通，绑定和添加时将跳过默认准入白名单，包括宿主机占用、关键设备和分组风险策略。列表中的风险说明仅供参考，不再阻止选择。</p>
        <p>设备必须真实存在、具备 IOMMU 分组且未分配给其他虚拟机；修改直通配置需要虚拟机关机。确认仅适用于本次从完整列表选入的设备。</p>
      </div>
    </Modal>
  )
}
