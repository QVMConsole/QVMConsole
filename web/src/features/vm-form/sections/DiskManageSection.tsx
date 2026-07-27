/**
 * 磁盘与驱动器管理分区（编辑模式）
 * 现有磁盘表 / 新增磁盘 / CD-DVD 光驱 / 软盘驱动器。
 */
import { useState } from 'react'
import { Button, InputNumber, Modal, Select, Table, Tag, Toast } from '@douyinfe/semi-ui'
import { IconDelete, IconDisc, IconLink, IconPlus } from '@douyinfe/semi-icons'
import { DiskIcon } from '../icons'
import type { VmDiskItem } from '@/api/vm'
import SectionCard from './SectionCard'
import FormField from './FormField'
import { useVmFormScope } from '../scopeContext'
import type { VmEditDevices } from '../useVmEditDevices'
import { DISK_BUS_OPTIONS } from '../constants'
import { storageTargetLabel } from './storageTargetUtils'
import DiskIopsDialog from '../dialogs/DiskIopsDialog'
import AttachDiskDialog from '../dialogs/AttachDiskDialog'
import ResizeDiskDialog from '../dialogs/ResizeDiskDialog'
import RemoveDiskDialog from '../dialogs/RemoveDiskDialog'

interface DiskManageSectionProps {
  devices: VmEditDevices
}

