/**
 * 虚拟机卡片视图
 * 每张卡片：头部（图标 + 名称 + 操作图标）/ 规格 / 资源条 / 信息行
 */
import { Checkbox, Empty, Tooltip } from '@douyinfe/semi-ui'
import { IllustrationNoContent, IllustrationNoContentDark } from '@douyinfe/semi-illustrations'
import { IconDesktop, IconLock, IconWrench } from '@douyinfe/semi-icons'
import type { VmListItem, VmPowerAction } from '@/api/vm'
import { formatRuntime } from '@/utils/format'
import { vmConfigText } from '../utils'
import VmStatusIcon from './VmStatusIcon'
import VmResourceBars from './VmResourceBars'
import VmIpCell from './VmIpCell'
import VmActionsCell, { type VmMenuCommand } from './VmActionsCell'
import VmTagsEditor from './VmTagsEditor'
import { shouldOpenVmDetail } from '../utils'

interface VmCardViewProps {
  vms: VmListItem[]
  selectedKeys: string[]
  onToggleSelect: (name: string, checked: boolean) => void
  operatingMap: Record<string, boolean>
  isAdmin: boolean
  isLightweight: boolean
  onPower: (vm: VmListItem, action: VmPowerAction) => void
  onMenu: (cmd: VmMenuCommand, vm: VmListItem) => void
  onConsole: (vm: VmListItem) => void
  onTagsSave: (vm: VmListItem, tags: string[]) => Promise<void>
  /** 点击虚拟机卡片跳转详情页 */
  onOpenDetail: (vm: VmListItem) => void
}

export default function VmCardView({
  vms,
  selectedKeys,
  onToggleSelect,
  operatingMap,
  isAdmin,
  isLightweight,
  onPower,
  onMenu,
  onConsole,
  onTagsSave,
  onOpenDetail,
}: VmCardViewProps) {
  if (vms.length === 0) {
    return (
      <div className="qvm-vm-cards-empty">
        <Empty
          image={<IllustrationNoContent />}
          darkModeImage={<IllustrationNoContentDark />}
          title="暂无虚拟机"
          description="点击右上角「新建虚拟机」开始创建"
        />
      </div>
    )
  }

  return (
    <div className="qvm-vm-cards">
      {vms.map((vm, index) => (
        <div
          key={vm.name}
          className={`qvm-vcard qvm-vcard-clickable qvm-fade-up ${selectedKeys.includes(vm.name) ? 'selected' : ''}`}
          style={{ '--qvm-delay': `${Math.min(index, 12) * 40}ms` } as React.CSSProperties}
          onClick={(event) => {
            if (shouldOpenVmDetail(event.target)) onOpenDetail(vm)
          }}
        >
          <div className="qvm-vcard-head">
            <Checkbox
              checked={selectedKeys.includes(vm.name)}
              onChange={(e) => onToggleSelect(vm.name, !!e.target.checked)}
              className="qvm-vcard-chk"
            />
            <div className={`qvm-vm-ic ${vm.status === 'running' ? '' : 'off'}`}>
              <IconDesktop size="small" />
            </div>
            <div className="qvm-vcard-title">
              <span className="qvm-vm-name-text" title={vm.name}>
                {vm.name}
              </span>
              <span className="qvm-vcard-badges">
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
                <VmStatusIcon status={vm.status} />
              </span>
            </div>
            <VmActionsCell
              vm={vm}
              isAdmin={isAdmin}
              isLightweight={isLightweight}
              operating={!!operatingMap[vm.name]}
              onPower={onPower}
              onMenu={onMenu}
              onConsole={onConsole}
            />
          </div>

          <div className="qvm-vcard-specs">
            <span className="qvm-num">{vmConfigText(vm)}</span>
            {vm.group && <span className="qvm-vm-group-tag">{vm.group}</span>}
          </div>

          <div className="qvm-vcard-tags">
            <span className="qvm-vcard-label">标签</span>
            <VmTagsEditor vm={vm} onSave={onTagsSave} />
          </div>

          <VmResourceBars vm={vm} />

          <div className="qvm-vcard-info">
            <div className="qvm-vcard-row">
              <span className="qvm-vcard-label">IP 地址</span>
              <VmIpCell vm={vm} />
            </div>
            <div className="qvm-vcard-row">
              <span className="qvm-vcard-label">模板</span>
              <span className="qvm-vcard-value" title={vm.template || ''}>
                {vm.template || '-'}
              </span>
            </div>
            <div className="qvm-vcard-row">
              <span className="qvm-vcard-label">运行时长</span>
              <span className="qvm-vcard-value">
                {vm.status === 'running' || vm.status === 'paused'
                  ? formatRuntime(vm.continuous_runtime_seconds)
                  : '—'}
              </span>
            </div>
          </div>
        </div>
      ))}
    </div>
  )
}
