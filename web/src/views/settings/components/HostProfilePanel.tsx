/**
 * 宿主机挡位面板（KSM / zRAM 共用）
 * 挡位单选按钮 + 状态标签 + 挡位说明卡片网格
 */
import { Button, RadioGroup, Radio, Tag } from '@douyinfe/semi-ui'
import { IconInfoCircle } from '@douyinfe/semi-icons'
import type { HostProfileOption } from '@/api/settings'

interface HostProfilePanelProps {
  options: HostProfileOption[]
  selected: string
  disabled: boolean
  /** 当前是否运行中 */
  enabled: boolean
  /** 持久配置挡位名称（未配置时不显示） */
  persistentName?: string
  /** 状态摘要文字 */
  summary: string
  onChange: (profileKey: string) => void
  onHelp: () => void
}

export default function HostProfilePanel({
  options,
  selected,
  disabled,
  enabled,
  persistentName,
  summary,
  onChange,
  onHelp,
}: HostProfilePanelProps) {
  return (
    <div className="stg-host-field">
      <div className="stg-host-row">
        <RadioGroup
          type="button"
          value={selected}
          disabled={disabled}
          onChange={(e) => onChange(e.target.value as string)}
        >
          {options.map((profile) => (
            <Radio key={profile.key} value={profile.key}>
              {profile.name}
            </Radio>
          ))}
        </RadioGroup>
        <Tag size="small" color={enabled ? 'green' : 'grey'}>
          {enabled ? '运行中' : '已关闭'}
        </Tag>
        {persistentName && (
          <Tag size="small" color="cyan">
            持久配置：{persistentName}
          </Tag>
        )}
        <Button size="small" theme="borderless" type="primary" onClick={onHelp}>
          说明
        </Button>
      </div>
      <div className="stg-row-tip">
        <IconInfoCircle size="small" />
        <span>{summary}</span>
      </div>
      <div className="stg-profile-grid">
        {options.map((profile) => (
          <div
            key={profile.key}
            className={`stg-profile-item${selected === profile.key ? ' active' : ''}`}
          >
            <strong>{profile.name}</strong>
            <span>{profile.description}</span>
          </div>
        ))}
      </div>
    </div>
  )
}
