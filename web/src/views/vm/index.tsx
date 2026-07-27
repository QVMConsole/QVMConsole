/**
 * 虚拟机列表页（深空极光版）
 * - SSE 常驻实时刷新（缓存优先，静默更新）
 * - 表格 / 卡片双视图，列头点击排序，客户端分页
 * - 状态与操作纯图标展示（悬停 Tooltip）
 * - 单机/批量电源操作、锁定/救援/导出/转独立、删除/备注/分组/制作模板/重装/迁移
 */
import { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import { useNavigate } from 'react-router-dom'
import { Button, Checkbox, Pagination, Toast, Tooltip } from '@douyinfe/semi-ui'
import { IconGridView, IconList, IconRefresh, IconAlertTriangle } from '@douyinfe/semi-icons'
import type { VmListItem, VmPowerAction } from '@/api/vm'
import { lockVm, makeVMIndependent, operateVm, rescueVm, unlockVm } from '@/api/vm'
import { exportVM } from '@/api/storage'
import { useUserStore } from '@/stores/user'
import { useVmStore } from '@/stores/vm'
import { useVmListSSE } from '@/hooks/useVmListSSE'
import { useMediaQuery } from '@/hooks/useMediaQuery'
import { confirmModal } from '@/utils/confirm'
import { CLOUD_TYPES, ROLES } from '@/config/constants'
import { isVmMigrating, POWER_ACTION_TEXT } from './utils'
import VmToolbar, { type BatchAction } from './components/VmToolbar'
import VmTableView, { type VmSortField, type VmSortOrder } from './components/VmTableView'
import VmCardView from './components/VmCardView'
import PendingRegistrations from './components/PendingRegistrations'
import type { VmMenuCommand } from './components/VmActionsCell'
import VmDeleteDialog from './dialogs/VmDeleteDialog'
import VmRemarkDialog from './dialogs/VmRemarkDialog'
import VmGroupDialog from './dialogs/VmGroupDialog'
import MakeTemplateDialog from './dialogs/MakeTemplateDialog'
import VmReinstallDialog from './dialogs/VmReinstallDialog'
import VmMigrationDialog from './dialogs/VmMigrationDialog'
import './vm.css'

/** 弹窗状态 */
type DialogState =
  | { type: 'delete'; vm?: VmListItem; batch?: VmListItem[] }
  | { type: 'remark'; vm: VmListItem }
  | { type: 'group'; vm: VmListItem }
  | { type: 'template'; vm: VmListItem }
  | { type: 'reinstall'; vm: VmListItem }
  | { type: 'migration'; vm: VmListItem }
  | null

const PAGE_SIZE = 20
const VIEW_MODE_KEY = 'vmListViewMode'

/** 排序字段文案 */
const SORT_FIELD_LABEL: Record<VmSortField, string> = {
  name: '名称',
  resource: '资源使用率',
  ip: 'IP 地址',
}

export default function VmListPage() {
  const role = useUserStore((s) => s.role)
  const cloudType = useUserStore((s) => s.cloudType)
  const security = useUserStore((s) => s.security)
  const setVmList = useVmStore((s) => s.setVmList)
  const isAdmin = role === ROLES.admin
  const isLightweight = !isAdmin && cloudType === CLOUD_TYPES.lightweight
  const maintenanceMode = !!security?.maintenance_mode

  const { list, sseStatus, loaded, reload } = useVmListSSE({ isAdmin })

  const [selectedKeys, setSelectedKeys] = useState<string[]>([])
  const [viewMode, setViewMode] = useState<'table' | 'card'>(() =>
    localStorage.getItem(VIEW_MODE_KEY) === 'card' ? 'card' : 'table',
  )
  const [sortField, setSortField] = useState<VmSortField>('name')
  const [sortOrder, setSortOrder] = useState<VmSortOrder>('ascend')
  const [page, setPage] = useState(1)
  const [operatingMap, setOperatingMap] = useState<Record<string, boolean>>({})
  const [batchOperating, setBatchOperating] = useState(false)
  const [refreshing, setRefreshing] = useState(false)
  const [dialog, setDialog] = useState<DialogState>(null)
  const compact = useMediaQuery('(max-width: 820px)')

  // ==================== 列表变化副作用 ====================
  const prevListRef = useRef<VmListItem[]>([])
  useEffect(() => {
    const prevByName = new Map(prevListRef.current.map((v) => [v.name, v]))
    const names = new Set(list.map((v) => v.name))
    // 状态变化 → 清除对应 VM 的操作 loading
    setOperatingMap((map) => {
      const next = { ...map }
      list.forEach((vm) => {
        const old = prevByName.get(vm.name)
        if (old && old.status !== vm.status) next[vm.name] = false
      })
      return next
    })
    // 清理已消失 VM 的选中态
    setSelectedKeys((keys) => keys.filter((k) => names.has(k)))
    prevListRef.current = list
  }, [list])

  // ==================== 排序与分页 ====================
  const sortedList = useMemo(() => {
    const dir = sortOrder === 'ascend' ? 1 : -1
    return [...list].sort((a, b) => {
      if (sortField === 'name') return a.name.localeCompare(b.name) * dir
      if (sortField === 'ip') return (a.ip || '').localeCompare(b.ip || '') * dir
      return ((a.cpu_percent ?? -1) - (b.cpu_percent ?? -1)) * dir
    })
  }, [list, sortField, sortOrder])

  const maxPage = Math.max(1, Math.ceil(sortedList.length / PAGE_SIZE))
  useEffect(() => {
    if (page > maxPage) setPage(maxPage)
  }, [page, maxPage])

  const pagedList = useMemo(
    () => sortedList.slice((page - 1) * PAGE_SIZE, page * PAGE_SIZE),
    [sortedList, page],
  )

  const handleSortChange = useCallback((field: VmSortField, order: VmSortOrder) => {
    setSortField(field)
    setSortOrder(order)
    setPage(1)
  }, [])

  const sortLabel = `${SORT_FIELD_LABEL[sortField]} · ${sortOrder === 'ascend' ? '升序' : '降序'}`

  // ==================== 选择 ====================
  const runningCount = useMemo(
    () => list.filter((v) => selectedKeys.includes(v.name) && v.status === 'running').length,
    [list, selectedKeys],
  )
  const allChecked = sortedList.length > 0 && selectedKeys.length >= sortedList.length
  const halfChecked = selectedKeys.length > 0 && !allChecked

  const toggleSelectAll = useCallback(
    (checked: boolean) => {
      setSelectedKeys(checked ? sortedList.map((v) => v.name) : [])
    },
    [sortedList],
  )

  const toggleSelectOne = useCallback((name: string, checked: boolean) => {
    setSelectedKeys((keys) => (checked ? [...new Set([...keys, name])] : keys.filter((k) => k !== name)))
  }, [])

  const selectedVms = useMemo(
    () => list.filter((v) => selectedKeys.includes(v.name)),
    [list, selectedKeys],
  )

  // ==================== 电源操作 ====================
  const handlePower = useCallback(async (vm: VmListItem, action: VmPowerAction) => {
    if (isVmMigrating(vm)) {
      Toast.warning('虚拟机正在迁移中，暂不能执行操作')
      return
    }
    const actionText =
      action === 'start' && vm.status === 'paused' ? '继续启动' : POWER_ACTION_TEXT[action]
    let ok: boolean
    if (vm.locked && (action === 'shutdown' || action === 'destroy')) {
      ok = await confirmModal({
        title: '虚拟机已锁定 - 二次确认',
        content: `虚拟机「${vm.name}」已锁定，该操作可能影响正在运行的服务，确定要继续执行${actionText}操作吗？`,
        okText: `确认${actionText}`,
        danger: true,
      })
    } else {
      ok = await confirmModal({
        title: '操作确认',
        content: `确定要对 ${vm.name} 执行${actionText}操作吗？`,
      })
    }
    if (!ok) return
    setOperatingMap((m) => ({ ...m, [vm.name]: true }))
    try {
      await operateVm(vm.name, action)
      Toast.success(`${actionText}指令已下发`)
    } catch {
      setOperatingMap((m) => ({ ...m, [vm.name]: false }))
    }
  }, [])

  // ==================== 批量操作 ====================
  const handleBatch = useCallback(
    async (action: BatchAction) => {
      const vms = selectedVms
      if (vms.length === 0) return
      if (vms.some((v) => v.status === 'migrating')) {
        Toast.warning('选中的虚拟机包含迁移中状态，请先取消选择后再操作')
        return
      }
      if (action === 'delete') {
        if (vms.some((v) => v.locked)) {
          const lockedNames = vms.filter((v) => v.locked).map((v) => v.name).join(', ')
          Toast.warning(`选中的虚拟机中包含已锁定的虚拟机（${lockedNames}），请先解锁后再删除`)
          return
        }
        setDialog({ type: 'delete', batch: vms })
        return
      }
      const actionText = POWER_ACTION_TEXT[action]
      if ((action === 'shutdown' || action === 'destroy') && vms.some((v) => v.locked)) {
        const lockedNames = vms.filter((v) => v.locked).map((v) => v.name).join(', ')
        const ok = await confirmModal({
          title: '虚拟机已锁定 - 批量操作二次确认',
          content: `选中的虚拟机中包含已锁定的虚拟机（${lockedNames}），对已锁定虚拟机执行${actionText}操作可能影响正在运行的服务，确定要继续吗？`,
          okText: '确认执行',
          danger: true,
        })
        if (!ok) return
      }
      const ok = await confirmModal({
        title: '批量操作提示',
        content: `确定要对选中的 ${vms.length} 台虚拟机执行${actionText}操作吗？`,
      })
      if (!ok) return
      setBatchOperating(true)
      try {
        const results = await Promise.allSettled(vms.map((vm) => operateVm(vm.name, action)))
        const successCount = results.filter((r) => r.status === 'fulfilled').length
        const failCount = results.length - successCount
        if (failCount === 0) {
          Toast.success(`批量${actionText}完成，成功 ${successCount} 台`)
        } else {
          Toast.warning(`批量${actionText}完成。成功: ${successCount}, 失败: ${failCount}`)
        }
        void reload()
      } finally {
        setBatchOperating(false)
      }
    },
    [selectedVms, reload],
  )

  // ==================== 更多菜单 ====================
  const handleMenu = useCallback(
    async (cmd: VmMenuCommand, vm: VmListItem) => {
      if (isVmMigrating(vm)) {
        Toast.warning('虚拟机正在迁移中，暂不能执行操作')
        return
      }
      switch (cmd) {
        case 'reboot':
        case 'destroy':
        case 'reset':
          void handlePower(vm, cmd)
          return
        case 'remark':
          setDialog({ type: 'remark', vm })
          return
        case 'group':
          setDialog({ type: 'group', vm })
          return
        case 'template':
          setDialog({ type: 'template', vm })
          return
        case 'reinstall':
          setDialog({ type: 'reinstall', vm })
          return
        case 'migrate':
          setDialog({ type: 'migration', vm })
          return
        case 'delete':
          setDialog({ type: 'delete', vm })
          return
        case 'export': {
          const quotaTip = isAdmin
            ? ''
            : '\n\n注意：导出的磁盘文件将占用您的存储配额。如果导出过程中配额不足，系统将自动中止导出并清理不完整的文件。'
          const ok = await confirmModal({
            title: '导出虚拟机',
            content: `确定要导出虚拟机「${vm.name}」的磁盘到我的存储吗？\n导出过程可能需要较长时间，请在任务中心查看进度。${quotaTip}`,
          })
          if (!ok) return
          try {
            const res = await exportVM({ vm_name: vm.name })
            Toast.success(res.message || '导出任务已提交')
          } catch {
            // 错误提示由请求层统一处理
          }
          return
        }
        case 'rescue': {
          const isStart = !vm.in_rescue
          const actionText = isStart ? '启动救援系统' : '关闭救援系统'
          const steps = isStart
            ? '1. 虚拟机将被强制关机\n2. 磁盘和网卡将切换为兼容模式\n3. 挂载救援ISO并重新开机\n4. 请在任务中心查看进度'
            : '1. 虚拟机将被强制关机\n2. 卸载救援ISO并恢复原始配置\n3. 虚拟机将重新开机\n4. 请在任务中心查看进度'
          const ok = await confirmModal({
            title: actionText,
            content: `确定要为虚拟机「${vm.name}」${actionText}吗？\n\n操作说明：\n${steps}`,
          })
          if (!ok) return
          try {
            const res = await rescueVm(vm.name, isStart ? 'start' : 'stop')
            Toast.success(res.message || `${actionText}任务已提交`)
          } catch {
            // 错误提示由请求层统一处理
          }
          return
        }
        case 'lock': {
          const locking = !vm.locked
          const ok = await confirmModal({
            title: locking ? '锁定虚拟机' : '解除锁定',
            content: locking
              ? `确定要锁定虚拟机「${vm.name}」吗？\n锁定后虚拟机将无法删除，关机需二次确认。`
              : `确定要解除虚拟机「${vm.name}」的锁定吗？\n解除锁定需要进行二次验证。`,
          })
          if (!ok) return
          try {
            if (locking) {
              await lockVm(vm.name)
              Toast.success('虚拟机已锁定')
            } else {
              await unlockVm(vm.name)
              Toast.success('虚拟机已解锁')
            }
            setVmList(list.map((v) => (v.name === vm.name ? { ...v, locked: locking } : v)))
          } catch {
            // 错误提示由请求层统一处理（解锁的 428 二次验证自动处理）
          }
          return
        }
        case 'make_independent': {
          const ok = await confirmModal({
            title: '转为独立虚拟机',
            content: `确定要将虚拟机「${vm.name}」转为独立虚拟机吗？\n\n操作说明：\n1. 虚拟机必须处于关机状态\n2. 将通过 qemu-img convert 将模板 backing chain 合并为独立磁盘镜像\n3. 操作完成后虚拟机将脱离链式克隆关系，不再依赖原始模板\n4. 此操作需要较长时间，请在任务中心查看进度`,
            okText: '确定转换',
          })
          if (!ok) return
          try {
            const res = await makeVMIndependent(vm.name)
            Toast.success(res.message || '转为独立虚拟机任务已提交')
            setVmList(list.map((v) => (v.name === vm.name ? { ...v, is_linked_clone: false } : v)))
          } catch {
            // 错误提示由请求层统一处理
          }
          return
        }
      }
    },
    [handlePower, isAdmin, list, setVmList],
  )

  // ==================== 其他入口 ====================
  const handleConsole = useCallback(() => {
    Toast.info({ content: '控制台（VNC）将在后续迭代提供', duration: 2 })
  }, [])

  /** 点击虚拟机名称跳转详情页 */
  const navigate = useNavigate()
  const handleOpenDetail = useCallback(
    (vm: VmListItem) => {
      navigate(`/vm/detail/${encodeURIComponent(vm.name)}`)
    },
    [navigate],
  )

  const handleCreate = useCallback(() => {
    Toast.info({ content: '新建虚拟机（VmForm）将在后续迭代提供', duration: 2 })
  }, [])

  const handleRefresh = useCallback(async () => {
    setRefreshing(true)
    try {
      await reload()
    } finally {
      setRefreshing(false)
    }
  }, [reload])

  const changeViewMode = (mode: 'table' | 'card') => {
    setViewMode(mode)
    localStorage.setItem(VIEW_MODE_KEY, mode)
  }

  // ==================== 弹窗回调 ====================
  const closeDialog = () => setDialog(null)
  const allGroups = useMemo(
    () => [...new Set(list.map((v) => v.group).filter(Boolean))] as string[],
    [list],
  )
  const handleRemarkSuccess = (name: string, remark: string) => {
    setVmList(list.map((v) => (v.name === name ? { ...v, remark } : v)))
  }
  const handleGroupSuccess = (name: string, group: string) => {
    setVmList(list.map((v) => (v.name === name ? { ...v, group } : v)))
  }
  const handleDeleteSuccess = () => {
    setSelectedKeys([])
    void reload()
  }

  // ==================== 渲染 ====================
  const pageContent = (
    <div className="qvm-vm-page">
      <VmToolbar
        selectedCount={selectedKeys.length}
        batchOperating={batchOperating}
        isLightweight={isLightweight}
        onBatch={(action) => void handleBatch(action)}
        onCreate={handleCreate}
        sortLabel={sortLabel}
      />

      {isLightweight && <PendingRegistrations onProvisioned={() => void reload()} />}

      <section
        className="qvm-vm-list-section qvm-g-border qvm-fade-up"
        style={{ '--qvm-delay': '120ms' } as React.CSSProperties}
      >
        <div className="qvm-vm-grid-header">
          <div className="qvm-grid-info">
            {!compact && (
              <Checkbox
                checked={allChecked}
                indeterminate={halfChecked}
                onChange={(e) => toggleSelectAll(!!e.target.checked)}
              />
            )}
            <span className="qvm-grid-total">共 {list.length} 台虚拟机</span>
            <span className="qvm-selected-count qvm-num">
              {selectedKeys.length} 台已选 · {runningCount} 台运行中
            </span>
            <Tooltip
              content={sseStatus === 'connected' ? '实时推送已连接' : '实时推送连接中…'}
              position="top"
            >
              <span className={`qvm-live-dot ${sseStatus === 'connected' ? 'on' : ''}`} />
            </Tooltip>
          </div>
          <div className="qvm-grid-ops">
            <Tooltip content="手动刷新" position="top">
              <span
                className={`qvm-grid-refresh ${refreshing ? 'spinning' : ''}`}
                onClick={() => void handleRefresh()}
              >
                <IconRefresh spin={refreshing} />
              </span>
            </Tooltip>
            <div className="qvm-view-toggle">
              <button
                className={`qvm-view-btn ${viewMode === 'table' ? 'active' : ''}`}
                onClick={() => changeViewMode('table')}
              >
                <IconList size="small" />
                表格
              </button>
              <button
                className={`qvm-view-btn ${viewMode === 'card' ? 'active' : ''}`}
                onClick={() => changeViewMode('card')}
              >
                <IconGridView size="small" />
                卡片
              </button>
            </div>
          </div>
        </div>

        {viewMode === 'table' ? (
          <VmTableView
            vms={pagedList}
            loading={!loaded}
            selectedKeys={selectedKeys}
            onSelectionChange={setSelectedKeys}
            sortField={sortField}
            sortOrder={sortOrder}
            onSortChange={handleSortChange}
            operatingMap={operatingMap}
            isAdmin={isAdmin}
            isLightweight={isLightweight}
            onPower={(vm, action) => void handlePower(vm, action)}
            onMenu={(cmd, vm) => void handleMenu(cmd, vm)}
            onConsole={handleConsole}
            onOpenDetail={handleOpenDetail}
            compact={compact}
          />
        ) : loaded ? (
          <VmCardView
            vms={pagedList}
            selectedKeys={selectedKeys}
            onToggleSelect={toggleSelectOne}
            operatingMap={operatingMap}
            isAdmin={isAdmin}
            isLightweight={isLightweight}
            onPower={(vm, action) => void handlePower(vm, action)}
            onMenu={(cmd, vm) => void handleMenu(cmd, vm)}
            onConsole={handleConsole}
            onOpenDetail={handleOpenDetail}
          />
        ) : (
          <div className="qvm-vm-skel-list">
            {Array.from({ length: 6 }).map((_, i) => (
              <div className="qvm-skel qvm-vm-skel-card" key={i} />
            ))}
          </div>
        )}

        {sortedList.length > PAGE_SIZE && (
          <div className="qvm-vm-pagination">
            <Pagination
              total={sortedList.length}
              currentPage={page}
              pageSize={PAGE_SIZE}
              onPageChange={setPage}
              showTotal
            />
          </div>
        )}
      </section>

      {/* ==================== 弹窗 ==================== */}
      <VmDeleteDialog
        visible={dialog?.type === 'delete'}
        vm={dialog?.type === 'delete' ? dialog.vm : undefined}
        batch={dialog?.type === 'delete' ? dialog.batch : undefined}
        isAdmin={isAdmin}
        onClose={closeDialog}
        onSuccess={handleDeleteSuccess}
      />
      {dialog?.type === 'remark' && (
        <VmRemarkDialog vm={dialog.vm} onClose={closeDialog} onSuccess={handleRemarkSuccess} />
      )}
      {dialog?.type === 'group' && (
        <VmGroupDialog
          vm={dialog.vm}
          groups={allGroups}
          onClose={closeDialog}
          onSuccess={handleGroupSuccess}
        />
      )}
      {dialog?.type === 'template' && (
        <MakeTemplateDialog vmName={dialog.vm.name} onClose={closeDialog} />
      )}
      {dialog?.type === 'reinstall' && (
        <VmReinstallDialog vm={dialog.vm} onClose={closeDialog} onSuccess={() => void reload()} />
      )}
      {dialog?.type === 'migration' && isAdmin && (
        <VmMigrationDialog vm={dialog.vm} onClose={closeDialog} onSuccess={() => void reload()} />
      )}
    </div>
  )

  // 维护模式：内容虚化 + 居中提示
  if (maintenanceMode) {
    return (
      <div className="qvm-maint-shell">
        <div className="qvm-maint-blur">{pageContent}</div>
        <div className="qvm-maint-mask">
          <div className="qvm-maint-card qvm-g-border">
            <IconAlertTriangle size="large" className="qvm-maint-ic" />
            <div className="qvm-maint-title">系统维护中</div>
            <div className="qvm-maint-desc">当前处于维护模式，虚拟机管理功能暂不可用，请稍后再试</div>
            <Button theme="solid" onClick={() => window.location.reload()}>
              刷新页面
            </Button>
          </div>
        </div>
      </div>
    )
  }

  return pageContent
}
