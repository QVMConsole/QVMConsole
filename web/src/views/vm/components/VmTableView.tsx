/**
 * 虚拟机表格视图（Semi Table）
 * - 名称 / 配置 / IP 三列点击表头排序（受控）
 * - 状态与操作列纯图标展示，悬停 Tooltip
 * - 小屏隐藏次要列（模板 / IP / 运行时长 / 勾选列）
 */
import { useMemo } from 'react'
import { Empty, Table, Tooltip } from '@douyinfe/semi-ui'
import { IllustrationNoContent, IllustrationNoContentDark } from '@douyinfe/semi-illustrations'
import type { ColumnProps } from '@douyinfe/semi-ui/lib/es/table'
import { IconDesktop, IconLock, IconWrench } from '@douyinfe/semi-icons'
import type { VmListItem, VmPowerAction } from '@/api/vm'
import { formatRuntime } from '@/utils/format'
import VmStatusIcon from './VmStatusIcon'
import VmResourceBars from './VmResourceBars'
import VmIpCell from './VmIpCell'
import VmActionsCell, { type VmMenuCommand } from './VmActionsCell'
import VmTagsEditor from './VmTagsEditor'
import { shouldOpenVmDetail } from '../utils'

export type VmSortField = 'name' | 'resource' | 'ip'
export type VmSortOrder = 'ascend' | 'descend'

interface VmTableViewProps {
  vms: VmListItem[]
  loading: boolean
  selectedKeys: string[]
  onSelectionChange: (keys: string[]) => void
  sortField: VmSortField
  sortOrder: VmSortOrder
  onSortChange: (field: VmSortField, order: VmSortOrder) => void
  operatingMap: Record<string, boolean>
  isAdmin: boolean
  isLightweight: boolean
  onPower: (vm: VmListItem, action: VmPowerAction) => void
  onMenu: (cmd: VmMenuCommand, vm: VmListItem) => void
  onConsole: (vm: VmListItem) => void
  onTagsSave: (vm: VmListItem, tags: string[]) => Promise<void>
  /** 点击虚拟机列表项跳转详情页 */
  onOpenDetail: (vm: VmListItem) => void
  /** 小屏模式：隐藏次要列与勾选列 */
  compact: boolean
}

/** 列排序字段映射（dataIndex ↔ 排序状态字段） */
const SORT_FIELD_BY_INDEX: Record<string, VmSortField> = {
  name: 'name',
  cpu_percent: 'resource',
  ip: 'ip',
}

