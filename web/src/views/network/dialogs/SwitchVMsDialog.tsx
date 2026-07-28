/**
 * 交换机虚拟机列表对话框
 * 展示绑定到该交换机的 VM 及网口序号
 */
import { useEffect, useState } from 'react'
import { Empty, Modal, Table, Tag } from '@douyinfe/semi-ui'
import type { ColumnProps } from '@douyinfe/semi-ui/lib/es/table'
import { getVPCSwitchVMs, type VpcSwitch, type VpcSwitchVM } from '@/api/vpc'
import { useMountModalLifecycle } from '@/hooks/useMountModalLifecycle'

interface SwitchVMsDialogProps {
  row: VpcSwitch
  onClose: () => void
}

export default function SwitchVMsDialog({ row, onClose }: SwitchVMsDialogProps) {
  const { modalVisible, requestClose, afterModalClose } = useMountModalLifecycle(onClose)
  const [vms, setVms] = useState<VpcSwitchVM[]>([])
  const [loading, setLoading] = useState(false)

  useEffect(() => {
    setLoading(true)
    getVPCSwitchVMs(row.id)
      .then((res) => setVms(res.data || []))
      .catch(() => setVms([]))
      .finally(() => setLoading(false))
  }, [row.id])

  const columns: ColumnProps<VpcSwitchVM>[] = [
    { title: '虚拟机名称', dataIndex: 'vm_name' },
    { title: '所属用户', dataIndex: 'username', width: 140 },
    {
      title: '网口序号',
      dataIndex: 'interface_order',
      width: 100,
      align: 'center',
      render: (text) => <Tag size="small">#{Number(text) + 1}</Tag>,
    },
  ]

  return (
    <Modal
      title={`交换机 - ${row.name} - 虚拟机列表`}
      visible={modalVisible}
      afterClose={afterModalClose}
      onCancel={requestClose}
      footer={null}
      width={600}
      closeOnEsc
    >
      {!loading && vms.length === 0 ? (
        <Empty description="该交换机下暂无虚拟机绑定" />
      ) : (
        <Table<VpcSwitchVM>
          rowKey={(r) => `${r?.vm_name ?? ''}-${r?.interface_order ?? 0}`}
          columns={columns}
          dataSource={vms}
          loading={loading}
          pagination={false}
          size="small"
        />
      )}
    </Modal>
  )
}
