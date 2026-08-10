/**
 * 快照管理 Tab
 * - 快照列表（名称/类型/状态/描述/创建时间/操作）
 * - 创建快照（描述 + 内存快照 + 暂停方式选择）
 * - 恢复 / 删除 / 删除全部（外部快照与含子快照的特殊提示）
 * - UEFI NVRAM 修复流程（require_nvram_fix 错误自动处理）
 */
import { useCallback, useEffect, useMemo, useState } from 'react'
import {
  Button,
  Checkbox,
  Modal,
  Radio,
  Table,
  Tag,
  TextArea,
  Toast,
} from '@douyinfe/semi-ui'
import { IconPlus, IconRefresh, IconDelete } from '@douyinfe/semi-icons'
import type { ColumnProps } from '@douyinfe/semi-ui/lib/es/table'
import type { SnapshotItem, SnapshotQuota, VmDetailInfo } from '@/api/vm'
import {
  createSnapshot,
  deleteAllSnapshots,
  deleteSnapshot,
  getSnapshots,
  revertSnapshot,
} from '@/api/vm'
import { confirmModal } from '@/utils/confirm'

interface SnapshotTabProps {
  vm: VmDetailInfo | null
  live: boolean
  liveTick: number
  onQuotaChange?: (quota: SnapshotQuota | null) => void
}

/** 快照状态文案 */
function stateLabel(state: string): string {
  const map: Record<string, string> = {
    running: '运行中',
    shutoff: '关机',
    'disk-snapshot': '仅磁盘',
    paused: '暂停',
  }
  return map[state] || state
}

