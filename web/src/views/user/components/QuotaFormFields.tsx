/**
 * 用户级配额表单字段（创建用户 / 编辑用户配置共用）
 * 对应旧版 QuotaForm.vue：计算 / 存储 / 运行时长 / 网络（端口转发、公网 IP、快照、带宽、流量）
 * 支持编辑模式下展示当前使用量与进度条
 */
import { InputNumber, Progress } from '@douyinfe/semi-ui'
import type { UserQuotaPayload, UserQuotaUsage } from '@/api/user'
import TextSwitch from '@/features/vm-form/sections/TextSwitch'
import { quotaPercent } from '../utils'

interface QuotaFormFieldsProps {
  value: UserQuotaPayload
  onChange: (patch: Partial<UserQuotaPayload>) => void
  /** 编辑模式下展示使用量 */
  usage?: UserQuotaUsage | null
}

/** 单个数字配额项（右侧可选使用量标注） */
function QuotaNumberField({
  label,
  value,
  onChange,
  max,
  precision,
  suffix,
  usageText,
  usagePercent,
  usageDanger,
}: {
  label: string
  value: number
  onChange: (v: number) => void
  max: number
  precision?: number
  suffix?: string
  usageText?: string
  usagePercent?: number
  usageDanger?: boolean
}) {
  return (
    <div className="usr-quota-field">
      <div className="usr-quota-field-label">{label}</div>
      <div className="usr-quota-field-row">
        <InputNumber
          value={value}
          onNumberChange={(v) => onChange(Number(v) || 0)}
          min={0}
          max={max}
          precision={precision}
          suffix={suffix}
          style={{ width: '100%' }}
        />
        {usageText !== undefined && (
          <span className="usr-quota-used">
            {usageText}
            {usagePercent !== undefined && usagePercent > 0 && (
              <Progress
                percent={usagePercent}
                stroke={usageDanger ? 'var(--semi-color-danger)' : undefined}
                showInfo={false}
                style={{ marginTop: 2 }}
              />
            )}
          </span>
        )}
      </div>
    </div>
  )
}

