/**
 * ISO 存储分区（创建向导 · ISO 模式）
 * ISO 镜像选择（多选）+ 系统盘配置 + 额外磁盘 + 软盘镜像。
 */
import { useMemo } from 'react'
import { InputNumber, Select, Tag } from '@douyinfe/semi-ui'
import { IconDisc } from '@douyinfe/semi-icons'
import { DiskIcon } from '../icons'
import SectionCard from './SectionCard'
import FormField from './FormField'
import { useVmFormScope } from '../scopeContext'
import { DISK_BUS_OPTIONS, DISK_FORMAT_OPTIONS } from '../constants'
import ExtraDiskSection from './ExtraDiskSection'

export default function IsoStorageSection() {
  const { form, options, ctx } = useVmFormScope()
  const { form: f, setField, onISOChange } = form

  // ISO 按存储池分组
  const groupedISOs = useMemo(() => {
    const groups: Record<string, typeof options.isoList> = {}
    options.isoList.forEach((iso) => {
      const pool = iso.pool || '默认路径'
      if (!groups[pool]) groups[pool] = []
      groups[pool].push(iso)
    })
    return Object.entries(groups).map(([label, items]) => ({ label, items }))
  }, [options.isoList])

  return (
    <>
      {/* 安装镜像 */}
      <SectionCard icon={<IconDisc />} title="安装镜像">
        <FormField
          label="ISO 镜像"
          tip="支持同时挂载多个 ISO，首个 ISO 会作为主安装盘并自动补全系统类型和版本，其余 ISO 会作为额外挂载光驱"
        >
          <Select
            style={{ width: '100%' }}
            value={f.iso_paths}
            placeholder="选择一个或多个 ISO 镜像"
            filter
            multiple
            maxTagCount={2}
            showClear
            loading={options.isoLoading}
            onFocus={() => void options.loadISOs()}
            onChange={(v) => onISOChange(v as string[], options.isoList)}
            emptyContent={
              options.isoLoading ? (
                <div style={{ padding: '8px 0', textAlign: 'center' }}>加载中…</div>
              ) : (
                <div style={{ padding: '8px 0', textAlign: 'center' }}>
                  暂无可用 ISO 镜像{ctx.isAdmin ? `（当前目录：${options.isoStorageDir}）` : ''}
                </div>
              )
            }
          >
            {groupedISOs.map((group) => (
              <Select.OptGroup key={group.label} label={group.label}>
                {group.items.map((iso) => (
                  <Select.Option key={iso.path} value={iso.path}>
                    <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', gap: 8 }}>
                      <span>{iso.name}</span>
                      <span style={{ display: 'flex', gap: 4 }}>
                        <Tag size="small" color={iso.os_type === 'windows' ? 'orange' : 'green'}>
                          {iso.os_type === 'windows' ? 'Win' : 'Linux'}
                        </Tag>
                        <Tag size="small" color="blue">{iso.size}</Tag>
                      </span>
                    </div>
                  </Select.Option>
                ))}
              </Select.OptGroup>
            ))}
          </Select>
        </FormField>
        {f.iso_paths.length > 0 && (
          <div className="qvm-vf-iso-order">
            <span>当前挂载顺序：</span>
            {f.iso_paths.map((path, index) => (
              <Tag key={path} size="small" color={index === 0 ? 'green' : 'blue'}>
                {index + 1}. {path.split('/').pop()}
              </Tag>
            ))}
          </div>
        )}
      </SectionCard>

      {/* 系统磁盘 */}
      <SectionCard icon={<DiskIcon />} title="系统磁盘">
        <div className="qvm-vf-grid-2">
          <FormField label="系统盘（GB）" required>
            <InputNumber
              style={{ width: '100%' }}
              value={f.disk_size}
              min={10}
              max={2000}
              step={10}
              onChange={(v) => setField('disk_size', Number(v || 0))}
            />
          </FormField>
          <FormField label="磁盘格式">
            <Select
              style={{ width: '100%' }}
              value={f.disk_format}
              onChange={(v) => setField('disk_format', v as string)}
              optionList={DISK_FORMAT_OPTIONS.map((item) => ({ value: item.value, label: item.label }))}
            />
          </FormField>
          <FormField label="驱动类型">
            <Select
              style={{ width: '100%' }}
              value={f.disk_bus}
              onChange={(v) => setField('disk_bus', v as string)}
              optionList={DISK_BUS_OPTIONS.map((item) => ({ value: item.value, label: item.label }))}
            />
          </FormField>
        </div>

        {ctx.isAdmin && (
          <FormField label="系统盘 IOPS">
            <div className="qvm-vf-iops-row">
              <span className="qvm-vf-iops-label">总</span>
              <InputNumber size="small" style={{ width: 100 }} value={f.system_disk_iops_total} min={0} step={100} placeholder="总IOPS" onChange={(v) => setField('system_disk_iops_total', Number(v || 0))} />
              <span className="qvm-vf-iops-label">读</span>
              <InputNumber size="small" style={{ width: 100 }} value={f.system_disk_iops_read} min={0} step={100} placeholder="读IOPS" disabled={f.system_disk_iops_total > 0} onChange={(v) => setField('system_disk_iops_read', Number(v || 0))} />
              <span className="qvm-vf-iops-label">写</span>
              <InputNumber size="small" style={{ width: 100 }} value={f.system_disk_iops_write} min={0} step={100} placeholder="写IOPS" disabled={f.system_disk_iops_total > 0} onChange={(v) => setField('system_disk_iops_write', Number(v || 0))} />
              <span className="qvm-vf-iops-mutex">互斥</span>
            </div>
          </FormField>
        )}

      </SectionCard>

      {/* 额外磁盘（与模板模式共用同一分区组件） */}
      <ExtraDiskSection tip="额外磁盘默认跟随系统盘驱动类型；存储位置留空时使用上方虚拟机硬盘默认位置" />

      {/* 软盘驱动器 */}
      <SectionCard icon={<IconDisc />} title="软盘驱动器（可选）">
        <FormField label="软盘镜像" tip="从「我的存储 → 虚拟磁盘」中选择 .img、.vfd 等软盘镜像，虚拟机创建后将自动挂载为软盘驱动器 (fda)">
          <Select
            style={{ width: '100%' }}
            value={f.floppy_image || undefined}
            placeholder="从我的存储选择软盘镜像（可选）"
            filter
            showClear
            loading={options.diskFilesLoading}
            onFocus={() => void options.loadDiskFiles()}
            onChange={(v) => setField('floppy_image', (v as string) || '')}
          >
            {options.diskFiles.map((file) => (
              <Select.Option key={file.name} value={file.path}>
                <div style={{ display: 'flex', justifyContent: 'space-between', gap: 8 }}>
                  <span>{file.name}</span>
                  <span style={{ color: 'var(--qvm-text-2)', fontSize: 12 }}>{file.size_text}</span>
                </div>
              </Select.Option>
            ))}
          </Select>
        </FormField>
      </SectionCard>

    </>
  )
}
