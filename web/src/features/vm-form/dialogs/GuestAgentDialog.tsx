/**
 * QEMU Guest Agent 配置弹窗（创建 / 编辑共用）
 */
import { Banner, Button, Modal } from '@douyinfe/semi-ui'
import { useVmFormScope } from '../scopeContext'
import { ADVANCED_HELP_TEXT } from '../constants'
import FormField from '../sections/FormField'
import TextSwitch from '../sections/TextSwitch'

interface GuestAgentDialogProps {
  visible: boolean
  onClose: () => void
}

export default function GuestAgentDialog({ visible, onClose }: GuestAgentDialogProps) {
  const { form } = useVmFormScope()
  const { form: f, setField } = form

  return (
    <Modal
      title="QEMU Guest Agent 配置"
      visible={visible}
      onCancel={onClose}
      footer={
        <Button type="primary" theme="solid" onClick={onClose}>
          关闭
        </Button>
      }
      width={620}
      closeOnEsc
    >
      <Banner
        type="info"
        closeIcon={null}
        style={{ marginBottom: 16 }}
        description="客户机代理安装在虚拟机系统内部，可协助宿主机获取来宾信息、执行更可靠的关机，并为冻结文件系统后再做快照等场景提供支持。"
      />
      <FormField
        label="使用 QEMU Guest Agent"
        help={ADVANCED_HELP_TEXT.guestAgent}
        tip="Linux / Windows 服务器建议启用，但需要虚拟机内部已安装 qemu-guest-agent。编辑已存在虚拟机时，建议关机后修改并重新开机生效。"
      >
        <TextSwitch
          checked={f.guest_agent.enabled}
          onChange={(v) => setField('guest_agent', { enabled: v })}
        />
      </FormField>
    </Modal>
  )
}
