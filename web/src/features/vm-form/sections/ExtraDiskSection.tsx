/**
 * 额外数据盘分区（创建向导 · ISO / 模板模式共用）
 */
import { useState } from 'react'
import { Button, InputNumber, Select } from '@douyinfe/semi-ui'
import { IconDelete, IconPlus } from '@douyinfe/semi-icons'
import { DiskIcon } from '../icons'
import SectionCard from './SectionCard'
import FormField from './FormField'
import { useVmFormScope } from '../scopeContext'
import { DISK_BUS_OPTIONS } from '../constants'
import { storageTargetLabel } from './storageTargetUtils'
import DiskIopsDialog from '../dialogs/DiskIopsDialog'

interface ExtraDiskSectionProps {
  /** 分区标题 */
  title?: string
  /** 底部提示 */
  tip?: string
}

export default function ExtraDiskSection({ title = '额外数据盘', tip }: ExtraDiskSectionProps) {
  const { form, options, ctx } = useVmFormScope()
  const { form: f, setField } = form
  const [iopsDialogIndex, setIopsDialogIndex] = useState(-1)
  // 旧版规则：ISO 模式额外磁盘 raw 格式对所有角色开放；模板模式仅管理员
  const rawAllowed = ctx.isAdmin || f.create_mode === 'iso'

  const addExtraDisk = () => {
    const defaultTarget = options.storageTargets.find((t) => t.is_default)
    setField('extra_disks', [
      ...f.extra_disks,
      {
        size: 20,
        format: 'qcow2',
        bus: f.disk_bus || 'virtio',
        storage_pool_id: f.storage_pool_id || defaultTarget?.id || '',
        iops_total: 0,
        iops_read: 0,
        iops_write: 0,
      },
    ])
  }

  const updateExtraDisk = (index: number, key: string, value: unknown) => {
    setField(
      'extra_disks',
      f.extra_disks.map((disk, i) => (i === index ? { ...disk, [key]: value } : disk)),
    )
  }

  const removeExtraDisk = (index: number) => {
    setField('extra_disks', f.extra_disks.filter((_, i) => i !== index))
  }

  const iopsDialogDisk = iopsDialogIndex >= 0 ? f.extra_disks[iopsDialogIndex] : null

  return (
    <SectionCard icon={<DiskIcon />} title={title}>
      <FormField label="额外磁盘" tip={tip}>
        {f.extra_disks.map((disk, index) => (
          <div key={index} className="qvm-vf-disk-row">
            <InputNumber
              style={{ width: 110 }}
              value={disk.size}
              min={1}
              max={2000}
              placeholder="大小(GB)"
              onChange={(v) => updateExtraDisk(index, 'size', Number(v || 0))}
            />
            <Select
              style={{ width: 96 }}
              value={disk.format}
              onChange={(v) => updateExtraDisk(index, 'format', v)}
              optionList={[
                { value: 'qcow2', label: 'qcow2' },
                ...(rawAllowed ? [{ value: 'raw', label: 'raw' }] : []),
              ]}
            />
            <Select
              style={{ width: 104 }}
              value={disk.bus}
              onChange={(v) => updateExtraDisk(index, 'bus', v)}
              optionList={DISK_BUS_OPTIONS.map((item) => ({ value: item.value, label: item.label }))}
            />
            <Select
              style={{ width: 170 }}
              value={disk.storage_pool_id || undefined}
              placeholder="默认存储"
              showClear
              filter
              onChange={(v) => updateExtraDisk(index, 'storage_pool_id', (v as string) || '')}
              optionList={options.storageTargets.map((t) => ({ value: t.id, label: storageTargetLabel(t) }))}
            />
            <span className="qvm-vf-disk-unit">GB</span>
            {ctx.isAdmin && (
              <Button size="small" onClick={() => setIopsDialogIndex(index)} style={{ fontSize: 11 }}>
                IOPS
              </Button>
            )}
            <Button size="small" type="danger" theme="borderless" icon={<IconDelete />} onClick={() => removeExtraDisk(index)} />
            {ctx.isAdmin && (disk.iops_total > 0 || disk.iops_read > 0 || disk.iops_write > 0) && (
              <span className="qvm-vf-iops-badge">
                IOPS: 总{disk.iops_total || 0} / 读{disk.iops_read || 0} / 写{disk.iops_write || 0}
              </span>
            )}
          </div>
        ))}
        <Button type="primary" theme="light" size="small" icon={<IconPlus />} onClick={addExtraDisk}>
          添加额外磁盘
        </Button>
      </FormField>

      {iopsDialogDisk && (
        <DiskIopsDialog
          visible={iopsDialogIndex >= 0}
          subtitle={`额外磁盘 #${iopsDialogIndex + 1}（${iopsDialogDisk.size}GB ${iopsDialogDisk.format} ${iopsDialogDisk.bus}）`}
          initial={{
            total: iopsDialogDisk.iops_total,
            read: iopsDialogDisk.iops_read,
            write: iopsDialogDisk.iops_write,
          }}
          onApply={(values) => {
            updateExtraDisk(iopsDialogIndex, 'iops_total', values.total)
            updateExtraDisk(iopsDialogIndex, 'iops_read', values.read)
            updateExtraDisk(iopsDialogIndex, 'iops_write', values.write)
          }}
          onClose={() => setIopsDialogIndex(-1)}
        />
      )}
    </SectionCard>
  )
}
