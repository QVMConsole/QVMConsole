/**
 * 编辑轻量云单 VM 配额弹窗
 * - 草稿行：仅本地更新，随注册列表一并保存
 * - 已保存行：调用后端接口立即生效（已开通 VM 会即时更新带宽限制）
 */
import { useState } from 'react'
import { Banner, InputNumber, Modal, Toast } from '@douyinfe/semi-ui'
import {
  updateLightweightVmQuota,
  type LightweightVmQuotaItem,
  type LightweightVmQuotaPayload,
  type LightweightVmRegistrationItem,
} from '@/api/user'
import { buildLightweightQuotaPayload } from '../utils'
import { useMountModalLifecycle } from '@/hooks/useMountModalLifecycle'

interface RegistrationQuotaDialogProps {
  username: string
  /** 待编辑行（含 vm_name 与当前配额值；draft=true 表示本地草稿） */
  row: LightweightVmQuotaPayload & { draft?: boolean }
  onClose: () => void
  /** 草稿：本地回填；已保存：接口返回后回填 */
  onSaved: (
    payload: LightweightVmQuotaPayload,
    serverData?: {
      registration?: LightweightVmRegistrationItem
      quota?: LightweightVmQuotaItem
    },
  ) => void
}

/** 数字字段配置 */
const FIELDS: { key: keyof LightweightVmQuotaPayload; label: string; precision?: number }[] = [
  { key: 'traffic_down_gb', label: '下行月流量 (GB)', precision: 2 },
  { key: 'traffic_up_gb', label: '上行月流量 (GB)', precision: 2 },
  { key: 'bandwidth_down_mbps', label: '下行带宽 (Mbps)' },
  { key: 'bandwidth_up_mbps', label: '上行带宽 (Mbps)' },
  { key: 'max_port_forwards', label: '端口转发上限' },
  { key: 'max_snapshots', label: '快照上限' },
  { key: 'max_runtime_hours', label: '运行时长配额 (小时)' },
]

export default function RegistrationQuotaDialog({
  username,
  row,
  onClose,
  onSaved,
}: RegistrationQuotaDialogProps) {
  const { modalVisible, requestClose, afterModalClose } = useMountModalLifecycle(onClose)
  const [submitting, setSubmitting] = useState(false)
  const [form, setForm] = useState<LightweightVmQuotaPayload>(buildLightweightQuotaPayload(row))

  const handleSubmit = async () => {
    const payload = buildLightweightQuotaPayload(form)
    // 草稿行仅本地更新
    if (row.draft) {
      onSaved(payload)
      Toast.success('注册草稿配额已更新')
      requestClose()
      return
    }
    setSubmitting(true)
    try {
      const res = await updateLightweightVmQuota(username, payload)
      Toast.success(res.message || '轻量云 VM 配额已更新')
      onSaved(payload, res.data)
      requestClose()
    } catch {
      // 请求层已提示
    } finally {
      setSubmitting(false)
    }
  }

  return (
    <Modal
      title="编辑轻量云 VM 配额"
      visible={modalVisible}
      afterClose={afterModalClose}
      onCancel={requestClose}
      onOk={() => void handleSubmit()}
      okText="保存"
      cancelText="取消"
      confirmLoading={submitting}
      width={520}
      closeOnEsc
    >
      <Banner
        type="info"
        closeIcon={null}
        description={
          <span>
            VM：<strong>{row.vm_name}</strong>，修改已开通 VM 会立即更新带宽限制。
          </span>
        }
        style={{ marginBottom: 16 }}
      />
      {FIELDS.map((f) => (
        <div className="qvm-form-item" key={f.key}>
          <div className="qvm-form-label">{f.label}</div>
          <InputNumber
            value={Number(form[f.key]) || 0}
            onNumberChange={(v) =>
              setForm((prev) => ({ ...prev, [f.key]: Number(v) || 0 }))
            }
            min={0}
            precision={f.precision}
            style={{ width: '100%' }}
          />
        </div>
      ))}
    </Modal>
  )
}
