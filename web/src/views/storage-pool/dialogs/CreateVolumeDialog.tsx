/**
 * 创建存储卷弹窗（高风险，任务队列）—— 两步向导
 * 1. 选择存储卷类型（LVM 可用；Btrfs 预留入口）
 * 2. LVM 配置：PV 选择 → 卷组 → 逻辑卷 → 文件系统与挂载
 * 选中的磁盘会被初始化为物理卷，数据全部清除
 */
import { useEffect, useState } from 'react'
import {
  Banner,
  Button,
  Checkbox,
  Input,
  InputNumber,
  Modal,
  Select,
  Spin,
  Toast,
} from '@douyinfe/semi-ui'
import { IconBox, IconBranch, IconInfoCircle } from '@douyinfe/semi-icons'
import type { HostStoragePoolInfo, LVMVolumePayload } from '@/api/storagePool'
import { createLVMVolume, getAvailablePVTargets } from '@/api/storagePool'
import { formatBytes } from '@/utils/format'
import TextSwitch from '@/features/vm-form/sections/TextSwitch'
import { useMountModalLifecycle } from '@/hooks/useMountModalLifecycle'

interface CreateVolumeDialogProps {
  onClose: () => void
  onSubmitted: () => void
}

type VolumeStep = 'type' | 'config'

/** LVM 表单初始值 */
const INIT_FORM: LVMVolumePayload = {
  device_ids: [],
  vg_name: '',
  pe_size: '4M',
  lv_name: '',
  lv_size: '',
  lv_type: 'linear',
  stripes: 2,
  mirrors: 1,
  fs_type: 'ext4',
  mount_path: '',
  add_fstab: true,
}

