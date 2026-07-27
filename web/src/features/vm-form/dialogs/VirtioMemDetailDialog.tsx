/**
 * Windows 弹性内存（virtio-mem）说明弹窗
 */
import { Banner, Button, Modal } from '@douyinfe/semi-ui'

interface VirtioMemDetailDialogProps {
  visible: boolean
  onClose: () => void
}

export default function VirtioMemDetailDialog({ visible, onClose }: VirtioMemDetailDialogProps) {
  return (
    <Modal
      title="Windows 弹性内存（实验）"
      visible={visible}
      onCancel={onClose}
      footer={
        <Button type="primary" theme="solid" onClick={onClose}>
          我知道了
        </Button>
      }
      width={620}
      closeOnEsc
    >
      <Banner
        type="warning"
        closeIcon={null}
        style={{ marginBottom: 16 }}
        description="该功能基于 virtio-mem / viomem 驱动，当前作为实验能力提供，请先在测试虚拟机验证后再用于生产。"
      />
      <div className="qvm-vf-richtext">
        <p>启用后，主表单填写的内存会作为规格内存。系统自动将基础内存设为规格的 50%（最低 1GB），并把可调度上限设为规格内存上浮 30%。</p>
        <p>虚拟机会先启动基础内存，再通过 virtio-mem 设备按需热插拔额外内存。宿主机占用会跟随基础内存和已插入的弹性内存变化。</p>
        <p>自动调度会按虚拟机内部内存使用率判断：超过 70% 时每次扩容 1GB，低于 50% 时计算缩容目标，并确保缩容后的使用率不超过 70%。</p>
        <p>首次启用或修改基础配置需要虚拟机关机后应用；Windows 内部必须已安装 VirtIO Viomem Driver，否则只能看到设备但无法正常调整。</p>
        <p>缩小弹性内存时，Windows 可能因为当前占用或内存碎片无法立刻全部释放，面板会尽量调低 requested，但实际 current 可能短时间保留一部分。</p>
        <p>该模式不使用气球回收机制。用户看到的内存规格始终按最大规格理解，基础内存由系统自动计算。</p>
      </div>
    </Modal>
  )
}
