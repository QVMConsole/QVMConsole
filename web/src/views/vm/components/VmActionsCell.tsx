/**
 * 虚拟机行操作区（纯图标 + 悬停 Tooltip）
 * - 控制台 / 电源 / 更多（下拉菜单）
 * - 更多菜单中的功能按角色与云类型裁剪
 */
import { Dropdown, Tooltip } from '@douyinfe/semi-ui'
import {
  IconTerminal,
  IconPlayCircle,
  IconRefresh,
  IconMore,
  IconRedo,
  IconRestart,
  IconEditStroked,
  IconFolder,
  IconTemplate,
  IconExport,
  IconRestore,
  IconSend,
  IconUnChainStroked,
  IconWrench,
  IconLock,
  IconUnlock,
  IconDelete,
} from '@douyinfe/semi-icons'
import type { VmListItem, VmPowerAction } from '@/api/vm'
import { PowerIcon } from './VmIcons'

export type VmMenuCommand =
  | 'reboot'
  | 'destroy'
  | 'reset'
  | 'remark'
  | 'group'
  | 'template'
  | 'export'
  | 'reinstall'
  | 'migrate'
  | 'make_independent'
  | 'rescue'
  | 'lock'
  | 'delete'

interface VmActionsCellProps {
  vm: VmListItem
  isAdmin: boolean
  isLightweight: boolean
  operating: boolean
  onPower: (vm: VmListItem, action: VmPowerAction) => void
  onMenu: (cmd: VmMenuCommand, vm: VmListItem) => void
  onConsole: (vm: VmListItem) => void
}

export default function VmActionsCell({
  vm,
  isAdmin,
  isLightweight,
  operating,
  onPower,
  onMenu,
  onConsole,
}: VmActionsCellProps) {
  const migrating = vm.status === 'migrating'
  const running = vm.status === 'running'
  const paused = vm.status === 'paused'

  // 电源按钮的图标/提示/动作
  const powerConfig = migrating
    ? { icon: <IconRefresh spin />, tip: '迁移中', action: null }
    : running
      ? {
          icon: <PowerIcon />,
          tip: vm.locked ? '虚拟机已锁定，关机需二次确认' : '关机',
          action: 'shutdown' as VmPowerAction,
        }
      : paused
        ? { icon: <IconPlayCircle />, tip: '继续启动', action: 'start' as VmPowerAction }
        : { icon: <IconPlayCircle />, tip: '开机', action: 'start' as VmPowerAction }

  return (
    <div className="qvm-act-cell" onClick={(event) => event.stopPropagation()}>
      <Tooltip content="控制台" position="top">
        <span
          className={`qvm-act-ic vnc ${running ? '' : 'disabled'}`}
          onClick={() => running && onConsole(vm)}
        >
          <IconTerminal />
        </span>
      </Tooltip>
      <Tooltip content={powerConfig.tip} position="top">
        <span
          className={`qvm-act-ic power ${running ? 'warn' : 'go'} ${migrating || operating ? 'disabled' : ''}`}
          onClick={() => {
            if (!migrating && !operating && powerConfig.action) {
              onPower(vm, powerConfig.action)
            }
          }}
        >
          {operating ? <IconRefresh spin /> : powerConfig.icon}
        </span>
      </Tooltip>
      <Dropdown
        trigger="click"
        position="bottomRight"
        clickToHide
        render={
          <Dropdown.Menu>
            {paused && (
              <Dropdown.Item icon={<IconRedo />} onClick={() => onMenu('reset', vm)}>
                重置
              </Dropdown.Item>
            )}
            {running && (
              <Dropdown.Item icon={<IconRestart />} onClick={() => onMenu('reboot', vm)}>
                重启
              </Dropdown.Item>
            )}
            {(running || paused) && (
              <Dropdown.Item
                icon={<PowerIcon size="small" />}
                type="warning"
                onClick={() => onMenu('destroy', vm)}
              >
                强制断电
              </Dropdown.Item>
            )}
            {(running || paused) && <Dropdown.Divider />}
            {!isLightweight && (
              <Dropdown.Item icon={<IconEditStroked />} onClick={() => onMenu('remark', vm)}>
                编辑备注
              </Dropdown.Item>
            )}
            {!isLightweight && (
              <Dropdown.Item icon={<IconFolder />} onClick={() => onMenu('group', vm)}>
                编辑分组
              </Dropdown.Item>
            )}
            {isAdmin && (
              <Dropdown.Item icon={<IconTemplate />} onClick={() => onMenu('template', vm)}>
                制作模板
              </Dropdown.Item>
            )}
            {!isLightweight && (
              <Dropdown.Item icon={<IconExport />} onClick={() => onMenu('export', vm)}>
                导出虚拟机
              </Dropdown.Item>
            )}
            {!isLightweight && (
              <Dropdown.Item icon={<IconRestore />} onClick={() => onMenu('reinstall', vm)}>
                重装系统
              </Dropdown.Item>
            )}
            {isAdmin && (
              <Dropdown.Item icon={<IconSend />} onClick={() => onMenu('migrate', vm)}>
                迁移
              </Dropdown.Item>
            )}
            {isAdmin && vm.is_linked_clone && (
              <Dropdown.Item
                icon={<IconUnChainStroked />}
                disabled={vm.status !== 'shut off'}
                onClick={() => onMenu('make_independent', vm)}
              >
                转为独立虚拟机
              </Dropdown.Item>
            )}
            <Dropdown.Divider />
            <Dropdown.Item
              icon={<IconWrench />}
              type={vm.in_rescue ? 'warning' : 'tertiary'}
              onClick={() => onMenu('rescue', vm)}
            >
              {vm.in_rescue ? '关闭救援系统' : '启动救援系统'}
            </Dropdown.Item>
            {!isLightweight && (
              <Dropdown.Item
                icon={vm.locked ? <IconUnlock /> : <IconLock />}
                type={vm.locked ? 'warning' : 'tertiary'}
                onClick={() => onMenu('lock', vm)}
              >
                {vm.locked ? '解除锁定' : '锁定虚拟机'}
              </Dropdown.Item>
            )}
            {!isLightweight && <Dropdown.Divider />}
            {!isLightweight && (
              <Dropdown.Item
                icon={<IconDelete />}
                type="danger"
                disabled={vm.locked}
                onClick={() => onMenu('delete', vm)}
              >
                删除
              </Dropdown.Item>
            )}
          </Dropdown.Menu>
        }
      >
        <span className={`qvm-act-ic more ${migrating ? 'disabled' : ''}`}>
          <IconMore />
        </span>
      </Dropdown>
    </div>
  )
}
