/**
 * CPU 与内存分区（创建 / 编辑共用）
 * CPU 核心、内存、CPU 热添加、CPU 限制（管理员）、动态内存。
 */
import { useState } from 'react'
import { Button, InputNumber, Radio, Tag } from '@douyinfe/semi-ui'
import { IconServer } from '@douyinfe/semi-icons'
import SectionCard from './SectionCard'
import FormField from './FormField'
import TextSwitch from './TextSwitch'
import { useVmFormScope } from '../scopeContext'
import MemoryDynamicDialog from '../dialogs/MemoryDynamicDialog'
import VirtioMemDetailDialog from '../dialogs/VirtioMemDetailDialog'

export default function CpuMemorySection() {
  const { form, ctx } = useVmFormScope()
  const { form: f, setField, handleDynamicMemoryEnabledChange, handleMemoryBackendChange, handleBaseMemoryChange, windowsElasticMemoryDisabled } = form
  const isEdit = ctx.mode === 'edit'
  const running = ctx.vmStatus === 'running'
  const [memoryDialogVisible, setMemoryDialogVisible] = useState(false)
  const [virtioMemDetailVisible, setVirtioMemDetailVisible] = useState(false)

  const vcpuMin = isEdit && running ? ctx.editOrigVcpu || 1 : 1
  const vcpuMax = ctx.hostCores > 0 ? ctx.hostCores : 64
  const memoryMin = isEdit && running ? ctx.editOrigMemory || 1 : 1
  const memoryValue = isEdit ? f.memory : f.ram

  const dynamicTip =
    f.memory_backend === 'virtio_mem'
      ? '弹性内存：设定内存作为规格内存，基础内存自动按 50% 计算，并额外提供 30% 突发上限；运行后按使用率自动伸缩。'
      : '开启后以设定内存作为启动/保障内存，最大内存默认上浮 30% 应对突发；宿主机内存紧张时可按策略回收。'

  return (
    <SectionCard icon={<IconServer />} title="CPU 与内存">
      <div className="qvm-vf-grid-2">
        <FormField label="CPU 核心" required>
          <InputNumber
            style={{ width: '100%' }}
            value={f.vcpu}
            min={vcpuMin}
            max={vcpuMax}
            disabled={isEdit && running && !f.cpu_hotplug_enabled}
            onChange={(v) => setField('vcpu', Number(v || 1))}
          />
        </FormField>
        <FormField label="内存（GB）" required>
          <InputNumber
            style={{ width: '100%' }}
            value={memoryValue}
            min={memoryMin}
            max={64}
            step={1}
            onChange={(v) => {
              setField(isEdit ? 'memory' : 'ram', Number(v || 1))
              handleBaseMemoryChange()
            }}
          />
        </FormField>
      </div>

      <FormField
        label="CPU 热添加"
        tip={`启用后可在宿主机 ${ctx.hostCores > 0 ? ctx.hostCores : '?'} 核范围内随时热添加 vCPU，无需重启。新建时需重启一次后热添加功能生效`}
      >
        <div className="qvm-vf-switch-row">
          <TextSwitch
            checked={f.cpu_hotplug_enabled}
            onChange={(v) => setField('cpu_hotplug_enabled', v)}
          />
          {f.cpu_hotplug_enabled && ctx.hostCores > 0 && (
            <span className="qvm-vf-inline-note">上限 {ctx.hostCores} 核</span>
          )}
        </div>
      </FormField>

      {ctx.isAdmin && (
        <FormField label="CPU 限制" tip="按当前配置的 vCPU 总能力限速；50% 表示限制为当前已分配 CPU 总能力的一半">
          <div className="qvm-vf-switch-row">
            <TextSwitch
              checked={f.cpu_limit_enabled}
              onChange={(v) => setField('cpu_limit_enabled', v)}
              checkedText="限"
              uncheckedText="无"
            />
            {f.cpu_limit_enabled && (
              <>
                <InputNumber
                  style={{ width: 140 }}
                  value={f.cpu_limit_percent}
                  min={1}
                  max={100}
                  step={1}
                  onChange={(v) => setField('cpu_limit_percent', Number(v || 100))}
                />
                <span className="qvm-vf-inline-note">%</span>
              </>
            )}
          </div>
        </FormField>
      )}

      <FormField label="动态内存" tip={dynamicTip}>
        <div className="qvm-vf-switch-row">
          <TextSwitch
            checked={f.memory_dynamic_enabled}
            onChange={(v) => handleDynamicMemoryEnabledChange(v)}
          />
          {f.memory_dynamic_enabled && (
            <>
              <Radio.Group
                type="button"
                value={f.memory_backend}
                onChange={(e) => handleMemoryBackendChange(e.target.value)}
                options={[
                  { label: '气球调度', value: 'balloon' },
                  { label: 'Windows 弹性内存', value: 'virtio_mem', disabled: windowsElasticMemoryDisabled },
                ]}
              />
              {f.memory_backend === 'virtio_mem' && (
                <Tag color="orange" size="small">弹性内存</Tag>
              )}
              {f.memory_backend === 'virtio_mem' && (
                <Button size="small" theme="borderless" type="primary" onClick={() => setVirtioMemDetailVisible(true)}>
                  详情
                </Button>
              )}
            </>
          )}
          {f.memory_dynamic_enabled && (
            <Button size="small" theme="borderless" type="primary" onClick={() => setMemoryDialogVisible(true)}>
              详细配置
            </Button>
          )}
        </div>
      </FormField>

      {isEdit && running && (
        <div className="qvm-vf-tip warn">运行中修改 CPU/内存为热插拔，部分配置可能需要重启后生效</div>
      )}

      <MemoryDynamicDialog visible={memoryDialogVisible} onClose={() => setMemoryDialogVisible(false)} />
      <VirtioMemDetailDialog visible={virtioMemDetailVisible} onClose={() => setVirtioMemDetailVisible(false)} />
    </SectionCard>
  )
}
