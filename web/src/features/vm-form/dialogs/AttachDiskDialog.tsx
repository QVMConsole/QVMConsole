/**
 * 挂载已有磁盘弹窗（编辑模式）
 * 普通用户从我的存储选择磁盘文件；管理员可用绝对路径导入（异步任务）。
 */
import { useEffect, useState } from 'react'
import { Banner, Input, Modal, Radio, Select, Switch } from '@douyinfe/semi-ui'
import { useVmFormScope } from '../scopeContext'
import type { VmEditDevices } from '../useVmEditDevices'
import FormField from '../sections/FormField'
import { storageTargetLabel } from '../sections/storageTargetUtils'
import { DISK_BUS_OPTIONS } from '../constants'

interface AttachDiskDialogProps {
  visible: boolean
  devices: VmEditDevices
  onClose: () => void
}

export default function AttachDiskDialog({ visible, devices, onClose }: AttachDiskDialogProps) {
  const { options, ctx } = useVmFormScope()
  const isAdmin = ctx.isAdmin

  const [sourceType, setSourceType] = useState<'storage' | 'path'>('storage')
  const [diskPath, setDiskPath] = useState('')
  const [absolutePath, setAbsolutePath] = useState('')
  const [storagePoolId, setStoragePoolId] = useState('')
  const [copyDisk, setCopyDisk] = useState(false)
  const [bus, setBus] = useState('virtio')
  const [autoMount, setAutoMount] = useState(false)
  const [mountPoint, setMountPoint] = useState('/data')
  const [driveLetter, setDriveLetter] = useState('')
  const [submitting, setSubmitting] = useState(false)

  useEffect(() => {
    if (!visible) return
    setSourceType('storage')
    setDiskPath('')
    setAbsolutePath('')
    setStoragePoolId('')
    setCopyDisk(false)
    setBus('virtio')
    setAutoMount(false)
    setMountPoint('/data')
    setDriveLetter('')
    void options.loadDiskFiles()
    void options.loadStorageTargets()
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [visible])

  const submitDisabled = isAdmin && sourceType === 'path' ? !absolutePath : !diskPath
  const guestType = ctx.guestType
  const canAutoMount =
    ctx.vmStatus === 'running' &&
    !!ctx.guestAgentConnected &&
    (guestType === 'linux' || guestType === 'windows')
  const guestMount = autoMount
    ? {
        enabled: true,
        filesystem: 'ext4' as const,
        mount_point: guestType === 'linux' ? mountPoint : undefined,
        drive_letter: guestType === 'windows' ? driveLetter : undefined,
      }
    : undefined

  const handleOk = async () => {
    setSubmitting(true)
    try {
      if (isAdmin && sourceType === 'path') {
        await devices.adminImportDiskAction({
          disk_path: absolutePath,
          disk_source_type: 'path',
          storage_pool_id: storagePoolId,
          copy_disk: copyDisk,
          bus,
          guest_mount: guestMount,
        })
      } else {
        await devices.attachDiskAction(diskPath, bus, guestMount)
      }
      onClose()
    } catch {
      // 错误由请求层统一提示
    } finally {
      setSubmitting(false)
    }
  }

  return (
    <Modal
      title="导入磁盘到虚拟机"
      visible={visible}
      onCancel={onClose}
      onOk={() => void handleOk()}
      okText={isAdmin && sourceType === 'path' ? '提交导入任务' : '挂载'}
      cancelText="取消"
      okButtonProps={{ disabled: submitDisabled }}
      confirmLoading={submitting}
      width={560}
      closeOnEsc
    >
      {isAdmin && (
        <FormField label="磁盘来源">
          <Radio.Group
            type="button"
            value={sourceType}
            onChange={(e) => {
              const value = e.target.value as 'storage' | 'path'
              setSourceType(value)
              if (value === 'path') setDiskPath('')
              else setAbsolutePath('')
            }}
            options={[
              { label: '从我的存储选择', value: 'storage' },
              { label: '输入绝对路径', value: 'path' },
            ]}
          />
        </FormField>
      )}

      {(!isAdmin || sourceType === 'storage') && (
        <FormField label="磁盘文件">
          <Select
            style={{ width: '100%' }}
            value={diskPath || undefined}
            placeholder="请选择磁盘文件"
            filter
            loading={options.diskFilesLoading}
            onChange={(v) => setDiskPath((v as string) || '')}
            optionList={options.diskFiles.map((file) => ({
              value: file.path,
              label: `${file.name}（${file.size_text || '-'}）`,
            }))}
          />
          {options.diskFiles.length === 0 && !options.diskFilesLoading && (
            <div className="qvm-vf-tip">没有可用的磁盘文件，请先在「我的存储 → 虚拟磁盘」中上传</div>
          )}
        </FormField>
      )}

      {isAdmin && sourceType === 'path' && (
        <>
          <FormField label="磁盘路径" tip="支持 qcow2、raw、vmdk 等格式，非 qcow2 自动转换">
            <Input
              value={absolutePath}
              placeholder="请输入磁盘文件的绝对路径，如 /data/disk.qcow2"
              showClear
              onChange={setAbsolutePath}
            />
          </FormField>
          <FormField label="目标存储">
            <Select
              style={{ width: '100%' }}
              value={storagePoolId || undefined}
              placeholder="使用默认存储位置"
              showClear
              filter
              onChange={(v) => setStoragePoolId((v as string) || '')}
              optionList={options.storageTargets.map((t) => ({ value: t.id, label: storageTargetLabel(t) }))}
            />
          </FormField>
          <FormField label="磁盘处理">
            <Radio.Group
              value={copyDisk ? 'keep' : 'remove'}
              onChange={(e) => setCopyDisk(e.target.value === 'keep')}
              options={[
                { label: '不保留原磁盘文件（推荐）', value: 'remove' },
                { label: '保留原磁盘文件', value: 'keep' },
              ]}
            />
          </FormField>
        </>
      )}

      <FormField label="总线类型">
        <Select
          style={{ width: '100%' }}
          value={bus}
          onChange={(v) => setBus(v as string)}
          optionList={DISK_BUS_OPTIONS.map((item) => ({
            value: item.value,
            label: item.value === 'virtio' ? 'VirtIO（推荐）' : item.label,
          }))}
        />
      </FormField>

      {canAutoMount && (
        <FormField label="自动挂载到系统" tip="只识别已有数据卷，不会重新格式化已有磁盘">
          <Switch checked={autoMount} checkedText="开" uncheckedText="关" onChange={setAutoMount} />
        </FormField>
      )}
      {autoMount && guestType === 'linux' && (
        <FormField label="基础挂载目录" tip="多卷磁盘将依次使用该目录及数字后缀">
          <Input value={mountPoint} onChange={setMountPoint} placeholder="/data" />
        </FormField>
      )}
      {autoMount && guestType === 'windows' && (
        <FormField label="首选盘符" tip="留空时自动选择 D 到 Z 的空闲盘符，多卷将继续分配后续空闲盘符">
          <Input value={driveLetter} onChange={setDriveLetter} maxLength={1} placeholder="自动分配" />
        </FormField>
      )}
      {autoMount && (
        <Banner type="warning" closeIcon={null} description="自动挂载通过 QEMU Guest Agent 异步执行，请在任务中心查看每个卷的处理结果。" />
      )}
    </Modal>
  )
}
