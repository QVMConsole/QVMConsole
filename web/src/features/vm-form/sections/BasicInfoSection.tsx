/**
 * 基础信息分区（创建向导）
 * 名称 / 批量数量 / 备注 / 系统类型 / 系统版本 / 导入模式系统初始化。
 */
import { useMemo, useState } from 'react'
import { Button, Input, InputNumber, Select, TextArea } from '@douyinfe/semi-ui'
import { IconInfoCircle, IconUser } from '@douyinfe/semi-icons'
import SectionCard from './SectionCard'
import FormField from './FormField'
import TextSwitch from './TextSwitch'
import { useVmFormScope } from '../scopeContext'
import { OS_QUICK_OPTIONS } from '../constants'
import { generateRandomVmName, generateRandomHostname } from '../defaults'
import { validateHostname, validateTemplateUsername, validateVmName } from '../validators'
import { generatePassword } from '@/utils/validate'
import {
  LINUX_TEMPLATE_CATEGORY_OPTIONS,
  WINDOWS_TEMPLATE_CATEGORY_OPTIONS,
} from '@/utils/templateCategory'

export default function BasicInfoSection() {
  const { form, options } = useVmFormScope()
  const { form: f, setField, isTemplateSourceMode, onOsTypeChange } = form
  const [errors, setErrors] = useState<Record<string, string>>({})

  const setError = (key: string, message: string) => {
    setErrors((prev) => {
      const next = { ...prev }
      if (message) next[key] = message
      else delete next[key]
      return next
    })
  }

  const showBatchCount = isTemplateSourceMode && f.system_init_enabled

  // OS 版本按类别过滤 + 前缀分组
  const osVariantGroups = useMemo(() => {
    const category = f.os_type === 'windows' ? 'Windows' : 'Linux'
    const filtered = options.osVariants.filter((v) => v.category === category)
    const groups: Record<string, typeof filtered> = {}
    filtered.forEach((v) => {
      let prefix = v.id.replace(/[\d.]+$/, '')
      if (!prefix) prefix = v.id
      if (!groups[prefix]) groups[prefix] = []
      groups[prefix].push(v)
    })
    return Object.entries(groups)
      .map(([label, items]) => ({ label, items }))
      .sort((a, b) => a.label.localeCompare(b.label))
  }, [options.osVariants, f.os_type])

  // 导入模式系统分类
  const importCategoryOptions = useMemo(() => {
    if (f.os_type === 'windows') {
      return [{ label: 'Windows Server', options: WINDOWS_TEMPLATE_CATEGORY_OPTIONS }]
    }
    if (f.os_type === 'linux') {
      return [{ label: 'Linux 发行版', options: LINUX_TEMPLATE_CATEGORY_OPTIONS }]
    }
    return []
  }, [f.os_type])

  return (
    <>
      <SectionCard icon={<IconInfoCircle />} title="基础信息">
        <FormField
          label="虚拟机名称"
          required
          error={errors.name}
          tip={
            showBatchCount && f.batch_count > 1
              ? `批量模式下名称将作为前缀，最终命名为 ${f.name || 'vm'}-01, ${f.name || 'vm'}-02...`
              : '名称仅支持字母、数字和短横线，且不能以短横线开头或结尾，例如 "web01" 或 "vm-2026"'
          }
        >
          <Input
            value={f.name}
            placeholder="默认自动生成，也可手动修改"
            onChange={(v) => {
              setField('name', v)
              if (errors.name) setError('name', validateVmName(v))
            }}
            onBlur={() => setError('name', validateVmName(f.name))}
            suffix={
              <Button
                size="small"
                theme="borderless"
                type="primary"
                onClick={() => {
                  const name = generateRandomVmName()
                  setField('name', name)
                  setError('name', '')
                }}
              >
                随机生成
              </Button>
            }
          />
        </FormField>

        {showBatchCount && (
          <FormField
            label="创建数量"
            tip={
              f.batch_count > 1
                ? `将创建 ${f.batch_count} 台虚拟机，共 ${f.batch_count * f.ram}GB 内存 / ${f.batch_count * f.disk_size}GB 磁盘。留空密码将为每台虚拟机自动生成独立随机强密码。`
                : undefined
            }
          >
            <InputNumber
              style={{ width: '100%' }}
              value={f.batch_count}
              min={1}
              max={100}
              onChange={(v) => setField('batch_count', Number(v || 1))}
            />
          </FormField>
        )}

        <FormField label="备注">
          <TextArea
            value={f.remark}
            rows={2}
            maxCount={200}
            placeholder="用于记录用途、环境或业务信息"
            onChange={(v) => setField('remark', v)}
          />
        </FormField>

        {!isTemplateSourceMode && (
          <>
            <FormField label="系统类型">
              <div className="qvm-vf-os-cards">
                {OS_QUICK_OPTIONS.map((os) => (
                  <div
                    key={os.value}
                    className={`qvm-vf-os-card${f.os_type === os.value ? ' selected' : ''}`}
                    onClick={() => onOsTypeChange(os.value)}
                  >
                    <div className="qvm-vf-os-card-icon">{os.icon}</div>
                    <div className="qvm-vf-os-card-name">{os.label}</div>
                    <div className="qvm-vf-os-card-examples">{os.examples}</div>
                  </div>
                ))}
              </div>
            </FormField>

            {f.create_mode === 'iso' && (
              <FormField label="系统版本">
                <Select
                  style={{ width: '100%' }}
                  value={f.os_variant || undefined}
                  placeholder="选择系统版本（可搜索）"
                  filter
                  showClear
                  onFocus={() => void options.loadOSVariants()}
                  onChange={(v) => setField('os_variant', (v as string) || '')}
                >
                  {osVariantGroups.map((group) => (
                    <Select.OptGroup key={group.label} label={group.label}>
                      {group.items.map((v) => (
                        <Select.Option key={v.id} value={v.id}>
                          {v.name}
                        </Select.Option>
                      ))}
                    </Select.OptGroup>
                  ))}
                </Select>
              </FormField>
            )}
          </>
        )}
      </SectionCard>

      {/* 导入模式：系统初始化 */}
      {f.create_mode === 'import' && (
        <SectionCard icon={<IconUser />} title="系统初始化">
          <FormField
            label="系统初始化"
            tip={
              f.system_init_enabled
                ? '导入后将注入主机名、用户名和密码，完成系统初始化'
                : '仅导入磁盘并创建虚拟机定义，不进行系统初始化'
            }
          >
            <TextSwitch
              checked={f.system_init_enabled}
              onChange={(v) => setField('system_init_enabled', v)}
              checkedText="是"
              uncheckedText="否"
            />
          </FormField>

          {f.system_init_enabled && (
            <>
              <FormField label="系统分类">
                <Select
                  style={{ width: '100%' }}
                  value={f.import_os_category || undefined}
                  placeholder="选择系统分类"
                  showClear
                  onChange={(v) => setField('import_os_category', (v as string) || '')}
                >
                  {importCategoryOptions.map((group) => (
                    <Select.OptGroup key={group.label} label={group.label}>
                      {group.options.map((cat) => (
                        <Select.Option key={cat} value={cat}>
                          {cat}
                        </Select.Option>
                      ))}
                    </Select.OptGroup>
                  ))}
                </Select>
              </FormField>

              <div className="qvm-vf-grid-2">
                <FormField label="主机名" error={errors.hostname}>
                  <Input
                    value={f.hostname}
                    placeholder="自动使用虚拟机名称"
                    onChange={(v) => {
                      setField('hostname', v)
                      if (errors.hostname) setError('hostname', validateHostname(v))
                    }}
                    onBlur={() => setError('hostname', validateHostname(f.hostname))}
                    suffix={
                      <Button
                        size="small"
                        theme="borderless"
                        type="primary"
                        onClick={() => {
                          setField('hostname', generateRandomHostname())
                          setError('hostname', '')
                        }}
                      >
                        随机生成
                      </Button>
                    }
                  />
                </FormField>
                <FormField label="用户名" error={errors.import_user}>
                  <Input
                    value={f.import_user}
                    placeholder="请输入登录用户名"
                    disabled={f.os_type === 'windows'}
                    onChange={(v) => {
                      setField('import_user', v)
                      if (errors.import_user) setError('import_user', validateTemplateUsername(v, false))
                    }}
                    onBlur={() => setError('import_user', validateTemplateUsername(f.import_user, false))}
                  />
                </FormField>
              </div>
              <FormField label="密码">
                <Input
                  mode="password"
                  value={f.import_password}
                  placeholder="请输入密码"
                  autoComplete="new-password"
                  onChange={(v) => setField('import_password', v)}
                  suffix={
                    <Button
                      size="small"
                      theme="borderless"
                      type="primary"
                      onClick={() => setField('import_password', generatePassword())}
                    >
                      生成强密码
                    </Button>
                  }
                />
              </FormField>
            </>
          )}
        </SectionCard>
      )}
    </>
  )
}
