/**
 * 硬件直通配置区（宿主机设置 Tab 内）
 * CPU 虚拟化 / BIOS IOMMU 诊断 + IOMMU 一键开启 / vfio-pci 加载 / 可直通设备列表
 */
import { useState } from 'react'
import { Banner, Button, Tag } from '@douyinfe/semi-ui'
import { IconDesktop, IconInfoCircle } from '@douyinfe/semi-icons'
import { enableIommu, loadVfioPci, type HardwarePassthroughStatus } from '@/api/settings'
import { confirmModal } from '@/utils/confirm'
import { Toast } from '@douyinfe/semi-ui'
import TextSwitch from '@/features/vm-form/sections/TextSwitch'
import { SectionHead, SettingRow } from './SettingRow'

interface PassthroughSectionProps {
  status: HardwarePassthroughStatus | null
  loading: boolean
  /** 启用硬件直通表单值（随整体设置保存） */
  enabled: boolean
  onEnabledChange: (enabled: boolean) => void
  /** 重新加载直通状态 */
  reload: () => Promise<void>
}

export default function PassthroughSection({
  status,
  loading,
  enabled,
  onEnabledChange,
  reload,
}: PassthroughSectionProps) {
  const [iommuEnabling, setIommuEnabling] = useState(false)
  const [vfioLoading, setVfioLoading] = useState(false)

  const ready = status?.ready === true
  // BIOS IOMMU 已开启时才显示直通配置
  const showConfig = status?.bios_iommu_enabled === true
  const hasDevices = (status?.passthrough_devices?.length || 0) > 0

  const handleEnableIommu = async () => {
    const ok = await confirmModal({
      title: '一键开启 IOMMU',
      content:
        '此操作将修改 /etc/default/grub 文件，添加 IOMMU 内核参数并执行 update-grub。操作后需重启宿主机才能生效。确认继续？',
      okText: '确认开启',
    })
    if (!ok) return
    setIommuEnabling(true)
    try {
      const res = await enableIommu()
      Toast.success(res.message || 'IOMMU 已配置')
      await reload()
    } catch {
      // 请求层已统一提示
    } finally {
      setIommuEnabling(false)
    }
  }

  const handleLoadVfio = async () => {
    const ok = await confirmModal({
      title: '一键加载 vfio-pci',
      content: '此操作将加载 vfio-pci 内核模块并配置开机自动加载。确认继续？',
      okText: '确认加载',
    })
    if (!ok) return
    setVfioLoading(true)
    try {
      const res = await loadVfioPci()
      Toast.success(res.message || 'vfio-pci 已加载')
      await reload()
    } catch {
      // 请求层已统一提示
    } finally {
      setVfioLoading(false)
    }
  }

  return (
    <>
      <SectionHead icon={<IconDesktop />} title="硬件直通" />

      <Banner
        type={ready ? 'success' : 'warning'}
        closeIcon={null}
        className="stg-banner"
        description={
          ready
            ? '硬件直通环境已就绪，可在硬件直通页面绑定设备到虚拟机。'
            : status?.ready_message || '正在检测硬件直通状态...'
        }
      />

      {/* CPU 虚拟化和 BIOS IOMMU 始终显示，用于诊断 */}
      <SettingRow label="CPU 虚拟化">
        <div className="stg-host-row">
          {status?.cpu_virt_flag ? (
            <Tag size="small" color="green">
              支持（{status.cpu_virt_flag.toUpperCase()}）
            </Tag>
          ) : (
            <Tag size="small" color="red">
              未检测到 CPU 虚拟化支持
            </Tag>
          )}
        </div>
      </SettingRow>

      <SettingRow label="BIOS IOMMU" tip={status?.bios_iommu_message || undefined}>
        <div className="stg-host-row">
          {status?.bios_iommu_enabled ? (
            <Tag size="small" color="green">
              BIOS 已开启 IOMMU
            </Tag>
          ) : (
            <Tag size="small" color="red">
              BIOS IOMMU 未开启
            </Tag>
          )}
        </div>
      </SettingRow>

      {showConfig && (
        <>
          {hasDevices && (
            <SettingRow
              label="启用硬件直通"
              tip="保存后生效。开启后系统将在支持的宿主机上自动配置硬件直通环境。需要 IOMMU 已启用。"
            >
              <TextSwitch
                checked={enabled}
                onChange={onEnabledChange}
                checkedText="开"
                uncheckedText="关"
              />
            </SettingRow>
          )}

          <SettingRow
            label="IOMMU 状态"
            tip={
              !status?.iommu_enabled
                ? '需要在 /etc/default/grub 的 GRUB_CMDLINE_LINUX 中添加 intel_iommu=on 或 amd_iommu=on，然后 update-grub 并重启宿主机。'
                : undefined
            }
          >
            <div className="stg-host-row">
              {status?.iommu_enabled ? (
                <>
                  <Tag size="small" color="green">
                    IOMMU 已启用 ({(status.iommu_type || '').toUpperCase()})
                  </Tag>
                  {status.iommu_in_cmdline ? (
                    <Tag size="small" color="cyan">
                      内核参数启用
                    </Tag>
                  ) : (
                    <Tag size="small" color="green">
                      内核自动启用
                    </Tag>
                  )}
                </>
              ) : (
                <>
                  <Tag size="small" color="red">
                    IOMMU 未启用
                  </Tag>
                  <Button
                    size="small"
                    type="primary"
                    theme="light"
                    loading={iommuEnabling}
                    onClick={() => void handleEnableIommu()}
                  >
                    一键开启
                  </Button>
                </>
              )}
            </div>
          </SettingRow>

          <SettingRow label="vfio-pci 模块">
            <div className="stg-host-row">
              {status?.vfio_pci_loaded ? (
                <Tag size="small" color="green">
                  vfio-pci 已加载
                </Tag>
              ) : (
                <>
                  <Tag size="small" color="orange">
                    vfio-pci 未加载
                  </Tag>
                  <Button
                    size="small"
                    type="primary"
                    theme="light"
                    loading={vfioLoading}
                    onClick={() => void handleLoadVfio()}
                  >
                    一键加载
                  </Button>
                </>
              )}
            </div>
          </SettingRow>

          <SettingRow label="可直通设备">
            {status?.passthrough_devices?.length ? (
              <div className="stg-device-list">
                {status.passthrough_devices.map((dev) => (
                  <div className="stg-host-row stg-device-item" key={dev.pci_address}>
                    <Tag size="small">{dev.pci_address}</Tag>
                    {dev.product_name && (
                      <Tag size="small" color="cyan">
                        {dev.product_name}
                      </Tag>
                    )}
                    {dev.is_vfio_bound ? (
                      <Tag size="small" color="green">
                        已绑定 vfio-pci
                      </Tag>
                    ) : (
                      <Tag size="small" color="grey">
                        {dev.current_driver || '无驱动'}
                      </Tag>
                    )}
                    {dev.is_active_framebuffer && (
                      <Tag size="small" color="red">
                        当前控制台（不可直通）
                      </Tag>
                    )}
                    {(dev.iommu_group ?? -1) >= 0 && (
                      <Tag size="small">IOMMU 组: {dev.iommu_group}</Tag>
                    )}
                  </div>
                ))}
              </div>
            ) : (
              !loading && (
                <Tag size="small" color="grey">
                  <IconInfoCircle size="small" style={{ marginRight: 4 }} />
                  未检测到可用于直通的设备
                </Tag>
              )
            )}
          </SettingRow>
        </>
      )}
    </>
  )
}
