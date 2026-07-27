/**
 * 引导顺序分区（创建 / 编辑共用）
 * 创建：按设备类型排序（硬盘/光驱/网络）；编辑：Cockpit 风格设备列表（勾选+排序）。
 */
import { Button, Checkbox, Dropdown, Tag, Toast } from '@douyinfe/semi-ui'
import { IconArrowUp, IconArrowDown, IconDelete, IconPlus, IconDisc, IconGlobe } from '@douyinfe/semi-icons'
import { DiskIcon } from '../icons'
import SectionCard from './SectionCard'
import FormField from './FormField'
import { useVmFormScope } from '../scopeContext'
import { ALL_BOOT_DEVICES } from '../constants'
import type { EditBootDevice } from '../types'

const BOOT_ICON: Record<string, React.ReactNode> = {
  hd: <DiskIcon />,
  disk: <DiskIcon />,
  cdrom: <IconDisc />,
  network: <IconGlobe />,
}

const DEVICE_TYPE_LABEL: Record<string, string> = {
  disk: '磁盘',
  cdrom: '光驱',
  network: '网络',
}

interface BootOrderSectionProps {
  /** 编辑模式传入引导设备列表（由详情 boot_devices 回填） */
  editBootDevices?: EditBootDevice[]
  onEditBootDevicesChange?: (devices: EditBootDevice[]) => void
}

export default function BootOrderSection({ editBootDevices, onEditBootDevicesChange }: BootOrderSectionProps) {
  const { form, ctx } = useVmFormScope()
  const { form: f, setField } = form
  const isEdit = ctx.mode === 'edit'

  // ==================== 创建模式：类型排序 ====================

  const availableDevices = ALL_BOOT_DEVICES.filter((d) => !f.boot_order.includes(d.value))

  const moveUp = (index: number) => {
    if (index <= 0) return
    const next = [...f.boot_order]
    ;[next[index], next[index - 1]] = [next[index - 1], next[index]]
    setField('boot_order', next)
  }

  const moveDown = (index: number) => {
    if (index >= f.boot_order.length - 1) return
    const next = [...f.boot_order]
    ;[next[index], next[index + 1]] = [next[index + 1], next[index]]
    setField('boot_order', next)
  }

  const remove = (index: number) => {
    if (f.boot_order.length <= 1) {
      Toast.warning('至少保留一个启动设备')
      return
    }
    setField('boot_order', f.boot_order.filter((_, i) => i !== index))
  }

  // ==================== 编辑模式：设备列表 ====================

  const devices = editBootDevices || []
  const emitDevices = (next: EditBootDevice[]) => onEditBootDevicesChange?.(next)

  const moveDeviceUp = (index: number) => {
    if (index <= 0) return
    const next = [...devices]
    ;[next[index], next[index - 1]] = [next[index - 1], next[index]]
    emitDevices(next)
  }

  const moveDeviceDown = (index: number) => {
    if (index >= devices.length - 1) return
    const next = [...devices]
    ;[next[index], next[index + 1]] = [next[index + 1], next[index]]
    emitDevices(next)
  }

  const toggleDevice = (index: number, enabled: boolean) => {
    emitDevices(devices.map((d, i) => (i === index ? { ...d, enabled } : d)))
  }

  if (isEdit && devices.length > 0) {
    return (
      <SectionCard icon={<IconDisc />} title="引导顺序">
        <FormField label="更改引导顺序" tip="勾选参与引导的设备并调整顺序，仅保存启用的设备">
          <div className="qvm-vf-boot-list">
            {devices.map((dev, index) => (
              <div key={dev.device || dev.file || index} className={`qvm-vf-boot-row${dev.enabled ? '' : ' disabled'}`}>
                <Checkbox checked={dev.enabled} onChange={(e) => toggleDevice(index, !!e.target.checked)} />
                <span className="qvm-vf-boot-icon">{BOOT_ICON[dev.type] || <DiskIcon />}</span>
                <div className="qvm-vf-boot-info">
                  <span className="qvm-vf-boot-type">{DEVICE_TYPE_LABEL[dev.type] || dev.type}</span>
                  <span className="qvm-vf-boot-file" title={dev.file || '（空）'}>
                    {dev.type === 'network' ? `MAC: ${dev.file || '-'}` : dev.file || '（空）'}
                  </span>
                </div>
                {dev.type !== 'network' && (dev.device || dev.bus) && (
                  <div className="qvm-vf-boot-meta">
                    <Tag size="small" color="blue">设备: {dev.type === 'cdrom' ? 'cdrom' : dev.device}</Tag>
                    {dev.bus && <Tag size="small" color="blue">总线: {dev.bus}</Tag>}
                  </div>
                )}
                <div className="qvm-vf-boot-actions">
                  <Button size="small" icon={<IconArrowUp />} disabled={index === 0} onClick={() => moveDeviceUp(index)} />
                  <Button size="small" icon={<IconArrowDown />} disabled={index === devices.length - 1} onClick={() => moveDeviceDown(index)} />
                </div>
              </div>
            ))}
          </div>
        </FormField>
      </SectionCard>
    )
  }

  // 创建模式 / 编辑模式无设备列表时回退类型排序
  return (
    <SectionCard icon={<IconDisc />} title="引导顺序">
      <FormField label="引导顺序">
        <div className="qvm-vf-boot-list">
          {f.boot_order.map((item, index) => {
            const meta = ALL_BOOT_DEVICES.find((d) => d.value === item)
            return (
              <div key={item} className="qvm-vf-boot-row">
                <span className="qvm-vf-boot-icon">{BOOT_ICON[item] || <DiskIcon />}</span>
                <div className="qvm-vf-boot-info">
                  <span className="qvm-vf-boot-type">{meta?.label || item}</span>
                </div>
                <div className="qvm-vf-boot-actions">
                  <Button size="small" icon={<IconArrowUp />} disabled={index === 0} onClick={() => moveUp(index)} />
                  <Button size="small" icon={<IconArrowDown />} disabled={index === f.boot_order.length - 1} onClick={() => moveDown(index)} />
                  <Button size="small" type="danger" theme="light" icon={<IconDelete />} disabled={f.boot_order.length <= 1} onClick={() => remove(index)} />
                </div>
              </div>
            )
          })}
          {availableDevices.length > 0 && (
            <Dropdown
              trigger="click"
              position="bottomLeft"
              clickToHide
              menu={availableDevices.map((dev) => ({
                node: 'item' as const,
                name: dev.label,
                icon: BOOT_ICON[dev.value],
                onClick: () => setField('boot_order', [...f.boot_order, dev.value]),
              }))}
            >
              <Button size="small" type="primary" theme="light" icon={<IconPlus />}>
                添加引导设备
              </Button>
            </Dropdown>
          )}
        </div>
      </FormField>
    </SectionCard>
  )
}
