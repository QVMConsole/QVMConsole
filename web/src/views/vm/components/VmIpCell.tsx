import { useCallback } from 'react'
import { Toast, Tooltip } from '@douyinfe/semi-ui'
import { IconCopy } from '@douyinfe/semi-icons'
import { copyTextWithFallback } from '@/utils/clipboard'
import type { VmListItem } from '@/api/vm'

interface VmIpCellProps {
  vm: VmListItem
}

function isIPv4(ip: string): boolean {
  return /^\d+\.\d+\.\d+\.\d+$/.test(ip)
}

function isIPv6(ip: string): boolean {
  return ip.includes(':')
}

export default function VmIpCell({ vm }: VmIpCellProps) {
  const ips = vm.ips?.filter(Boolean) || []
  const hasMultiple = ips.length > 1

  const copyIP = useCallback(async (ip: string, e: React.MouseEvent) => {
    e.stopPropagation()
    e.preventDefault()
    try {
      await copyTextWithFallback(ip)
      Toast.success('复制成功')
    } catch {
      Toast.warning('复制失败，请手动复制')
    }
  }, [])

  if (!vm.ip) {
    return <span className="qvm-ip-addr na">未分配</span>
  }

  if (!hasMultiple) {
    return <span className="qvm-ip-addr">{vm.ip}</span>
  }

  const firstIPv4 = ips.find(isIPv4)
  const firstIPv6 = ips.find(isIPv6)
  const displayParts: string[] = []
  if (firstIPv4) displayParts.push(firstIPv4)
  if (firstIPv6) displayParts.push(firstIPv6)
  const displayText = displayParts.join(' / ')

  return (
    <Tooltip
      position="top"
      content={
        <div style={{ maxWidth: 320, display: 'flex', flexDirection: 'column', gap: 6, fontSize: 12, lineHeight: 1.65 }}>
          {ips.map((ip) => (
            <div key={ip} style={{ display: 'inline-flex', alignItems: 'center', gap: 6 }}>
              <span>{ip}</span>
              <span
                style={{ cursor: 'pointer', display: 'inline-flex', alignItems: 'center', color: 'var(--semi-color-primary)' }}
                onClick={(e) => copyIP(ip, e)}
                role="button"
                tabIndex={0}
                onKeyDown={(e) => { if (e.key === 'Enter' || e.key === ' ') copyIP(ip, e as unknown as React.MouseEvent) }}
              >
                <IconCopy size="small" />
              </span>
            </div>
          ))}
        </div>
      }
      trigger="hover"
      mouseEnterDelay={200}
    >
      <span className="qvm-ip-addr" style={{ cursor: 'help' }}>
        {displayText}
      </span>
    </Tooltip>
  )
}