/**
 * 编辑 Tab：虚拟机硬件配置编辑
 * 复用 features/vm-form 共享表单（与创建向导同一套模型/规则/分区组件），
 * 差异快照提交，仅发送发生变化的字段。
 */
import type { VmDetailInfo } from '@/api/vm'
import EditVmForm from '@/features/vm-form/EditVmForm'

interface EditTabProps {
  vm: VmDetailInfo
  live: boolean
  liveTick: number
}

export default function EditTab({ vm, live, liveTick }: EditTabProps) {
  return <EditVmForm vm={vm} live={live} liveTick={liveTick} />
}
