/**
 * 迁移虚拟机弹窗（仅管理员）
 * - 迁移虚拟机：跨节点热/冷迁移，支持预检与逐盘目标存储
 * - 迁移硬盘：本机硬盘跨存储迁移（子组件）
 * 迁移自旧前端 VmMigrationDialog.vue
 */
import { useCallback, useEffect, useMemo, useState } from 'react'
import {
  Banner,
  Button,
  Checkbox,
  Descriptions,
  Input,
  InputNumber,
  Modal,
  Radio,
  Select,
  Table,
  Tag,
  Toast,
} from '@douyinfe/semi-ui'
import type { VmDiskItem, VmListItem } from '@/api/vm'
import { getDiskList } from '@/api/vm'
import {
  getNodeMigrationOptions,
  listNodes,
  migrateVm,
  previewVmMigration,
  type HostNode,
  type MigrationStorageTarget,
  type NodeMigrationOptions,
  type VmMigrationPayload,
  type VmMigrationPreview,
} from '@/api/migration'
import { formatBytes } from '@/utils/format'
import DiskMigrationPanel from './DiskMigrationPanel'
import { useMountModalLifecycle } from '@/hooks/useMountModalLifecycle'

interface VmMigrationDialogProps {
  vm: VmListItem
  onClose: () => void
  onSuccess: () => void
}

const formatMiB = (value?: number) => `${Number(value || 0).toFixed(2)} MiB`
const formatPercent = (value?: number) => `${Number(value || 0).toFixed(1)}%`

