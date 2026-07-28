/**
 * 启用宿主机防火墙确认对话框
 * - 展示推荐规则（SSH/面板保护规则 + 端口转发放通），保护行禁止编辑
 * - 确认后提交任务队列异步启用（428 二次验证由请求层处理）
 */
import { useState } from 'react'
import { Banner, Button, Input, InputNumber, Modal, Select, Table, Toast } from '@douyinfe/semi-ui'
import type { ColumnProps } from '@douyinfe/semi-ui/lib/es/table'
import {
  enableHostFirewall,
  type HostFirewallRule,
} from '@/api/firewall'
import { normalizeRulePayload } from '../utils'
import { useMountModalLifecycle } from '@/hooks/useMountModalLifecycle'

interface EnableHostFirewallDialogProps {
  /** 预览接口返回的推荐规则 */
  rules: HostFirewallRule[]
  onClose: () => void
  /** 启用任务提交成功后的回调 */
  onEnabled: () => void
}

export default function EnableHostFirewallDialog({
  rules,
  onClose,
  onEnabled,
}: EnableHostFirewallDialogProps) {
  const { modalVisible, requestClose, afterModalClose } = useMountModalLifecycle(onClose)
  const [rows, setRows] = useState<HostFirewallRule[]>(rules)
  const [submitting, setSubmitting] = useState(false)

  /** 更新某一行字段 */
  const patchRow = (index: number, patch: Partial<HostFirewallRule>) => {
    setRows((list) => list.map((r, i) => (i === index ? { ...r, ...patch } : r)))
  }

  const handleSubmit = async () => {
    setSubmitting(true)
    try {
      const res = await enableHostFirewall({ rules: rows.map(normalizeRulePayload) })
      Toast.success(res.message || '宿主机防火墙启用任务已提交')
      onEnabled()
      requestClose()
    } catch {
      // 请求层已提示
    } finally {
      setSubmitting(false)
    }
  }

  const columns: ColumnProps<HostFirewallRule>[] = [
    {
      title: '动作',
      dataIndex: 'action',
      width: 100,
      render: (_text, row, index) => (
        <Select
          size="small"
          value={row.action}
          disabled={row.protected}
          onChange={(v) => patchRow(index, { action: v as string })}
          style={{ width: '100%' }}
          optionList={[
            { label: '允许', value: 'allow' },
            { label: '拒绝', value: 'deny' },
          ]}
        />
      ),
    },
    {
      title: '协议',
      dataIndex: 'protocol',
      width: 100,
      render: (_text, row, index) => (
        <Select
          size="small"
          value={row.protocol}
          disabled={row.protected}
          onChange={(v) => patchRow(index, { protocol: v as string })}
          style={{ width: '100%' }}
          optionList={[
            { label: 'TCP', value: 'tcp' },
            { label: 'UDP', value: 'udp' },
          ]}
        />
      ),
    },
    {
      title: '起始端口',
      dataIndex: 'port_start',
      width: 110,
      render: (_text, row, index) => (
        <InputNumber
          size="small"
          value={row.port_start}
          min={1}
          max={65535}
          disabled={row.protected}
          onChange={(v) => patchRow(index, { port_start: Number(v) || undefined })}
          style={{ width: '100%' }}
        />
      ),
    },
    {
      title: '结束端口',
      dataIndex: 'port_end',
      width: 110,
      render: (_text, row, index) => (
        <InputNumber
          size="small"
          value={row.port_end}
          min={row.port_start || 1}
          max={65535}
          disabled={row.protected}
          onChange={(v) => patchRow(index, { port_end: Number(v) || undefined })}
          style={{ width: '100%' }}
        />
      ),
    },
    {
      title: '来源 CIDR',
      dataIndex: 'source_cidr',
      render: (_text, row, index) => (
        <Input
          size="small"
          value={row.source_cidr || ''}
          placeholder="any"
          disabled={row.protected}
          onChange={(v) => patchRow(index, { source_cidr: v })}
        />
      ),
    },
    {
      title: '备注',
      dataIndex: 'comment',
      render: (_text, row, index) => (
        <Input
          size="small"
          value={row.comment || ''}
          disabled={row.protected}
          onChange={(v) => patchRow(index, { comment: v })}
        />
      ),
    },
  ]

  return (
    <Modal
      title="确认启用宿主机防火墙"
      visible={modalVisible}
      afterClose={afterModalClose}
      onCancel={requestClose}
      width={960}
      closeOnEsc
      footer={
        <>
          <Button onClick={requestClose}>取消</Button>
          <Button type="primary" loading={submitting} onClick={() => void handleSubmit()}>
            确认启用
          </Button>
        </>
      }
    >
      <Banner
        type="warning"
        closeIcon={null}
        description="请确认 SSH 和面板端口无误，启用后这两个端口对应规则不允许删除和编辑。"
        style={{ marginBottom: 12 }}
      />
      <Table<HostFirewallRule>
        rowKey={(row) => row?.id || `${row?.protocol}-${row?.port_start}-${row?.port_end}-${row?.source_cidr || 'any'}`}
        columns={columns}
        dataSource={rows}
        pagination={false}
        size="small"
        empty="暂无推荐规则"
      />
    </Modal>
  )
}
