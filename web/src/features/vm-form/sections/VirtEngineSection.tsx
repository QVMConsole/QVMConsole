/**
 * 虚拟化引擎分区（创建 / 编辑共用）
 * 虚拟化方案（KVM/QEMU）、平台架构、机器类型、引导类型。
 */
import { Radio, Select, Tag } from '@douyinfe/semi-ui'
import { IconSetting } from '@douyinfe/semi-icons'
import SectionCard from './SectionCard'
import FormField from './FormField'
import { useVmFormScope } from '../scopeContext'
import { ARCH_OPTIONS } from '../constants'

export default function VirtEngineSection() {
  const { form, ctx } = useVmFormScope()
  const {
    form: f,
    i440fxWindowsBios,
    onVirtTypeChange,
    onArchChange,
    onMachineTypeChange,
    onBootTypeChange,
  } = form
  const isEdit = ctx.mode === 'edit'
  const runningOrPaused = ctx.vmStatus === 'running' || ctx.vmStatus === 'paused'
  const isArmFamily = f.arch === 'aarch64' || f.arch === 'riscv64'

  return (
    <SectionCard icon={<IconSetting />} title="虚拟化引擎">
      {!isEdit && (
        <FormField
          label="虚拟化方案"
          tip={
            f.virt_type === 'kvm'
              ? 'KVM 利用硬件虚拟化加速，性能最佳（需 CPU 支持 VT-x/AMD-V）'
              : 'QEMU 纯软件模拟，性能较低但可模拟不同平台架构'
          }
        >
          <Radio.Group
            type="button"
            value={f.virt_type}
            onChange={(e) => onVirtTypeChange(e.target.value)}
            options={[
              { label: '硬件虚拟化 (KVM)', value: 'kvm' },
              { label: '软件虚拟化 (QEMU)', value: 'qemu' },
            ]}
          />
        </FormField>
      )}

      {!isEdit && f.virt_type === 'qemu' && (
        <FormField label="平台架构" tip="软件虚拟化可模拟不同 CPU 架构，适合交叉编译和测试">
          <Select
            style={{ width: 260 }}
            value={f.arch}
            onChange={(v) => onArchChange(v as string)}
            optionList={ARCH_OPTIONS.map((item) => ({
              value: item.value,
              label: (
                <div style={{ display: 'flex', justifyContent: 'space-between', gap: 12 }}>
                  <span>{item.label}</span>
                  <Tag size="small" color={item.tagType === 'info' ? 'blue' : item.tagType === 'success' ? 'green' : 'orange'}>
                    {item.tag}
                  </Tag>
                </div>
              ),
            }))}
          />
        </FormField>
      )}

      <FormField label="虚拟机类型" required>
        <Radio.Group
          type="button"
          value={f.machine_type}
          disabled={isEdit}
          onChange={(e) => onMachineTypeChange(e.target.value)}
          options={[
            { label: 'Q35', value: 'q35', disabled: isArmFamily },
            { label: 'i440FX', value: 'i440fx', disabled: isArmFamily },
            ...(isArmFamily ? [{ label: 'virt', value: 'virt' }] : []),
          ]}
        />
        {i440fxWindowsBios && (
          <div className="qvm-vf-tip">当前宿主机上 Windows 搭配 i440FX 使用 BIOS 启动，以避免 UEFI 固件卡在启动画面</div>
        )}
      </FormField>

      <FormField label="引导类型" required>
        {!isEdit && f.create_mode === 'import' ? (
          <>
            <Tag color="blue">自动识别</Tag>
            <div className="qvm-vf-tip">导入时将自动检测磁盘的引导类型（BIOS/UEFI），无需手动选择</div>
          </>
        ) : (
          <>
            <Radio.Group
              type="button"
              value={f.boot_type}
              disabled={isEdit && runningOrPaused}
              onChange={(e) => onBootTypeChange(e.target.value)}
              options={[
                { label: 'BIOS', value: 'bios', disabled: f.arch === 'aarch64' },
                { label: 'UEFI', value: 'uefi', disabled: i440fxWindowsBios },
                {
                  label: 'UEFI + 安全引导',
                  value: 'uefi-secure',
                  disabled: f.machine_type === 'i440fx' || isArmFamily,
                },
              ]}
            />
            {isEdit ? (
              <>
                <div className="qvm-vf-tip warn">更改引导会导致原有已安装的操作系统无法启动</div>
                {runningOrPaused ? (
                  <div className="qvm-vf-tip warn">修改引导方式需要先关机后再保存</div>
                ) : (
                  <div className="qvm-vf-tip">
                    BIOS、UEFI 和安全引导切换都会改写固件配置，保存前请确认当前系统支持新的引导方式
                  </div>
                )}
              </>
            ) : null}
          </>
        )}
      </FormField>

    </SectionCard>
  )
}
