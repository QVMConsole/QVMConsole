/**
 * 存储与网络 Tab：存储路径 / 网络设置 / 全局带宽限制 / 默认磁盘 IOPS
 */
import { useState } from 'react'
import { Banner, Button, Input, Toast } from '@douyinfe/semi-ui'
import { IconBranch, IconFolder, IconPulse, IconSafeStroked, IconSetting } from '@douyinfe/semi-icons'
import { getUserStorageISOPath } from '@/api/settings'
import { SectionHead, SettingRow } from './SettingRow'
import NumField from './NumField'
import type { SettingsTabProps } from '../types'

export default function StorageNetworkTab({ form, patch }: SettingsTabProps) {
  const [isoPathLoading, setIsoPathLoading] = useState(false)

  // 一键替换 ISO 存放位置为当前用户存储 ISO 目录
  const handleUseUserStorageISO = async () => {
    setIsoPathLoading(true)
    try {
      const res = await getUserStorageISOPath()
      const isoPath = res.data?.iso_path
      if (isoPath) {
        patch({ iso_dir: isoPath })
      } else {
        Toast.error('获取存储 ISO 目录失败，请确保已开通存储池')
      }
    } catch {
      Toast.error('获取存储 ISO 目录失败，请确保已开通存储池')
    } finally {
      setIsoPathLoading(false)
    }
  }

  return (
    <div className="stg-tab-pane">
      <SectionHead icon={<IconFolder />} title="存储路径" />

      <SettingRow label="模板目录" tip="环境变量: KVM_TEMPLATE_DIR">
        <Input
          value={form.template_dir}
          onChange={(v) => patch({ template_dir: v })}
          placeholder="/var/lib/libvirt/images/templates"
        />
      </SettingRow>

      <SettingRow
        label="模板导入临时目录"
        tip="建议与模板目录放在同一磁盘，避免导入大模板时占满 /tmp | 环境变量: KVM_TEMPLATE_IMPORT_DIR"
      >
        <Input
          value={form.template_import_dir}
          onChange={(v) => patch({ template_import_dir: v })}
          placeholder="/var/lib/libvirt/images/templates/_imports"
        />
      </SettingRow>

      <SettingRow
        label="模板导出目录"
        tip="建议与模板目录放在同一磁盘，避免导出大模板时占满 /tmp | 环境变量: KVM_TEMPLATE_EXPORT_DIR"
      >
        <Input
          value={form.template_export_dir}
          onChange={(v) => patch({ template_export_dir: v })}
          placeholder="/var/lib/libvirt/images/templates/_exports"
        />
      </SettingRow>

      <SettingRow
        label="虚拟机包临时目录"
        tip="用于 OVF/OVA 安全解包、磁盘转换和封装；任务开始前会检查可用空间 | 环境变量: KVM_APPLIANCE_TEMP_DIR"
      >
        <Input
          value={form.appliance_temp_dir}
          onChange={(v) => patch({ appliance_temp_dir: v })}
          placeholder="/var/lib/libvirt/images/templates/_appliance"
        />
      </SettingRow>

      <SettingRow label="克隆磁盘目录" tip="环境变量: KVM_CLONE_DIR">
        <Input
          value={form.clone_dir}
          onChange={(v) => patch({ clone_dir: v })}
          placeholder="/var/lib/libvirt/images"
        />
      </SettingRow>

      <SettingRow
        label="ISO 存放位置"
        tip="创建虚拟机和救援系统下拉框都会读取这个目录下的 .iso 文件 | 环境变量: KVM_ISO_DIR"
      >
        <div className="stg-inline-group">
          <Input
            value={form.iso_dir}
            onChange={(v) => patch({ iso_dir: v })}
            placeholder="/var/lib/libvirt/images/ISO"
            style={{ flex: 1 }}
          />
          <Button loading={isoPathLoading} onClick={() => void handleUseUserStorageISO()}>
            替换为我的存储
          </Button>
        </div>
      </SettingRow>

      <SettingRow
        label="端口转发持久化目录"
        tip="环境变量: KVM_PORTFORWARD_DIR（仅通过环境变量修改）"
      >
        <Input value={form.port_forward_dir} disabled />
      </SettingRow>

      <SectionHead icon={<IconBranch />} title="网络设置" />

      <SettingRow
        label="默认网络"
        tip="保留给历史配置查看；新平台默认使用 OVS | 环境变量: KVM_DEFAULT_NETWORK"
      >
        <Input
          value={form.default_network}
          onChange={(v) => patch({ default_network: v })}
          placeholder="default"
        />
      </SettingRow>

      <SettingRow label="网络后端" tip="当前仅支持 OVS | 环境变量: KVM_NETWORK_BACKEND">
        <Input value={form.network_backend} disabled />
      </SettingRow>

      <SettingRow
        label="OVS 网桥"
        tip="VM 接入的 OVS 网桥，不迁移宿主机物理网卡 | 环境变量: KVM_OVS_BRIDGE"
      >
        <Input
          value={form.ovs_bridge}
          onChange={(v) => patch({ ovs_bridge: v })}
          placeholder="br-ovs"
        />
      </SettingRow>

      <SettingRow label="OVS 出口网卡" tip="OVS NAT 出口网卡，留空自动检测 | 环境变量: KVM_OVS_UPLINK">
        <Input
          value={form.ovs_uplink}
          onChange={(v) => patch({ ovs_uplink: v })}
          placeholder="留空自动检测默认路由网卡"
        />
      </SettingRow>

      <SettingRow label="网段前缀" tip="环境变量: KVM_SUBNET_PREFIX">
        <Input
          value={form.subnet_prefix}
          onChange={(v) => patch({ subnet_prefix: v })}
          placeholder="192.168.122"
        />
      </SettingRow>

      <SettingRow
        label="OVS DHCP 范围"
        tip="留空时按网段前缀自动使用 .2 - .254 | 环境变量: KVM_OVS_DHCP_START / KVM_OVS_DHCP_END"
      >
        <div className="stg-range-inputs">
          <Input
            value={form.ovs_dhcp_start}
            onChange={(v) => patch({ ovs_dhcp_start: v })}
            placeholder="192.168.122.2"
          />
          <span className="stg-range-sep">—</span>
          <Input
            value={form.ovs_dhcp_end}
            onChange={(v) => patch({ ovs_dhcp_end: v })}
            placeholder="192.168.122.254"
          />
        </div>
      </SettingRow>

      <SettingRow
        label="外网网卡"
        tip="端口转发用的外网网卡名称，留空通过默认路由自动检测 | 环境变量: KVM_EXTERNAL_NIC"
      >
        <Input
          value={form.external_nic}
          onChange={(v) => patch({ external_nic: v })}
          placeholder="留空自动检测（如 eth0、ens33）"
        />
      </SettingRow>

      <SettingRow
        label="公网 IP"
        tip="端口转发展示和规则优先使用这里的公网 IP，留空时自动检测默认出口 IP | 环境变量: KVM_HOST_IP"
      >
        <Input
          value={form.host_ip}
          onChange={(v) => patch({ host_ip: v })}
          placeholder="留空自动检测，也可手动填写固定公网 IP"
        />
      </SettingRow>

      <SectionHead icon={<IconSafeStroked />} title="端口安全参数" />

      {!form.port_security_enabled ? (
        <Banner
          type="info"
          closeIcon={null}
          className="stg-banner"
          description="端口安全总开关当前关闭。请在“网络中心 → 网络概览”完成预检并启用；关闭时这些高级阈值不参与校验。"
        />
      ) : (
        <>
          <Banner
            type="warning"
            closeIcon={null}
            className="stg-banner"
            description="保存后会触发后台协调。总包速率使用 OVS Interface packet policing，ARP/ND 与广播/组播使用独立 packet meter。"
          />
          <div className="stg-field-grid">
            <NumField
              label="端口总包速率"
              suffix="kpps"
              value={form.port_security_total_kpps}
              onChange={(v) => patch({ port_security_total_kpps: v })}
              min={1}
              max={1000000}
            />
            <NumField
              label="端口总包突发"
              suffix="kpackets"
              value={form.port_security_total_burst_kpackets}
              onChange={(v) => patch({ port_security_total_burst_kpackets: v })}
              min={1}
              max={1000000}
            />
            <NumField
              label="ARP / ND 速率"
              suffix="pps"
              value={form.port_security_neighbor_pps}
              onChange={(v) => patch({ port_security_neighbor_pps: v })}
              min={1}
              max={1000000}
            />
            <NumField
              label="ARP / ND 突发"
              suffix="packets"
              value={form.port_security_neighbor_burst_packets}
              onChange={(v) => patch({ port_security_neighbor_burst_packets: v })}
              min={1}
              max={2000000}
            />
            <NumField
              label="广播 / 组播速率"
              suffix="pps"
              value={form.port_security_broadcast_pps}
              onChange={(v) => patch({ port_security_broadcast_pps: v })}
              min={1}
              max={1000000}
            />
            <NumField
              label="广播 / 组播突发"
              suffix="packets"
              value={form.port_security_broadcast_burst_packets}
              onChange={(v) => patch({ port_security_broadcast_burst_packets: v })}
              min={1}
              max={2000000}
            />
            <NumField
              label="全量协调周期"
              suffix="秒"
              value={form.port_security_reconcile_interval_seconds}
              onChange={(v) => patch({ port_security_reconcile_interval_seconds: v })}
              min={10}
              max={3600}
            />
          </div>
        </>
      )}

      <SectionHead icon={<IconPulse />} title="全局带宽限制" />

      <Banner
        type="info"
        closeIcon={null}
        className="stg-banner"
        description="全局带宽限制会应用于所有非轻量云的虚拟机及 VPC 交换机。有效带宽 = 配置值 - 5Mbps（保留缓冲），所有运行中的虚拟机均分总带宽。0 = 不限制。"
      />

      <div className="stg-field-grid">
        <NumField
          label="下行总带宽"
          suffix="Mbps"
          value={form.max_burst_inbound}
          onChange={(v) => patch({ max_burst_inbound: v })}
          min={0}
          max={100000}
          tip="全局限速下行总带宽，所有 VM 均分"
        />
        <NumField
          label="上行总带宽"
          suffix="Mbps"
          value={form.max_burst_outbound}
          onChange={(v) => patch({ max_burst_outbound: v })}
          min={0}
          max={100000}
          tip="全局限速上行总带宽，所有 VM 均分"
        />
      </div>

      <div className="stg-plain-tip">
        保存后立即生效：每台运行中 VM 设置全量有效带宽为上限（配置 50Mbps 时有效
        45Mbps，每台 VM 上限均为 45Mbps）。多台 VM 同时跑满时由 TCP 拥塞控制自然分享带宽。环境变量:
        KVM_MAX_BURST_INBOUND / KVM_MAX_BURST_OUTBOUND
      </div>

      <SectionHead icon={<IconSetting />} title="默认磁盘 IOPS 限制" />

      <Banner
        type="info"
        closeIcon={null}
        className="stg-banner"
        description="此设置仅作为新建虚拟机时的参考默认值。已存在的虚拟机需在编辑页面中单独配置磁盘 IOPS 限制。0 表示不限制。"
      />

      <div className="stg-field-grid">
        <NumField
          label="默认总 IOPS"
          value={form.default_disk_iops_total}
          onChange={(v) => patch({ default_disk_iops_total: v })}
          min={0}
          step={100}
          tip="新建虚拟机磁盘的默认总 IOPS 限制"
        />
        <NumField
          label="默认读 IOPS"
          value={form.default_disk_iops_read}
          onChange={(v) => patch({ default_disk_iops_read: v })}
          min={0}
          step={100}
          tip="新建虚拟机磁盘的默认读 IOPS 限制"
        />
        <NumField
          label="默认写 IOPS"
          value={form.default_disk_iops_write}
          onChange={(v) => patch({ default_disk_iops_write: v })}
          min={0}
          step={100}
          tip="新建虚拟机磁盘的默认写 IOPS 限制"
        />
      </div>
      <div className="stg-plain-tip">
        环境变量: KVM_DEFAULT_DISK_IOPS_TOTAL / KVM_DEFAULT_DISK_IOPS_READ / KVM_DEFAULT_DISK_IOPS_WRITE
      </div>
    </div>
  )
}
