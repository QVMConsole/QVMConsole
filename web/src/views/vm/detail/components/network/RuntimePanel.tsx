/**
 * 运行状态面板（仅管理员）
 * - OVS 网桥 / 接口运行状态 / 异常提示
 * - 带宽限速配置详情（QoS / Queue / tc）
 */
import { useEffect } from 'react'
import { Banner, Button, Descriptions, Table, Tag } from '@douyinfe/semi-ui'
import { IconRefresh } from '@douyinfe/semi-icons'
import type { ColumnProps } from '@douyinfe/semi-ui/lib/es/table'
import type { VmNetworkInterface } from '@/api/vm'
import { ipSourceLabel } from '../../utils'
import type { NetworkSharedData } from './NetworkTab'

interface RuntimePanelProps {
  shared: NetworkSharedData
}

export default function RuntimePanel({ shared }: RuntimePanelProps) {
  const { runtimeStatus, refreshRuntimeStatus } = shared

  // 面板激活时刷新一次
  useEffect(() => {
    void refreshRuntimeStatus()
  }, [refreshRuntimeStatus])

  const issues = runtimeStatus?.issues || []
  const bandwidth = runtimeStatus?.bandwidth

  const columns: ColumnProps<VmNetworkInterface>[] = [
    { title: '接口', dataIndex: 'target', width: 90 },
    { title: '网桥', dataIndex: 'source_bridge', width: 110, render: (t) => t || '-' },
    { title: 'VirtualPort', dataIndex: 'virtualport_type', width: 110, render: (t) => t || '-' },
    { title: 'ofport', dataIndex: 'ofport', width: 80, render: (t) => t || '-' },
    { title: '模型', dataIndex: 'model', width: 90, render: (t) => t || '-' },
    { title: 'MAC', dataIndex: 'mac', render: (t) => <span className="qvm-mono">{t || '-'}</span> },
    { title: 'IP', dataIndex: 'ip', render: (t) => <span className="qvm-mono">{t || '-'}</span> },
    {
      title: 'IP 来源',
      dataIndex: 'ip_source',
      width: 110,
      render: (t) => ipSourceLabel(String(t || '')),
    },
    {
      title: '异常',
      dataIndex: 'issues',
      render: (_t, row) => (row.issues?.length ? row.issues.join('；') : '-'),
    },
  ]

  return (
    <div className="qvm-runtime-panel">
      <div className="qvm-tab-toolbar">
        <div className="qvm-tab-toolbar-left">
          <Button size="small" icon={<IconRefresh />} onClick={() => void refreshRuntimeStatus()}>
            刷新状态
          </Button>
          <Tag size="small" color={issues.length ? 'orange' : 'green'}>
            {issues.length ? '存在异常' : '状态正常'}
          </Tag>
        </div>
      </div>

      <Descriptions align="left" className="qvm-runtime-summary">
        <Descriptions.Item itemKey="虚拟机">{runtimeStatus?.vm_name || '-'}</Descriptions.Item>
        <Descriptions.Item itemKey="状态">{runtimeStatus?.state || '-'}</Descriptions.Item>
        <Descriptions.Item itemKey="OVS 网桥">{runtimeStatus?.bridge || '-'}</Descriptions.Item>
        <Descriptions.Item itemKey="限速状态">
          <Tag size="small" color={bandwidth?.enabled ? 'green' : 'grey'}>
            {bandwidth?.enabled ? '已配置' : '未配置'}
          </Tag>
        </Descriptions.Item>
      </Descriptions>

      {issues.length > 0 && (
        <Banner type="warning" closeIcon={null} description={issues.join('；')} style={{ marginBottom: 12 }} />
      )}

      <Table<VmNetworkInterface>
        rowKey="target"
        columns={columns}
        dataSource={runtimeStatus?.interfaces || []}
        pagination={false}
        size="small"
        empty="暂无接口数据"
      />

      {bandwidth && (
        <>
          <div className="qvm-sub-title" style={{ marginTop: 16 }}>带宽配置详情</div>
          <Descriptions align="left">
            <Descriptions.Item itemKey="Cookie">{bandwidth.cookie || '-'}</Descriptions.Item>
            <Descriptions.Item itemKey="Flow">{bandwidth.flow_exists ? '存在' : '不存在'}</Descriptions.Item>
            <Descriptions.Item itemKey="检查端口">{bandwidth.checked_port || '-'}</Descriptions.Item>
            <Descriptions.Item itemKey="下行 QoS">{bandwidth.down_qos ? '存在' : '不存在'}</Descriptions.Item>
            <Descriptions.Item itemKey="上行 Bridge QoS">{bandwidth.bridge_qos ? '存在' : '不存在'}</Descriptions.Item>
            <Descriptions.Item itemKey="Queue">{bandwidth.queue || '-'}</Descriptions.Item>
            <Descriptions.Item itemKey="网卡下行 tc">{bandwidth.tc_root ? '存在' : '不存在'}</Descriptions.Item>
            <Descriptions.Item itemKey="网卡上行 tc">{bandwidth.tc_upload_police ? '存在' : '不存在'}</Descriptions.Item>
            <Descriptions.Item itemKey="网卡 ingress">{bandwidth.tc_ingress ? '存在' : '不存在'}</Descriptions.Item>
          </Descriptions>
        </>
      )}
    </div>
  )
}
