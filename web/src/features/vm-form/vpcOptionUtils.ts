/**
 * VPC 选项过滤工具。
 * 安全组必须跟随所选交换机的归属用户；系统基础网络则跟随 VM 归属用户。
 */
import type { VpcSecurityGroup, VpcSwitch } from '@/api/vpc'

export function resolveSecurityGroupOwnerForSwitch(
  sw: Pick<VpcSwitch, 'is_system' | 'username'> | null | undefined,
  fallbackUsername?: string,
): string {
  const fallback = (fallbackUsername || '').trim()
  const switchUsername = (sw?.username || '').trim()
  if (!sw) return fallback || switchUsername
  return sw.is_system ? fallback || switchUsername : switchUsername || fallback
}

export function filterSecurityGroupsForSwitch(
  groups: VpcSecurityGroup[],
  sw: Pick<VpcSwitch, 'is_system' | 'username'> | null | undefined,
  fallbackUsername?: string,
  vmName?: string,
): VpcSecurityGroup[] {
  const owner = resolveSecurityGroupOwnerForSwitch(sw, fallbackUsername)
  if (!owner) return groups
  const normalizedVmName = (vmName || '').trim()
  return groups.filter((group) => {
    const groupUsername = (group.username || '').trim()
    if (groupUsername !== owner) return false
    if (!group.is_vm_scoped) return true
    return !!normalizedVmName && (group.vm_name || '').trim() === normalizedVmName
  })
}

export function formatSecurityGroupOptionLabel(group: VpcSecurityGroup, showUsername = false): string {
  const name = group.is_default ? `${group.name}（默认）` : group.name
  if (showUsername && group.username) return `${group.username} / ${name}`
  return name
}
