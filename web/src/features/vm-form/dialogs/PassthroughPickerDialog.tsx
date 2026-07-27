/**
 * 直通设备选择弹窗（仅管理员，创建 / 编辑共用）
 * 展示宿主机 PCI 设备，勾选未占用设备加入虚拟机直通列表。
 */
import { useEffect, useState } from 'react'
import { Banner, Modal, Table, Tag } from '@douyinfe/semi-ui'
import type { PassthroughDevice } from '@/api/vm'
import { useVmFormScope } from '../scopeContext'

interface PassthroughPickerDialogProps {
  visible: boolean
  onClose: () => void
}

export default function PassthroughPickerDialog({ visible, onClose }: PassthroughPickerDialogProps) {
  const { form, options } = useVmFormScope()
  const { form: f, setField } = form
  const [loading, setLoading] = useState(false)
  const [selectedKeys, setSelectedKeys] = useState<string[]>([])

  useEffect(() => {
    if (!visible) return
    setSelectedKeys([])
    setLoading(true)
    void options.loadPassthroughDevices().finally(() => setLoading(false))
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [visible])

  const isSelectable = (row: PassthroughDevice) => {
    if (row.is_used_by_vm) return false
    return !f.host_devices.some((d) => d.pci_address === row.pci_address)
  }

  const handleOk = () => {
    const selected = options.passthroughDevices.filter((d) => selectedKeys.includes(d.pci_address))
    const next = [...f.host_devices]
    for (const dev of selected) {
      if (!next.some((d) => d.pci_address === dev.pci_address)) {
        next.push({ pci_address: dev.pci_address })
      }
    }
    setField('host_devices', next)
    setField('host_devices_touched', true)
    onClose()
  }

  const columns = [
    { title: 'PCI 地址', dataIndex: 'pci_address', width: 150 },
    {
      title: '设备名称',
      dataIndex: 'vendor_name',
      render: (_: unknown, row: PassthroughDevice) => (
        <div>
          <div>{[row.vendor_name, row.product_name].filter(Boolean).join(' ') || '未知设备'}</div>
          <div style={{ fontSize: 11, color: 'var(--qvm-text-2)' }}>
            {row.class_name} · {row.vendor_id}:{row.product_id}
          </div>
        </div>
      ),
    },
    {
      title: '驱动',
      dataIndex: 'driver_in_use',
      width: 110,
      render: (_: unknown, row: PassthroughDevice) => {
        if (row.is_vfio_bound) return <Tag color="green" size="small">vfio-pci</Tag>
        if (row.driver_in_use) return <Tag color="orange" size="small">{row.driver_in_use}</Tag>
        return <Tag size="small">无驱动</Tag>
      },
    },
    {
      title: '占用',
      dataIndex: 'is_used_by_vm',
      width: 100,
      render: (_: unknown, row: PassthroughDevice) =>
        row.is_used_by_vm ? (
          <Tag color="red" size="small">{row.used_by_vm_name}</Tag>
        ) : (
          <Tag color="green" size="small">空闲</Tag>
        ),
    },
  ]

  return (
    <Modal
      title="选择直通设备"
      visible={visible}
      onCancel={onClose}
      onOk={handleOk}
      okText="添加选中设备"
      cancelText="取消"
      width={720}
      closeOnEsc
    >
      <Banner
        type="warning"
        closeIcon={null}
        style={{ marginBottom: 12 }}
        description="请确认设备未被其他虚拟机使用，且 IOMMU 组已正确隔离。直通操作需要虚拟机在关机状态下进行。"
      />
      <Table
        rowKey="pci_address"
        size="small"
        bordered
        loading={loading}
        columns={columns}
        dataSource={options.passthroughDevices}
        pagination={false}
        scroll={{ y: 400 }}
        rowSelection={{
          selectedRowKeys: selectedKeys,
          onChange: (keys) => setSelectedKeys((keys || []) as string[]),
          getCheckboxProps: (row?: PassthroughDevice) => ({ disabled: row ? !isSelectable(row) : true }),
        }}
      />
    </Modal>
  )
}
