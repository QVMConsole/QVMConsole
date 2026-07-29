/**
 * 虚拟机列表顶部工具栏
 * - 左：批量电源下拉 + 新建虚拟机
 * - 中：搜索框（名称 / 备注 / 模板 / 标签）与标签筛选
 * - 右：当前排序依据指示
 */
import { Button, Dropdown, Input, Select } from '@douyinfe/semi-ui'
import {
  IconPlayCircle,
  IconRestart,
  IconDelete,
  IconPlus,
  IconChevronDown,
  IconSearch,
} from '@douyinfe/semi-icons'
import { PowerIcon } from './VmIcons'

export type BatchAction = 'start' | 'reboot' | 'shutdown' | 'destroy' | 'delete'

interface VmToolbarProps {
  selectedCount: number
  batchOperating: boolean
  isLightweight: boolean
  onBatch: (action: BatchAction) => void
  onCreate: () => void
  sortLabel: string
  searchText: string
  onSearchChange: (text: string) => void
  tagOptions: string[]
  tagFilter: string[]
  onTagFilterChange: (tags: string[]) => void
}

export default function VmToolbar({
  selectedCount,
  batchOperating,
  isLightweight,
  onBatch,
  onCreate,
  sortLabel,
  searchText,
  onSearchChange,
  tagOptions,
  tagFilter,
  onTagFilterChange,
}: VmToolbarProps) {
  const disabled = selectedCount === 0 || batchOperating

  return (
    <section className="qvm-vm-toolbar qvm-fade-up" style={{ '--qvm-delay': '60ms' } as React.CSSProperties}>
      <div className="qvm-vm-toolbar-left">
        <Dropdown
          trigger="click"
          position="bottomLeft"
          clickToHide
          menu={[
            { node: 'item', name: '开机', icon: <IconPlayCircle />, onClick: () => onBatch('start') },
            { node: 'item', name: '重启', icon: <IconRestart />, onClick: () => onBatch('reboot') },
            {
              node: 'item',
              name: '关机',
              icon: <PowerIcon size="small" />,
              onClick: () => onBatch('shutdown'),
            },
            {
              node: 'item',
              name: '强制断电',
              icon: <PowerIcon size="small" />,
              type: 'warning',
              onClick: () => onBatch('destroy'),
            },
            ...(isLightweight
              ? []
              : ([
                  { node: 'divider' },
                  {
                    node: 'item',
                    name: '删除',
                    icon: <IconDelete />,
                    type: 'danger',
                    onClick: () => onBatch('delete'),
                  },
                ] as const)),
          ]}
        >
          <Button
            className="qvm-action-btn"
            icon={<PowerIcon size="small" />}
            disabled={disabled}
            loading={batchOperating}
          >
            批量操作
            <IconChevronDown size="small" style={{ marginLeft: 2 }} />
          </Button>
        </Dropdown>
        <div className="qvm-bulk-divider" />
        {!isLightweight && (
          <Button className="qvm-btn-grad qvm-btn-new-sm" icon={<IconPlus />} onClick={onCreate}>
            新建虚拟机
          </Button>
        )}
      </div>
      <div className="qvm-vm-toolbar-search">
        <Input
          className="qvm-vm-search"
          prefix={<IconSearch />}
          placeholder="搜索名称 / 备注 / 模板 / 标签"
          showClear
          value={searchText}
          onChange={(value) => onSearchChange(value)}
        />
        <Select
          className="qvm-tag-filter"
          multiple
          filter
          showClear
          placeholder="筛选标签"
          value={tagFilter}
          onChange={(value) => onTagFilterChange((value as string[]) || [])}
          optionList={tagOptions.map((tag) => ({ label: tag, value: tag }))}
        />
      </div>
      <div className="qvm-vm-toolbar-right">
        <span className="qvm-current-sort">
          正在按<b>{sortLabel}</b>排序
        </span>
      </div>
    </section>
  )
}
