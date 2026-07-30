/**
 * OVF/OVA 虚拟机包来源与配置策略选择。
 */
import { useMemo } from 'react'
import { Banner, Input, Radio, RadioGroup, Select } from '@douyinfe/semi-ui'
import { IconBox } from '@douyinfe/semi-icons'
import type { ApplianceMetadata } from '@/api/storage'
import SectionCard from './SectionCard'
import FormField from './FormField'
import TextSwitch from './TextSwitch'
import { useVmFormScope } from '../scopeContext'

const formatBytes = (bytes: number) => {
  if (!bytes) return '未知容量'
  const gb = bytes / 1024 / 1024 / 1024
  return `${gb >= 10 ? gb.toFixed(0) : gb.toFixed(2)} GB`
}

// 兼容旧版本接口省略空数组，以及第三方虚拟机包未声明网卡等可选信息的情况。
const normalizeApplianceMetadata = (metadata: ApplianceMetadata): ApplianceMetadata => ({
  ...metadata,
  source_format: metadata?.source_format || '',
  disks: Array.isArray(metadata?.disks) ? metadata.disks : [],
  networks: Array.isArray(metadata?.networks) ? metadata.networks : [],
  warnings: Array.isArray(metadata?.warnings) ? metadata.warnings : [],
})

export default function ApplianceImportSection() {
  const { form, options, ctx } = useVmFormScope()
  const { form: f, setField } = form
  const applianceFiles = useMemo(
    () => options.diskFiles.filter((file) => /\.(ova|ovf)$/i.test(file.name)),
    [options.diskFiles],
  )

  const clearCachedMetadata = () => setField('appliance_metadata', null)

  return (
    <SectionCard icon={<IconBox />} title="虚拟机包">
      <RadioGroup
        type="card"
        value={f.appliance_config_mode}
        className="qvm-vf-appliance-mode-list"
        aria-label="虚拟机包配置方式"
        name="appliance-config-mode"
        onChange={(event) => {
          setField('appliance_config_mode', event.target.value === 'custom' ? 'custom' : 'ovf')
          clearCachedMetadata()
        }}
      >
        <Radio
          value="ovf"
          className="qvm-vf-appliance-mode-option"
          extra="创建任务读取并采用包内 CPU、内存、固件、机型、磁盘总线和网卡型号，网络映射到本机默认配置"
        >
          跟随 OVF 配置
        </Radio>
        <Radio
          value="custom"
          className="qvm-vf-appliance-mode-option"
          extra="直接进入完整向导，由当前表单配置覆盖包内硬件、目标存储、网络和高级选项"
        >
          自定义
        </Radio>
      </RadioGroup>

      {f.appliance_config_mode === 'ovf' && (
        <FormField label="虚拟机名称" required tip="包内硬件配置不会覆盖此名称">
          <Input
            value={f.name}
            placeholder="请输入虚拟机名称"
            showClear
            onChange={(value) => setField('name', value)}
          />
        </FormField>
      )}

      {ctx.isAdmin && (
        <FormField label="虚拟机包来源">
          <Radio.Group
            type="button"
            value={f.appliance_source_type}
            onChange={(event) => {
              const source = event.target.value
              setField('appliance_source_type', source)
              setField(source === 'path' ? 'appliance_file' : 'appliance_path', '')
              clearCachedMetadata()
            }}
            options={[
              { label: '从我的存储选择', value: 'storage' },
              { label: '输入绝对路径', value: 'path' },
            ]}
          />
        </FormField>
      )}

      {(!ctx.isAdmin || f.appliance_source_type === 'storage') && (
        <FormField label="OVA / OVF 文件" required tip="OVF 引用的 VMDK 和清单文件需位于同一虚拟磁盘目录">
          <Select
            style={{ width: '100%' }}
            value={f.appliance_file || undefined}
            placeholder="从我的存储选择 .ova 或 .ovf 文件"
            loading={options.diskFilesLoading}
            onFocus={() => void options.loadDiskFiles()}
            onChange={(value) => {
              setField('appliance_file', (value as string) || '')
              clearCachedMetadata()
            }}
          >
            {applianceFiles.map((file) => (
              <Select.Option key={file.name} value={file.name}>
                <div className="qvm-vf-select-file">
                  <span>{file.name}</span>
                  <span>{file.size_text}</span>
                </div>
              </Select.Option>
            ))}
          </Select>
        </FormField>
      )}

      {ctx.isAdmin && f.appliance_source_type === 'path' && (
        <FormField label="虚拟机包路径" required tip="支持 .ova 文件，或 .ovf 描述文件及同目录的配套磁盘">
          <Input
            value={f.appliance_path}
            placeholder="请输入 OVA/OVF 绝对路径"
            showClear
            onChange={(value) => {
              setField('appliance_path', value)
              clearCachedMetadata()
            }}
          />
        </FormField>
      )}

      <Banner
        type="info"
        closeIcon={null}
        description={
          f.appliance_config_mode === 'ovf'
            ? '点击“按 OVF 配置创建”后立即进入异步任务；包解析、兼容性、清单、空间和配额校验均在任务中执行。'
            : '选择文件后点击“下一步”配置虚拟机；包解析、兼容性、清单、空间和配额校验均在最终任务中执行。'
        }
      />

      <FormField label="源文件保留" tip="关闭后，仅在全部磁盘导入、虚拟机定义和网络配置均成功后删除源包">
        <TextSwitch
          checked={f.copy_source}
          onChange={(value) => setField('copy_source', value)}
          checkedText="留"
          uncheckedText="删"
        />
      </FormField>
      <FormField label="导入后启动" tip="虚拟机包导入不会改变源虚拟机状态">
        <TextSwitch
          checked={f.start_after_import}
          onChange={(value) => setField('start_after_import', value)}
          checkedText="开"
          uncheckedText="停"
        />
      </FormField>
    </SectionCard>
  )
}

export function ApplianceDiskSummarySection() {
  const { form } = useVmFormScope()
  const sourceMetadata = form.form.appliance_metadata
  const metadata = sourceMetadata ? normalizeApplianceMetadata(sourceMetadata) : null
  return (
    <SectionCard icon={<IconBox />} title="虚拟机包磁盘">
      <div className="qvm-vf-tip">包内声明的全部磁盘都会转换为 QCOW2 并导入到所选目标存储。</div>
      {metadata ? (
        <div className="qvm-vf-appliance-disk-list">
          {metadata.disks.map((disk, index) => (
            <div key={disk.id}>
              <strong>{disk.is_system ? '系统盘' : `数据盘 ${index}`}</strong>
              <span>{disk.file_ref}</span>
              <span>{formatBytes(disk.capacity_bytes)}</span>
              <span>{disk.bus || 'scsi'}</span>
            </div>
          ))}
        </div>
      ) : (
        <Banner
          type="info"
          closeIcon={null}
          description="磁盘清单将在异步任务中读取；包内全部声明磁盘都会导入，无需在提交前重复检查。"
        />
      )}
    </SectionCard>
  )
}
