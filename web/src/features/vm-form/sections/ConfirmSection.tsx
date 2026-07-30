/**
 * 确认信息分区（创建向导最后一步）
 * 汇总展示全部已选配置，提交前最终确认。
 */
import { useMemo } from 'react'
import { Banner, Tag } from '@douyinfe/semi-ui'
import { IconCheckList } from '@douyinfe/semi-icons'
import SectionCard from './SectionCard'
import { useVmFormScope } from '../scopeContext'
import { ALL_BOOT_DEVICES } from '../constants'
import { resolveStorageTargetLabel } from './storageTargetUtils'

interface Row {
  label: string
  value: React.ReactNode
}

function SummaryGroup({ title, rows }: { title: string; rows: (Row | null)[] }) {
  const visible = rows.filter((r): r is Row => !!r)
  if (visible.length === 0) return null
  return (
    <div className="qvm-vf-confirm-group">
      <div className="qvm-vf-confirm-group-title">{title}</div>
      {visible.map((row) => (
        <div key={row.label} className="qvm-vf-confirm-row">
          <span className="qvm-vf-confirm-label">{row.label}</span>
          <span className="qvm-vf-confirm-value">{row.value}</span>
        </div>
      ))}
    </div>
  )
}

export default function ConfirmSection() {
  const { form, options, ctx } = useVmFormScope()
  const { form: f, isTemplateSourceMode } = form

  const createModeLabel = useMemo(() => {
    if (ctx.registration.enabled) return '轻量云服务器登记'
    const map: Record<string, string> = { iso: 'ISO 镜像安装', template: '模板克隆', import: '导入已有磁盘', appliance: '导入虚拟机' }
    const base = map[f.create_mode] || f.create_mode
    if (isTemplateSourceMode) {
      return base + (f.clone_mode === 'full' ? '（完整克隆）' : '（链式克隆）')
    }
    return base
  }, [ctx.registration.enabled, f.create_mode, f.clone_mode, isTemplateSourceMode])

  const osLabel = useMemo(() => {
    if (isTemplateSourceMode) {
      const tpl = options.templates.find((t) => t.name === f.template)
      return tpl?.display_name || tpl?.admin_name || f.template || '未选择'
    }
    const map: Record<string, string> = { linux: '🐧 Linux', windows: '🪟 Windows', other: '🖥️ 其他' }
    return map[f.os_type] || f.os_type
  }, [isTemplateSourceMode, options.templates, f.template, f.os_type])

  const bootTypeLabel = useMemo(() => {
    const map: Record<string, string> = { bios: 'BIOS', uefi: 'UEFI', 'uefi-secure': 'UEFI + 安全引导' }
    return map[f.boot_type] || '自动'
  }, [f.boot_type])

  const bootOrderLabel = useMemo(
    () =>
      f.boot_order
        .map((item) => ALL_BOOT_DEVICES.find((d) => d.value === item)?.label || item)
        .join(' → '),
    [f.boot_order],
  )

  const storageLabel = resolveStorageTargetLabel(options.storageTargets, f.storage_pool_id)

  const validNics = f.extra_nics.filter((n) => n.switch_id)
  const applianceDiskGB = useMemo(
    () => Math.ceil((f.appliance_metadata?.disks.reduce((sum, disk) => sum + disk.capacity_bytes, 0) || 0) / 1024 / 1024 / 1024),
    [f.appliance_metadata],
  )

  // 高级选项非默认项汇总
  const advancedItems = useMemo(() => {
    const items: string[] = []
    if (f.freeze) items.push('启动冻结 CPU')
    if (!f.apic) items.push('关闭 APIC')
    if (!f.pae) items.push('关闭 PAE')
    if (f.kvm_hidden) items.push('隐藏 KVM 标志')
    if (f.vendor_id) items.push(`Vendor ID 伪装：${f.vendor_id}`)
    if (!f.nested_virt) items.push('关闭嵌套虚拟化')
    if (f.cpu_topology_mode !== 'auto') items.push(`CPU 拓扑：${f.cpu_topology_mode}`)
    if (f.cpu_affinity) items.push(`CPU 亲和性：${f.cpu_affinity}`)
    if (f.pcie_root_ports !== 4) items.push(`PCIe 热插槽：${f.pcie_root_ports}`)
    if (f.spice_enabled) items.push('启用 SPICE')
    if (f.rtc_offset !== 'utc') items.push('RTC 本地时间')
    if (f.rtc_startdate && f.rtc_startdate !== 'now') items.push(`RTC 固定时间：${f.rtc_startdate}`)
    if (f.direct_boot_enabled) items.push('直接内核引导')
    if (f.firmware_compat) items.push('UEFI 固件兼容')
    if (f.cpu_hotplug_enabled) items.push('CPU 热添加')
    if (f.cpu_limit_enabled) items.push(`CPU 限制 ${f.cpu_limit_percent}%`)
    if (f.memory_dynamic_enabled) {
      items.push(
        f.memory_backend === 'virtio_mem'
          ? `Windows 弹性内存（基础 ${f.memory_initial}GB / 最大 ${f.memory_max_dynamic}GB）`
          : `动态内存（启动 ${f.memory_initial}GB / 最大 ${f.memory_max_dynamic}GB）`,
      )
    }
    return items
  }, [f])

  return (
    <>
      <Banner
        type="info"
        closeIcon={null}
        style={{ marginBottom: 14 }}
        description="请确认以下配置信息无误后提交，创建任务将在后台异步执行，可在底部任务栏查看进度。"
      />
      <SectionCard icon={<IconCheckList />} title="配置确认">
        <SummaryGroup
          title="基础信息"
          rows={[
            { label: '创建方式', value: createModeLabel },
            { label: '虚拟机名称', value: <strong>{f.name || '未命名'}</strong> },
            isTemplateSourceMode && f.system_init_enabled && f.batch_count > 1
              ? { label: '创建数量', value: `${f.batch_count} 台` }
              : null,
            f.remark ? { label: '备注', value: f.remark } : null,
            { label: isTemplateSourceMode ? '模板' : '操作系统', value: osLabel },
            !isTemplateSourceMode && f.os_variant ? { label: '系统版本', value: f.os_variant } : null,
          ]}
        />
        <SummaryGroup
          title="硬件规格"
          rows={[
            { label: 'CPU', value: `${f.vcpu} 核` },
            { label: '内存', value: `${f.ram} GB` },
            f.create_mode === 'iso' || f.create_mode === 'import' || f.create_mode === 'appliance'
              ? { label: '虚拟化方案', value: f.virt_type === 'kvm' ? 'KVM 硬件虚拟化' : `QEMU 软件虚拟化（${f.arch}）` }
              : null,
            { label: '芯片组', value: f.machine_type?.toUpperCase() },
            f.create_mode !== 'import' ? { label: '引导固件', value: bootTypeLabel } : null,
          ]}
        />
        <SummaryGroup
          title="存储"
          rows={[
            f.create_mode !== 'import' ? { label: '存储位置', value: storageLabel } : null,
            f.create_mode === 'iso'
              ? { label: '系统盘', value: `${f.disk_size} GB（${f.disk_format?.toUpperCase()} / ${f.disk_bus}）` }
              : null,
            isTemplateSourceMode ? { label: '系统盘', value: `${f.disk_size} GB（${f.disk_bus}）` } : null,
            f.create_mode === 'iso' && f.iso_path
              ? { label: 'ISO', value: f.iso_path.split('/').pop() }
              : null,
            f.iso_paths.length > 1 ? { label: '额外挂载', value: `+${f.iso_paths.length - 1} 个 ISO` } : null,
            f.create_mode === 'import'
              ? {
                  label: '磁盘文件',
                  value: f.disk_source_type === 'path' ? f.disk_path : f.disk_file || '未选择',
                }
              : null,
            f.create_mode === 'import'
              ? { label: '磁盘处理', value: f.copy_disk ? '保留原磁盘文件' : '不保留原磁盘文件' }
              : null,
            f.create_mode === 'import'
              ? { label: '导入后', value: f.start_after_import ? '自动开机' : '仅创建不开启' }
              : null,
            f.floppy_image ? { label: '软盘', value: f.floppy_image.split('/').pop() } : null,
            f.extra_disks.length > 0
              ? {
                  label: '额外磁盘',
                  value: f.extra_disks.map((d, i) => (
                    <Tag key={i} size="small" style={{ marginRight: 4 }}>
                      {d.size}GB {d.bus}
                    </Tag>
                  )),
                }
              : null,
            f.extra_import_disks.length > 0
              ? { label: '额外导入磁盘', value: `${f.extra_import_disks.length} 块` }
              : null,
          ]}
        />
        {f.create_mode === 'appliance' && (
          <SummaryGroup
            title="虚拟机包信息"
            rows={[
              { label: '源文件', value: f.appliance_source_type === 'path' ? f.appliance_path : f.appliance_file },
              { label: '配置方式', value: f.appliance_config_mode === 'ovf' ? '跟随 OVF 配置' : '自定义配置' },
              f.appliance_metadata
                ? { label: '包格式', value: f.appliance_metadata.source_format.toUpperCase() }
                : null,
              f.appliance_metadata
                ? { label: '包内名称', value: f.appliance_metadata.name || '未声明' }
                : null,
              f.appliance_metadata
                ? { label: '原始配置', value: `${f.appliance_metadata.vcpu} 核 / ${f.appliance_metadata.ram} GB / ${f.appliance_metadata.boot_type || '默认固件'}` }
                : null,
              f.appliance_metadata
                ? { label: '包内设备', value: `${f.appliance_metadata.disks.length} 块磁盘 / ${f.appliance_metadata.networks.length} 个网口` }
                : { label: '任务校验', value: '提交后在异步任务中解析并校验虚拟机包' },
              { label: '源文件策略', value: f.copy_source ? '导入成功后保留' : '完整导入成功后删除' },
              { label: '导入后', value: f.start_after_import ? '自动开机' : '保持关机' },
            ]}
          />
        )}
        <SummaryGroup
          title="网络与系统"
          rows={[
            { label: '网卡型号', value: f.nic_model },
            validNics.length > 0 ? { label: '网口数量', value: `${validNics.length} 个` } : null,
            { label: '引导顺序', value: bootOrderLabel },
            { label: '开机自启', value: f.autostart ? '启用' : '关闭' },
            f.watchdog !== 'none' ? { label: 'Watchdog', value: f.watchdog } : null,
          ]}
        />
        {advancedItems.length > 0 && (
          <SummaryGroup
            title="高级选项"
            rows={[
              {
                label: '非默认项',
                value: advancedItems.map((item) => (
                  <Tag key={item} size="small" color="orange" style={{ marginRight: 4, marginBottom: 4 }}>
                    {item}
                  </Tag>
                )),
              },
            ]}
          />
        )}
        {f.host_devices.length > 0 && (
          <SummaryGroup
            title="硬件直通"
            rows={[
              {
                label: '直通设备',
                value: f.host_devices.map((d) => (
                  <Tag key={d.pci_address} size="small" style={{ marginRight: 4 }}>
                    {d.pci_address}
                  </Tag>
                )),
              },
            ]}
          />
        )}
        <div className="qvm-vf-confirm-total">
          <span>预估资源占用</span>
          <strong>
            CPU {f.vcpu} 核 / 内存 {f.ram} GB{f.create_mode === 'appliance' && applianceDiskGB > 0 ? ` / 包内磁盘 ${applianceDiskGB} GB` : f.disk_size > 0 ? ` / 磁盘 ${f.disk_size} GB` : ''}
            {isTemplateSourceMode && f.batch_count > 1 ? ` × ${f.batch_count} 台` : ''}
          </strong>
        </div>
      </SectionCard>
    </>
  )
}
