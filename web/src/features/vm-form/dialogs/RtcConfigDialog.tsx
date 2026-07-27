/**
 * RTC 配置弹窗（创建 / 编辑共用）
 */
import { Button, Input, Modal, Radio } from '@douyinfe/semi-ui'
import { useVmFormScope } from '../scopeContext'
import { ADVANCED_HELP_TEXT } from '../constants'
import FormField from '../sections/FormField'

interface RtcConfigDialogProps {
  visible: boolean
  onClose: () => void
}

export default function RtcConfigDialog({ visible, onClose }: RtcConfigDialogProps) {
  const { form } = useVmFormScope()
  const { form: f, setField } = form

  return (
    <Modal
      title="RTC 配置"
      visible={visible}
      onCancel={onClose}
      footer={
        <Button type="primary" theme="solid" onClick={onClose}>
          关闭
        </Button>
      }
      width={560}
      closeOnEsc
    >
      <FormField
        label="RTC 时间基准"
        help={ADVANCED_HELP_TEXT.rtc}
        tip="Linux 通常使用 UTC；Windows 默认使用本地时间。运行中的虚拟机修改后需重启生效。"
      >
        <Radio.Group
          type="button"
          value={f.rtc_offset}
          onChange={(e) => setField('rtc_offset', e.target.value)}
          options={[
            { label: 'UTC', value: 'utc' },
            { label: '本地时间', value: 'localtime' },
          ]}
        />
      </FormField>
      <FormField
        label="RTC 开始日期"
        help={ADVANCED_HELP_TEXT.rtcStartDate}
        tip="默认 `now` 表示每次启动时使用当前时间。若填写固定日期时间，将按该时间初始化 RTC，并切换为固定时间模式。"
      >
        <Input
          value={f.rtc_startdate}
          onChange={(v) => setField('rtc_startdate', v)}
          placeholder="now 或 2026-04-26 12:00:00"
          showClear
        />
      </FormField>
    </Modal>
  )
}
