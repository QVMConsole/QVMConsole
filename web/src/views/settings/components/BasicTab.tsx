/**
 * 基础设置 Tab：站点展示 / 端口自动分配 / 访问链接 / 服务信息
 */
import { Input, InputNumber } from '@douyinfe/semi-ui'
import { IconInfoCircle, IconLink } from '@douyinfe/semi-icons'
import { SectionHead, SettingRow } from './SettingRow'
import type { SettingsTabProps } from '../types'

export default function BasicTab({ form, patch }: SettingsTabProps) {
  const portCount = Math.max((form.auto_port_end || 0) - (form.auto_port_start || 0), 0)

  return (
    <div className="stg-tab-pane">
      <SectionHead icon={<IconInfoCircle />} title="站点展示" />

      <SettingRow
        label="网站标题"
        tip="将用于登录页标题、侧边栏名称和浏览器标签页标题 | 环境变量: KVM_SITE_TITLE"
      >
        <Input
          value={form.site_title}
          onChange={(v) => patch({ site_title: v })}
          placeholder="请输入网站标题"
          maxLength={60}
        />
      </SettingRow>

      <SectionHead icon={<IconLink />} title="端口自动分配" />

      <SettingRow
        label="分配范围"
        tip={`端口转发自动分配时使用此范围（当前: ${form.auto_port_start} - ${form.auto_port_end}，共 ${portCount} 个端口） | 环境变量: KVM_AUTO_PORT_START / KVM_AUTO_PORT_END`}
      >
        <div className="stg-range-inputs">
          <InputNumber
            value={form.auto_port_start}
            onNumberChange={(v) => patch({ auto_port_start: v })}
            min={1024}
            max={65535}
            style={{ flex: 1 }}
          />
          <span className="stg-range-sep">—</span>
          <InputNumber
            value={form.auto_port_end}
            onNumberChange={(v) => patch({ auto_port_end: v })}
            min={1024}
            max={65535}
            style={{ flex: 1 }}
          />
        </div>
      </SettingRow>

      <SettingRow
        label="访问链接"
        tip="邀请注册、找回密码等邮件里的跳转链接会优先使用这里 | 环境变量: KVM_PUBLIC_BASE_URL"
      >
        <Input
          value={form.public_base_url}
          onChange={(v) => patch({ public_base_url: v })}
          placeholder="如 panel.example.com:8080 或 https://panel.example.com"
        />
      </SettingRow>

      <SectionHead icon={<IconInfoCircle />} title="服务信息" />

      <SettingRow label="服务端口" tip="环境变量: KVM_PORT（重启后生效）">
        <Input value={String(form.port)} disabled />
      </SettingRow>
    </div>
  )
}