export default function SnapshotTab({ vm, live, liveTick, onQuotaChange }: SnapshotTabProps) {
  const vmName = vm?.name || ''
  const vmIsRunning = vm?.status === 'running'

  const [loading, setLoading] = useState(false)
  const [list, setList] = useState<SnapshotItem[]>([])
  const [quota, setQuota] = useState<SnapshotQuota | null>(null)
  const [createVisible, setCreateVisible] = useState(false)
  const [submitting, setSubmitting] = useState(false)
  const [description, setDescription] = useState('')
  const [includeMemory, setIncludeMemory] = useState(false)
  const [pauseForMemory, setPauseForMemory] = useState(true)

  const quotaReached = !!quota && quota.max_snapshots > 0 && (quota.used_snapshots || 0) >= quota.max_snapshots
  const quotaText = useMemo(() => {
    if (!quota) return ''
    const used = quota.used_snapshots || 0
    const max = quota.max_snapshots || 0
    return max > 0 ? `快照配额：${used} / ${max}` : `快照配额：已用 ${used} / 不限`
  }, [quota])

  const fetchData = useCallback(async (silent = false) => {
    if (!vmName) return
    if (!silent) setLoading(true)
    try {
      const res = await getSnapshots(vmName)
      setList(res.data || [])
      const q = res.quota || null
      setQuota(q)
      onQuotaChange?.(q)
    } catch {
      if (!silent) {
        setQuota(null)
        onQuotaChange?.(null)
      }
    } finally {
      if (!silent) setLoading(false)
    }
  }, [vmName, onQuotaChange])

  useEffect(() => {
    if (live) void fetchData(liveTick > 0)
  }, [fetchData, live, liveTick])

  // ============ 创建 ============
  const openCreate = () => {
    if (quotaReached) {
      Toast.warning('当前快照数量已达到配额上限，请先删除旧快照或联系管理员调整配额')
      return
    }
    setDescription('')
    setIncludeMemory(vmIsRunning)
    setPauseForMemory(true)
    setCreateVisible(true)
  }

  /** 提交创建（含 NVRAM 修复分支） */
  const submitCreate = async (autoFixNvram = false) => {
    const payload = {
      description,
      include_memory: vmIsRunning ? includeMemory : false,
      pause_for_memory_snapshot: includeMemory ? pauseForMemory : true,
      ...(autoFixNvram ? { auto_fix_nvram: true } : {}),
    }
    try {
      await createSnapshot(vmName, payload)
      Toast.success('快照创建任务已提交，可在任务中心查看进度')
      setCreateVisible(false)
    } catch (err) {
      // NVRAM 修复分支：后端返回 require_nvram_fix 时引导自动修复
      const respData = (err as { response?: { data?: { data?: { require_nvram_fix?: boolean }; message?: string } } })
        ?.response?.data
      if (respData?.data?.require_nvram_fix) {
        const ok = await confirmModal({
          title: '修复 UEFI NVRAM',
          content:
            respData.message ||
            '当前虚拟机需要先修复 UEFI NVRAM 才能创建内存快照。是否立即正常关机、修复并重新开机后继续创建快照？',
          okText: '立即修复并创建',
        })
        if (ok) {
          await submitCreate(true)
        }
        return
      }
      throw err
    }
  }

  const handleSubmitCreate = async () => {
    setSubmitting(true)
    try {
      await submitCreate()
    } catch {
      // 错误提示由请求层统一处理
    } finally {
      setSubmitting(false)
    }
  }

  // ============ 恢复 ============
  const handleRestore = async (row: SnapshotItem) => {
    const isExternal = row.location === 'external' || row.state === 'disk-snapshot'
    const message = isExternal
      ? `快照 [${row.name}] 是外部快照，恢复时将：\n1. 关闭虚拟机\n2. 切回快照创建时的磁盘状态\n3. 重新启动虚拟机\n\n恢复流程不会合并或删除快照后的增量文件，也不会自动删除快照记录。\n确定要继续吗？`
      : `确定要恢复到快照 [${row.name}] 吗？当前未保存的数据将丢失。`
    const ok = await confirmModal({ title: '恢复快照', content: message, okText: '确定恢复' })
    if (!ok) return
    try {
      await revertSnapshot(vmName, row.name)
      Toast.success('快照恢复任务已提交，可在任务中心查看进度')
    } catch {
      // 请求层已提示
    }
  }

  // ============ 删除 ============
  const handleDelete = async (row: SnapshotItem) => {
    if ((row.children || 0) > 0) {
      Toast.warning(
        `快照 [${row.name}] 还有 ${row.children} 个子快照，不能直接删除父级快照。请先从快照树最末端的子快照开始处理。`,
      )
      return
    }
    const isExternal = row.location === 'external' || row.state === 'disk-snapshot'
    const message = isExternal
      ? `快照 [${row.name}] 是外部快照，删除时将合并增量数据到原始磁盘并清理 overlay 文件。确定要删除吗？`
      : `确定要删除快照 [${row.name}] 吗？`
    const ok = await confirmModal({ title: '删除快照', content: message })
    if (!ok) return
    try {
      await deleteSnapshot(vmName, row.name)
      Toast.success('快照删除任务已提交，可在任务中心查看进度')
    } catch {
      // 请求层已提示
    }
  }

  const handleDeleteAll = async () => {
    if (list.length === 0) {
      Toast.info('当前虚拟机没有快照')
      return
    }
    const ok = await confirmModal({
      title: '删除全部快照',
      content: `确定要删除当前虚拟机的全部 ${list.length} 个快照吗？\n\n系统会按快照树从末端开始删除；外部快照会尽量合并并切回当前磁盘状态。该操作不会回滚虚拟机，但会清空快照记录。`,
      okText: '删除全部',
      danger: true,
    })
    if (!ok) return
    try {
      await deleteAllSnapshots(vmName)
      Toast.success('全部快照删除任务已提交，可在任务中心查看进度')
    } catch {
      // 请求层已提示
    }
  }

  const columns: ColumnProps<SnapshotItem>[] = [
    { title: '名称', dataIndex: 'name', ellipsis: true },
    { title: '创建时间', dataIndex: 'created_at', width: 170 },
    {
      title: '类型',
      dataIndex: 'location',
      width: 100,
      render: (_text, row) =>
        row.location === 'external' || row.state === 'disk-snapshot' ? (
          <Tag size="small" color="orange">外部快照</Tag>
        ) : (
          <Tag size="small" color="green">内部快照</Tag>
        ),
    },
    {
      title: '状态',
      dataIndex: 'state',
      width: 90,
      render: (text) => stateLabel(String(text || '')),
    },
    { title: '描述', dataIndex: 'description', ellipsis: true },
    {
      title: '操作',
      dataIndex: 'actions',
      width: 220,
      render: (_text, row) => (
        <div className="qvm-snap-actions">
          {row.is_current && (
            <Tag size="small" color="green" className="qvm-snap-current-tag">当前</Tag>
          )}
          <Button size="small" type="warning" theme="light" onClick={() => void handleRestore(row)}>
            恢复
          </Button>
          <Button size="small" type="danger" theme="light" onClick={() => void handleDelete(row)}>
            删除
          </Button>
        </div>
      ),
    },
  ]

  return (
    <div className="qvm-snapshot-tab">
      <div className="qvm-tab-toolbar">
        <div className="qvm-tab-toolbar-left">
          <Button type="primary" icon={<IconPlus />} disabled={quotaReached} onClick={openCreate}>
            创建快照
          </Button>
          <Button icon={<IconRefresh />} onClick={() => void fetchData()} loading={loading}>
            刷新
          </Button>
          <Button
            type="danger"
            theme="light"
            icon={<IconDelete />}
            disabled={list.length === 0}
            onClick={() => void handleDeleteAll()}
          >
            删除全部快照
          </Button>
        </div>
        {quota && (
          <Tag color={quotaReached ? 'red' : 'blue'} size="large">
            {quotaText}
          </Tag>
        )}
      </div>

      <Table<SnapshotItem>
        rowKey="name"
        columns={columns}
        dataSource={list}
        loading={loading}
        pagination={false}
        size="middle"
        empty="暂无快照"
      />

      {/* 新建快照对话框 */}
      <Modal
        title="新建快照"
        visible={createVisible}
        onCancel={() => setCreateVisible(false)}
        onOk={() => void handleSubmitCreate()}
        okText="确定"
        cancelText="取消"
        confirmLoading={submitting}
        width={520}
        closeOnEsc
      >
        <div className="qvm-form-item">
          <div className="qvm-form-label">描述</div>
          <TextArea
            value={description}
            onChange={setDescription}
            rows={3}
            placeholder="记录该快照的用途，便于后续识别"
          />
        </div>
        {vmIsRunning && (
          <div className="qvm-form-item">
            <Checkbox checked={includeMemory} onChange={(e) => setIncludeMemory(!!e.target.checked)}>
              创建快照时保存虚拟机内存状态
            </Checkbox>
            <div className="qvm-form-hint">
              勾选后将创建包含内存的内部快照，恢复时虚拟机将回到运行状态；不勾选则创建仅磁盘的外部快照。
              <br />
              <span className="qvm-warn-text">内存快照耗时取决于虚拟机内存大小，大内存虚拟机可能需要数分钟。</span>
            </div>
          </div>
        )}
        {vmIsRunning && includeMemory && (
          <div className="qvm-form-item">
            <div className="qvm-form-label">创建方式</div>
            <Radio.Group
              type="button"
              value={pauseForMemory}
              onChange={(e) => setPauseForMemory(!!e.target.value)}
            >
              <Radio value={true}>暂停后创建（推荐）</Radio>
              <Radio value={false}>不主动暂停（实验）</Radio>
            </Radio.Group>
            <div className="qvm-form-hint">
              {pauseForMemory
                ? '面板会先暂停虚拟机，快照写入完成后自动恢复运行，一致性更稳但期间业务会停顿。'
                : '面板不主动暂停虚拟机，但 QEMU 保存内存状态时虚拟机仍会自动进入 paused (saving) 状态，这是虚拟化层的固有机制。'}
            </div>
          </div>
        )}
      </Modal>
    </div>
  )
}
