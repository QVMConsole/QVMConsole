/**
 * 静态 IP 面板
 * - 静态绑定列表（绑定 / 解绑）
 * - DHCP 租约列表（只读）
 * - 桥接直通交换机提示（不使用面板 DHCP）
 */
import { useMemo, useState } from 'react'
import { Banner, Button, Input, Modal, Select, Table, Toast } from '@douyinfe/semi-ui'
import { IconPlus } from '@douyinfe/semi-icons'
import type { ColumnProps } from '@douyinfe/semi-ui/lib/es/table'
import { bindStaticIP, unbindStaticIP, type DhcpLease, type StaticIPBinding } from '@/api/network'
import { confirmModal } from '@/utils/confirm'
import type { NetworkSharedData } from './NetworkTab'

interface StaticIpPanelProps {
  vmName: string
  shared: NetworkSharedData
}

export default function StaticIpPanel({ vmName, shared }: StaticIpPanelProps) {
  const { vpcInfo, staticBindings, dhcpLeases, refreshStaticIPs } = shared
  const [bindVisible, setBindVisible] = useState(false)
  const [selectedLeaseIP, setSelectedLeaseIP] = useState<string>('')
  const [customIP, setCustomIP] = useState('')
  const [submitting, setSubmitting] = useState(false)

  const currentSwitchIsBridge = vpcInfo?.switch?.bridge_mode === 'bridge'

  const currentVmBindings = useMemo(
    () => staticBindings.filter((b) => b.vm_name === vmName),
    [staticBindings, vmName],
  )
  const currentVmDhcpLeases = useMemo(() => {
    const staticIPs = new Set(currentVmBindings.map((b) => b.ip))
    const staticMACs = new Set(currentVmBindings.map((b) => b.mac))
    return dhcpLeases.filter(
      (l) => l.vm_name === vmName && !staticIPs.has(l.ip) && !staticMACs.has(l.mac),
    )
  }, [dhcpLeases, currentVmBindings, vmName])

  // ============ 绑定 ============
  const openBind = () => {
    if (currentSwitchIsBridge) {
      Toast.warning('当前二层交换机不使用面板 DHCP，不能在这里绑定静态 IP')
      return
    }
    if (currentVmDhcpLeases.length === 1) {
      setSelectedLeaseIP(currentVmDhcpLeases[0].ip)
    } else {
      setSelectedLeaseIP('')
    }
    setCustomIP('')
    setBindVisible(true)
  }

  const handleBind = async () => {
    setSubmitting(true)
    try {
      const ip = selectedLeaseIP || customIP
      const res = await bindStaticIP({ vm_name: vmName, ip })
      Toast.success(res.message || '静态 IP 绑定成功')
      setBindVisible(false)
      await refreshStaticIPs()
    } catch {
      // 请求层已提示
    } finally {
      setSubmitting(false)
    }
  }

  // ============ 解绑 ============
  const handleUnbind = async (row: StaticIPBinding) => {
    const ok = await confirmModal({
      title: '解绑静态 IP',
      content: `确定解绑 ${row.vm_name} 的静态 IP（${row.ip}）？`,
    })
    if (!ok) return
    try {
      await unbindStaticIP({ vm_name: row.vm_name, ip: row.ip })
      Toast.success('静态 IP 已解绑')
      await refreshStaticIPs()
    } catch {
      // 请求层已提示
    }
  }

  const bindingColumns: ColumnProps<StaticIPBinding>[] = [
    { title: 'IP 地址', dataIndex: 'ip', render: (text) => <span className="qvm-mono">{text}</span> },
    { title: 'MAC 地址', dataIndex: 'mac', render: (text) => <span className="qvm-mono">{text}</span> },
    {
      title: '操作',
      dataIndex: 'actions',
      width: 90,
      render: (_text, row) => (
        <Button size="small" type="danger" theme="light" onClick={() => void handleUnbind(row)}>
          解绑
        </Button>
      ),
    },
  ]

  const leaseColumns: ColumnProps<DhcpLease>[] = [
    { title: '主机名', dataIndex: 'hostname', render: (text) => text || '-' },
    { title: 'IP', dataIndex: 'ip', render: (text) => <span className="qvm-mono">{text}</span> },
    { title: 'MAC', dataIndex: 'mac', render: (text) => <span className="qvm-mono">{text}</span> },
    { title: '过期时间', dataIndex: 'expiry_time', render: (text) => text || '-' },
  ]

  return (
    <div className="qvm-staticip-panel">
      <div className="qvm-tab-toolbar">
        <Button type="primary" size="small" icon={<IconPlus />} onClick={openBind} disabled={currentSwitchIsBridge}>
          绑定 IP
        </Button>
      </div>

      {currentSwitchIsBridge && (
        <Banner
          type="info"
          closeIcon={null}
          description="当前 VM 接入空交换机或物理直通交换机，不使用面板 DHCP；地址应由软路由、虚拟机系统或上级网络配置。"
          style={{ marginBottom: 12 }}
        />
      )}

      <div className="qvm-sub-title">静态绑定</div>
      <Table<StaticIPBinding>
        rowKey={(r) => `${r?.ip}-${r?.mac}`}
        columns={bindingColumns}
        dataSource={currentVmBindings}
        pagination={false}
        size="small"
        empty="暂无静态绑定"
      />

      <div className="qvm-sub-title" style={{ marginTop: 16 }}>DHCP 租约</div>
      <Table<DhcpLease>
        rowKey={(r) => `${r?.ip}-${r?.mac}`}
        columns={leaseColumns}
        dataSource={currentVmDhcpLeases}
        pagination={false}
        size="small"
        empty="暂无 DHCP 租约"
      />

      {/* 绑定对话框 */}
      <Modal
        title="绑定静态 IP"
        visible={bindVisible}
        onCancel={() => setBindVisible(false)}
        onOk={() => void handleBind()}
        okText="确定"
        cancelText="取消"
        confirmLoading={submitting}
        width={440}
        closeOnEsc
      >
        <div className="qvm-form-item">
          <div className="qvm-form-label">DHCP 租约</div>
          <Select
            style={{ width: '100%' }}
            placeholder="选择要绑定的租约"
            value={selectedLeaseIP}
            onChange={(v) => setSelectedLeaseIP(String(v ?? ''))}
            optionList={[
              ...currentVmDhcpLeases.map((l) => ({
                value: l.ip,
                label: `${l.ip}（${l.hostname || l.vm_name}）`,
              })),
              { value: '', label: '自动分配 IP' },
            ]}
          />
        </div>
        {selectedLeaseIP === '' && (
          <div className="qvm-form-item">
            <div className="qvm-form-label">IP 地址</div>
            <Input
              value={customIP}
              onChange={setCustomIP}
              placeholder="留空自动分配，或输入完整 IP / 最后一位数字"
            />
          </div>
        )}
      </Modal>
    </div>
  )
}
