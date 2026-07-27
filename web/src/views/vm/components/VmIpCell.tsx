/**
 * IP 地址单元格（列表随SSE直接下发，无需额外请求）
 */
import type { VmListItem } from '@/api/vm'

interface VmIpCellProps {
  vm: VmListItem
}

export default function VmIpCell({ vm }: VmIpCellProps) {
  if (vm.ip) {
    return <span className="qvm-ip-addr">{vm.ip}</span>
  }
  return <span className="qvm-ip-addr na">未分配</span>
}
