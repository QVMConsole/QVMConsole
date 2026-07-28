/**
 * 存储池管理页（仅管理员）
 * - 管理宿主机硬盘分区：配置显示名称/启用、设为默认落盘位置、格式化挂载
 * - 分区管理：创建分区、清除磁盘；LVM 存储卷：创建/删除卷组
 * - 概览统计 + 容量分布/对比图表 + 磁盘卡片树（VG 卡分 PV/LV 展示）
 * - 格式化/分区/存储卷操作为高风险项，提交任务队列异步执行（428 二次验证由请求层处理）
 */
import { useCallback, useEffect, useMemo, useState } from 'react'
import { Banner, Button, Spin, Toast } from '@douyinfe/semi-ui'
import { IconPlus, IconRefresh, IconServer } from '@douyinfe/semi-icons'
import type { HostStoragePoolInfo } from '@/api/storagePool'
import { getStoragePoolList, setDefaultStoragePool } from '@/api/storagePool'
import { useUserStore } from '@/stores/user'
import { confirmModal } from '@/utils/confirm'
import { ROLES } from '@/config/constants'
import { computeVGStats, type DiskCategory, getDiskCategory } from './utils'
import OverviewCards from './components/OverviewCards'
import DiskCard, { type DiskCardHandlers } from './components/DiskCard'
import ConfigDialog from './dialogs/ConfigDialog'
import FormatDialog from './dialogs/FormatDialog'
import CreatePartitionDialog from './dialogs/CreatePartitionDialog'
import ClearDiskDialog from './dialogs/ClearDiskDialog'
import CreateVolumeDialog from './dialogs/CreateVolumeDialog'
import DeleteVolumeDialog from './dialogs/DeleteVolumeDialog'
import './storage-pool.css'

/** 弹窗状态 */
type DialogState =
  | { type: 'config'; row: HostStoragePoolInfo }
  | { type: 'format'; row: HostStoragePoolInfo }
  | { type: 'partition'; row: HostStoragePoolInfo }
  | { type: 'clear'; row: HostStoragePoolInfo }
  | { type: 'createVolume' }
  | { type: 'deleteVolume'; row: HostStoragePoolInfo }
  | null

