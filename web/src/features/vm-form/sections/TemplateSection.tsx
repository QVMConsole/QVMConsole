/**
 * 模板克隆分区（创建向导 · 模板模式）
 * 模板选择 / 克隆模式 / 系统初始化 / 登录凭据 / FnOS 设备 ID / OpenWrt 网络 / 登记模式摘要。
 */
import { useMemo, useState } from 'react'
import { Banner, Button, Input, InputNumber, Radio, Select, Tooltip } from '@douyinfe/semi-ui'
import { IconCopy, IconHelpCircle, IconInfoCircle, IconSetting, IconUser } from '@douyinfe/semi-icons'
import SectionCard from './SectionCard'
import FormField from './FormField'
import TextSwitch from './TextSwitch'
import { useVmFormScope } from '../scopeContext'
import { DISK_BUS_OPTIONS } from '../constants'
import { generateRandomHostname } from '../defaults'
import { STRONG_PASSWORD_MIN_LENGTH } from '@/utils/validate'
import {
  validateFnosDeviceId,
  validateHostname,
  validateTemplateDiskSize,
  validateTemplatePassword,
  validateTemplateUsername,
} from '../validators'
import { generatePassword } from '@/utils/validate'
import { resolveTemplateMinDiskSize } from '@/views/vm/utils'
import { templateCategoryLabel, templateGroupLabel } from '@/utils/templateCategory'
import type { TemplateItem } from '@/api/template'

