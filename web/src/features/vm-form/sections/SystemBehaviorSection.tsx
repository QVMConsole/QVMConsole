/**
 * 系统行为分区（创建 / 编辑共用）：Watchdog 与开机自启
 */
import { Select } from '@douyinfe/semi-ui'
import { IconBolt } from '@douyinfe/semi-icons'
import SectionCard from './SectionCard'
import FormField from './FormField'
import TextSwitch from './TextSwitch'
import { useVmFormScope } from '../scopeContext'
import { WATCHDOG_OPTIONS } from '../constants'

interface SystemBehaviorSectionProps {
  /** 是否显示 Watchdog（编辑模式 watchdog 不在可编辑字段内，仅创建显示） */
  showWatchdog?: boolean
}

export default function SystemBehaviorSection({ showWatchdog = true }: SystemBehaviorSectionProps) {
  const { form } = useVmFormScope()
  const { form: f, setField } = form

  return (
    <SectionCard icon={<IconBolt />} title="系统行为">
      {showWatchdog && (
        <FormField label="监督者 (Watchdog)">
          <Select
            style={{ width: '100%' }}
            value={f.watchdog}
            onChange={(v) => setField('watchdog', v as string)}
            optionList={WATCHDOG_OPTIONS.map((item) => ({ value: item.value, label: item.label }))}
          />
        </FormField>
      )}
      <FormField label="开机自启">
        <TextSwitch checked={f.autostart} onChange={(v) => setField('autostart', v)} />
      </FormField>
    </SectionCard>
  )
}
