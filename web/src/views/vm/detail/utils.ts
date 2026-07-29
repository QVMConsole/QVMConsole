/**
 * 虚拟机详情页共享工具（状态文案 / 格式化）
 */
import type { VmDetailInfo, VmListItem } from '@/api/vm'

/** 将详情数据适配为列表项结构（用于复用列表页的备注/重装等弹窗） */
export function detailToListItem(vm: VmDetailInfo): VmListItem {
  return {
    name: vm.name,
    remark: vm.remark || '',
    group: vm.group || '',
    tags: vm.tags || [],
    status: vm.status,
    vcpu: vm.vcpu,
    memory: vm.memory,
    max_memory: vm.max_memory || vm.memory,
    ip: vm.ip || '',
    disk_size: vm.disk_size || '',
    template: vm.template || '',
    network: vm.network || '',
    autostart: !!vm.autostart,
    cpu_percent: vm.cpu_percent ?? 0,
    mem_percent: vm.mem_percent ?? 0,
    locked: !!vm.locked,
    in_rescue: !!vm.in_rescue,
    is_linked_clone: !!vm.is_linked_clone,
    continuous_runtime_seconds: vm.continuous_runtime_seconds || 0,
    continuous_running_since: vm.continuous_running_since || '',
    created_at: vm.created_at || '',
  }
}

/** 判断当前详情数据是否允许进入密码重置流程。 */
export function canResetVmPassword(vm: VmDetailInfo | null): boolean {
  if (!vm || !['linux', 'windows', 'fnos'].includes((vm.os_type || '').toLowerCase())) {
    return false
  }
  const status = (vm.status || '').trim().toLowerCase()
  if (status === 'shut off' || status === 'shutoff') return true
  return status === 'running' && !!vm.guest_agent_status?.connected
}

/** 状态文案映射 */
export function vmStatusText(status: string): string {
  const map: Record<string, string> = {
    running: '运行中',
    'shut off': '已关机',
    paused: '已暂停',
    migrating: '迁移中',
  }
  return map[status] || status || '-'
}

/** 状态对应的页面标签圆点 */
export function vmStatusDot(status: string): 'run' | 'warn' | 'off' {
  if (status === 'running') return 'run'
  if (status === 'paused' || status === 'migrating') return 'warn'
  return 'off'
}

/** 内存格式化（MB → GB） */
export function formatMemoryMB(mem: number): string {
  if (!mem) return '-'
  return mem >= 1024 ? `${(mem / 1024).toFixed(1)} GB` : `${mem} MB`
}

/** 流量速率格式化（B/s 自动换算） */
export function formatTrafficRate(bytesPerSec?: number | null): string {
  if (bytesPerSec == null || bytesPerSec < 0) return '0 B/s'
  const units = ['B/s', 'KB/s', 'MB/s', 'GB/s', 'TB/s']
  let val = Number(bytesPerSec)
  let idx = 0
  while (val >= 1024 && idx < units.length - 1) {
    val /= 1024
    idx += 1
  }
  return `${val.toFixed(1)} ${units[idx]}`
}

/** IOPS 格式化 */
export function formatIOPS(opsPerSec?: number | null): string {
  if (opsPerSec == null || opsPerSec < 0) return '0 IOPS'
  if (opsPerSec >= 1000) return `${(opsPerSec / 1000).toFixed(1)}K IOPS`
  return `${opsPerSec.toFixed(0)} IOPS`
}

/** 连续运行时长格式化 */
export function formatContinuousRuntime(seconds: number, status: string): string {
  const normalized = Number.isFinite(seconds) ? Math.max(0, Math.floor(seconds)) : 0
  if (normalized <= 0) {
    return status === 'running' || status === 'paused' ? '不足 1 分钟' : '-'
  }
  const days = Math.floor(normalized / 86400)
  const hours = Math.floor((normalized % 86400) / 3600)
  const minutes = Math.floor((normalized % 3600) / 60)
  const remainSeconds = normalized % 60
  if (days > 0) return `${days} 天 ${hours} 小时 ${minutes} 分钟`
  if (hours > 0) return `${hours} 小时 ${minutes} 分钟`
  if (minutes > 0) return `${minutes} 分钟 ${remainSeconds} 秒`
  return `${remainSeconds} 秒`
}

/** IP 来源标签文案 */
export function ipSourceLabel(source?: string): string {
  const map: Record<string, string> = {
    guest_agent: 'Guest Agent',
    arp: 'ARP',
    ovs_dhcp: 'OVS DHCP',
    vpc_dhcp: 'VPC DHCP',
    libvirt_lease: 'libvirt 租约',
    static: '静态绑定',
  }
  return map[source || ''] || source || '-'
}

/** IP 来源标签颜色（Semi Tag color） */
export function ipSourceTagColor(source?: string): 'green' | 'orange' | 'grey' {
  if (source === 'guest_agent') return 'green'
  if (source === 'static') return 'orange'
  return 'grey'
}

/** 打开独立 VNC 窗口（居中） */
export function openVncWindow(vmName: string) {
  const url = `/vm/${encodeURIComponent(vmName)}/vnc-window`
  const width = Math.min(1280, window.screen.availWidth - 100)
  const height = Math.min(800, window.screen.availHeight - 100)
  const left = Math.round((window.screen.availWidth - width) / 2)
  const top = Math.round((window.screen.availHeight - height) / 2)
  window.open(
    url,
    `vnc_${vmName}`,
    `width=${width},height=${height},left=${left},top=${top},menubar=no,toolbar=no,location=no,status=no,resizable=yes,scrollbars=no`,
  )
}
