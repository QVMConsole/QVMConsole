/**
 * 轻量云专用 VPC 下拉选项工具
 * 创建用户 / 编辑配置 / 注册 VM 弹窗共用
 */
import { vpcSwitchModeDetail, type VpcSwitch } from '@/api/vpc'

/** VPC 下拉选项文案：用户 / 名称 (CIDR) */
export function vpcOptionLabel(item: VpcSwitch): string {
  const owner = item.username ? `${item.username} / ` : ''
  return `${owner}${item.name}（${vpcSwitchModeDetail(item)}）`
}

/** 过滤出 NAT 模式的 VPC 交换机（轻量云专用 VPC 仅支持 NAT） */
export function filterNatVpcSwitches(switches: VpcSwitch[]): VpcSwitch[] {
  return switches.filter((item) => !item.bridge_mode || item.bridge_mode === 'nat')
}
