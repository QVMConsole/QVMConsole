/**
 * 存储位置分区（创建向导）
 * 选择虚拟机硬盘存放的存储池，留空使用默认存储位置。
 */
import { Select, Tag } from '@douyinfe/semi-ui'
import { DiskIcon } from '../icons'
import SectionCard from './SectionCard'
import FormField from './FormField'
import { useVmFormScope } from '../scopeContext'
import { formatBytes } from '@/utils/format'

export default function StoragePoolSection() {
  const { form, options } = useVmFormScope()
  const { form: f, setField } = form

  return (
    <SectionCard icon={<DiskIcon />} title="存储位置">
      <FormField label="虚拟机硬盘" tip="留空时使用管理员设置的默认存储位置，没有默认时回退系统克隆目录">
        <Select
          style={{ width: '100%' }}
          value={f.storage_pool_id || undefined}
          placeholder="使用默认存储位置"
          showClear
          filter
          onChange={(v) => setField('storage_pool_id', (v as string) || '')}
        >
          {options.storageTargets.map((target) => (
            <Select.Option key={target.id} value={target.id}>
              <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', gap: 8 }}>
                <span>{target.display_name}</span>
                <span style={{ display: 'flex', gap: 4 }}>
                  {target.is_default && <Tag size="small" color="green">默认</Tag>}
                  <Tag size="small" color="blue">{formatBytes(target.available || 0)} 可用</Tag>
                </span>
              </div>
            </Select.Option>
          ))}
        </Select>
      </FormField>
    </SectionCard>
  )
}