export default function StoragePoolPage() {
  const role = useUserStore((s) => s.role)
  const isAdmin = role === ROLES.admin

  const [tableData, setTableData] = useState<HostStoragePoolInfo[]>([])
  const [loading, setLoading] = useState(false)
  const [loaded, setLoaded] = useState(false)
  const [currentTab, setCurrentTab] = useState<DiskCategory>('all')
  const [collapsedIds, setCollapsedIds] = useState<Record<string, boolean>>({})
  const [dialog, setDialog] = useState<DialogState>(null)

  // ==================== 数据加载 ====================
  const fetchData = useCallback(async () => {
    setLoading(true)
    try {
      const res = await getStoragePoolList()
      const data = res.data || []
      setTableData(data)
      // 自动折叠没有分区的磁盘（与旧版一致）
      const collapsed: Record<string, boolean> = {}
      for (const disk of data) {
        if (!disk.children || disk.children.length === 0) collapsed[disk.id] = true
      }
      setCollapsedIds(collapsed)
      setLoaded(true)
    } catch {
      // 错误提示由请求层统一处理
    } finally {
      setLoading(false)
    }
  }, [])

  useEffect(() => {
    if (isAdmin) void fetchData()
  }, [isAdmin, fetchData])

  /** 任务提交后延迟刷新（等待任务队列受理） */
  const refreshAfterTask = useCallback(() => {
    window.setTimeout(() => void fetchData(), 1200)
  }, [fetchData])

  // ==================== 筛选与折叠 ====================
  const filteredData = useMemo(
    () => tableData.filter((disk) => currentTab === 'all' || getDiskCategory(disk) === currentTab),
    [tableData, currentTab],
  )

  const handleToggle = useCallback((id: string) => {
    setCollapsedIds((prev) => ({ ...prev, [id]: !prev[id] }))
  }, [])

  // ==================== 设为默认 ====================
  const handleSetDefault = useCallback(
    async (row: HostStoragePoolInfo) => {
      const ok = await confirmModal({
        title: '设为默认存储位置',
        content: `确定将 ${row.display_name} 设为默认虚拟机存储位置吗？`,
        okText: '确定',
      })
      if (!ok) return
      try {
        await setDefaultStoragePool(row.id)
        Toast.success('已设为默认存储位置')
        void fetchData()
      } catch {
        // 错误提示由请求层统一处理
      }
    },
    [fetchData],
  )

  // ==================== 操作分发 ====================
  const handlers: DiskCardHandlers = useMemo(
    () => ({
      onConfig: (row) => setDialog({ type: 'config', row }),
      onSetDefault: (row) => void handleSetDefault(row),
      onFormat: (row) => setDialog({ type: 'format', row }),
      onCreatePartition: (row) => setDialog({ type: 'partition', row }),
      onClearDisk: (row) => setDialog({ type: 'clear', row }),
      onDeleteVolume: (row) => setDialog({ type: 'deleteVolume', row }),
    }),
    [handleSetDefault],
  )

  // Tab 分类计数
  const tabCounts = useMemo(() => {
    const counts: Record<DiskCategory, number> = { all: 0, pending: 0, inuse: 0, vg: 0, other: 0 }
    for (const disk of tableData) {
      const cat = getDiskCategory(disk)
      counts[cat]++
      counts.all++
    }
    return counts
  }, [tableData])

  // VG 统计（用于第 4 张卡）
  const vgStats = useMemo(() => computeVGStats(tableData), [tableData])

  // ==================== 渲染 ====================
  if (!isAdmin) {
    return (
      <div className="sp-page">
        <div className="sp-empty">
          <div className="sp-empty-icon">
            <IconServer />
          </div>
          <div>存储池管理仅对管理员开放</div>
        </div>
      </div>
    )
  }

  return (
    <div className="sp-page">
      {/* ==================== 页头 ==================== */}
      <div className="sp-page-header qvm-fade-up">
        <div>
          <h2>
            <IconServer style={{ marginRight: 8, color: 'var(--qvm-acc-ink)' }} />
            存储池
          </h2>
          <p className="sp-page-sub">管理宿主机硬盘分区，配置虚拟机落盘位置与格式化挂载</p>
        </div>
        <div className="sp-header-actions">
          <Button
            type="primary"
            theme="light"
            icon={<IconPlus />}
            onClick={() => setDialog({ type: 'createVolume' })}
          >
            创建存储卷
          </Button>
          <Button icon={<IconRefresh />} loading={loading} onClick={() => void fetchData()}>
            刷新
          </Button>
        </div>
      </div>

      <Banner
        type="warning"
        closeIcon={null}
        className="qvm-fade-up"
        description="格式化挂载、分区与存储卷操作会触发高风险验证，并通过任务队列异步执行，可在任务中心查看进度。"
        style={{ marginBottom: 14 }}
      />

      {/* ==================== 概览统计 ==================== */}
      <OverviewCards pools={tableData} vgStats={vgStats} />

      {/* ==================== 状态筛选 Tab ==================== */}
      <div className="sp-tab-row qvm-fade-up">
        <button className={`sp-tab ${currentTab === 'all' ? 'active' : ''}`} onClick={() => setCurrentTab('all')}>
          全部 <span className="sp-tab-count">{tabCounts.all}</span>
        </button>
        <button className={`sp-tab ${currentTab === 'pending' ? 'active' : ''}`} onClick={() => setCurrentTab('pending')}>
          待初始化 <span className="sp-tab-count warn">{tabCounts.pending}</span>
        </button>
        <button className={`sp-tab ${currentTab === 'inuse' ? 'active' : ''}`} onClick={() => setCurrentTab('inuse')}>
          使用中 <span className="sp-tab-count">{tabCounts.inuse}</span>
        </button>
        <button className={`sp-tab ${currentTab === 'vg' ? 'active' : ''}`} onClick={() => setCurrentTab('vg')}>
          存储卷 <span className="sp-tab-count">{tabCounts.vg}</span>
        </button>
        <div className="sp-tab-right">
          <span style={{ fontSize: 12, color: 'var(--qvm-text-2)' }}>按磁盘状态分组，快速定位可操作的硬盘</span>
        </div>
      </div>

      {/* ==================== 磁盘卡片列表 ==================== */}
      <Spin spinning={loading && !loaded} size="large" style={{ display: 'block' }}>
        <div className="sp-disk-list">
          {filteredData.map((disk, idx) => (
            <div
              key={disk.id}
              className="qvm-fade-up"
              style={{ '--qvm-delay': `${Math.min(idx, 6) * 60 + 320}ms` } as React.CSSProperties}
            >
              <DiskCard
                disk={disk}
                collapsed={!!collapsedIds[disk.id]}
                onToggle={handleToggle}
                handlers={handlers}
              />
            </div>
          ))}
        </div>

        {loaded && filteredData.length === 0 && (
          <div className="sp-empty">
            <svg width="44" height="44" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.5" strokeLinecap="round" strokeLinejoin="round">
              <rect x="3" y="4" width="18" height="7" rx="2"/><rect x="3" y="13" width="18" height="7" rx="2"/>
              <path d="M7 7.5h.01M7 16.5h.01"/>
            </svg>
            <div>当前筛选条件下没有存储设备</div>
          </div>
        )}
      </Spin>

      {/* ==================== 弹窗 ==================== */}
      {dialog?.type === 'config' && (
        <ConfigDialog row={dialog.row} onClose={() => setDialog(null)} onSaved={() => void fetchData()} />
      )}
      {dialog?.type === 'format' && (
        <FormatDialog row={dialog.row} onClose={() => setDialog(null)} onSubmitted={refreshAfterTask} />
      )}
      {dialog?.type === 'partition' && (
        <CreatePartitionDialog row={dialog.row} onClose={() => setDialog(null)} onSubmitted={refreshAfterTask} />
      )}
      {dialog?.type === 'clear' && (
        <ClearDiskDialog row={dialog.row} onClose={() => setDialog(null)} onSubmitted={refreshAfterTask} />
      )}
      {dialog?.type === 'createVolume' && (
        <CreateVolumeDialog onClose={() => setDialog(null)} onSubmitted={refreshAfterTask} />
      )}
      {dialog?.type === 'deleteVolume' && (
        <DeleteVolumeDialog row={dialog.row} onClose={() => setDialog(null)} onSubmitted={refreshAfterTask} />
      )}
    </div>
  )
}
