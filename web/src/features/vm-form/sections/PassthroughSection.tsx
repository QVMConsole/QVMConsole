/**
 * 硬件直通分区（仅管理员，创建 / 编辑共用）
 * 展示已配置直通设备，支持添加（弹窗选择）与移除。
 */
import { useState } from 'react'
import { Banner, Button, Table, Tag } from '@douyinfe/semi-ui'
import { IconPlus, IconPuzzle } from '@douyinfe/semi-icons'
import SectionCard from './SectionCard'
import { useVmFormScope } from '../scopeContext'
import PassthroughPickerDialog from '../dialogs/PassthroughPickerDialog'

export default function PassthroughSection() {
  const { form, options, ctx } = useVmFormScope()
  const { form: f, setField } = form
  const [pickerVisible, setPickerVisible] = useState(false)
  const runningOrPaused = ctx.vmStatus === 'running' || ctx.vmStatus === 'paused'

  if (!ctx.isAdmin) return null

  const deviceName = (pciAddress: string) => {
    const cached = options.passthroughDevices.find((d) => d.pci_address === pciAddress)
    if (cached) return [cached.vendor_name, cached.product_name].filter(Boolean).join(' ') || '未知设备'
    return pciAddress
  }

  const isVfioBound = (pciAddress: string) =>
    options.passthroughDevices.find((d) => d.pci_address === pciAddress)?.is_vfio_bound || false

  const removeDevice = (index: number) => {
    setField('host_devices', f.host_devices.filter((_, i) => i !== index))
    setField('host_devices_touched', true)
  }

  const columns = [
    { title: 'PCI 地址', dataIndex: 'pci_address', width: 160 },
    {
      title: '设备名称',
      dataIndex: 'pci_address',
      render: (pci: string) => deviceName(pci),
    },
    {
      title: '状态',
      dataIndex: 'pci_address',
      width: 100,
      render: (pci: string) =>
        isVfioBound(pci) ? (
          <Tag color="green" size="small">已绑定</Tag>
        ) : (
          <Tag color="orange" size="small">待绑定</Tag>
        ),
    },
    {
      title: '操作',
      dataIndex: 'pci_address',
      width: 100,
      align: 'center' as const,
      render: (_: string, __: { pci_address: string }, index: number) => (
        <Button size="small" type="danger" theme="light" onClick={() => removeDevice(index)}>
          移除
        </Button>
      ),
    },
  ]

  return (
    <SectionCard icon={<IconPuzzle />} title="硬件直通">
      <Banner
        type="info"
        closeIcon={null}
        style={{ marginBottom: 14 }}
        description="硬件直通可将 GPU、NVMe 硬盘、网卡等 PCI 设备直接分配给虚拟机，获得接近原生的性能。配置后设备将在虚拟机启动时自动绑定到 vfio-pci 驱动。"
      />
      {f.host_devices.length > 0 ? (
        <Table
          rowKey="pci_address"
          size="small"
          bordered
          columns={columns}
          dataSource={f.host_devices}
          pagination={false}
          style={{ marginBottom: 10 }}
        />
      ) : (
        <div className="qvm-vf-empty-text">未配置直通设备</div>
      )}
      <Button type="primary" theme="light" size="small" icon={<IconPlus />} onClick={() => setPickerVisible(true)}>
        添加直通设备
      </Button>
      {ctx.mode === 'edit' && runningOrPaused && (
        <div className="qvm-vf-tip warn" style={{ marginTop: 8 }}>
          修改硬件直通设备需要先关机
        </div>
      )}
      <PassthroughPickerDialog visible={pickerVisible} onClose={() => setPickerVisible(false)} />
    </SectionCard>
  )
}
