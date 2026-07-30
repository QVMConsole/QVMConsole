/**
 * 导入磁盘分区（创建向导 · 导入模式）
 * 磁盘来源 / 磁盘文件或绝对路径 / 磁盘处理 / 系统盘 IOPS / 导入后操作 / 额外导入磁盘。
 */
import { useEffect, useState } from 'react'
import { Button, Checkbox, Input, InputNumber, Radio, Select } from '@douyinfe/semi-ui'
import { IconDelete, IconPlus, IconUpload } from '@douyinfe/semi-icons'
import SectionCard from './SectionCard'
import FormField from './FormField'
import TextSwitch from './TextSwitch'
import { useVmFormScope } from '../scopeContext'
import { DISK_BUS_OPTIONS } from '../constants'
import { storageTargetLabel } from './storageTargetUtils'
import DiskIopsDialog from '../dialogs/DiskIopsDialog'

export default function ImportStorageSection() {
  const { form, options, ctx } = useVmFormScope()
  const { form: f, setField } = form
  const isAdmin = ctx.isAdmin
  const [iopsDialogIndex, setIopsDialogIndex] = useState(-1)
  const diskFiles = options.diskFiles.filter((file) => !/\.(ova|ovf|mf)$/i.test(file.name))

  const addExtraImportDisk = () => {
    const defaultTarget = options.storageTargets.find((t) => t.is_default)
    setField('extra_import_disks', [
      ...f.extra_import_disks,
      {
        disk_path: '',
        disk_file: '',
        disk_source_type: 'path',
        storage_pool_id: f.storage_pool_id || defaultTarget?.id || '',
        copy_disk: false,
        bus: 'virtio',
        iops_total: 0,
        iops_read: 0,
        iops_write: 0,
      },
    ])
  }

  const updateExtraDisk = (index: number, key: string, value: unknown) => {
    setField(
      'extra_import_disks',
      f.extra_import_disks.map((disk, i) => (i === index ? { ...disk, [key]: value } : disk)),
    )
  }

  const removeExtraDisk = (index: number) => {
    setField('extra_import_disks', f.extra_import_disks.filter((_, i) => i !== index))
  }

  const iopsDialogDisk = iopsDialogIndex >= 0 ? f.extra_import_disks[iopsDialogIndex] : null
  const [lastIopsDialog, setLastIopsDialog] = useState<{
    index: number
    disk: NonNullable<typeof iopsDialogDisk>
  } | null>(null)
  useEffect(() => {
    if (iopsDialogDisk) setLastIopsDialog({ index: iopsDialogIndex, disk: iopsDialogDisk })
  }, [iopsDialogDisk, iopsDialogIndex])
  const activeIopsDialog = iopsDialogDisk
    ? { index: iopsDialogIndex, disk: iopsDialogDisk }
    : lastIopsDialog

  return (
    <SectionCard icon={<IconUpload />} title="磁盘导入">
      {isAdmin && (
        <FormField label="磁盘来源">
          <Radio.Group
            type="button"
            value={f.disk_source_type}
            onChange={(e) => {
              const value = e.target.value
              setField('disk_source_type', value)
              if (value === 'path') setField('disk_file', '')
              else setField('disk_path', '')
            }}
            options={[
              { label: '从我的存储选择', value: 'storage' },
              { label: '输入绝对路径', value: 'path' },
            ]}
          />
        </FormField>
      )}

      {(!isAdmin || f.disk_source_type === 'storage') && (
        <FormField label="磁盘文件" required tip="从「我的存储 → 虚拟磁盘」中选择导出或上传的磁盘文件">
          <Select
            style={{ width: '100%' }}
            value={f.disk_file || undefined}
            placeholder="从我的存储选择磁盘文件"
            loading={options.diskFilesLoading}
            onFocus={() => void options.loadDiskFiles()}
            onChange={(v) => setField('disk_file', (v as string) || '')}
          >
            {diskFiles.map((file) => (
              <Select.Option key={file.name} value={file.name}>
                <div style={{ display: 'flex', justifyContent: 'space-between', gap: 8 }}>
                  <span>{file.name}</span>
                  <span style={{ color: 'var(--qvm-text-2)', fontSize: 12 }}>{file.size_text}</span>
                </div>
              </Select.Option>
            ))}
          </Select>
        </FormField>
      )}

      {isAdmin && f.disk_source_type === 'path' && (
        <FormField label="磁盘路径" required tip="支持 qcow2、raw、vmdk、vhd、vhdx、img 等格式，非 qcow2 格式将自动转换为 qcow2">
          <Input
            value={f.disk_path}
            placeholder="请输入磁盘文件的绝对路径，如 /data/disk.qcow2"
            showClear
            onChange={(v) => setField('disk_path', v)}
          />
        </FormField>
      )}

      <FormField label="磁盘处理" tip="无论原磁盘是 qcow2 还是其他格式（raw/vmdk 等），选择不保留都会在导入完成后删除原文件">
        <Radio.Group
          value={f.copy_disk ? 'keep' : 'remove'}
          onChange={(e) => setField('copy_disk', e.target.value === 'keep')}
          options={[
            { label: '不保留原磁盘文件（推荐，节省空间）', value: 'remove' },
            { label: '保留原磁盘文件', value: 'keep' },
          ]}
        />
      </FormField>

      {isAdmin && (
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

      <FormField label="导入后操作" tip="选择「仅创建不开启」时，虚拟机将被定义但不会启动，您可以稍后手动开机">
        <TextSwitch
          checked={f.start_after_import}
          onChange={(v) => setField('start_after_import', v)}
          checkedText="开"
          uncheckedText="建"
        />
      </FormField>

      {isAdmin && (
        <>
          <div className="qvm-vf-subdivider">额外磁盘导入</div>
          {f.extra_import_disks.map((disk, index) => (
            <div key={index} className="qvm-vf-import-disk">
              <div className="qvm-vf-import-disk-header">
                <span>磁盘 {index + 1}</span>
                <Button size="small" type="danger" theme="borderless" icon={<IconDelete />} onClick={() => removeExtraDisk(index)} />
              </div>
              <Radio.Group
                style={{ marginBottom: 6 }}
                value={disk.disk_source_type || 'path'}
                onChange={(e) => updateExtraDisk(index, 'disk_source_type', e.target.value)}
                options={[
                  { label: '绝对路径', value: 'path' },
                  { label: '从存储选择', value: 'storage' },
                ]}
              />
              {disk.disk_source_type === 'storage' ? (
                <Select
                  size="small"
                  style={{ width: '100%', marginBottom: 6 }}
                  value={disk.disk_file || undefined}
                  placeholder="选择磁盘文件"
                  loading={options.diskFilesLoading}
                  onFocus={() => void options.loadDiskFiles()}
                  onChange={(v) => updateExtraDisk(index, 'disk_file', v)}
                  optionList={diskFiles.map((file) => ({ value: file.name, label: file.name }))}
                />
              ) : (
                <Input
                  size="small"
                  style={{ marginBottom: 6 }}
                  value={disk.disk_path}
                  placeholder="磁盘文件绝对路径，如 /data/disk.qcow2"
                  onChange={(v) => updateExtraDisk(index, 'disk_path', v)}
                />
              )}
              <div style={{ display: 'flex', gap: 6 }}>
                <Select
                  size="small"
                  style={{ flex: 1 }}
                  value={disk.storage_pool_id || undefined}
                  placeholder="目标存储位置"
                  showClear
                  filter
                  onChange={(v) => updateExtraDisk(index, 'storage_pool_id', (v as string) || '')}
                  optionList={options.storageTargets.map((t) => ({ value: t.id, label: storageTargetLabel(t) }))}
                />
                <Select
                  size="small"
                  style={{ width: 100 }}
                  value={disk.bus}
                  onChange={(v) => updateExtraDisk(index, 'bus', v)}
                  optionList={DISK_BUS_OPTIONS.map((item) => ({ value: item.value, label: item.label }))}
                />
              </div>
              <div className="qvm-vf-import-disk-footer">
                <Button size="small" onClick={() => setIopsDialogIndex(index)} style={{ fontSize: 11 }}>
                  IOPS 限制
                </Button>
                {(disk.iops_total > 0 || disk.iops_read > 0 || disk.iops_write > 0) && (
                  <span className="qvm-vf-iops-badge">
                    总:{disk.iops_total || 0} 读:{disk.iops_read || 0} 写:{disk.iops_write || 0}
                  </span>
                )}
                <Checkbox
                  checked={!!disk.copy_disk}
                  onChange={(e) => updateExtraDisk(index, 'copy_disk', !!e.target.checked)}
                >
                  保留原文件
                </Checkbox>
              </div>
            </div>
          ))}
          <Button type="primary" theme="light" size="small" icon={<IconPlus />} block onClick={addExtraImportDisk}>
            添加额外导入磁盘
          </Button>
          <div className="qvm-vf-tip">额外磁盘将在虚拟机创建后依次挂载，支持绝对路径和存储选择，非 qcow2 格式自动转换</div>
        </>
      )}

      {activeIopsDialog && (
        <DiskIopsDialog
          visible={iopsDialogIndex >= 0}
          subtitle={`额外磁盘 #${activeIopsDialog.index + 1}（${activeIopsDialog.disk.disk_path || activeIopsDialog.disk.disk_file || '(未选择)'} ${activeIopsDialog.disk.bus}）`}
          initial={{
            total: activeIopsDialog.disk.iops_total || 0,
            read: activeIopsDialog.disk.iops_read || 0,
            write: activeIopsDialog.disk.iops_write || 0,
          }}
          onApply={(values) => {
            updateExtraDisk(activeIopsDialog.index, 'iops_total', values.total)
            updateExtraDisk(activeIopsDialog.index, 'iops_read', values.read)
            updateExtraDisk(activeIopsDialog.index, 'iops_write', values.write)
          }}
          onClose={() => setIopsDialogIndex(-1)}
        />
      )}
    </SectionCard>
  )
}
