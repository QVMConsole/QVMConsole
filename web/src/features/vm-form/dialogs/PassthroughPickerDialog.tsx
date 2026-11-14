/**
 * 直通设备选择弹窗（仅管理员，创建 / 编辑共用）
 * 默认展示通过准入策略的设备；确认风险后可查看未经筛选的完整列表。
 */
import { useEffect, useState } from 'react'
import { Banner, Button, Modal, Table, Tag } from '@douyinfe/semi-ui'
import { getPassthroughDevices, type PassthroughDevice } from '@/api/vm'
import { useVmFormScope } from '../scopeContext'
import PassthroughRiskDialog from './PassthroughRiskDialog'

interface PassthroughPickerDialogProps {
  visible: boolean
  onClose: () => void
}

export default function PassthroughPickerDialog({ visible, onClose }: PassthroughPickerDialogProps) {
  const { form, options } = useVmFormScope()
  const { form: f, setField } = form
  const { loadPassthroughDevices } = options
  const [loading, setLoading] = useState(true)
  const [selectedKeys, setSelectedKeys] = useState<string[]>([])
  const [showAllDevices, setShowAllDevices] = useState(false)
  const [allDevices, setAllDevices] = useState<PassthroughDevice[]>([])
  const [riskVisible, setRiskVisible] = useState(false)
  const [loadError, setLoadError] = useState(false)
  const [reloadKey, setReloadKey] = useState(0)
  const devices = showAllDevices ? allDevices : options.passthroughDevices

  useEffect(() => {
    if (!visible) return
    let active = true
    void (async () => {
      try {
        if (showAllDevices) {
          const result = await getPassthroughDevices(true)
          if (active) setAllDevices(result.data || [])
        } else {
          await loadPassthroughDevices(true)
        }
      } catch {
        if (active) {
          setAllDevices([])
          setLoadError(true)
        }
      } finally {
        if (active) setLoading(false)
      }
    })()
    // 关闭弹窗或切换模式后忽略旧请求，完整列表不写入默认选项缓存。
    return () => { active = false }
  }, [visible, showAllDevices, reloadKey, loadPassthroughDevices])

  const resetAfterClose = () => {
    setLoading(true)
    setSelectedKeys([])
    setShowAllDevices(false)
    setAllDevices([])
    setRiskVisible(false)
    setLoadError(false)
  }

  const isSelectable = (row: PassthroughDevice) => {
    if (row.is_used_by_vm) return false
    if (!showAllDevices && row.is_passthrough_capable === false) return false
    return !f.host_devices.some((d) => d.pci_address === row.pci_address)
  }

  const handleOk = () => {
    if (loading || riskVisible || loadError) return
    const selected = devices.filter((d) => selectedKeys.includes(d.pci_address) && isSelectable(d))
    const next = [...f.host_devices]
    for (const dev of selected) {
      if (!next.some((d) => d.pci_address === dev.pci_address)) {
        next.push({ pci_address: dev.pci_address, unsafe_confirmed: showAllDevices })
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
        <div style={{ color: 'var(--qvm-text-1)', overflowWrap: 'anywhere' }}>
          <div>{[row.vendor_name, row.product_name].filter(Boolean).join(' ') || '未知设备'}</div>
          <div style={{ fontSize: 11, color: 'var(--qvm-text-2)' }}>
            {row.class_name} · {row.vendor_id}:{row.product_id}
            {showAllDevices && ` · IOMMU 组：${row.iommu_group !== undefined && row.iommu_group >= 0 ? row.iommu_group : '无'}`}
          </div>
          {showAllDevices && row.is_passthrough_capable === false && (
            <div style={{ fontSize: 12, color: 'var(--semi-color-warning)', marginTop: 4 }}>
              直通风险：{row.capability_note || '设备未通过默认准入策略'}
            </div>
          )}
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
      render: (_: unknown, row: PassthroughDevice) => {
        if (row.is_used_by_vm) return <Tag color="red" size="small">{row.used_by_vm_name}</Tag>
        if (row.is_passthrough_capable === false) return <Tag color="orange" size="small">高风险</Tag>
        return <Tag color="green" size="small">未分配</Tag>
      },
    },
  ]

  return (
    <>
      <Modal
        title={showAllDevices ? '选择直通设备（全部设备）' : '选择直通设备'}
        visible={visible}
        onCancel={() => { setRiskVisible(false); onClose() }}
        afterClose={resetAfterClose}
        onOk={handleOk}
        okText="添加选中设备"
        okButtonProps={{ disabled: loading || riskVisible || loadError || selectedKeys.length === 0 }}
        cancelText="取消"
        width={720}
        closeOnEsc={!riskVisible}
        maskClosable={!riskVisible}
      >
        <Banner
          type="warning"
          closeIcon={null}
          style={{ marginBottom: 12 }}
          description={showAllDevices
            ? '当前显示全部 PCI 设备，可选择并直通原先被过滤的设备，提交时将跳过默认准入策略。风险说明仅供参考；未分配不代表宿主机未使用。'
            : '请确认设备未被其他虚拟机使用，且 IOMMU 组已正确隔离。直通操作需要虚拟机在关机状态下进行。'}
        />
        {loadError && (
          <Banner type="danger" closeIcon={null} style={{ marginBottom: 12 }} description="获取全部设备失败，请在列表底部重新加载。" />
        )}
        <div style={{ maxHeight: 400, overflow: 'auto' }}>
          <Table
            rowKey="pci_address"
            size="small"
            bordered
            loading={loading}
            columns={columns}
            dataSource={devices}
            pagination={false}
            rowSelection={{
              selectedRowKeys: selectedKeys,
              onChange: (keys) => setSelectedKeys((keys || []) as string[]),
              getCheckboxProps: (row?: PassthroughDevice) => ({ disabled: loading || (row ? !isSelectable(row) : true) }),
            }}
          />
          <div style={{ padding: '16px 12px', textAlign: 'center', color: 'var(--qvm-text-2)' }}>
            {loadError ? (
              <Button onClick={() => { setLoading(true); setLoadError(false); setReloadKey((key) => key + 1) }}>重新加载设备</Button>
            ) : showAllDevices ? (
              <span>已显示全部 {allDevices.length} 个 PCI 设备</span>
            ) : (
              <>
                <div style={{ marginBottom: 10 }}>没有找到需要的设备？可在了解风险后查看完整设备列表。</div>
                <Button disabled={loading} onClick={() => setRiskVisible(true)}>查看全部直通设备</Button>
              </>
            )}
          </div>
        </div>
      </Modal>
      <PassthroughRiskDialog
        visible={visible && riskVisible}
        onClose={() => setRiskVisible(false)}
        onConfirm={() => { setRiskVisible(false); setLoading(true); setLoadError(false); setShowAllDevices(true) }}
      />
    </>
  )
}