export default function CreateVolumeDialog({ onClose, onSubmitted }: CreateVolumeDialogProps) {
  const { modalVisible, requestClose, afterModalClose } = useMountModalLifecycle(onClose)
  const [step, setStep] = useState<VolumeStep>('type')
  const [volumeType, setVolumeType] = useState('lvm')
  const [pvTargets, setPvTargets] = useState<HostStoragePoolInfo[]>([])
  const [pvLoading, setPvLoading] = useState(true)
  const [form, setForm] = useState<LVMVolumePayload>(INIT_FORM)
  const [confirmed, setConfirmed] = useState(false)
  const [submitting, setSubmitting] = useState(false)

  // 打开时预加载可用 PV 磁盘
  useEffect(() => {
    let mounted = true
    setPvLoading(true)
    getAvailablePVTargets()
      .then((res) => {
        if (mounted) setPvTargets(res.data || [])
      })
      .catch(() => {
        if (mounted) setPvTargets([])
      })
      .finally(() => {
        if (mounted) setPvLoading(false)
      })
    return () => {
      mounted = false
    }
  }, [])

  const patchForm = (patch: Partial<LVMVolumePayload>) =>
    setForm((prev) => ({ ...prev, ...patch }))

  // 提交按钮可用条件（与旧版一致）
  const canSubmit =
    confirmed &&
    form.vg_name.trim() !== '' &&
    form.lv_name.trim() !== '' &&
    form.lv_size.trim() !== '' &&
    form.device_ids.length > 0

  const handleSubmit = async () => {
    setSubmitting(true)
    try {
      await createLVMVolume({
        ...form,
        vg_name: form.vg_name.trim(),
        lv_name: form.lv_name.trim(),
        lv_size: form.lv_size.trim(),
        mount_path: form.mount_path.trim(),
      })
      Toast.success('创建 LVM 存储卷任务已提交，请在任务中心查看进度')
      onSubmitted()
      requestClose()
    } catch {
      // 错误提示由请求层统一处理
    } finally {
      setSubmitting(false)
    }
  }

  return (
    <Modal
      title="创建存储卷"
      visible={modalVisible}
      afterClose={afterModalClose}
      onCancel={requestClose}
      width={680}
      closeOnEsc
      footer={
        step === 'type' ? (
          <>
            <Button onClick={requestClose}>取消</Button>
            <Button type="primary" theme="solid" disabled={!volumeType} onClick={() => setStep('config')}>
              下一步：配置卷
            </Button>
          </>
        ) : (
          <>
            <Button onClick={() => setStep('type')}>上一步</Button>
            <Button onClick={requestClose}>取消</Button>
            <Button
              type="primary"
              theme="solid"
              disabled={!canSubmit}
              loading={submitting}
              onClick={() => void handleSubmit()}
            >
              提交任务
            </Button>
          </>
        )
      }
    >
      {/* ==================== 第一步：选择类型 ==================== */}
      {step === 'type' && (
        <>
          <Banner
            type="info"
            closeIcon={null}
            style={{ marginBottom: 16 }}
            description="选择一种存储卷类型。LVM 存储卷支持将多个磁盘合并为一个逻辑卷组，实现容量聚合和灵活管理。"
          />
          <div className="sp-vol-type-row">
            <div
              className={`sp-vol-type-card ${volumeType === 'lvm' ? 'selected' : ''}`}
              onClick={() => setVolumeType('lvm')}
            >
              <IconBranch size="extra-large" className="sp-vol-type-icon lvm" />
              <div className="sp-vol-type-name">LVM 存储卷</div>
              <div className="sp-vol-type-desc">支持条带、镜像、多磁盘合并</div>
            </div>
            <div className="sp-vol-type-card disabled">
              <IconBox size="extra-large" className="sp-vol-type-icon" />
              <div className="sp-vol-type-name">Btrfs 存储池</div>
              <div className="sp-vol-type-desc">即将推出</div>
            </div>
          </div>
        </>
      )}

      {/* ==================== 第二步：LVM 配置 ==================== */}
      {step === 'config' && (
        <Spin spinning={pvLoading}>
          <Banner
            type="warning"
            closeIcon={null}
            style={{ marginBottom: 16 }}
            description="此操作会将选中的磁盘初始化为 LVM 物理卷，并创建卷组和逻辑卷。磁盘上的所有数据将被清除。"
          />

          {/* PV 选择 */}
          <div className="qvm-form-item">
            <div className="qvm-form-label required">物理卷 (PV) 选择</div>
            {pvTargets.length === 0 && !pvLoading ? (
              <Banner
                type="info"
                closeIcon={null}
                description="未找到可用的磁盘设备。请确保有未挂载、非系统盘的磁盘。"
              />
            ) : (
              <Checkbox.Group
                style={{ display: 'flex', flexDirection: 'column', gap: 8 }}
                value={form.device_ids}
                onChange={(values) => patchForm({ device_ids: values as string[] })}
              >
                {pvTargets.map((disk) => (
                  <div key={disk.id} className="sp-pv-item">
                    <Checkbox value={disk.id} style={{ width: '100%' }}>
                      <div className="sp-pv-item-inner">
                        <span className="sp-pv-item-name">{disk.display_name}</span>
                        <span className="sp-pv-item-meta">
                          {disk.device_path} · {formatBytes(disk.size)}
                        </span>
                      </div>
                    </Checkbox>
                  </div>
                ))}
              </Checkbox.Group>
            )}
          </div>

          {/* 卷组配置 */}
          <div className="sp-vol-section">卷组 (VG) 配置</div>
          <div className="sp-vol-grid cols-2">
            <div className="qvm-form-item">
              <div className="qvm-form-label required">卷组名称</div>
              <Input
                value={form.vg_name}
                onChange={(v) => patchForm({ vg_name: v })}
                placeholder="例如: vg-storage"
              />
            </div>
            <div className="qvm-form-item">
              <div className="qvm-form-label">PE 大小</div>
              <Select
                value={form.pe_size}
                onChange={(v) => patchForm({ pe_size: v as string })}
                style={{ width: '100%' }}
                optionList={[
                  { label: '4M（默认，推荐）', value: '4M' },
                  { label: '8M', value: '8M' },
                  { label: '16M', value: '16M' },
                  { label: '32M', value: '32M' },
                  { label: '64M', value: '64M' },
                ]}
              />
            </div>
          </div>

          {/* 逻辑卷配置 */}
          <div className="sp-vol-section">逻辑卷 (LV) 配置</div>
          <div className="sp-vol-grid cols-2">
            <div className="qvm-form-item">
              <div className="qvm-form-label required">逻辑卷名称</div>
              <Input
                value={form.lv_name}
                onChange={(v) => patchForm({ lv_name: v })}
                placeholder="例如: lv-data"
              />
            </div>
            <div className="qvm-form-item">
              <div className="qvm-form-label required">逻辑卷大小</div>
              <Input
                value={form.lv_size}
                onChange={(v) => patchForm({ lv_size: v })}
                placeholder="10G / 50%VG / 100%FREE"
              />
              <div className="qvm-form-tip">
                <IconInfoCircle size="small" style={{ marginRight: 4, verticalAlign: -2 }} />
                支持绝对值 (10G/500M) 或百分比 (50%VG/100%FREE)
              </div>
            </div>
            <div className="qvm-form-item">
              <div className="qvm-form-label">LV 类型</div>
              <Select
                value={form.lv_type}
                onChange={(v) => patchForm({ lv_type: v as string })}
                style={{ width: '100%' }}
                optionList={[
                  { label: '线性 (linear) — 顺序写入', value: 'linear' },
                  { label: '条带 (striped) — 并行写入，高性能', value: 'striped' },
                  { label: '镜像 (mirrored) — 数据冗余', value: 'mirrored' },
                ]}
              />
            </div>
            {form.lv_type === 'striped' && (
              <div className="qvm-form-item">
                <div className="qvm-form-label">条带数</div>
                <InputNumber
                  value={form.stripes}
                  onChange={(v) => patchForm({ stripes: Number(v) || 2 })}
                  min={2}
                  max={16}
                  style={{ width: '100%' }}
                />
              </div>
            )}
            {form.lv_type === 'mirrored' && (
              <div className="qvm-form-item">
                <div className="qvm-form-label">镜像数</div>
                <InputNumber
                  value={form.mirrors}
                  onChange={(v) => patchForm({ mirrors: Number(v) || 1 })}
                  min={1}
                  max={3}
                  style={{ width: '100%' }}
                />
              </div>
            )}
          </div>

          {/* 文件系统与挂载 */}
          <div className="sp-vol-section">文件系统与挂载</div>
          <div className="sp-vol-grid cols-3">
            <div className="qvm-form-item">
              <div className="qvm-form-label">文件系统</div>
              <Select
                value={form.fs_type}
                onChange={(v) => patchForm({ fs_type: v as string })}
                style={{ width: '100%' }}
                optionList={[
                  { label: 'ext4（推荐）', value: 'ext4' },
                  { label: 'xfs', value: 'xfs' },
                  { label: 'btrfs', value: 'btrfs' },
                  { label: '不格式化', value: 'none' },
                ]}
              />
            </div>
            <div className="qvm-form-item">
              <div className="qvm-form-label">挂载路径</div>
              <Input
                value={form.mount_path}
                onChange={(v) => patchForm({ mount_path: v })}
                placeholder="留空则自动生成 /var/lib/kvm-storage/..."
              />
            </div>
            <div className="qvm-form-item">
              <div className="qvm-form-label">开机自动挂载</div>
              <TextSwitch
                checked={form.add_fstab}
                onChange={(checked) => patchForm({ add_fstab: checked })}
                checkedText="写"
                uncheckedText="否"
              />
            </div>
          </div>

          <div className="sp-confirm-line">
            <Checkbox checked={confirmed} onChange={(e) => setConfirmed(!!e.target.checked)}>
              我确认要创建 LVM 存储卷，选中的磁盘数据将被清除
            </Checkbox>
          </div>
        </Spin>
      )}
    </Modal>
  )
}