export default function DiskManageSection({ devices }: DiskManageSectionProps) {
  const { form, options, ctx } = useVmFormScope()
  const { form: f, setField } = form
  const { editDisks, setEditDisks, editCdroms, editFloppys } = devices
  const running = ctx.vmStatus === 'running'

  const [cdromIsoPath, setCdromIsoPath] = useState('')
  const [floppyImagePath, setFloppyImagePath] = useState('')
  const [attachVisible, setAttachVisible] = useState(false)
  const [resizeDisk, setResizeDisk] = useState<VmDiskItem | null>(null)
  const [removeDiskTarget, setRemoveDiskTarget] = useState<VmDiskItem | null>(null)
  const [iopsDisk, setIopsDisk] = useState<VmDiskItem | null>(null)

  // ISO 按存储池分组（光驱插入用）
  const isoOptions = options.isoList.map((iso) => ({
    value: iso.path,
    label: `${iso.name}（${iso.size || '-'}）`,
  }))
  const diskFileOptions = options.diskFiles.map((file) => ({
    value: file.path,
    label: `${file.name}（${file.size_text || '-'}）`,
  }))

  const addNewDisk = () => {
    const defaultTarget = options.storageTargets.find((t) => t.is_default)
    setField('add_disks', [
      ...f.add_disks,
      {
        size: 20,
        format: 'qcow2',
        bus: 'virtio',
        storage_pool_id: f.storage_pool_id || defaultTarget?.id || '',
      },
    ])
  }

  const updateNewDisk = (index: number, key: string, value: unknown) => {
    setField(
      'add_disks',
      f.add_disks.map((disk, i) => (i === index ? { ...disk, [key]: value } : disk)),
    )
  }

  const diskColumns = [
    { title: '设备', dataIndex: 'device', width: 70 },
    {
      title: '容量',
      dataIndex: 'capacity_gb',
      width: 90,
      render: (v: number | string | undefined) => (v ? `${v} GB` : '-'),
    },
    {
      title: '占用',
      dataIndex: 'used_gb',
      width: 90,
      render: (v: number | string | undefined) => (v ? `${v} GB` : '-'),
    },
    { title: '格式', dataIndex: 'format', width: 70 },
    {
      title: '驱动',
      dataIndex: 'bus',
      width: 120,
      render: (bus: string, row: VmDiskItem) => (
        <Select
          size="small"
          style={{ width: 100 }}
          value={bus}
          disabled={running}
          onChange={(v) => void devices.changeDiskBusAction(row.device, v as string)}
          optionList={DISK_BUS_OPTIONS.map((item) => ({ value: item.value, label: item.label }))}
        />
      ),
    },
    {
      title: '路径',
      dataIndex: 'path',
      render: (path: string) => (
        <span className="qvm-vf-disk-path" title={path}>
          {path || '-'}
        </span>
      ),
    },
    {
      title: '操作',
      width: 200,
      align: 'center' as const,
      render: (_: unknown, row: VmDiskItem) => (
        <div style={{ display: 'flex', gap: 6, justifyContent: 'center' }}>
          <Button size="small" type="primary" theme="light" onClick={() => setResizeDisk(row)}>
            扩容
          </Button>
          {ctx.isAdmin && (
            <Button size="small" type="warning" theme="light" onClick={() => setIopsDisk(row)}>
              IOPS
            </Button>
          )}
          <Button size="small" type="danger" theme="light" onClick={() => setRemoveDiskTarget(row)}>
            删除
          </Button>
        </div>
      ),
    },
  ]

  const handleInsertCdrom = async (device: string) => {
    if (!cdromIsoPath) return
    try {
      await devices.insertCDROMAction(cdromIsoPath, device)
      setCdromIsoPath('')
    } catch {
      // 错误由请求层统一提示
    }
  }

  const handleAddCdrom = async () => {
    if (!cdromIsoPath) return
    try {
      await devices.insertCDROMAction(cdromIsoPath, '', true)
      setCdromIsoPath('')
    } catch {
      // 错误由请求层统一提示
    }
  }

  const handleInsertFloppy = async (device: string) => {
    if (!floppyImagePath) return
    try {
      await devices.insertFloppyAction(floppyImagePath, device)
      setFloppyImagePath('')
    } catch {
      // 错误由请求层统一提示
    }
  }

  const handleAddFloppy = async () => {
    if (!floppyImagePath) return
    try {
      await devices.insertFloppyAction(floppyImagePath, '', true)
      setFloppyImagePath('')
    } catch {
      // 错误由请求层统一提示
    }
  }

  return (
    <>
      {/* 当前磁盘 */}
      <SectionCard icon={<DiskIcon />} title="当前磁盘">
        {editDisks.length > 0 ? (
          <Table
            rowKey="device"
            size="small"
            bordered
            columns={diskColumns}
            dataSource={editDisks}
            pagination={false}
          />
        ) : (
          <div className="qvm-vf-empty-text">暂无磁盘设备</div>
        )}

        <FormField label="新增磁盘" style={{ marginTop: 14 }}>
          {f.add_disks.map((disk, index) => (
            <div key={index} className="qvm-vf-disk-row">
              <InputNumber
                style={{ width: 110 }}
                value={disk.size}
                min={1}
                max={2000}
                placeholder="大小(GB)"
                onChange={(v) => updateNewDisk(index, 'size', Number(v || 0))}
              />
              <Select
                style={{ width: 96 }}
                value={disk.format}
                onChange={(v) => updateNewDisk(index, 'format', v)}
                optionList={[
                  { value: 'qcow2', label: 'qcow2' },
                  ...(ctx.isAdmin ? [{ value: 'raw', label: 'raw' }] : []),
                ]}
              />
              <Select
                style={{ width: 104 }}
                value={disk.bus}
                onChange={(v) => updateNewDisk(index, 'bus', v)}
                optionList={DISK_BUS_OPTIONS.map((item) => ({ value: item.value, label: item.label }))}
              />
              <Select
                style={{ width: 170 }}
                value={disk.storage_pool_id || undefined}
                placeholder="默认存储"
                showClear
                filter
                onChange={(v) => updateNewDisk(index, 'storage_pool_id', (v as string) || '')}
                optionList={options.storageTargets.map((t) => ({ value: t.id, label: storageTargetLabel(t) }))}
              />
              <span className="qvm-vf-disk-unit">GB</span>
              <Button
                size="small"
                type="danger"
                theme="borderless"
                icon={<IconDelete />}
                onClick={() => setField('add_disks', f.add_disks.filter((_, i) => i !== index))}
              />
            </div>
          ))}
          <div style={{ display: 'flex', gap: 8 }}>
            <Button type="primary" theme="light" size="small" icon={<IconPlus />} onClick={addNewDisk}>
              新建磁盘
            </Button>
            <Button type="primary" theme="light" size="small" icon={<IconLink />} onClick={() => setAttachVisible(true)}>
              挂载已有磁盘
            </Button>
          </div>
          {running && <div className="qvm-vf-tip warn">运行中添加磁盘将使用热插拔，修改驱动类型需关机</div>}
        </FormField>
      </SectionCard>

      {/* CD/DVD 光驱 */}
      <SectionCard icon={<IconDisc />} title="CD/DVD 光驱">
        {editCdroms.length > 0 ? (
          editCdroms.map((cdrom) => (
            <div key={cdrom.device} className="qvm-vf-media-row">
              <Tag size="small" color="blue">{cdrom.device}</Tag>
              <span className="qvm-vf-media-path">{cdrom.path || '（空光驱）'}</span>
              {!cdrom.path && (
                <Button size="small" type="primary" theme="light" disabled={!cdromIsoPath} onClick={() => void handleInsertCdrom(cdrom.device)}>
                  插入
                </Button>
              )}
              {cdrom.path && (
                <Button size="small" theme="light" onClick={() => void devices.ejectCDROMAction(cdrom.device)}>
                  弹出
                </Button>
              )}
              <Button
                size="small"
                type="danger"
                theme="light"
                onClick={() => {
                  Modal.confirm({
                    title: '移除光驱',
                    content: `确定要移除光驱设备 ${cdrom.device} 吗？`,
                    okText: '移除',
                    cancelText: '取消',
                    okButtonProps: { type: 'danger' },
                    onOk: () => devices.removeCDROMAction(cdrom.device),
                  })
                }}
              >
                移除
              </Button>
            </div>
          ))
        ) : (
          <div className="qvm-vf-empty-text">无光驱设备</div>
        )}
        <div style={{ display: 'flex', gap: 8, marginTop: 8 }}>
          <Select
            style={{ flex: 1 }}
            value={cdromIsoPath || undefined}
            placeholder="从存储池选择 ISO"
            filter
            showClear
            onFocus={() => void options.loadISOs()}
            onChange={(v) => setCdromIsoPath((v as string) || '')}
            optionList={isoOptions}
          />
          <Button type="primary" theme="solid" disabled={!cdromIsoPath} onClick={() => void handleAddCdrom()}>
            添加光驱
          </Button>
        </div>
        {running && (
          <div className="qvm-vf-tip" style={{ marginTop: 6 }}>
            运行中新增光驱会自动改用支持热插的 SCSI 总线；已有光驱插入 ISO 仍复用原设备
          </div>
        )}
      </SectionCard>

      {/* 软盘驱动器 */}
      <SectionCard icon={<IconDisc />} title="软盘驱动器">
        {editFloppys.length > 0 ? (
          editFloppys.map((floppy) => (
            <div key={floppy.device} className="qvm-vf-media-row">
              <Tag size="small" color="blue">{floppy.device}</Tag>
              <span className="qvm-vf-media-path">{floppy.path || '（空软盘）'}</span>
              {!floppy.path && (
                <Button size="small" type="primary" theme="light" disabled={!floppyImagePath} onClick={() => void handleInsertFloppy(floppy.device)}>
                  插入
                </Button>
              )}
              {floppy.path && (
                <Button size="small" theme="light" onClick={() => void devices.ejectFloppyAction(floppy.device)}>
                  弹出
                </Button>
              )}
              <Button
                size="small"
                type="danger"
                theme="light"
                onClick={() => {
                  Modal.confirm({
                    title: '移除软盘',
                    content: `确定要移除软盘设备 ${floppy.device} 吗？`,
                    okText: '移除',
                    cancelText: '取消',
                    okButtonProps: { type: 'danger' },
                    onOk: () => devices.removeFloppyAction(floppy.device),
                  })
                }}
              >
                移除
              </Button>
            </div>
          ))
        ) : (
          <div className="qvm-vf-empty-text">无软盘设备</div>
        )}
        <div style={{ display: 'flex', gap: 8, marginTop: 8 }}>
          <Select
            style={{ flex: 1 }}
            value={floppyImagePath || undefined}
            placeholder="从我的存储选择软盘镜像"
            filter
            showClear
            loading={options.diskFilesLoading}
            onFocus={() => void options.loadDiskFiles()}
            onChange={(v) => setFloppyImagePath((v as string) || '')}
            optionList={diskFileOptions}
          />
          <Button type="primary" theme="solid" disabled={!floppyImagePath} onClick={() => void handleAddFloppy()}>
            添加软盘
          </Button>
        </div>
      </SectionCard>

      <AttachDiskDialog visible={attachVisible} devices={devices} onClose={() => setAttachVisible(false)} />
      <ResizeDiskDialog visible={!!resizeDisk} disk={resizeDisk} devices={devices} onClose={() => setResizeDisk(null)} />
      <RemoveDiskDialog visible={!!removeDiskTarget} disk={removeDiskTarget} devices={devices} onClose={() => setRemoveDiskTarget(null)} />
      {iopsDisk && (
        <DiskIopsDialog
          visible={!!iopsDisk}
          subtitle={`磁盘 ${iopsDisk.device}（${iopsDisk.path || '-'}）`}
          initial={{
            total: iopsDisk.iops_total?.value || 0,
            read: iopsDisk.iops_read?.value || 0,
            write: iopsDisk.iops_write?.value || 0,
          }}
          onApply={(values) => {
            // 将 IOPS 设置挂到磁盘对象上，保存编辑时一并提交
            setEditDisks(
              editDisks.map((d) =>
                d.device === iopsDisk.device
                  ? ({
                      ...d,
                      _iops_total: values.total,
                      _iops_read: values.read,
                      _iops_write: values.write,
                      iops_total: { value: values.total, is_set: values.total > 0 },
                      iops_read: { value: values.read, is_set: values.read > 0 },
                      iops_write: { value: values.write, is_set: values.write > 0 },
                    } as VmDiskItem & { _iops_total: number; _iops_read: number; _iops_write: number })
                  : d,
              ),
            )
            Toast.success(`磁盘 ${iopsDisk.device} IOPS 限制已设置（将在保存编辑时生效）`)
          }}
          onClose={() => setIopsDisk(null)}
        />
      )}
    </>
  )
}
