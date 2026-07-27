/**
 * 虚拟机配置 + 实时资源进度条（CPU 青色渐变 / 内存紫色渐变）
 * 运行中显示动态百分比；非运行状态显示灰色空条与状态标注
 */
import type { VmListItem } from '@/api/vm'
import { vmConfigText } from '../utils'

interface VmResourceBarsProps {
  vm: VmListItem
}

export default function VmResourceBars({ vm }: VmResourceBarsProps) {
  const running = vm.status === 'running'
  const usable = running && vm.cpu_percent >= 0 && vm.mem_percent >= 0
  const cpu = usable ? Math.min(vm.cpu_percent, 100) : 0
  const mem = usable ? Math.min(vm.mem_percent, 100) : 0

  return (
    <div className="qvm-res-cell">
      <div className="qvm-res-config">{vmConfigText(vm)}</div>
      {usable ? (
        <>
          <div className="qvm-res-bar">
            <div className="qvm-res-track">
              <div className="qvm-res-fill cpu qvm-bar-anim" style={{ width: `${cpu}%` }} />
            </div>
            <span className="qvm-res-val">CPU {vm.cpu_percent.toFixed(0)}%</span>
          </div>
          <div className="qvm-res-bar">
            <div className="qvm-res-track">
              <div className="qvm-res-fill mem qvm-bar-anim" style={{ width: `${mem}%` }} />
            </div>
            <span className="qvm-res-val">MEM {vm.mem_percent.toFixed(0)}%</span>
          </div>
        </>
      ) : (
        <div className="qvm-res-bar">
          <div className="qvm-res-track">
            <div className="qvm-res-fill stopped" style={{ width: '0%' }} />
          </div>
          <span className="qvm-res-val stopped">
            {vm.status === 'migrating' ? '迁移中' : vm.status === 'paused' ? '已暂停' : '已停止'}
          </span>
        </div>
      )}
    </div>
  )
}
