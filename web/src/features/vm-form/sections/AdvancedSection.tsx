/**
 * 高级选项分区（创建 / 编辑共用）
 * 开发者选项与底层参数：冻结 CPU / APIC / PAE / KVM 隐藏 / Vendor ID /
 * 嵌套虚拟化 / CPU 拓扑 / 亲和性 / PCIe 热插槽 / 显示设备 / SPICE /
 * RTC / Guest Agent / 动态内存 / SMBIOS / 直接内核引导 / XML 编辑。
 * 含首次进入提醒遮罩（localStorage 按用户记忆）。
 */
import { useMemo, useState } from 'react'
import { Banner, Button, Card, Input, InputNumber, Select } from '@douyinfe/semi-ui'
import { IconArrowRight, IconCodeStroked, IconLightningStroked } from '@douyinfe/semi-icons'
import SectionCard from './SectionCard'
import FormField from './FormField'
import TextSwitch from './TextSwitch'
import { useVmFormScope } from '../scopeContext'
import {
  ADVANCED_HELP_TEXT,
  CPU_TOPOLOGY_MODE_OPTIONS,
  FIRST_BOOT_REBOOT_MODE_OPTIONS,
  VIDEO_MODEL_OPTIONS,
} from '../constants'
import { normalizeRTCStartDate, normalizeSMBIOS1Value } from '../recommend'
import RtcConfigDialog from '../dialogs/RtcConfigDialog'
import GuestAgentDialog from '../dialogs/GuestAgentDialog'
import MemoryDynamicDialog from '../dialogs/MemoryDynamicDialog'
import SmbiosDialog from '../dialogs/SmbiosDialog'

interface AdvancedSectionProps {
  /** 编辑模式传入当前虚拟机 UUID（SMBIOS 只读展示） */
  currentVmUUID?: string
  /** 编辑模式 XML 编辑入口 */
  onOpenXmlEditor?: () => void
}

/** 高级配置入口行（标题 + 摘要 + 箭头，点击打开弹窗） */
function ConfigEntry({ title, summary, onClick }: { title: string; summary: string; onClick: () => void }) {
  return (
    <div className="qvm-vf-config-entry" onClick={onClick}>
      <div className="qvm-vf-config-entry-main">
        <div className="qvm-vf-config-entry-title">{title}</div>
        <div className="qvm-vf-config-entry-summary">{summary}</div>
      </div>
      <IconArrowRight className="qvm-vf-config-entry-icon" />
    </div>
  )
}