export default function VmTableView({
  vms,
  loading,
  selectedKeys,
  onSelectionChange,
  sortField,
  sortOrder,
  onSortChange,
  operatingMap,
  isAdmin,
  isLightweight,
  onPower,
  onMenu,
  onConsole,
  onTagsSave,
  onOpenDetail,
  compact,
}: VmTableViewProps) {
  const columns = useMemo<ColumnProps<VmListItem>[]>(() => {
    const sortState = (field: VmSortField) => (sortField === field ? sortOrder : false)
    return [
      {
        title: '名称',
        dataIndex: 'name',
        sorter: true,
        sortOrder: sortState('name'),
        render: (_text, vm) => (
          <div className="qvm-vm-cell">
            <div className={`qvm-vm-ic ${vm.status === 'running' ? '' : 'off'}`}>
              <IconDesktop size="small" />
            </div>
            <span className="qvm-vm-name-text" title={vm.remark || undefined}>
              {vm.name}
            </span>
            {vm.locked && (
              <Tooltip content="已锁定" position="top">
                <IconLock size="small" className="qvm-vm-badge lock" />
              </Tooltip>
            )}
            {vm.in_rescue && (
              <Tooltip content="救援系统中" position="top">
                <IconWrench size="small" className="qvm-vm-badge rescue" />
              </Tooltip>
            )}
            {vm.group && <span className="qvm-vm-group-tag">{vm.group}</span>}
          </div>
        ),
      },
      {
        title: '状态',
        dataIndex: 'status',
        width: 64,
        align: 'center',
        render: (_text, vm) => <VmStatusIcon status={vm.status} />,
      },
      {
        title: '模板',
        dataIndex: 'template',
        className: 'col-hide-md',
        onHeaderCell: () => ({ className: 'col-hide-md' }),
        ellipsis: true,
        render: (text) => <span className="qvm-tpl-name">{text || '-'}</span>,
      },
      {
        title: '标签',
        dataIndex: 'tags',
        width: 250,
        render: (_text, vm) => <VmTagsEditor vm={vm} onSave={onTagsSave} />,
      },
      {
        title: '配置 (资源使用)',
        dataIndex: 'cpu_percent',
        sorter: true,
        sortOrder: sortState('resource'),
        width: 230,
        render: (_text, vm) => <VmResourceBars vm={vm} />,
      },
      {
        title: 'IP 地址',
        dataIndex: 'ip',
        sorter: true,
        sortOrder: sortState('ip'),
        width: 140,
        className: 'col-hide-sm',
        onHeaderCell: () => ({ className: 'col-hide-sm' }),
        render: (_text, vm) => <VmIpCell vm={vm} />,
      },
      {
        title: '运行时长',
        dataIndex: 'continuous_runtime_seconds',
        width: 120,
        className: 'col-hide-sm',
        onHeaderCell: () => ({ className: 'col-hide-sm' }),
        render: (_text, vm) => {
          const text =
            vm.status === 'running' || vm.status === 'paused'
              ? formatRuntime(vm.continuous_runtime_seconds)
              : '—'
          if (vm.continuous_running_since && text !== '—') {
            return (
              <Tooltip content={`开始时间：${vm.continuous_running_since}`} position="top">
                <span className="qvm-runtime">{text}</span>
              </Tooltip>
            )
          }
          return <span className="qvm-runtime">{text}</span>
        },
      },
      {
        title: '操作',
        dataIndex: 'actions',
        width: 120,
        render: (_text, vm) => (
          <VmActionsCell
            vm={vm}
            isAdmin={isAdmin}
            isLightweight={isLightweight}
            operating={!!operatingMap[vm.name]}
            onPower={onPower}
            onMenu={onMenu}
            onConsole={onConsole}
          />
        ),
      },
    ]
  }, [
    sortField,
    sortOrder,
    operatingMap,
    isAdmin,
    isLightweight,
    onPower,
    onMenu,
    onConsole,
    onTagsSave,
  ])

  return (
    <div className="qvm-vm-table-wrap">
      <Table<VmListItem>
        rowKey="name"
        className="qvm-vm-table"
        columns={columns}
        dataSource={vms}
        loading={loading}
        pagination={false}
        size="middle"
        onRow={(vm) => ({
          className: 'qvm-vm-table-row-clickable',
          onClick: (event) => {
            if (vm && shouldOpenVmDetail(event.target)) onOpenDetail(vm)
          },
        })}
        rowSelection={
          compact
            ? undefined
            : {
                selectedRowKeys: selectedKeys,
                onChange: (keys) => onSelectionChange((keys || []) as string[]),
              }
        }
        onChange={({ sorter }) => {
          const field = SORT_FIELD_BY_INDEX[(sorter?.dataIndex as string) || '']
          if (field) {
            // Semi 排序循环含第三态 false（取消排序），本页始终保留排序，映射回升序
            const order = sorter?.sortOrder === 'descend' ? 'descend' : 'ascend'
            onSortChange(field, order)
          }
        }}
        empty={
          <Empty
            image={<IllustrationNoContent />}
            darkModeImage={<IllustrationNoContentDark />}
            title="暂无虚拟机"
            description="点击右上角「新建虚拟机」开始创建"
          />
        }
      />
    </div>
  )
}
