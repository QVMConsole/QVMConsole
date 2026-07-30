/** 虚拟机磁盘 / 标准 OVA 导出弹窗。 */
import { useEffect, useMemo, useState } from 'react'
import {
  Banner,
  Button,
  Checkbox,
  CheckboxGroup,
  Modal,
  Radio,
  RadioGroup,
  Spin,
  Tag,
  Toast,
} from '@douyinfe/semi-ui'
import { IconExport } from '@douyinfe/semi-icons'
import type { VmListItem } from '@/api/vm'
import {
  exportVM,
  getVMExportOptions,
  type VmExportOptions,
} from '@/api/storage'
import { useMountModalLifecycle } from '@/hooks/useMountModalLifecycle'

interface VmExportDialogProps {
  vm: VmListItem
  onClose: () => void
}

const formatBytes = (bytes: number) => {
  if (!bytes) return '未知容量'
  return `${(bytes / 1024 / 1024 / 1024).toFixed(2)} GB`
}

export default function VmExportDialog({ vm, onClose }: VmExportDialogProps) {
  const { modalVisible, requestClose, afterModalClose } = useMountModalLifecycle(onClose)
  const [format, setFormat] = useState<'qcow2' | 'ova'>('qcow2')
  const [options, setOptions] = useState<VmExportOptions | null>(null)
  const [selected, setSelected] = useState<string[]>([])
  const [loading, setLoading] = useState(true)
  const [submitting, setSubmitting] = useState(false)

  useEffect(() => {
    void getVMExportOptions(vm.name)
      .then((response) => {
        setOptions(response.data)
        setSelected(
          response.data.disks.filter((disk) => disk.supported).map((disk) => disk.device),
        )
      })
      .finally(() => setLoading(false))
  }, [vm.name])

  const shutoff = ['shut off', 'shutoff'].includes((options?.status || vm.status).toLowerCase())
  const selectedDisks = useMemo(
    () => options?.disks.filter((disk) => selected.includes(disk.device)) || [],
    [options, selected],
  )
  const estimated = selectedDisks.reduce(
    (sum, disk) => sum + (disk.actual_bytes || disk.capacity_bytes || 0),
    0,
  )

  const handleDiskGroupChange = (values: Array<string | number>) => {
    const systemDisks = options?.disks
      .filter((disk) => disk.is_system && disk.supported)
      .map((disk) => disk.device) || []
    setSelected([...new Set([...values.map(String), ...systemDisks])])
  }

  const handleSubmit = async () => {
    setSubmitting(true)
    try {
      const response = await exportVM({
        vm_name: vm.name,
        format,
        disk_devices: format === 'ova' ? selected : undefined,
      })
      Toast.success(response.message || '导出任务已提交，请在任务中心查看进度')
      requestClose()
    } finally {
      setSubmitting(false)
    }
  }

  return (
    <Modal
      title={`导出虚拟机 · ${vm.name}`}
      visible={modalVisible}
      afterClose={afterModalClose}
      onCancel={requestClose}
      footer={
        <>
          <Button onClick={requestClose}>取消</Button>
          <Button
            type="primary"
            theme="solid"
            icon={<IconExport />}
            loading={submitting}
            disabled={loading || (format === 'ova' && (!shutoff || selectedDisks.length === 0))}
            onClick={() => void handleSubmit()}
          >
            提交导出
          </Button>
        </>
      }
      width={640}
      maskClosable={false}
    >
      <RadioGroup
        type="card"
        value={format}
        onChange={(event) => setFormat(event.target.value as 'qcow2' | 'ova')}
        className="qvm-export-format-list"
        aria-label="导出格式"
        name="vm-export-format"
      >
        <Radio
          value="qcow2"
          className="qvm-export-format-option"
          extra="保持原有行为，仅导出系统盘到“我的存储 → 虚拟磁盘”"
        >
          QCOW2 磁盘
        </Radio>
        <Radio
          value="ova"
          className="qvm-export-format-option"
          extra="导出 OVF 描述、SHA-256 清单和所选磁盘，封装为单个 OVA"
        >
          标准 OVA 虚拟机
        </Radio>
      </RadioGroup>

      {format === 'ova' && (
        <>
          {!shutoff && <Banner type="warning" description="标准 OVA 导出要求虚拟机处于关机状态，任务不会自动关机。" closeIcon={null} />}
          <div className="qvm-export-section-title">选择导出磁盘</div>
          {loading ? (
            <div className="qvm-export-loading"><Spin /> 正在读取磁盘信息…</div>
          ) : (
            <CheckboxGroup
              type="card"
              value={selected}
              onChange={handleDiskGroupChange}
              className="qvm-export-disk-list"
              aria-label="选择导出磁盘"
              name="vm-export-disks"
            >
              {options?.disks.map((disk) => (
                <Checkbox
                  key={disk.device}
                  value={disk.device}
                  disabled={disk.is_system || !disk.supported}
                  className={`qvm-export-disk-option${disk.is_system ? ' qvm-export-system-disk' : ''}${!disk.supported ? ' qvm-export-unsupported-disk' : ''}`}
                  extra={`${disk.format || '未知格式'} · ${disk.bus || '未知总线'} · ${formatBytes(disk.capacity_bytes)}${disk.reason ? ` · ${disk.reason}` : ''}`}
                >
                  <span className="qvm-export-disk-title">
                    <Tag color={disk.is_system ? 'green' : 'blue'} size="small" type="light">
                      {disk.is_system ? '系统盘' : '数据盘'}
                    </Tag>
                    <strong>{disk.device}</strong>
                    {disk.is_system && <Tag color="green" size="small">固定</Tag>}
                  </span>
                </Checkbox>
              ))}
            </CheckboxGroup>
          )}
          <div className="qvm-export-estimate">已选 {selectedDisks.length} 块磁盘，源数据约 {formatBytes(estimated)}</div>
          <Banner
            type="info"
            description="导出会展平 backing chain 并转换为 streamOptimized VMDK；ISO、软盘、快照树、原 NVRAM、保存态和宿主机直通设备不会写入 OVA。"
            closeIcon={null}
          />
        </>
      )}
      <div className="qvm-export-quota-tip">导出结果计入我的存储配额；失败或取消时会清理临时文件与半成品。</div>
    </Modal>
  )
}
