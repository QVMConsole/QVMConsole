/**
 * 动态内存详细配置弹窗（创建 / 编辑共用）
 */
import { useEffect } from 'react'
import { Banner, Button, InputNumber, Modal, Radio, Tag } from '@douyinfe/semi-ui'
import { useVmFormScope } from '../scopeContext'
import { recommendedMemoryDynamicValues } from '../recommend'
import TextSwitch from '../sections/TextSwitch'

interface MemoryDynamicDialogProps {
  visible: boolean
  onClose: () => void
}

const COMPAT_LABEL: Record<string, string> = {
  legacy_static: '静态兼容',
  dynamic: '动态内存',
  pending_apply: '待迁移应用',
}

const BALLOON_STATUS_TEXT: Record<string, string> = {
  ok: '气球统计正常，自动调度可使用。',
  no_stats: '未获取到气球统计，可能需要来宾系统驱动或等待统计上报。',
  not_running: '虚拟机未运行，启动后才能读取实时气球统计。',
  missing_balloon: '缺少气球设备，运行中不能热兼容，需关机后应用。',
  pending_apply: '配置已保存，等待下次关机后启动时应用。',
}

export default function MemoryDynamicDialog({ visible, onClose }: MemoryDynamicDialogProps) {
  const { form, ctx } = useVmFormScope()
  const {
    form: f,
    setField,
    handleDynamicMemoryEnabledChange,
    handleMemoryBackendChange,
    ensureMemoryDynamicDefaults,
    windowsElasticMemoryDisabled,
  } = form
  const isEdit = ctx.mode === 'edit'

  // 打开弹窗时兜底动态内存默认值
  useEffect(() => {
    if (visible) ensureMemoryDynamicDefaults()
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [visible])

  const markTouched = () => setField('memory_dynamic_touched', true)

  const resetDefaults = () => {
    const base = isEdit ? f.memory || f.ram || 1 : f.ram || 1
    const values = recommendedMemoryDynamicValues(f.memory_backend, base)
    setField('memory_initial', values.memory_initial)
    setField('memory_min', values.memory_min)
    setField('memory_max_dynamic', values.memory_max_dynamic)
    setField('memory_auto_balloon', values.memory_auto_balloon)
    markTouched()
  }

  const compatTagType =
    f.memory_compat_mode === 'dynamic' ? 'green' : f.memory_compat_mode === 'pending_apply' ? 'orange' : 'grey'

  return (
    <Modal
      title="动态内存配置"
      visible={visible}
      onCancel={onClose}
      footer={
        <>
          <Button onClick={resetDefaults}>推荐值</Button>
          <Button type="primary" theme="solid" onClick={onClose}>
            关闭
          </Button>
        </>
      }
      width={640}
      closeOnEsc
    >
      <Banner
        type="warning"
        closeIcon={null}
        style={{ marginBottom: 16 }}
        description="已有运行中的虚拟机启用后会先保存为待迁移状态，下次关机后启动时再应用最大内存和气球设备配置。"
      />

      <div className="qvm-vf-field">
        <div className="qvm-vf-label">启用动态内存</div>
        <TextSwitch checked={f.memory_dynamic_enabled} onChange={(v) => handleDynamicMemoryEnabledChange(v)} />
      </div>

      {f.memory_dynamic_enabled && (
        <>
          <div className="qvm-vf-field">
            <div className="qvm-vf-label">内存模式</div>
            <div className="qvm-vf-switch-row">
              <Radio.Group
                type="button"
                value={f.memory_backend}
                onChange={(e) => handleMemoryBackendChange(e.target.value)}
                options={[
                  { label: '气球调度', value: 'balloon' },
                  { label: 'Windows 弹性内存', value: 'virtio_mem', disabled: windowsElasticMemoryDisabled },
                ]}
              />
              {f.memory_backend === 'virtio_mem' && <Tag color="orange" size="small">实验</Tag>}
            </div>
            <div className="qvm-vf-tip">
              {f.memory_backend === 'virtio_mem'
                ? '实验功能：主表单内存作为规格内存，基础内存自动按 50% 计算，最大上限默认上浮 30%；运行后按 70%/50% 阈值自动伸缩。'
                : '默认模式：基于 virtio-balloon 调整当前内存，Linux 可配合 free page reporting 回收空闲页。'}
            </div>
          </div>

          <div className="qvm-vf-grid-2">
            <div className="qvm-vf-field">
              <div className="qvm-vf-label">启动内存（GB）</div>
              <InputNumber
                style={{ width: '100%' }}
                value={f.memory_initial}
                min={1}
                max={256}
                onChange={(v) => {
                  setField('memory_initial', Number(v || 1))
                  markTouched()
                }}
              />
              <div className="qvm-vf-tip">
                {f.memory_backend === 'virtio_mem'
                  ? 'Windows 启动时固定拥有的基础内存，计入用户内存配额'
                  : '计入用户内存配额'}
              </div>
            </div>
            {f.memory_backend !== 'virtio_mem' && (
              <div className="qvm-vf-field">
                <div className="qvm-vf-label">最小内存（GB）</div>
                <InputNumber
                  style={{ width: '100%' }}
                  value={f.memory_min}
                  min={1}
                  max={256}
                  onChange={(v) => {
                    setField('memory_min', Number(v || 1))
                    markTouched()
                  }}
                />
                <div className="qvm-vf-tip">普通状态不会自动回收到启动内存以下</div>
              </div>
            )}
            <div className="qvm-vf-field">
              <div className="qvm-vf-label">最大内存（GB）</div>
              <InputNumber
                style={{ width: '100%' }}
                value={f.memory_max_dynamic}
                min={1}
                max={512}
                onChange={(v) => {
                  setField('memory_max_dynamic', Number(v || 1))
                  markTouched()
                }}
              />
              <div className="qvm-vf-tip">
                {f.memory_backend === 'virtio_mem' ? '基础内存 + 可热插拔弹性内存的总上限' : '动态增长上限'}
              </div>
            </div>
            {f.memory_backend !== 'virtio_mem' && (
              <div className="qvm-vf-field">
                <div className="qvm-vf-label">自动调度</div>
                <TextSwitch
                  checked={f.memory_auto_balloon}
                  onChange={(v) => {
                    setField('memory_auto_balloon', v)
                    markTouched()
                  }}
                />
                <div className="qvm-vf-tip">
                  后台会按可用内存和宿主机余量自动调整，手动调整当前内存后会暂停自动调度 10 分钟
                </div>
              </div>
            )}
          </div>

          {isEdit && (
            <div className="qvm-vf-field">
              <div className="qvm-vf-label">当前内存（GB）</div>
              <InputNumber
                style={{ width: 200 }}
                value={f.memory_current}
                min={0}
                max={f.memory_max_dynamic || 512}
                onChange={(v) => {
                  setField('memory_current', Number(v || 0))
                  markTouched()
                }}
              />
              <div className="qvm-vf-tip">
                {f.memory_backend === 'virtio_mem'
                  ? '仅对已启用 Windows 弹性内存的运行中虚拟机立即生效；0 表示不手动调整'
                  : '仅对运行中的虚拟机立即生效；0 表示不手动调整'}
              </div>
            </div>
          )}

          {isEdit && (
            <div className="qvm-vf-field">
              <div className="qvm-vf-label">兼容状态</div>
              <div className="qvm-vf-switch-row">
                <Tag color={compatTagType} size="small">
                  {COMPAT_LABEL[f.memory_compat_mode] || '未识别'}
                </Tag>
                {f.memory_backend === 'virtio_mem' && (
                  <Tag color="orange" size="small">Windows 弹性内存实验</Tag>
                )}
                <Tag color={f.memory_balloon_supported ? 'green' : 'orange'} size="small">
                  {f.memory_balloon_supported ? '已配置气球设备' : '缺少气球设备'}
                </Tag>
              </div>
              <div className="qvm-vf-tip">{BALLOON_STATUS_TEXT[f.memory_balloon_status] || '暂无状态'}</div>
              {f.memory_backend === 'virtio_mem' && (
                <div className="qvm-vf-tip">当前已插入弹性内存：{f.memory_virtio_mem_current}GB</div>
              )}
            </div>
          )}
        </>
      )}
    </Modal>
  )
}
