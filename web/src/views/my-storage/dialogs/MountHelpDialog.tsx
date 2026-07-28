/**
 * 虚拟机内挂载命令说明弹窗：展示 9p VirtFS 挂载的 Linux 命令，支持一键复制
 * 迁移自旧前端 views/storage/index.vue 的 mount help dialog
 */
import { Banner, Button, Modal, Toast } from '@douyinfe/semi-ui'
import { copyTextWithFallback } from '@/utils/clipboard'
import { useMountModalLifecycle } from '@/hooks/useMountModalLifecycle'

interface MountHelpDialogProps {
  /** 挂载标签（user_xxx_category 格式） */
  tag: string
  /** 是否只读模式 */
  readonly: boolean
  onClose: () => void
}

export default function MountHelpDialog({ tag, readonly, onClose }: MountHelpDialogProps) {
  const { modalVisible, requestClose, afterModalClose } = useMountModalLifecycle(onClose)
  const roOpt = readonly ? ',ro' : ''

  const cmdMkdir = `mkdir -p /mnt/${tag}`
  const cmdMount = `mount -t 9p -o trans=virtio,version=9p2000.L${roOpt} ${tag} /mnt/${tag}`
  const cmdFstab = `echo '${tag} /mnt/${tag} 9p trans=virtio,version=9p2000.L${roOpt},nofail 0 0' >> /etc/fstab`

  const handleCopy = () => {
    const cmds = `${cmdMkdir}\n${cmdMount}\n${cmdFstab}`
    copyTextWithFallback(cmds)
      .then(() => Toast.success('命令已复制到剪贴板'))
      .catch(() => Toast.warning('复制失败，请手动复制'))
  }

  return (
    <Modal
      title="虚拟机内挂载说明"
      visible={modalVisible}
      afterClose={afterModalClose}
      onCancel={requestClose}
      width={620}
      footer={
        <>
          <Button onClick={requestClose}>关闭</Button>
          <Button type="primary" onClick={handleCopy}>
            复制命令
          </Button>
        </>
      }
    >
      <Banner
        type="warning"
        closeIcon={null}
        description="仅支持 Linux 虚拟机，Windows 不支持 9p VirtFS 协议"
        style={{ marginBottom: 16 }}
      />
      <div className="mst-mount-cmds">
        <p className="cmd-label"># 步骤 1: 创建挂载点</p>
        <code>{cmdMkdir}</code>
        <p className="cmd-label"># 步骤 2: 挂载共享目录</p>
        <code>{cmdMount}</code>
        <p className="cmd-label"># 步骤 3: 开机自动挂载（可选）</p>
        <code>{cmdFstab}</code>
      </div>
    </Modal>
  )
}
