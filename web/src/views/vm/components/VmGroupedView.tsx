/**
 * 虚拟机分组视图（按状态 / 按模板 / 自定义分组）
 * - 组头：折叠箭头 + 分组标签 + 数量 + 组内全选（支持跨组多选）
 * - 组内容：复用表格 / 卡片视图渲染
 */
import { Checkbox, Empty, Tag } from '@douyinfe/semi-ui'
import { IllustrationNoContent, IllustrationNoContentDark } from '@douyinfe/semi-illustrations'
import { IconChevronDown } from '@douyinfe/semi-icons'
import type { VmListItem, VmPowerAction } from '@/api/vm'
import type { VmGroupBucket } from '../utils'
import VmTableView, { type VmSortField, type VmSortOrder } from './VmTableView'
import VmCardView from './VmCardView'
import type { VmMenuCommand } from './VmActionsCell'

interface VmGroupedViewProps {
  groups: VmGroupBucket[]
  viewMode: 'table' | 'card'
  /** 判断某组是否展开（默认展开） */
  isExpanded: (key: string) => boolean
  onToggleExpand: (key: string) => void
  selectedKeys: string[]
  onSelectionChange: (keys: string[]) => void
  onToggleSelect: (name: string, checked: boolean) => void
  /** 组内全选 / 取消全选 */
  onSelectGroup: (group: VmGroupBucket, checked: boolean) => void
  sortField: VmSortField
  sortOrder: VmSortOrder
  onSortChange: (field: VmSortField, order: VmSortOrder) => void
  operatingMap: Record<string, VmPowerAction | undefined>
  shutdownPendingMap: Record<string, boolean | undefined>
  isAdmin: boolean
  isLightweight: boolean
  onPower: (vm: VmListItem, action: VmPowerAction) => void
  onMenu: (cmd: VmMenuCommand, vm: VmListItem) => void
  onConsole: (vm: VmListItem) => void
  onTagsSave: (vm: VmListItem, tags: string[]) => Promise<void>
  onOpenDetail: (vm: VmListItem) => void
  compact: boolean
}

export default function VmGroupedView({
  groups,
  viewMode,
  isExpanded,
  onToggleExpand,
  selectedKeys,
  onSelectionChange,
  onToggleSelect,
  onSelectGroup,
  sortField,
  sortOrder,
  onSortChange,
  operatingMap,
  shutdownPendingMap,
  isAdmin,
  isLightweight,
  onPower,
  onMenu,
  onConsole,
  onTagsSave,
  onOpenDetail,
  compact,
}: VmGroupedViewProps) {
  if (groups.length === 0) {
    return (
      <div className="qvm-vm-cards-empty">
        <Empty
          image={<IllustrationNoContent />}
          darkModeImage={<IllustrationNoContentDark />}
          title="暂无虚拟机"
          description="没有匹配的虚拟机，试试调整搜索或分组条件"
        />
      </div>
    )
  }

  return (
    <div className="qvm-grouped-view">
      {groups.map((group) => {
        const expanded = isExpanded(group.key)
        const selectedInGroup = group.vms.filter((vm) => selectedKeys.includes(vm.name)).length
        const allSelected = group.vms.length > 0 && selectedInGroup === group.vms.length
        const halfSelected = selectedInGroup > 0 && !allSelected
        return (
          <div className={`qvm-group-section ${expanded ? '' : 'collapsed'}`} key={group.key}>
            <div className="qvm-group-header" onClick={() => onToggleExpand(group.key)}>
              <span className={`qvm-group-arrow ${expanded ? 'open' : ''}`}>
                <IconChevronDown size="small" />
              </span>
              <Tag color={group.color} type="light" className="qvm-group-tag">
                {group.label}
              </Tag>
              <span className="qvm-group-count qvm-num">{group.vms.length} 台</span>
              <span className="qvm-group-flex" />
              <span className="qvm-group-check" onClick={(e) => e.stopPropagation()}>
                <Checkbox
                  checked={allSelected}
                  indeterminate={halfSelected}
                  onChange={(e) => onSelectGroup(group, !!e.target.checked)}
                >
                  全选本组
                </Checkbox>
              </span>
            </div>
            {expanded && (
              <div className="qvm-group-body">
                {viewMode === 'table' ? (
                  <VmTableView
                    vms={group.vms}
                    loading={false}
                    selectedKeys={selectedKeys}
                    onSelectionChange={onSelectionChange}
                    sortField={sortField}
                    sortOrder={sortOrder}
                    onSortChange={onSortChange}
                    operatingMap={operatingMap}
                    shutdownPendingMap={shutdownPendingMap}
                    isAdmin={isAdmin}
                    isLightweight={isLightweight}
                    onPower={onPower}
                    onMenu={onMenu}
                    onConsole={onConsole}
                    onTagsSave={onTagsSave}
                    onOpenDetail={onOpenDetail}
                    compact={compact}
                  />
                ) : (
                  <VmCardView
                    vms={group.vms}
                    selectedKeys={selectedKeys}
                    onToggleSelect={onToggleSelect}
                    operatingMap={operatingMap}
                    shutdownPendingMap={shutdownPendingMap}
                    isAdmin={isAdmin}
                    isLightweight={isLightweight}
                    onPower={onPower}
                    onMenu={onMenu}
                    onConsole={onConsole}
                    onTagsSave={onTagsSave}
                    onOpenDetail={onOpenDetail}
                  />
                )}
              </div>
            )}
          </div>
        )
      })}
    </div>
  )
}