export default function AdvancedSection({ currentVmUUID, onOpenXmlEditor }: AdvancedSectionProps) {
  const { form, options, ctx } = useVmFormScope()
  const { form: f, setField, isTemplateSourceMode, isWindowsTemplate } = form
  const isEdit = ctx.mode === 'edit'
  const runningOrPaused = ctx.vmStatus === 'running' || ctx.vmStatus === 'paused'

  const [rtcVisible, setRtcVisible] = useState(false)
  const [guestAgentVisible, setGuestAgentVisible] = useState(false)
  const [memoryVisible, setMemoryVisible] = useState(false)
  const [smbiosVisible, setSmbiosVisible] = useState(false)

  // ===== 首次进入提醒遮罩（按用户记忆） =====
  const introStorageKey = 'vm-advanced-settings-intro-seen'
  const [introDismissed, setIntroDismissed] = useState(
    () => localStorage.getItem(introStorageKey) === '1',
  )
  const dismissIntro = () => {
    setIntroDismissed(true)
    localStorage.setItem(introStorageKey, '1')
  }

  // ===== 弹窗摘要 =====
  const rtcSummary = useMemo(() => {
    const offsetText = f.rtc_offset === 'localtime' ? '本地时间' : 'UTC'
    return `时间基准：${offsetText} / 开始日期：${normalizeRTCStartDate(f.rtc_startdate)}`
  }, [f.rtc_offset, f.rtc_startdate])

  const guestAgentSummary = f.guest_agent.enabled ? '已启用，需虚拟机内安装 qemu-guest-agent' : '默认（已禁用）'

  const memorySummary = useMemo(() => {
    if (!f.memory_dynamic_enabled) {
      if (f.memory_compat_mode === 'legacy_static') return '静态兼容模式，可由管理员启用'
      return '默认（已关闭）'
    }
    if (f.memory_backend === 'virtio_mem') {
      const status = f.memory_pending_apply ? '待下次启动应用' : '实验'
      return `${status} / 基础 ${f.memory_initial}GB / 最大 ${f.memory_max_dynamic}GB`
    }
    const status = f.memory_pending_apply ? '待下次启动应用' : '已启用'
    return `${status} / 启动 ${f.memory_initial}GB / 最小 ${f.memory_min}GB / 最大 ${f.memory_max_dynamic}GB`
  }, [f.memory_dynamic_enabled, f.memory_compat_mode, f.memory_backend, f.memory_pending_apply, f.memory_initial, f.memory_min, f.memory_max_dynamic])

  const smbiosSummary = useMemo(() => {
    const parts: string[] = []
    if (normalizeSMBIOS1Value(f.smbios1.manufacturer)) parts.push(`厂商：${normalizeSMBIOS1Value(f.smbios1.manufacturer)}`)
    if (normalizeSMBIOS1Value(f.smbios1.product)) parts.push(`产品：${normalizeSMBIOS1Value(f.smbios1.product)}`)
    if (normalizeSMBIOS1Value(f.smbios1.serial)) parts.push(`序列号：${normalizeSMBIOS1Value(f.smbios1.serial)}`)
    if (normalizeSMBIOS1Value(f.smbios1.uuid)) parts.push(`UUID：${normalizeSMBIOS1Value(f.smbios1.uuid)}`)
    if (parts.length === 0) {
      if (isEdit && currentVmUUID) return `未额外配置 / 当前 UUID：${currentVmUUID}`
      return '未配置，保持系统默认'
    }
    const summary = parts.slice(0, 3).join(' / ')
    return f.smbios1.base64 ? `${summary} / Base64 解码写入` : summary
  }, [f.smbios1, isEdit, currentVmUUID])

  return (
    <SectionCard icon={<IconLightningStroked />} title="高级选项">
      <div className={`qvm-vf-advanced${introDismissed ? '' : ' blurred'}`}>
        <Banner
          type="warning"
          closeIcon={null}
          style={{ marginBottom: 14 }}
          description="高级设置仅建议在调试或排查启动问题时使用，普通情况下请保持默认配置。"
        />

        <div className="qvm-vf-subdivider">开发者选项</div>

        <FormField label="启动时冻结 CPU" help={ADVANCED_HELP_TEXT.freeze}>
          <TextSwitch checked={f.freeze} onChange={(v) => setField('freeze', v)} />
        </FormField>

        <FormField label="APIC" help={ADVANCED_HELP_TEXT.apic} tip="常规虚拟机建议保持启用；仅在排查非常早期的启动兼容性问题时再尝试关闭">
          <TextSwitch checked={f.apic} onChange={(v) => setField('apic', v)} />
        </FormField>

        <FormField label="PAE" help={ADVANCED_HELP_TEXT.pae} tip="主要用于 x86 老系统或 32 位来宾的大内存兼容场景；非 x86 架构会自动忽略该设置">
          <TextSwitch checked={f.pae} onChange={(v) => setField('pae', v)} />
        </FormField>

        <FormField
          label="隐藏 KVM 标志"
          help="启用后在 features 中注入 <kvm><hidden state='on'/></kvm>，让虚拟机更难被检测为 KVM 虚拟化环境"
          tip="用于规避部分软件/游戏的反虚拟化检测"
        >
          <TextSwitch checked={f.kvm_hidden} onChange={(v) => setField('kvm_hidden', v)} />
        </FormField>

        <FormField
          label="Vendor ID 伪装"
          help="在 Hyper-V enlightenments 中注入 <vendor_id state='on' value='...'/>，伪装 CPU 厂商 ID"
          tip="常用于绕过特定软件（如 N 卡驱动）的虚拟化检测；仅 x86_64 架构生效"
        >
          <Input
            style={{ width: 280 }}
            value={f.vendor_id}
            onChange={(v) => setField('vendor_id', v)}
            placeholder="留空不伪装，如: AuthenticAMD"
            showClear
          />
        </FormField>

        <FormField
          label="嵌套虚拟化"
          help="启用后在 CPU 配置中注入 vmx（Intel）或 svm（AMD）特性，允许虚拟机内再运行虚拟机"
          tip="默认启用；若宿主机未开启 KVM 嵌套模块（kvm_intel nested=1 或 kvm_amd nested=1），该选项实际不生效"
        >
          <TextSwitch checked={f.nested_virt} onChange={(v) => setField('nested_virt', v)} />
        </FormField>

        <FormField label="CPU 拓扑" tip="自动模式会为 Windows 使用单插槽多核心；排查兼容性时可显式选择宿主默认拓扑">
          <Select
            style={{ width: 280 }}
            value={f.cpu_topology_mode}
            onChange={(v) => setField('cpu_topology_mode', v as string)}
            optionList={CPU_TOPOLOGY_MODE_OPTIONS.map((item) => ({ value: item.value, label: item.label }))}
          />
        </FormField>

        {ctx.isAdmin && (
          <FormField label="CPU 亲和性" tip="将虚拟机的 vCPU 绑定到指定的物理 CPU 核心上，多个核心用逗号或空格分隔，支持范围格式（如 0-3）。留空表示不限制亲和性">
            <Select
              style={{ width: 320 }}
              value={f.cpu_affinity || undefined}
              filter
              allowCreate
              showClear
              placeholder="例如: 0,2,4 或 0-3"
              onChange={(v) => setField('cpu_affinity', (v as string) || '')}
              optionList={options.cpuAffinityPresets.map((p) => ({ value: p.value, label: p.name }))}
            />
          </FormField>
        )}

        {f.machine_type === 'q35' && (
          <FormField label="PCIe 热插槽" tip="预留的 pcie-root-port 数量，设为 0 使用默认值（4）。足够的插槽可避免后续热添加磁盘时提示「无可用 PCI 插槽」。最大支持 32 个。">
            <InputNumber
              style={{ width: 160 }}
              value={f.pcie_root_ports}
              min={0}
              max={32}
              step={1}
              onChange={(v) => setField('pcie_root_ports', Number(v || 0))}
            />
          </FormField>
        )}

        {!isEdit && isTemplateSourceMode && isWindowsTemplate && (
          <FormField label="首次重启" tip="LTSC 等模板若在 Sysprep/OOBE 自动重启后黑屏，可选择宿主冷启动">
            <Select
              style={{ width: 280 }}
              value={f.first_boot_reboot_mode}
              onChange={(v) => setField('first_boot_reboot_mode', v as string)}
              optionList={FIRST_BOOT_REBOOT_MODE_OPTIONS.map((item) => ({ value: item.value, label: item.label }))}
            />
          </FormField>
        )}

        <div className="qvm-vf-subdivider">显示与协议</div>

        <FormField
          label="显示设备"
          tip={
            f.video_model === 'none'
              ? '已禁用虚拟显示设备，VNC/SPICE 控制台将隐藏；若仅直通一张 VGA，系统会自动将其设为主显卡'
              : 'Windows 安装或 VMware 嵌套环境可优先尝试 VGA / VMVGA；若系统已安装 VirtIO 驱动可再切回 VirtIO'
          }
          tipType={f.video_model === 'none' ? 'warn' : 'info'}
        >
          <Select
            style={{ width: 280 }}
            value={f.video_model}
            onChange={(v) => {
              const value = v as string
              setField('video_model', value)
              if (value === 'none') setField('spice_enabled', false)
            }}
            optionList={VIDEO_MODEL_OPTIONS.map((item) => ({
              value: item.value,
              label: (
                <div style={{ display: 'flex', justifyContent: 'space-between', gap: 12 }}>
                  <span>{item.label}</span>
                  <span className="qvm-vf-option-tag">{item.tag}</span>
                </div>
              ),
            }))}
          />
        </FormField>

        {f.video_model !== 'none' && (
          <FormField
            label="SPICE 协议"
            tip={
              !ctx.spiceSupported
                ? '当前 QEMU 未编译 SPICE 支持，无法启用'
                : isEdit && runningOrPaused
                  ? '运行中不可修改，请关机后更改'
                  : '启用后将附带 SPICE 显示协议（与 VNC 共存，默认仅本地监听）。部分机器/客户机不支持 SPICE，按需开启。'
            }
            tipType={!ctx.spiceSupported || (isEdit && runningOrPaused) ? 'warn' : 'info'}
          >
            <TextSwitch
              checked={f.spice_enabled}
              disabled={(isEdit && runningOrPaused) || !ctx.spiceSupported}
              onChange={(v) => setField('spice_enabled', v)}
            />
          </FormField>
        )}

        <div className="qvm-vf-subdivider">设备与固件</div>

        <FormField label="RTC">
          <ConfigEntry title="配置" summary={rtcSummary} onClick={() => setRtcVisible(true)} />
        </FormField>

        <FormField label="QEMU Guest Agent">
          <ConfigEntry title="配置" summary={guestAgentSummary} onClick={() => setGuestAgentVisible(true)} />
        </FormField>

        {ctx.isAdmin && (
          <FormField label="动态内存">
            <ConfigEntry title="配置" summary={memorySummary} onClick={() => setMemoryVisible(true)} />
          </FormField>
        )}

        <FormField label="SMBIOS">
          <ConfigEntry title="类型 1" summary={smbiosSummary} onClick={() => setSmbiosVisible(true)} />
        </FormField>

        {f.arch === 'aarch64' && (
          <FormField
            label="UEFI 固件兼容"
            help="启用后使用旧版 EDK2 固件，解决统信 UOS 等系统在 ARM 平台的 UEFI 引导兼容性问题"
            tip="仅 ARM 架构可用。当系统安装 ISO 出现 Synchronous Exception 报错时建议开启"
          >
            <TextSwitch
              checked={f.firmware_compat}
              onChange={(v) => setField('firmware_compat', v)}
            />
          </FormField>
        )}

        <FormField
          label="直接内核引导"
          help="绕过 UEFI 引导器直接加载内核，适用于 ISO 的 EFI 引导器与当前固件不兼容的场景"
        >
          <TextSwitch
            checked={f.direct_boot_enabled}
            onChange={(v) => setField('direct_boot_enabled', v)}
          />
          {f.direct_boot_enabled && (
            <div style={{ marginTop: 8, width: '100%' }}>
              <Input
                value={f.direct_boot_cmdline}
                onChange={(v) => setField('direct_boot_cmdline', v)}
                placeholder="内核命令行参数（可选，留空使用默认）"
              />
              <div className="qvm-vf-tip">会自动从 ISO 中提取 vmlinuz 和 initrd。安装完成后请关闭此选项并重启虚拟机</div>
            </div>
          )}
        </FormField>

        {isEdit && onOpenXmlEditor && (
          <FormField label="虚拟机 XML">
            <Button icon={<IconCodeStroked />} onClick={onOpenXmlEditor}>
              查看 / 编辑持久化 XML
            </Button>
            <div className="qvm-vf-tip">保存后立即写入 libvirt 定义；建议先关机后再修改，不支持通过此功能修改虚拟机名称</div>
          </FormField>
        )}
      </div>

      {!introDismissed && (
        <div className="qvm-vf-advanced-mask">
          <Card className="qvm-vf-advanced-intro" shadows="always">
            <div className="qvm-vf-advanced-intro-title">进阶设置提醒</div>
            <p>此页面属于进阶设置，通常您并不需要调整此页面功能，若您不是开发者或不熟悉虚拟机的情况下请保持默认。</p>
            <p className="qvm-vf-advanced-intro-warn">仅在明确了解这些选项的用途及可能影响时再修改这里的选项。</p>
            <Button type="primary" theme="solid" onClick={dismissIntro}>
              我知道了
            </Button>
          </Card>
        </div>
      )}

      <RtcConfigDialog visible={rtcVisible} onClose={() => setRtcVisible(false)} />
      <GuestAgentDialog visible={guestAgentVisible} onClose={() => setGuestAgentVisible(false)} />
      <MemoryDynamicDialog visible={memoryVisible} onClose={() => setMemoryVisible(false)} />
      <SmbiosDialog visible={smbiosVisible} onClose={() => setSmbiosVisible(false)} currentVmUUID={currentVmUUID} />
    </SectionCard>
  )
}