export default function QuotaFormFields({ value, onChange, usage }: QuotaFormFieldsProps) {
  const showUsage = !!usage
  /** 生成「已用/上限」标注文本（上限为 0 时仅显示已用） */
  const usageLabel = (max: number, usedText: string | number, unit = '') =>
    showUsage ? (max > 0 ? `${usedText}/${max}${unit}` : `已用 ${usedText}${unit}`) : undefined

  return (
    <div className="usr-quota-form">
      <div className="usr-quota-section">
        <span className="usr-quota-section-title">计算资源</span>
        <span className="usr-quota-section-tip">设为 0 表示不限制</span>
      </div>
      <div className="usr-quota-grid cols-3">
        <QuotaNumberField
          label="CPU 核心数"
          value={value.max_cpu}
          onChange={(v) => onChange({ max_cpu: v })}
          max={256}
          usageText={usageLabel(value.max_cpu, usage?.used_cpu || 0)}
          usagePercent={quotaPercent(value.max_cpu, usage?.used_cpu)}
        />
        <QuotaNumberField
          label="内存 (GB)"
          value={value.max_memory}
          onChange={(v) => onChange({ max_memory: v })}
          max={4096}
          usageText={usageLabel(value.max_memory, usage?.used_memory || 0, 'GB')}
          usagePercent={quotaPercent(value.max_memory, usage?.used_memory)}
        />
        <QuotaNumberField
          label="VM 数量"
          value={value.max_vm}
          onChange={(v) => onChange({ max_vm: v })}
          max={1000}
          usageText={usageLabel(value.max_vm, usage?.used_vm || 0)}
          usagePercent={quotaPercent(value.max_vm, usage?.used_vm)}
        />
      </div>

      <div className="usr-quota-section">
        <span className="usr-quota-section-title">存储资源</span>
        <span className="usr-quota-section-tip">设为 0 表示不限制</span>
      </div>
      <div className="usr-quota-grid cols-2">
        <QuotaNumberField
          label="磁盘 (GB)"
          value={value.max_disk}
          onChange={(v) => onChange({ max_disk: v })}
          max={102400}
          usageText={usageLabel(value.max_disk, usage?.used_disk || 0, 'GB')}
          usagePercent={quotaPercent(value.max_disk, usage?.used_disk)}
        />
        <QuotaNumberField
          label="存储配额 (GB)"
          value={value.max_storage}
          onChange={(v) => onChange({ max_storage: v })}
          max={102400}
          usageText={
            showUsage
              ? value.max_storage > 0
                ? `${usage?.used_storage_gb || '0 B'}/${value.max_storage}GB`
                : `已用 ${usage?.used_storage_gb || '0 B'}`
              : undefined
          }
          usagePercent={quotaPercent(
            value.max_storage * 1073741824,
            usage?.used_storage,
          )}
        />
      </div>

      <div className="usr-quota-section">
        <span className="usr-quota-section-title">运行时长配额</span>
        <span className="usr-quota-section-tip">设为 0 表示不限制，耗尽后无法开机</span>
      </div>
      <div className="usr-quota-grid cols-2">
        <QuotaNumberField
          label="总运行时长 (小时)"
          value={value.max_runtime_hours}
          onChange={(v) => onChange({ max_runtime_hours: v })}
          max={1000000}
          usageText={
            showUsage
              ? value.max_runtime_hours > 0
                ? `${usage?.used_runtime_display || '0秒'}/${value.max_runtime_hours}小时`
                : `已用 ${usage?.used_runtime_display || '0秒'}`
              : undefined
          }
          usagePercent={quotaPercent(
            value.max_runtime_hours * 3600,
            usage?.used_runtime_seconds,
          )}
          usageDanger={usage?.runtime_quota_reached}
        />
      </div>

      <div className="usr-quota-section">
        <span className="usr-quota-section-title">网络资源</span>
        <span className="usr-quota-section-tip">设为 0 表示不限制</span>
      </div>
      <div className="usr-quota-grid cols-2">
        <div className="usr-quota-field">
          <div className="usr-quota-field-label">端口转发</div>
          <div className="usr-quota-field-row" style={{ minHeight: 32 }}>
            <TextSwitch
              checked={value.enable_port_forward}
              onChange={(v) => onChange({ enable_port_forward: v })}
              checkedText="开"
              uncheckedText="关"
            />
            {showUsage && value.enable_port_forward && (
              <span className="usr-quota-used">
                {value.max_port_forwards > 0
                  ? `${usage?.used_port_forwards || 0}/${value.max_port_forwards}`
                  : `已用 ${usage?.used_port_forwards || 0}`}
              </span>
            )}
          </div>
        </div>
        {value.enable_port_forward && (
          <QuotaNumberField
            label="转发上限"
            value={value.max_port_forwards}
            onChange={(v) => onChange({ max_port_forwards: v })}
            max={100000}
          />
        )}
        <QuotaNumberField
          label="公网 IP"
          value={value.max_public_ips}
          onChange={(v) => onChange({ max_public_ips: v })}
          max={10000}
          usageText={usageLabel(value.max_public_ips, usage?.used_public_ips || 0)}
          usagePercent={quotaPercent(value.max_public_ips, usage?.used_public_ips)}
        />
        <QuotaNumberField
          label="快照数量"
          value={value.max_snapshots}
          onChange={(v) => onChange({ max_snapshots: v })}
          max={100000}
          usageText={usageLabel(value.max_snapshots, usage?.used_snapshots || 0)}
          usagePercent={quotaPercent(value.max_snapshots, usage?.used_snapshots)}
        />
      </div>

      <div className="usr-quota-section sub">
        <span className="usr-quota-section-title">带宽配额</span>
        <span className="usr-quota-section-tip">Mbps，设为 0 表示不限制</span>
      </div>
      <div className="usr-quota-grid cols-2">
        <QuotaNumberField
          label="下行带宽"
          value={value.max_bandwidth_down}
          onChange={(v) => onChange({ max_bandwidth_down: v })}
          max={100000}
          suffix="Mbps"
        />
        <QuotaNumberField
          label="上行带宽"
          value={value.max_bandwidth_up}
          onChange={(v) => onChange({ max_bandwidth_up: v })}
          max={100000}
          suffix="Mbps"
        />
      </div>

      <div className="usr-quota-section sub">
        <span className="usr-quota-section-title">流量配额</span>
        <span className="usr-quota-section-tip">GB/月，超限后限速：下行10Mbps/上行1Mbps</span>
      </div>
      <div className="usr-quota-grid cols-2">
        <QuotaNumberField
          label="下行流量"
          value={value.max_traffic_down}
          onChange={(v) => onChange({ max_traffic_down: v })}
          max={1000000}
          precision={2}
          suffix="GB"
          usageText={
            showUsage
              ? value.max_traffic_down > 0
                ? `${usage?.used_traffic_down_gb || '0 B'}/${value.max_traffic_down}GB`
                : `已用 ${usage?.used_traffic_down_gb || '0 B'}`
              : undefined
          }
          usagePercent={quotaPercent(
            value.max_traffic_down * 1073741824,
            usage?.used_traffic_down,
          )}
          usageDanger={usage?.is_limited_down}
        />
        <QuotaNumberField
          label="上行流量"
          value={value.max_traffic_up}
          onChange={(v) => onChange({ max_traffic_up: v })}
          max={1000000}
          precision={2}
          suffix="GB"
          usageText={
            showUsage
              ? value.max_traffic_up > 0
                ? `${usage?.used_traffic_up_gb || '0 B'}/${value.max_traffic_up}GB`
                : `已用 ${usage?.used_traffic_up_gb || '0 B'}`
              : undefined
          }
          usagePercent={quotaPercent(
            value.max_traffic_up * 1073741824,
            usage?.used_traffic_up,
          )}
          usageDanger={usage?.is_limited_up}
        />
      </div>
    </div>
  )
}
