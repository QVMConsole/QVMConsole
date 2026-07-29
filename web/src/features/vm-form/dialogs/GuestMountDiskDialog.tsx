import { useEffect, useState } from 'react'
import { Banner, Input, Modal, Select } from '@douyinfe/semi-ui'
import type { VmDiskItem } from '@/api/vm'
import { useVmFormScope } from '../scopeContext'
import type { VmEditDevices } from '../useVmEditDevices'
import FormField from '../sections/FormField'

const filesystemOptions = [
  { value: 'ext4', label: 'ext4' },
  { value: 'xfs', label: 'XFS' },
  { value: 'btrfs', label: 'Btrfs' },
]

interface GuestMountDiskDialogProps {
  visible: boolean
  disk: VmDiskItem | null
  devices: VmEditDevices
  onClose: () => void
}

export default function GuestMountDiskDialog({ visible, disk, devices, onClose }: GuestMountDiskDialogProps) {
  const { ctx } = useVmFormScope()
  const [lastDisk, setLastDisk] = useState<VmDiskItem | null>(disk)
  const [modalVisible, setModalVisible] = useState(false)
  const [filesystem, setFilesystem] = useState<'ext4' | 'xfs' | 'btrfs'>('ext4')
  const [mountPoint, setMountPoint] = useState('/data')
  const [driveLetter, setDriveLetter] = useState('')
  const [diskMode, setDiskMode] = useState<'existing' | 'initialize'>('existing')
  const [submitting, setSubmitting] = useState(false)
  const activeDisk = disk || lastDisk

  useEffect(() => {
    if (disk) setLastDisk(disk)
    if (visible) {
      setFilesystem('ext4')
      setMountPoint('/data')
      setDriveLetter('')
      setDiskMode('existing')
      setModalVisible(true)
    }
  }, [visible, disk])

  const handleOk = async () => {
    if (!activeDisk) return
    setSubmitting(true)
    try {
      await devices.guestMountDiskAction(
        activeDisk.device,
        {
          enabled: true,
          filesystem: ctx.guestType === 'linux' ? filesystem : undefined,
          mount_point: ctx.guestType === 'linux' ? mountPoint : undefined,
          drive_letter: ctx.guestType === 'windows' ? driveLetter : undefined,
        },
        diskMode === 'existing',
      )
      setModalVisible(false)
    } finally {
      setSubmitting(false)
    }
  }

  return (
    <Modal
      title={`配置来宾挂载 ${activeDisk?.device || ''}`}
      visible={modalVisible}
      afterClose={onClose}
      onCancel={() => setModalVisible(false)}
      onOk={() => void handleOk()}
      okText="提交任务"
      cancelText="取消"
      confirmLoading={submitting}
      width={500}
    >
      <Banner
        type={diskMode === 'initialize' ? 'danger' : 'warning'}
        closeIcon={null}
        description={
          diskMode === 'initialize'
            ? `初始化会重新建立 GPT 分区并格式化整块磁盘，原有数据将被清除。${ctx.guestType === 'linux' ? `当前选择 ${filesystemOptions.find((item) => item.value === filesystem)?.label || filesystem}。` : 'Windows 将使用 NTFS。'}`
            : '保留磁盘现有内容，只识别并挂载已有数据卷；空盘需要切换为初始化模式。'
        }
        style={{ marginBottom: 16 }}
      />
      <FormField label="磁盘处理方式">
        <Select
          style={{ width: '100%' }}
          value={diskMode}
          onChange={(value) => setDiskMode(value as 'existing' | 'initialize')}
          optionList={[
            { value: 'existing', label: '挂载已有数据（保留内容）' },
            { value: 'initialize', label: '初始化空盘并挂载（清除数据）' },
          ]}
        />
      </FormField>
      {ctx.guestType === 'linux' ? (
        <>
          {diskMode === 'initialize' && (
            <FormField label="格式化文件系统" tip="仅作用于新空盘初始化">
              <Select
                style={{ width: '100%' }}
                value={filesystem}
                onChange={(value) => setFilesystem(value as 'ext4' | 'xfs' | 'btrfs')}
                optionList={filesystemOptions}
              />
            </FormField>
          )}
          <FormField label="基础挂载目录" tip="多卷依次使用数字后缀">
            <Input value={mountPoint} onChange={setMountPoint} placeholder="/data" />
          </FormField>
        </>
      ) : (
        <>
          {diskMode === 'initialize' && (
            <FormField label="格式化文件系统">
              <Select
                style={{ width: '100%' }}
                value="ntfs"
                disabled
                optionList={[{ value: 'ntfs', label: 'NTFS' }]}
              />
            </FormField>
          )}
          <FormField label="首选盘符" tip="多卷继续使用后续空闲盘符">
            <Input value={driveLetter} onChange={setDriveLetter} maxLength={1} placeholder="自动分配" />
          </FormField>
        </>
      )}
    </Modal>
  )
}
