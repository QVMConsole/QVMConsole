/**
 * Hero 状态卡片（详情页左栏）
 * - 状态图标 + 状态文案 + 连续运行时长
 * - 电源操作（开机/继续启动/重启/关机/强制断电/重置）
 * - 快捷操作（锁定/救援模式/重装系统/编辑备注）
 * - 迁移中状态展示提示条，禁用全部操作
 */
import { Banner, Button, Popconfirm, Tag } from '@douyinfe/semi-ui'
import {
  IconLock,
  IconUnlock,
  IconWrench,
  IconRestore,
  IconPlayCircle,
  IconPause,
  IconStop,
  IconRefresh,
  IconEditStroked,
  IconDesktop,
} from '@douyinfe/semi-icons'
import type { VmDetailInfo, VmPowerAction } from '@/api/vm'
import { vmStatusText, formatContinuousRuntime } from '../utils'

interface HeroStatusCardProps {
  vm: VmDetailInfo | null
  operating: boolean
  isLightweight: boolean
  onPower: (action: VmPowerAction) => void
  onLock: (action: 'lock' | 'unlock') => void
  onRescue: () => void
  onReinstall: () => void
  onRemark: () => void
}

/** 状态对应的图标与色彩 */
function statusVisual(status: string) {
  if (status === 'running') return { icon: <IconPlayCircle size="extra-large" />, cls: 'running' }
  if (status === 'paused') return { icon: <IconPause size="extra-large" />, cls: 'paused' }
  if (status === 'migrating') return { icon: <IconRefresh spin size="extra-large" />, cls: 'migrating' }
  return { icon: <IconStop size="extra-large" />, cls: 'stopped' }
}

export default function HeroStatusCard({
  vm,
  operating,
  isLightweight,
  onPower,
  onLock,
  onRescue,
  onReinstall,
  onRemark,
}: HeroStatusCardProps) {
  const status = vm?.status || ''
  const visual = statusVisual(status)
  const running = status === 'running'
  const paused = status === 'paused'
  const migrating = status === 'migrating'
  const locked = !!vm?.locked

  return (
    <div className={`qvm-hero-card qvm-hero-status ${visual.cls}`}>
      <div className={`qvm-hero-status-icon ${visual.cls}`}>
        {running && <span className="qvm-status-pulse" />}
        {vm ? visual.icon : <IconDesktop size="extra-large" />}
      </div>
      <div className="qvm-hero-status-text">
        {vm ? vmStatusText(status) : '加载中…'}
        {locked && (
          <Tag size="small" color="orange" className="qvm-hero-lock-tag">
            <IconLock size="small" /> 已锁定
          </Tag>
        )}
        {vm?.in_rescue && (
          <Tag size="small" color="blue" className="qvm-hero-lock-tag">
            救援系统中
          </Tag>
        )}
      </div>
      <div className="qvm-hero-status-sub">
        {vm ? `已稳定运行 ${formatContinuousRuntime(vm.continuous_runtime_seconds, status)}` : '正在获取虚拟机状态'}
      </div>

      {migrating ? (
        <Banner
          type="warning"
          closeIcon={null}
          description="虚拟机正在迁移中，暂不能执行电源、快照、救援或密码重置等操作"
          className="qvm-hero-migrate-banner"
        />
      ) : (
        <>
          {/* 电源操作 */}
          <div className="qvm-hero-power-group">
            {!running ? (
              <>
                <Popconfirm
                  title={paused ? '确定要继续启动吗？' : '确定要开机吗？'}
                  onConfirm={() => onPower('start')}
                >
                  <Button
                    type="primary"
                    theme="solid"
                    icon={<IconPlayCircle />}
                    loading={operating}
                    block
                  >
                    {paused ? '继续启动' : '开机'}
                  </Button>
                </Popconfirm>
                {paused && (
                  <Popconfirm
                    title="确定要重置虚拟机吗？相当于硬重启，适用于无法继续启动的暂停状态。"
                    onConfirm={() => onPower('reset')}
                  >
                    <Button type="danger" icon={<IconRefresh />} loading={operating} block>
                      重置
                    </Button>
                  </Popconfirm>
                )}
              </>
            ) : (
              <>
                <Popconfirm title="确定要重启吗？" onConfirm={() => onPower('reboot')}>
                  <Button type="warning" theme="solid" icon={<IconRefresh />} loading={operating}>
                    重启
                  </Button>
                </Popconfirm>
                <Popconfirm
                  title={locked ? '虚拟机已锁定，关机可能影响正在运行的服务，确定要关机吗？' : '确定要关机吗？'}
                  onConfirm={() => onPower('shutdown')}
                >
                  <Button type="warning" theme="light" icon={<IconStop />} loading={operating}>
                    关机
                  </Button>
                </Popconfirm>
                <Popconfirm
                  title={locked ? '虚拟机已锁定，强制断电可能影响正在运行的服务，确定要断电吗？' : '确定要强制断电吗？'}
                  onConfirm={() => onPower('destroy')}
                >
                  <Button type="danger" theme="light" icon={<IconStop />} loading={operating}>
                    强制断电
                  </Button>
                </Popconfirm>
              </>
            )}
          </div>

          {/* 快捷操作 */}
          <div className="qvm-hero-actions-grid">
            {!isLightweight &&
              (locked ? (
                <Popconfirm title="解除锁定需要进行二次验证，确定要解锁吗？" onConfirm={() => onLock('unlock')}>
                  <span className="qvm-action-item lock">
                    <IconUnlock size="small" />
                    解除锁定
                  </span>
                </Popconfirm>
              ) : (
                <Popconfirm
                  title="锁定后虚拟机将无法删除，关机需二次确认，确定要锁定吗？"
                  onConfirm={() => onLock('lock')}
                >
                  <span className="qvm-action-item lock">
                    <IconLock size="small" />
                    锁定虚拟机
                  </span>
                </Popconfirm>
              ))}
            <Popconfirm
              title={
                vm?.in_rescue
                  ? '关闭救援系统需要重启虚拟机，确定要关闭吗？'
                  : '启动救援系统需要重启虚拟机并挂载救援镜像，确定要开启吗？'
              }
              onConfirm={onRescue}
            >
              <span className="qvm-action-item rescue">
                <IconWrench size="small" />
                {vm?.in_rescue ? '关闭救援' : '救援模式'}
              </span>
            </Popconfirm>
            {!isLightweight && (
              <span className="qvm-action-item reinstall" onClick={onReinstall}>
                <IconRestore size="small" />
                重装系统
              </span>
            )}
            {!isLightweight && (
              <span className="qvm-action-item remark" onClick={onRemark}>
                <IconEditStroked size="small" />
                编辑备注
              </span>
            )}
          </div>
        </>
      )}
    </div>
  )
}