export default function VmMigrationDialog({ vm, onClose, onSuccess }: VmMigrationDialogProps) {
  const { modalVisible, requestClose, afterModalClose } = useMountModalLifecycle(onClose)
  const [kind, setKind] = useState<'vm' | 'disk'>('vm')
  const [nodes, setNodes] = useState<HostNode[]>([])
  const [optionsData, setOptionsData] = useState<NodeMigrationOptions | null>(null)
  const [previewData, setPreviewData] = useState<VmMigrationPreview | null>(null)
  const [sourceDisks, setSourceDisks] = useState<VmDiskItem[]>([])
  const [optionsLoading, setOptionsLoading] = useState(false)
  const [previewLoading, setPreviewLoading] = useState(false)
  const [sourceDisksLoading, setSourceDisksLoading] = useState(false)
  const [submitting, setSubmitting] = useState(false)

  const [nodeId, setNodeId] = useState<number | null>(null)
  const [targetStoragePoolId, setTargetStoragePoolId] = useState('')
  const [targetSwitchId, setTargetSwitchId] = useState<number>(0)
  const [targetSecurityGroupId, setTargetSecurityGroupId] = useState<number>(0)
  const [skipPrecheck, setSkipPrecheck] = useState(false)
  const [enableCpuThrottle, setEnableCpuThrottle] = useState(false)
  const [cpuThrottlePercent, setCpuThrottlePercent] = useState(50)
  const [diskStorageForm, setDiskStorageForm] = useState<Record<string, string>>({})

  const migrationMode = optionsData?.mode || (vm.status === 'running' ? 'live' : 'cold')
  const sourceStateLabel = optionsData?.source_state || vm.status || '-'
  const targetStorageTargets = useMemo(
    () => optionsData?.target_storage_targets || [],
    [optionsData],
  )
  const showTargetNetwork = !!optionsData?.target_user_exists
  const lightweightSwitches = useMemo(
    () =>
      (optionsData?.target_switches || []).filter(
        (item) => !item.bridge_mode || item.bridge_mode === 'nat',
      ),
    [optionsData],
  )

  const storageLabel = (item: MigrationStorageTarget) =>
    `${item.display_name || item.id}（可用 ${formatBytes(item.available)}）`

  const storageName = useCallback(
    (id: string) =>
      targetStorageTargets.find((target) => target.id === id)?.display_name || id || '-',
    [targetStorageTargets],
  )

  const applyDefaultStorageToDisks = useCallback(
    (disks: VmDiskItem[], poolId: string, current: Record<string, string>) => {
      const next = { ...current }
      disks.forEach((disk) => {
        if (!next[disk.device]) next[disk.device] = poolId
      })
      return next
    },
    [],
  )

  // 加载目标节点的迁移选项
  const loadOptions = useCallback(
    async (targetNodeId: number) => {
      setOptionsData(null)
      setPreviewData(null)
      setSourceDisks([])
      setDiskStorageForm({})
      setTargetStoragePoolId('')
      setTargetSwitchId(0)
      setTargetSecurityGroupId(0)
      setOptionsLoading(true)
      try {
        const res = await getNodeMigrationOptions(targetNodeId, { vm_name: vm.name })
        const data = res.data || {}
        setOptionsData(data)
        const enabledTargets = (data.target_storage_targets || []).filter((item) => item.enabled)
        const defaultStorage = enabledTargets.find((item) => item.is_default) || enabledTargets[0]
        const defaultPoolId = defaultStorage?.id || ''
        if (defaultPoolId) setTargetStoragePoolId(defaultPoolId)
        // 源磁盘列表（多块硬盘可分别选择对端存储）
        setSourceDisksLoading(true)
        try {
          const diskRes = await getDiskList(vm.name)
          const disks = (diskRes.data || []).filter(
            (item) => item.device_type === 'disk' && item.path,
          )
          setSourceDisks(disks)
          setDiskStorageForm(applyDefaultStorageToDisks(disks, defaultPoolId, {}))
        } finally {
          setSourceDisksLoading(false)
        }
        if (data.target_switch_id) setTargetSwitchId(data.target_switch_id)
        if (data.target_security_group_id) setTargetSecurityGroupId(data.target_security_group_id)
      } catch (err) {
        console.error('获取迁移选项失败', err)
      } finally {
        setOptionsLoading(false)
      }
    },
    [vm.name, applyDefaultStorageToDisks],
  )

  // 初始化：加载节点列表并选中第一个可用节点
  useEffect(() => {
    let cancelled = false
    listNodes()
      .then(async (res) => {
        if (cancelled) return
        const nodeList = res.data || []
        setNodes(nodeList)
        const first = nodeList.find((item) => item.enabled)
        if (first) {
          setNodeId(first.id)
          await loadOptions(first.id)
        }
      })
      .catch((err) => console.error('获取节点列表失败', err))
    return () => {
      cancelled = true
    }
  }, [loadOptions])

  const markDirty = () => setPreviewData(null)

  const handleNodeChange = (value: number) => {
    setNodeId(value)
    void loadOptions(value)
  }

  const handleDefaultStorageChange = (value: string) => {
    setTargetStoragePoolId(value)
    setPreviewData(null)
    setDiskStorageForm((current) => {
      const next = { ...current }
      sourceDisks.forEach((disk) => {
        next[disk.device] = value
      })
      return next
    })
  }

  // ==================== 预检与提交 ====================
  const canPreview =
    !!nodeId &&
    !!targetStoragePoolId &&
    !optionsLoading &&
    !sourceDisksLoading &&
    !(sourceDisks.length > 1 && sourceDisks.some((disk) => !diskStorageForm[disk.device])) &&
    (!showTargetNetwork || !!targetSwitchId)

  const canSubmit = canPreview && !(previewData && !previewData.allowed && !skipPrecheck)

  const buildDiskStorageTargets = () =>
    sourceDisks
      .filter((disk) => disk.device && diskStorageForm[disk.device])
      .map((disk) => ({
        target: disk.device,
        device: disk.device,
        target_storage_pool_id: diskStorageForm[disk.device],
      }))

  const buildBasePayload = (): VmMigrationPayload => ({
    node_id: nodeId || 0,
    mode: migrationMode,
    skip_precheck: skipPrecheck,
    target_storage_pool_id: targetStoragePoolId,
    disk_storage_targets: buildDiskStorageTargets(),
    target_switch_id: targetSwitchId || 0,
    target_security_group_id: targetSecurityGroupId || 0,
    enable_cpu_throttle: enableCpuThrottle,
    cpu_throttle_percent: cpuThrottlePercent || 50,
  })

  const handlePreview = async () => {
    if (!canPreview) {
      Toast.warning('请先补全目标节点、目标存储和网络选择')
      return
    }
    setPreviewLoading(true)
    try {
      const res = await previewVmMigration(vm.name, buildBasePayload())
      const data = res.data || null
      setPreviewData(data)
      if (data?.target_switch_id) setTargetSwitchId(data.target_switch_id)
      if (data?.target_security_group_id) setTargetSecurityGroupId(data.target_security_group_id)
      if (data?.target_storage_pool_id) setTargetStoragePoolId(data.target_storage_pool_id)
    } catch {
      // 错误提示由请求层统一处理
    } finally {
      setPreviewLoading(false)
    }
  }

  const handleSubmit = async () => {
    if (!canSubmit) {
      Toast.warning('请先补全目标节点、目标存储和网络选择')
      return
    }
    setSubmitting(true)
    try {
      const payload: VmMigrationPayload = {
        ...buildBasePayload(),
        node_id: previewData?.node?.id || nodeId || 0,
        mode: previewData?.mode || migrationMode,
        preview_id: skipPrecheck ? '' : previewData?.preview_id || '',
        target_storage_pool_id: previewData?.target_storage_pool_id || targetStoragePoolId,
        target_switch_id: previewData?.target_switch_id || targetSwitchId || 0,
        target_security_group_id:
          previewData?.target_security_group_id || targetSecurityGroupId || 0,
      }
      const res = await migrateVm(vm.name, payload)
      Toast.success(res.message || '迁移任务已提交')
      requestClose()
      onSuccess()
    } catch {
      // 错误提示由请求层统一处理
    } finally {
      setSubmitting(false)
    }
  }

  const optionsSummary = useMemo(() => {
    if (!optionsData) return ''
    const userAction = optionsData.will_create_target_user
      ? '目标将自动注册同名用户，并使用该用户默认网络'
      : '目标将绑定已有同名用户，请选择该用户下的网络'
    const mode = migrationMode === 'live' ? '开机状态按热迁移处理' : '关机状态按冷迁移处理'
    return `${mode}，${userAction}`
  }, [optionsData, migrationMode])

  // ==================== 预检结果表格列 ====================
  const previewDiskColumns = [
    { title: '磁盘', dataIndex: 'target', width: 90 },
    {
      title: '目标存储',
      dataIndex: 'target_storage_pool_id',
      width: 160,
      render: (text: string) => storageName(text),
    },
    { title: '源 overlay', dataIndex: 'source_path', ellipsis: true },
    { title: '目标 overlay', dataIndex: 'target_path', ellipsis: true },
    { title: 'backing', dataIndex: 'backing_path', ellipsis: true },
  ]
  const backingCheckColumns = [
    {
      title: '状态',
      dataIndex: 'ok',
      width: 90,
      render: (ok: boolean) =>
        ok ? <Tag color="green">通过</Tag> : <Tag color="red">失败</Tag>,
    },
    { title: 'backing 路径', dataIndex: 'path', ellipsis: true },
    { title: '说明', dataIndex: 'message', ellipsis: true },
  ]
  const portForwardColumns = [
    { title: '协议', dataIndex: 'protocol', width: 90 },
    { title: '源端口', dataIndex: 'source_host_port', width: 100 },
    {
      title: '目标端口',
      dataIndex: 'target_host_port',
      width: 120,
      render: (text: number) => text || '自动分配',
    },
    { title: 'VM 端口', dataIndex: 'vm_port', width: 100 },
    { title: '源目标 IP', dataIndex: 'dest_ip', ellipsis: true },
  ]

  return (
    <Modal
      title={kind === 'disk' ? '迁移硬盘' : '迁移虚拟机'}
      visible={modalVisible}
      afterClose={afterModalClose}
      onCancel={requestClose}
      width={920}
      closeOnEsc
      footer={
        <>
          <Button onClick={requestClose}>取消</Button>
          {kind === 'vm' ? (
            <Button
              theme="solid"
              disabled={!canSubmit}
              loading={submitting}
              onClick={() => void handleSubmit()}
            >
              提交迁移
            </Button>
          ) : null}
        </>
      }
    >
      <div className="qvm-form-item">
        <div className="qvm-form-label">虚拟机</div>
        <Input value={vm.name} disabled />
      </div>
      <div className="qvm-form-item">
        <div className="qvm-form-label">迁移硬件</div>
        <Radio.Group
          type="button"
          value={kind}
          onChange={(e) => setKind(e.target.value as 'vm' | 'disk')}
          options={[
            { value: 'vm', label: '迁移虚拟机' },
            { value: 'disk', label: '迁移硬盘' },
          ]}
        />
      </div>

      {kind === 'disk' ? (
        <DiskMigrationPanel vm={vm} onClose={requestClose} onSuccess={onSuccess} />
      ) : (
        <>
          <div className="qvm-form-item">
            <div className="qvm-form-label">迁移方式</div>
            <Tag color={migrationMode === 'live' ? 'orange' : 'grey'}>
              {migrationMode === 'live' ? '热迁移' : '冷迁移'}
            </Tag>
            <span className="qvm-form-tip" style={{ marginLeft: 10 }}>
              当前状态：{sourceStateLabel}
            </span>
          </div>
          <div className="qvm-form-item">
            <div className="qvm-form-label required">目标节点</div>
            <Select
              style={{ width: '100%' }}
              value={nodeId ?? undefined}
              onChange={(value) => handleNodeChange(value as number)}
              filter
              placeholder="请选择目标节点"
              optionList={nodes.map((node) => ({
                label: `${node.name}（${node.ssh_host}）`,
                value: node.id,
                disabled: !node.enabled,
              }))}
            />
          </div>
          <div className="qvm-form-item">
            <div className="qvm-form-label required">目标存储</div>
            <Select
              style={{ width: '100%' }}
              value={targetStoragePoolId || undefined}
              onChange={(value) => handleDefaultStorageChange(value as string)}
              filter
              loading={optionsLoading}
              disabled={!optionsData}
              placeholder="请选择目标存储"
              optionList={targetStorageTargets.map((item) => ({
                label: storageLabel(item),
                value: item.id,
                disabled: !item.enabled,
              }))}
            />
            <div className="qvm-form-tip">
              作为默认目标存储；多块硬盘可在下方为每块硬盘单独选择对端存储位置。
            </div>
          </div>

          {sourceDisks.length > 1 && (
            <div className="qvm-form-item">
              <div className="qvm-form-label">硬盘目标存储</div>
              <Table
                rowKey="device"
                size="small"
                pagination={false}
                loading={sourceDisksLoading}
                dataSource={sourceDisks}
                columns={[
                  { title: '设备', dataIndex: 'device', width: 80 },
                  {
                    title: '容量',
                    dataIndex: 'capacity_gb',
                    width: 100,
                    render: (text) => (text ? `${text} GB` : '-'),
                  },
                  { title: '源路径', dataIndex: 'path', ellipsis: true },
                  {
                    title: '对端存储',
                    dataIndex: 'device',
                    width: 220,
                    render: (device: string) => (
                      <Select
                        style={{ width: '100%' }}
                        size="small"
                        value={diskStorageForm[device] || undefined}
                        onChange={(value) => {
                          markDirty()
                          setDiskStorageForm((current) => ({
                            ...current,
                            [device]: value as string,
                          }))
                        }}
                        filter
                        placeholder="请选择目标存储"
                        optionList={targetStorageTargets.map((item) => ({
                          label: storageLabel(item),
                          value: item.id,
                          disabled: !item.enabled,
                        }))}
                      />
                    ),
                  },
                ]}
              />
              <div className="qvm-form-tip">
                迁移执行前会按每块硬盘选择的存储生成目标路径，并分别检查空间和冲突。
              </div>
            </div>
          )}

          {showTargetNetwork && optionsData?.is_lightweight ? (
            <div className="qvm-form-item">
              <div className="qvm-form-label required">轻量云 VPC</div>
              <Select
                style={{ width: '100%' }}
                value={targetSwitchId || undefined}
                onChange={(value) => {
                  markDirty()
                  setTargetSwitchId(value as number)
                }}
                filter
                placeholder="请选择目标轻量云 VPC"
                optionList={lightweightSwitches.map((item) => ({
                  label: `${item.username} / ${item.name} / ${item.cidr}`,
                  value: item.id,
                }))}
              />
            </div>
          ) : (
            showTargetNetwork && (
              <>
                <div className="qvm-form-item">
                  <div className="qvm-form-label required">目标 VPC</div>
                  <Select
                    style={{ width: '100%' }}
                    value={targetSwitchId || undefined}
                    onChange={(value) => {
                      markDirty()
                      setTargetSwitchId(value as number)
                    }}
                    filter
                    placeholder="请选择目标 VPC"
                    optionList={(optionsData?.target_switches || []).map((item) => ({
                      label: `${item.username} / ${item.name} / ${item.cidr}`,
                      value: item.id,
                    }))}
                  />
                </div>
                <div className="qvm-form-item">
                  <div className="qvm-form-label">目标安全组</div>
                  <Select
                    style={{ width: '100%' }}
                    value={targetSecurityGroupId || undefined}
                    onChange={(value) => {
                      markDirty()
                      setTargetSecurityGroupId((value as number) || 0)
                    }}
                    filter
                    showClear
                    placeholder="默认安全组"
                    optionList={(optionsData?.target_security_groups || []).map((item) => ({
                      label: `${item.username} / ${item.name}`,
                      value: item.id,
                    }))}
                  />
                </div>
              </>
            )
          )}

          <div className="qvm-form-item">
            <div className="qvm-form-label">预检策略</div>
            <Checkbox
              checked={skipPrecheck}
              onChange={(e) => {
                markDirty()
                setSkipPrecheck(!!e.target.checked)
              }}
            >
              跳过完整预检
            </Checkbox>
            <div className="qvm-form-tip">
              不勾选时可直接提交，任务开始后自动执行预检；勾选后跳过耗时 backing hash 对比。
            </div>
          </div>

          {migrationMode === 'live' && (
            <div className="qvm-form-item">
              <div className="qvm-form-label">迁移 CPU 限制</div>
              <div style={{ display: 'flex', alignItems: 'center', gap: 12 }}>
                <Checkbox
                  checked={enableCpuThrottle}
                  onChange={(e) => {
                    markDirty()
                    setEnableCpuThrottle(!!e.target.checked)
                  }}
                >
                  迁移时限制 CPU 使用率
                </Checkbox>
                <InputNumber
                  value={cpuThrottlePercent}
                  onChange={(value) => {
                    markDirty()
                    setCpuThrottlePercent(Number(value || 50))
                  }}
                  min={10}
                  max={100}
                  step={5}
                  style={{ width: 120 }}
                  suffix="%"
                />
              </div>
              <div className="qvm-form-tip">
                脏页速率达到平均带宽 20% - 50% 时，后端会强制启用该限制；达到 50% 会阻止热迁移。
              </div>
            </div>
          )}

          {optionsData && (
            <Banner type="info" closeIcon={null} style={{ margin: '12px 0' }} description={optionsSummary} />
          )}

          <Button disabled={!canPreview} loading={previewLoading} onClick={() => void handlePreview()}>
            执行预检
          </Button>

          {previewData && (
            <Banner
              type={previewData.allowed ? 'success' : 'danger'}
              closeIcon={null}
              style={{ margin: '14px 0' }}
              description={previewData.allowed ? '预检通过，可以提交迁移任务' : '预检未通过'}
            />
          )}
          {!previewData && canSubmit && (
            <Banner
              type="info"
              closeIcon={null}
              style={{ margin: '14px 0' }}
              description="未执行预检也可以直接提交，迁移任务会在开始后生成执行计划"
            />
          )}
          {skipPrecheck && (
            <Banner
              type="warning"
              closeIcon={null}
              style={{ marginTop: 10 }}
              description="已选择跳过完整预检：任务不会提前计算 backing hash，失败原因会在任务详情中展示"
            />
          )}

          {previewData && (
            <Descriptions
              className="qvm-mig-summary"
              data={[
                { key: '源状态', value: previewData.source_state || '-' },
                { key: '用户', value: `${previewData.owner || '-'} / ${previewData.cloud_type || '-'}` },
                {
                  key: '目标用户',
                  value: previewData.will_create_target_user ? '自动注册' : '绑定已有用户',
                },
                { key: '目标存储', value: previewData.target_storage_dir || '-' },
                { key: '所需容量', value: formatBytes(previewData.required_storage_bytes || 0) },
                { key: 'VM 凭据', value: previewData.credential ? '同步' : '无凭据记录' },
              ]}
              row
              size="small"
            />
          )}

          {previewData?.live_assessment && (
            <Descriptions
              className="qvm-mig-summary"
              data={[
                {
                  key: '平均带宽',
                  value: `${formatMiB(previewData.live_assessment.average_bandwidth_mib)}/s`,
                },
                {
                  key: '脏页速率',
                  value: `${formatMiB(previewData.live_assessment.dirty_rate_mib)}/s`,
                },
                {
                  key: '脏页占比',
                  value: formatPercent(previewData.live_assessment.dirty_rate_ratio_percent),
                },
                {
                  key: 'CPU 限制',
                  value: previewData.live_assessment.cpu_throttle_enabled
                    ? `${previewData.live_assessment.cpu_throttle_percent}%`
                    : '不限制',
                },
                {
                  key: 'kvm_page_fault',
                  value: previewData.live_assessment.kvm_stat_available
                    ? `${previewData.live_assessment.kvm_page_fault_rate || 0}`
                    : '不可用',
                },
                {
                  key: '评估结论',
                  value: previewData.live_assessment.allowed ? (
                    <Tag color="green">允许热迁移</Tag>
                  ) : (
                    <Tag color="red">阻止热迁移</Tag>
                  ),
                },
              ]}
              row
              size="small"
            />
          )}

          {!!previewData?.disks?.length && (
            <Table
              style={{ marginTop: 12 }}
              size="small"
              rowKey="target"
              pagination={false}
              dataSource={previewData.disks}
              columns={previewDiskColumns}
            />
          )}
          {!!previewData?.backing_checks?.length && (
            <Table
              style={{ marginTop: 12 }}
              size="small"
              rowKey="path"
              pagination={false}
              dataSource={previewData.backing_checks}
              columns={backingCheckColumns}
            />
          )}
          {!!previewData?.port_forwards?.length && (
            <Table
              style={{ marginTop: 12 }}
              size="small"
              rowKey={(record) => `${record?.protocol}-${record?.source_host_port}`}
              pagination={false}
              dataSource={previewData.port_forwards}
              columns={portForwardColumns}
            />
          )}

          {(previewData?.warnings || []).map((item) => (
            <Banner key={item} type="warning" closeIcon={null} style={{ marginTop: 10 }} description={item} />
          ))}
          {(previewData?.blockers || []).map((item) => (
            <Banner key={item} type="danger" closeIcon={null} style={{ marginTop: 10 }} description={item} />
          ))}
        </>
      )}
    </Modal>
  )
}
