/**
 * 调度与高级 Tab：动态内存调度 / 显示协议 / 批量克隆 / 救援系统 / CPU 亲和性预设
 */
import { useEffect, useState } from 'react'
import { Button, Input, InputNumber, Select, Toast, Tooltip } from '@douyinfe/semi-ui'
import {
  IconClockStroked,
  IconCopy,
  IconDelete,
  IconDesktop,
  IconPlus,
  IconRefresh,
  IconSetting,
  IconShield,
  IconTick,
} from '@douyinfe/semi-icons'
import TextSwitch from '@/features/vm-form/sections/TextSwitch'
import {
  getCPUAffinityPresets,
  saveCPUAffinityPresets,
  type CpuAffinityPreset,
} from '@/api/settings'
import { getAllISOs, type IsoItem } from '@/api/infra'
import { SectionHead, SettingRow } from './SettingRow'
import NumField from './NumField'
import type { SettingsTabProps } from '../types'

export default function AdvancedTab({ form, patch }: SettingsTabProps) {
  // 救援系统 ISO 候选列表
  const [isoList, setIsoList] = useState<IsoItem[]>([])
  // CPU 亲和性预设（独立保存，不随整体表单提交）
  const [presets, setPresets] = useState<CpuAffinityPreset[]>([])
  const [presetsSaving, setPresetsSaving] = useState(false)

  const loadPresets = async (showMessage = false) => {
    try {
      const res = await getCPUAffinityPresets()
      setPresets((res.data || []).map((p) => ({ ...p })))
      if (showMessage) Toast.success('预设已重置')
    } catch {
      // 请求层已统一提示
    }
  }

  useEffect(() => {
    void loadPresets()
    getAllISOs()
      .then((res) => setIsoList(res.data || []))
      .catch(() => {})
  }, [])

  const handleSavePresets = async () => {
    setPresetsSaving(true)
    try {
      const payload = { presets: presets.filter((p) => p.name.trim() && p.value.trim()) }
      const res = await saveCPUAffinityPresets(payload)
      Toast.success(res.message || '预设已保存')
      await loadPresets()
    } catch {
      // 请求层已统一提示
    } finally {
      setPresetsSaving(false)
    }
  }

  return (
    <div className="stg-tab-pane">
      <SectionHead icon={<IconClockStroked />} title="动态内存调度" />

      <SettingRow
        label="启用自动调度"
        tip="仅对已启用动态内存且允许自动气球调度的 VM 生效 | 环境变量: KVM_DYNAMIC_MEMORY_SCHEDULER_ENABLED"
      >
        <TextSwitch
          checked={form.dynamic_memory_scheduler_enabled}
          onChange={(v) => patch({ dynamic_memory_scheduler_enabled: v })}
        />
      </SettingRow>

      <div className="stg-field-grid">
        <NumField
          label="调度间隔"
          suffix="秒"
          value={form.dynamic_memory_interval_seconds}
          onChange={(v) => patch({ dynamic_memory_interval_seconds: v })}
          min={10}
          max={3600}
          tip="默认 30"
        />
        <NumField
          label="调整冷却"
          suffix="秒"
          value={form.dynamic_memory_cooldown_seconds}
          onChange={(v) => patch({ dynamic_memory_cooldown_seconds: v })}
          min={30}
          max={7200}
          tip="同一 VM 两次调整之间的最短间隔"
        />
        <NumField
          label="宿主保留内存"
          suffix="MB"
          value={form.dynamic_memory_host_reserve_mb}
          onChange={(v) => patch({ dynamic_memory_host_reserve_mb: v })}
          min={512}
          max={1048576}
          tip="默认 2048"
        />
        <NumField
          label="宿主保留比例"
          suffix="%"
          value={form.dynamic_memory_host_reserve_percent}
          onChange={(v) => patch({ dynamic_memory_host_reserve_percent: v })}
          min={5}
          max={80}
          tip="最终保留值取固定值与比例值中的较大者"
        />
        <NumField
          label="增长阈值"
          suffix="%"
          value={form.dynamic_memory_increase_threshold_percent}
          onChange={(v) => patch({ dynamic_memory_increase_threshold_percent: v })}
          min={5}
          max={50}
          tip="可用内存比例低于该值时尝试增长"
        />
        <NumField
          label="回收阈值"
          suffix="%"
          value={form.dynamic_memory_reclaim_threshold_percent}
          onChange={(v) => patch({ dynamic_memory_reclaim_threshold_percent: v })}
          min={10}
          max={90}
          tip="空闲内存比例高于该值时才考虑回收"
        />
        <NumField
          label="首次观察期"
          suffix="小时"
          value={form.dynamic_memory_observation_hours}
          onChange={(v) => patch({ dynamic_memory_observation_hours: v })}
          min={0}
          max={168}
          tip="观察期内不自动回收到启动内存以下"
        />
        <NumField
          label="调度事件保留"
          suffix="小时"
          value={form.scheduler_event_retention_hours}
          onChange={(v) => patch({ scheduler_event_retention_hours: v })}
          min={1}
          max={2160}
          tip="默认 168，小于该时长的调度事件会被后台定时清理"
        />
      </div>
      <div className="stg-plain-tip">
        环境变量前缀: KVM_DYNAMIC_MEMORY_* | 调度事件保留: KVM_SCHEDULER_EVENT_RETENTION_HOURS
      </div>

      <SectionHead icon={<IconDesktop />} title="显示协议" />

      <SettingRow
        label="SPICE 默认开启"
        tip="开启后，新建虚拟机表单的 SPICE 开关初始为开启状态（每台 VM 仍可单独关闭）。部分机器/客户机不支持 SPICE，默认关闭更稳妥 | 环境变量: KVM_SPICE_ENABLED_BY_DEFAULT"
      >
        <TextSwitch
          checked={form.spice_enabled_by_default}
          onChange={(v) => patch({ spice_enabled_by_default: v })}
        />
      </SettingRow>

      <SectionHead icon={<IconCopy />} title="批量克隆" />

      <SettingRow
        label="最大同时克隆数"
        tip="批量克隆时最多允许同时克隆的虚拟机数量，默认 10，设为 1 时退化为顺序克隆 | 环境变量: KVM_BATCH_CLONE_MAX_CONCURRENCY"
      >
        <InputNumber
          value={form.batch_clone_max_concurrency}
          onNumberChange={(v) => patch({ batch_clone_max_concurrency: v })}
          min={1}
          max={100}
          style={{ width: '100%' }}
        />
      </SettingRow>

      <SectionHead icon={<IconShield />} title="救援系统" />

      <SettingRow
        label="救援系统 ISO"
        tip="选择一个 ISO 文件作为虚拟机救援系统，列表来源于 ISO 存放位置 | 环境变量: KVM_RESCUE_ISO"
      >
        <Select
          value={form.rescue_iso || undefined}
          onChange={(v) => patch({ rescue_iso: (v as string) || '' })}
          placeholder="请选择救援系统 ISO"
          showClear
          filter
          style={{ width: '100%' }}
          optionList={isoList.map((iso) => ({ label: iso.name, value: iso.path }))}
        />
      </SettingRow>

      <SectionHead icon={<IconSetting />} title="CPU 亲和性预设" />

      <div className="stg-preset-manager">
        {presets.length === 0 && (
          <div className="stg-plain-tip">暂无预设，可点击下方按钮添加。</div>
        )}
        {presets.map((preset, idx) => (
          <div className="stg-preset-row" key={idx}>
            <Input
              value={preset.name}
              onChange={(v) =>
                setPresets((list) => list.map((p, i) => (i === idx ? { ...p, name: v } : p)))
              }
              placeholder="预设名称"
              style={{ width: 200 }}
            />
            <Input
              value={preset.value}
              onChange={(v) =>
                setPresets((list) => list.map((p, i) => (i === idx ? { ...p, value: v } : p)))
              }
              placeholder="核心值，如 0-3"
              style={{ width: 260 }}
            />
            <Tooltip content="删除预设" position="top">
              <Button
                type="danger"
                theme="borderless"
                icon={<IconDelete />}
                onClick={() => setPresets((list) => list.filter((_, i) => i !== idx))}
              />
            </Tooltip>
          </div>
        ))}
        <div className="stg-preset-actions">
          <Button
            icon={<IconPlus />}
            onClick={() => setPresets((list) => [...list, { name: '', value: '' }])}
          >
            添加预设
          </Button>
          <Button
            type="primary"
            theme="light"
            icon={<IconTick />}
            loading={presetsSaving}
            onClick={() => void handleSavePresets()}
          >
            保存预设
          </Button>
          <Button icon={<IconRefresh />} onClick={() => void loadPresets(true)}>
            重置
          </Button>
        </div>
      </div>
    </div>
  )
}
