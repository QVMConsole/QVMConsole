/**
 * 轻量云单 VM 配额编辑表格
 * 用于「分配已有 VM」场景：为每台已选 VM 设置流量 / 带宽 / 端口转发 / 快照 / 运行时长配额
 * 创建用户、分配 VM、注册 VM 三处弹窗复用
 */
import { InputNumber, Table } from '@douyinfe/semi-ui'
import type { ColumnProps } from '@douyinfe/semi-ui/lib/es/table'
import type { LightweightVmQuotaPayload } from '@/api/user'

interface LightweightQuotaTableProps {
  rows: LightweightVmQuotaPayload[]
  onRowChange: (vmName: string, patch: Partial<LightweightVmQuotaPayload>) => void
}

export default function LightweightQuotaTable({ rows, onRowChange }: LightweightQuotaTableProps) {
  /** 生成数字输入列 */
  const numberColumn = (
    title: string,
    field: keyof LightweightVmQuotaPayload,
    precision?: number,
  ): ColumnProps<LightweightVmQuotaPayload> => ({
    title,
    dataIndex: field,
    width: 120,
    render: (_text, row) => (
      <InputNumber
        value={Number(row[field]) || 0}
        onNumberChange={(v) => onRowChange(row.vm_name, { [field]: Number(v) || 0 })}
        min={0}
        precision={precision}
        size="small"
        style={{ width: '100%' }}
      />
    ),
  })

  const columns: ColumnProps<LightweightVmQuotaPayload>[] = [
    {
      title: '虚拟机',
      dataIndex: 'vm_name',
      width: 140,
      fixed: true,
      render: (text) => <span className="usr-lw-vm-name">{text}</span>,
    },
    numberColumn('下行月流量(GB)', 'traffic_down_gb', 2),
    numberColumn('上行月流量(GB)', 'traffic_up_gb', 2),
    numberColumn('下行带宽(Mbps)', 'bandwidth_down_mbps'),
    numberColumn('上行带宽(Mbps)', 'bandwidth_up_mbps'),
    numberColumn('端口转发上限', 'max_port_forwards'),
    numberColumn('快照上限', 'max_snapshots'),
    numberColumn('运行时长(小时)', 'max_runtime_hours'),
  ]

  return (
    <Table<LightweightVmQuotaPayload>
      rowKey="vm_name"
      columns={columns}
      dataSource={rows}
      pagination={false}
      size="small"
      scroll={{ x: 1020 }}
      empty="暂无已选 VM"
    />
  )
}