export default function TemplateSection() {
  const { form, options, ctx } = useVmFormScope()
  const {
    form: f,
    setField,
    isWindowsTemplate,
    isFnOSTemplate,
    isOpenWrtTemplate,
    disableSystemInit,
    onTemplateChange,
  } = form
  const [errors, setErrors] = useState<Record<string, string>>({})

  const setError = (key: string, message: string) => {
    setErrors((prev) => {
      const next = { ...prev }
      if (message) next[key] = message
      else delete next[key]
      return next
    })
  }

  const selectedTemplate = useMemo(
    () => options.templates.find((tpl) => tpl.name === f.template) || null,
    [options.templates, f.template],
  )
  const isNoInitTemplate = selectedTemplate?.cloud_init_mode === 'none'
  const isOtherTemplate = selectedTemplate?.type === 'other'
  const templateMinDiskSize = resolveTemplateMinDiskSize(selectedTemplate)
  const registrationMode = ctx.registration.enabled

  // 模板分组选项
  const groupedTemplates = useMemo(() => {
    const groups: Record<string, TemplateItem[]> = {}
    options.templates
      .filter((tpl) => !tpl.disabled)
      .forEach((tpl) => {
        const key = templateGroupLabel(tpl.type || '', tpl.category)
        if (!groups[key]) groups[key] = []
        groups[key].push(tpl)
      })
    return Object.entries(groups)
      .filter(([, items]) => items.length > 0)
      .map(([label, items]) => ({
        label,
        items: items
          .slice()
          .sort((a, b) => (a.display_name || a.name).localeCompare(b.display_name || b.name)),
      }))
      .sort((a, b) => a.label.localeCompare(b.label))
  }, [options.templates])

  const templateOptionLabel = (tpl: TemplateItem): string => {
    const display = tpl.display_name || tpl.admin_name || tpl.name
    const categoryLabel = templateCategoryLabel(tpl.type || '', tpl.category)
    const prefix = categoryLabel ? `${categoryLabel} / ` : ''
    if (ctx.isAdmin) {
      const status = tpl.clone_visible ? '用户可见' : '仅管理员'
      const indent = (tpl.level || 0) > 0 ? `${'　'.repeat(tpl.level || 0)}└ ` : ''
      return `${indent}${prefix}${tpl.admin_name || tpl.name} / ${display}（${status}）`
    }
    return `${prefix}${display}`
  }

  const handleTemplateChange = async (name: string) => {
    setField('template', name)
    // 切换模板时刷新列表，确保刚编辑过的默认配置能立即带出
    const list = await options.loadTemplates(true)
    const tpl = list.find((t) => t.name === name) || null
    onTemplateChange(tpl)
    if (tpl?.type === 'other') {
      setField('system_init_enabled', false)
    }
    setError('disk_size', validateTemplateDiskSize(f.disk_size, true, resolveTemplateMinDiskSize(tpl)))
  }

  const templateUserTip = isWindowsTemplate
    ? 'Windows 模板默认使用 administrator 账号；Windows Server 请保持默认，不支持修改。'
    : isFnOSTemplate
      ? 'fnOS 会将该账号离线注入为首次管理员账号，克隆完成后可直接在网页登录；仅支持小写字母、数字、下划线和短横线，且需以字母或下划线开头'
      : '仅支持小写字母、数字、下划线和短横线，且需以字母或下划线开头'

  const passwordBaseTip = `至少 ${STRONG_PASSWORD_MIN_LENGTH} 位（支持 !@#$%^&*_-+=?）`
  const templatePasswordTip = isFnOSTemplate
    ? `fnOS 会将该密码作为首次管理员网页登录密码；${passwordBaseTip}`
    : passwordBaseTip

  const diskSizeTip =
    templateMinDiskSize > 0
      ? `默认值为模板磁盘大小 ${templateMinDiskSize} GB，且不能小于该值`
      : '选择模板后会自动带出模板磁盘大小，且不能小于模板原始磁盘大小'

  return (
    <>
      {/* 模板选择 */}
      <SectionCard icon={<IconCopy />} title="模板选择">
        <FormField label="选择模板" required error={errors.template}>
          <Select
            style={{ width: '100%' }}
            value={f.template || undefined}
            placeholder="选择模板"
            filter
            onFocus={() => void options.loadTemplates(true)}
            onChange={(v) => void handleTemplateChange(v as string)}
          >
            {groupedTemplates.map((group) => (
              <Select.OptGroup key={group.label} label={group.label}>
                {group.items.map((tpl) => (
                  <Select.Option key={tpl.name} value={tpl.name}>
                    {templateOptionLabel(tpl)}
                  </Select.Option>
                ))}
              </Select.OptGroup>
            ))}
          </Select>
        </FormField>

        <div className="qvm-vf-grid-2">
          <FormField label="磁盘大小（GB）" required tip={diskSizeTip} error={errors.disk_size}>
            <InputNumber
              style={{ width: '100%' }}
              value={f.disk_size}
              min={templateMinDiskSize || 1}
              max={2000}
              step={10}
              onChange={(v) => {
                setField('disk_size', Number(v || 0))
                if (errors.disk_size) {
                  setError('disk_size', validateTemplateDiskSize(Number(v || 0), true, templateMinDiskSize))
                }
              }}
              onBlur={() => setError('disk_size', validateTemplateDiskSize(f.disk_size, true, templateMinDiskSize))}
            />
          </FormField>
          <FormField label="系统盘驱动" tip="优先按模板记录自动带出；旧模板会回退到当前默认值">
            <Select
              style={{ width: '100%' }}
              value={f.disk_bus}
              onChange={(v) => setField('disk_bus', v as string)}
              optionList={DISK_BUS_OPTIONS.map((item) => ({ value: item.value, label: item.label }))}
            />
          </FormField>
        </div>

        {ctx.isAdmin && (
          <FormField label="系统盘 IOPS">
            <div className="qvm-vf-iops-row">
              <span className="qvm-vf-iops-label">总</span>
              <InputNumber
                size="small"
                style={{ width: 100 }}
                value={f.system_disk_iops_total}
                min={0}
                step={100}
                placeholder="总IOPS"
                onChange={(v) => setField('system_disk_iops_total', Number(v || 0))}
              />
              <span className="qvm-vf-iops-label">读</span>
              <InputNumber
                size="small"
                style={{ width: 100 }}
                value={f.system_disk_iops_read}
                min={0}
                step={100}
                placeholder="读IOPS"
                disabled={f.system_disk_iops_total > 0}
                onChange={(v) => setField('system_disk_iops_read', Number(v || 0))}
              />
              <span className="qvm-vf-iops-label">写</span>
              <InputNumber
                size="small"
                style={{ width: 100 }}
                value={f.system_disk_iops_write}
                min={0}
                step={100}
                placeholder="写IOPS"
                disabled={f.system_disk_iops_total > 0}
                onChange={(v) => setField('system_disk_iops_write', Number(v || 0))}
              />
              <span className="qvm-vf-iops-mutex">互斥</span>
            </div>
          </FormField>
        )}
      </SectionCard>

      {/* 克隆模式与系统初始化 */}
      <SectionCard icon={<IconSetting />} title="克隆模式">
        <FormField
          label="克隆模式"
          tip={
            isOtherTemplate
              ? '其它模板支持链式克隆和完整克隆，但不会执行系统初始化。'
              : f.clone_mode === 'linked'
              ? '基于模板创建 backing_file 链式磁盘，依赖模板存在。磁盘创建速度快，节省存储空间。'
              : '将模板数据完整复制到独立磁盘，不依赖模板，脱离链式条件。磁盘创建较慢，占用完整磁盘空间。'
          }
        >
          <Radio.Group
            type="button"
            value={f.clone_mode}
            onChange={(e) => setField('clone_mode', e.target.value)}
            options={[
              { label: '链式克隆', value: 'linked' },
              { label: '完整克隆', value: 'full' },
            ]}
          />
        </FormField>
        <FormField
          label="系统初始化"
          tip={
            f.system_init_enabled
              ? '克隆后将注入主机名、用户名和密码，完成系统初始化'
              : '仅创建磁盘和虚拟机定义，不修改模板内的系统配置。登录凭据需使用模板中已有的账号'
          }
        >
          <TextSwitch
            checked={f.system_init_enabled}
            onChange={(v) => setField('system_init_enabled', v)}
            checkedText="是"
            uncheckedText="否"
            disabled={isOtherTemplate}
          />
        </FormField>
      </SectionCard>

      {!registrationMode && isOtherTemplate && (
        <Banner
          type="warning"
          closeIcon={null}
          style={{ marginBottom: 14 }}
          description="当前为其它模板：可选择链式克隆或完整克隆，但系统不会初始化，也不会修改主机名、用户名、密码或网络配置。"
        />
      )}

      {/* 登记模式摘要 */}
      {registrationMode && (
        <>
          <Banner
            type="info"
            closeIcon={null}
            style={{ marginBottom: 14 }}
            description="当前仅登记服务器配置。登录用户名和密码会由用户登录后确认开通时自行填写，邮件不会包含密码。"
          />
          <SectionCard icon={<IconInfoCircle />} title="网络与配额摘要">
            <FormField label="主机名" error={errors.hostname}>
              <Input
                value={f.hostname}
                placeholder="自动随机生成"
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
            <div className="qvm-vf-summary-grid">
              <div><span>专用 VPC</span><strong>{ctx.registration.dedicated_vpc_label || '管理员已配置'}</strong></div>
              <div><span>网卡类型</span><strong>{f.nic_model || 'virtio'}</strong></div>
              <div><span>系统盘驱动</span><strong>{f.disk_bus || 'virtio'}</strong></div>
              <div><span>月流量</span><strong>下行 {f.traffic_down_gb || 0}GB / 上行 {f.traffic_up_gb || 0}GB</strong></div>
              <div><span>带宽</span><strong>下行 {f.bandwidth_down_mbps || 0}Mbps / 上行 {f.bandwidth_up_mbps || 0}Mbps</strong></div>
              <div><span>端口转发上限</span><strong>{f.max_port_forwards ?? 10}</strong></div>
              <div><span>运行时长配额</span><strong>{f.max_runtime_hours ? `${f.max_runtime_hours}小时` : '不限'}</strong></div>
            </div>
          </SectionCard>
        </>
      )}

      {/* 关闭初始化说明 */}
      {!registrationMode && disableSystemInit && (
        <Banner
          type="warning"
          closeIcon={null}
          style={{ marginBottom: 14 }}
          description="已关闭系统初始化，将不会修改模板内的主机名、用户名、密码或网络配置，只会创建磁盘、定义并启动虚拟机。"
        />
      )}

      {/* 登录凭据 */}
      {!registrationMode && !disableSystemInit && !isNoInitTemplate && !isOpenWrtTemplate && (
        <SectionCard icon={<IconUser />} title="登录凭据">
          <div className="qvm-vf-grid-2">
            <FormField label="主机名" error={errors.hostname}>
              <Input
                value={f.hostname}
                placeholder="自动随机生成"
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
            <FormField
              label={
                <span className="qvm-vf-label-inline">
                  用户名
                  <Tooltip content={templateUserTip} position="top">
                    <IconHelpCircle className="qvm-vf-label-help" size="small" />
                  </Tooltip>
                </span>
              }
              required
              error={errors.import_user}
            >
              <Input
                value={f.import_user}
                placeholder={isWindowsTemplate ? 'administrator' : isFnOSTemplate ? '请输入 fnOS 首次管理员用户名' : '请输入克隆后的登录用户名'}
                disabled={isWindowsTemplate}
                onChange={(v) => {
                  setField('import_user', v)
                  if (errors.import_user) setError('import_user', validateTemplateUsername(v, isWindowsTemplate))
                }}
                onBlur={() => setError('import_user', validateTemplateUsername(f.import_user, isWindowsTemplate))}
              />
            </FormField>
          </div>
          <FormField
            label={
              <span className="qvm-vf-label-inline">
                密码
                <Tooltip content={templatePasswordTip} position="top">
                  <IconHelpCircle className="qvm-vf-label-help" size="small" />
                </Tooltip>
              </span>
            }
            required={f.batch_count <= 1}
            error={errors.import_password}
            tip={f.batch_count > 1 ? '批量创建留空将为每台虚拟机自动生成独立随机强密码' : undefined}
          >
            <Input
              mode="password"
              value={f.import_password}
              placeholder="请输入强密码"
              autoComplete="new-password"
              onChange={(v) => {
                setField('import_password', v)
                if (errors.import_password) setError('import_password', validateTemplatePassword(v, f.batch_count))
              }}
              onBlur={() => setError('import_password', validateTemplatePassword(f.import_password, f.batch_count))}
              suffix={
                <Button
                  size="small"
                  theme="borderless"
                  type="primary"
                  onClick={() => {
                    setField('import_password', generatePassword())
                    setError('import_password', '')
                  }}
                >
                  生成强密码
                </Button>
              }
            />
          </FormField>
        </SectionCard>
      )}

      {/* 不初始化模板说明 */}
      {!registrationMode && !disableSystemInit && isNoInitTemplate && (
        <Banner
          type="info"
          closeIcon={null}
          style={{ marginBottom: 14 }}
          description="该模板已设置为「不初始化」，克隆时将直接复制磁盘，不会注入用户名、密码和主机名。登录凭据需使用模板中已有的账号。"
        />
      )}

      {/* FnOS 设备 ID */}
      {isFnOSTemplate && !disableSystemInit && (
        <SectionCard icon={<IconInfoCircle />} title="FnOS 标识">
          <FormField label="设备 ID" tip="适合需要特殊使用设备ID的授权场景">
            <Radio.Group
              type="button"
              value={f.fnos_device_id_mode}
              onChange={(e) => setField('fnos_device_id_mode', e.target.value)}
              options={[
                { label: '重新生成', value: 'regenerate' },
                { label: '保留设备 ID', value: 'preserve' },
                { label: '指定设备 ID', value: 'custom' },
              ]}
            />
            {f.fnos_device_id_mode === 'custom' && (
              <div style={{ marginTop: 8 }}>
                <Input
                  style={{ maxWidth: 420 }}
                  value={f.fnos_device_id}
                  placeholder="请输入 32 位或 40 位十六进制设备 ID"
                  onChange={(v) => {
                    setField('fnos_device_id', v)
                    if (errors.fnos_device_id) setError('fnos_device_id', validateFnosDeviceId(v))
                  }}
                  onBlur={() => setError('fnos_device_id', validateFnosDeviceId(f.fnos_device_id))}
                />
                {errors.fnos_device_id && <div className="qvm-vf-tip error">{errors.fnos_device_id}</div>}
              </div>
            )}
          </FormField>
        </SectionCard>
      )}

      {/* OpenWrt 网络配置 */}
      {isOpenWrtTemplate && !disableSystemInit && (
        <SectionCard icon={<IconInfoCircle />} title="OpenWrt 网络配置">
          <FormField label="主机名">
            <Input
              value={f.hostname}
              placeholder="自动随机生成"
              onChange={(v) => setField('hostname', v)}
              suffix={
                <Button
                  size="small"
                  theme="borderless"
                  type="primary"
                  onClick={() => setField('hostname', generateRandomHostname())}
                >
                  随机生成
                </Button>
              }
            />
          </FormField>
          <FormField
            label="静态 IP"
            required
            error={errors.static_ip}
            tip="克隆后第一个网卡（eth0/br-lan）将被设置为此静态 IP，格式为 IP/子网掩码"
          >
            <Input
              value={f.static_ip}
              placeholder="如 192.168.1.100/24"
              onChange={(v) => {
                setField('static_ip', v)
                if (errors.static_ip) setError('static_ip', '')
              }}
            />
          </FormField>
          <div className="qvm-vf-grid-2">
            <FormField label="网关">
              <Input value={f.gateway} placeholder="如 192.168.1.1" onChange={(v) => setField('gateway', v)} />
            </FormField>
            <FormField label="DNS">
              <Input value={f.dns} placeholder="如 8.8.8.8,114.114.114.114" onChange={(v) => setField('dns', v)} />
            </FormField>
          </div>
          <FormField label="Root 密码" tip="OpenWrt 默认只有 root 账户，密码可选（留空则不修改）">
            <Input
              mode="password"
              value={f.import_password}
              placeholder="留空则保持模板原始密码"
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
        </SectionCard>
      )}
    </>
  )
}
