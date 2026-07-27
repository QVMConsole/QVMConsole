/**
 * 迁移硬盘面板（本机硬盘跨存储迁移，迁移弹窗子组件）
 * 运行中虚拟机执行热迁移并切换硬盘路径；关机虚拟机执行冷迁移并更新持久化 XML
 */
import { useEffect, useMemo, useState } from 'react'
import { Banner, Button, Select, Table, Tag, Toast } from '@douyinfe/semi-ui'
import {
  getDiskMigrationOptions,
  migrateDisk,
  type DiskMigrationOptions,
  type MigrationStorageTarget,
} from '@/api/migration'
import { formatBytes } from '@/utils/format'
import type { VmListItem } from '@/api/vm'

interface DiskMigrationPanelProps {
  vm: VmListItem
  onClose: () => void
  onSuccess: () => void
}

type DiskOption = NonNullable<DiskMigrationOptions['disks']>[number]

export default function DiskMigrationPanel({ vm, onClose, onSuccess }: DiskMigrationPanelProps) {
  const [optionsData, setOptionsData] = useState<DiskMigrationOptions | null>(null)
  const [loading, setLoading] = useState(false)
  const [submitting, setSubmitting] = useState(false)
  const [device, setDevice] = useState('')
  const [targetStoragePoolId, setTargetStoragePoolId] = useState('')

  const disks = useMemo(() => optionsData?.disks || [], [optionsData])
  const storageTargets = useMemo(() => optionsData?.target_storage_targets || [], [optionsData])
  const selectedDisk = useMemo(
    () => disks.find((item) => item.device === device),
    [disks, device],
  )
  const mode = optionsData?.mode || (vm.status === 'running' ? 'live' : 'cold')

  useEffect(() => {
    let cancelled = false
    setLoading(true)
    getDiskMigrationOptions(vm.name)
      .then((res) => {
        if (cancelled) return
        const data = res.data || {}
        setOptionsData(data)
        const firstDisk = (data.disks || []).find((item) => item.can_migrate)
        if (firstDisk) setDevice(firstDisk.device)
        const enabledTargets = (data.target_storage_targets || []).filter((item) => item.enabled)
        const defaultStorage = enabledTargets.find((item) => item.is_default) || enabledTargets[0]
        if (defaultStorage) setTargetStoragePoolId(defaultStorage.id)
      })
      .catch((err) => console.error('获取硬盘迁移选项失败', err))
      .finally(() => {
        if (!cancelled) setLoading(false)
      })
    return () => {
      cancelled = true
    }
  }, [vm.name])

  const canSubmit =
    !!optionsData && !loading && !!device && !!targetStoragePoolId && !!selectedDisk?.can_migrate

  const summary = useMemo(() => {
    if (!optionsData) return ''
    const modeText = mode === 'live' ? '运行中硬盘将按热迁移处理' : '关机硬盘将按冷迁移处理'
    if (!selectedDisk) return `${modeText}，请选择要迁移的硬盘和目标存储`
    const chainText = selectedDisk.backing_path ? '，链式硬盘仅迁移活动 overlay' : ''
    return `${modeText}，已选择 ${selectedDisk.device}${chainText}`
  }, [optionsData, mode, selectedDisk])

  const storageLabel = (item: MigrationStorageTarget) =>
    `${item.display_name || item.id}（可用 ${formatBytes(item.available)}）`

  const handleSubmit = async () => {
    if (!canSubmit) {
      Toast.warning('请先选择可迁移硬盘和目标存储')
      return
    }
    setSubmitting(true)
    try {
      const res = await migrateDisk(vm.name, device, {
        target_storage_pool_id: targetStoragePoolId,
      })
      Toast.success(res.message || '硬盘迁移任务已提交')
      onClose()
      onSuccess()
    } catch {
      // 错误提示由请求层统一处理
    } finally {
      setSubmitting(false)
    }
  }

  return (
    <div>
      <div className="qvm-form-item">
        <div className="qvm-form-label">迁移方式</div>
        <Tag color={mode === 'live' ? 'orange' : 'grey'}>{mode === 'live' ? '热迁移' : '冷迁移'}</Tag>
        <span className="qvm-form-tip" style={{ marginLeft: 10 }}>
          当前状态：{optionsData?.source_state || vm.status || '-'}
        </span>
      </div>
      <div className="qvm-form-item">
        <div className="qvm-form-label required">目标存储</div>
        <Select
          style={{ width: '100%' }}
          value={targetStoragePoolId || undefined}
          onChange={(value) => setTargetStoragePoolId(value as string)}
          filter
          loading={loading}
          disabled={!optionsData}
          placeholder="请选择目标存储"
          optionList={storageTargets.map((item) => ({
            label: storageLabel(item),
            value: item.id,
            disabled: !item.enabled,
          }))}
        />
      </div>
      <div className="qvm-form-item">
        <div className="qvm-form-label required">目标硬盘</div>
        <Table
          rowKey="device"
          size="small"
          pagination={false}
          loading={loading}
          dataSource={disks}
          onRow={(record) => ({
            onClick: () => {
              if (record?.can_migrate) setDevice(record.device)
            },
          })}
          columns={[
            {
              title: '',
              dataIndex: 'device',
              width: 48,
              align: 'center',
              render: (text: string, record) => {
                const item = record as DiskOption | undefined
                return (
                  <input
                    type="radio"
                    checked={device === text}
                    disabled={!item?.can_migrate}
                    onChange={() => item?.can_migrate && setDevice(text)}
                  />
                )
              },
            },
            { title: '设备', dataIndex: 'device', width: 80 },
            {
              title: '容量',
              dataIndex: 'capacity_gb',
              width: 100,
              render: (text) => (text ? `${text} GB` : '-'),
            },
            { title: '格式', dataIndex: 'format', width: 90 },
            { title: '驱动', dataIndex: 'bus', width: 90 },
            { title: '当前路径', dataIndex: 'path', ellipsis: true },
            { title: 'backing', dataIndex: 'backing_path', ellipsis: true },
            {
              title: '状态',
              dataIndex: 'can_migrate',
              width: 110,
              render: (canMigrate: boolean) =>
                canMigrate ? <Tag color="green">可迁移</Tag> : <Tag color="red">不可迁移</Tag>,
            },
          ]}
          empty="暂无可迁移硬盘"
        />
        <div className="qvm-form-tip">
          运行中虚拟机会执行热迁移并在完成后切换硬盘路径；关机虚拟机会执行冷迁移并更新持久化 XML。
        </div>
      </div>

      {summary && <Banner type="info" closeIcon={null} style={{ margin: '12px 0' }} description={summary} />}
      {(optionsData?.warnings || []).map((item) => (
        <Banner key={item} type="warning" closeIcon={null} style={{ marginTop: 10 }} description={item} />
      ))}
      {selectedDisk?.block_reason && (
        <Banner
          type="danger"
          closeIcon={null}
          style={{ marginTop: 10 }}
          description={selectedDisk.block_reason}
        />
      )}

      <div style={{ display: 'flex', justifyContent: 'flex-end', marginTop: 16 }}>
        <Button theme="solid" disabled={!canSubmit} loading={submitting} onClick={() => void handleSubmit()}>
          提交硬盘迁移
        </Button>
      </div>
    </div>
  )
}
