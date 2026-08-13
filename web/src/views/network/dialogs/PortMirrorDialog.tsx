import { useMemo, useState } from 'react'
import { Banner, Button, Modal, RadioGroup, Select, Space, Tag, Toast } from '@douyinfe/semi-ui'
import { IconBranch, IconRefresh } from '@douyinfe/semi-icons'
import {
  enablePortMirror,
  type PortMirrorDirection,
  type PortMirrorOptions,
  type PortMirrorStatus,
} from '@/api/ovs'
import { useMountModalLifecycle } from '@/hooks/useMountModalLifecycle'

interface Props {
  options: PortMirrorOptions
  status: PortMirrorStatus | null
  onClose: () => void
  onSubmitted: (taskId: number) => void
}

const directionOptions = [
  { label: '双向', value: 'both' },
  { label: '仅入方向', value: 'ingress' },
  { label: '仅出方向', value: 'egress' },
]

export default function PortMirrorDialog({ options, status, onClose, onSubmitted }: Props) {
  const { modalVisible, requestClose, afterModalClose } = useMountModalLifecycle(onClose)
  const [sourceInterfaces, setSourceInterfaces] = useState<string[]>(status?.source_interfaces || [])
  const [targetSwitchIds, setTargetSwitchIds] = useState<number[]>(
    status?.targets?.map((item) => item.switch_id) || [],
  )
  const [direction, setDirection] = useState<PortMirrorDirection>(status?.direction || 'both')
  const [submitting, setSubmitting] = useState(false)

  const sources = useMemo(
    () => options.sources.filter((item) => sourceInterfaces.includes(item.name)),
    [options.sources, sourceInterfaces],
  )
  const targets = useMemo(
    () => options.targets.filter((item) => targetSwitchIds.includes(item.switch_id)),
    [options.targets, targetSwitchIds],
  )

  const submit = async () => {
    if (sourceInterfaces.length === 0 || targetSwitchIds.length === 0) {
      Toast.warning('请至少选择一个镜像来源和一个目标空交换机')
      return
    }
    setSubmitting(true)
    try {
      const response = await enablePortMirror({
        source_interfaces: sourceInterfaces,
        target_switch_ids: targetSwitchIds,
        direction,
      })
      const taskId = response.data?.task_id
      if (taskId) onSubmitted(taskId)
      Toast.success(response.message || '端口镜像任务已提交')
      requestClose()
    } finally {
      setSubmitting(false)
    }
  }

  return (
    <Modal
      title="配置端口镜像"
      visible={modalVisible}
      onCancel={requestClose}
      afterClose={afterModalClose}
      width={620}
      footer={(
        <Space>
          <Button onClick={requestClose}>取消</Button>
          <Button type="primary" icon={submitting ? <IconRefresh spin /> : <IconBranch />} loading={submitting} onClick={() => void submit()}>
            {status?.enabled ? '更新镜像' : '启用镜像'}
          </Button>
        </Space>
      )}
    >
      <div className="net-port-mirror-form">
        <Banner
          type="warning"
          description="支持同时选择多个来源和目标；系统会为每个来源到每个目标建立独立连接。启用前会建立两分钟自动回滚看门狗。"
        />
        <div className="qvm-form-divider">镜像来源</div>
        <Select
          multiple
          value={sourceInterfaces}
          placeholder="选择一个或多个需要复制流量的接口"
          style={{ width: '100%' }}
          filter
          onChange={(value) => setSourceInterfaces((value as string[]) || [])}
        >
          {options.sources.map((item) => (
            <Select.Option key={item.name} value={item.name}>
              <div className="net-port-mirror-option">
                <span className="qvm-mono">{item.name}</span>
                <span>
                  {item.capture_stage === 'pre_nat' ? 'NAT 前' : item.capture_stage === 'post_nat' ? 'NAT 后' : '接口流量'}
                  {item.default_route ? ' · 默认路由' : ''}
                </span>
              </div>
            </Select.Option>
          ))}
        </Select>
        {sources.length > 0 ? (
          <div className="net-port-mirror-hint">
            {sources.map((source) => (
              <span key={source.name} className="net-port-mirror-selected-item">
                <Tag size="small" color={source.capture_stage === 'pre_nat' ? 'green' : 'orange'}>{source.name}</Tag>
                <span>{source.capture_stage === 'pre_nat' ? 'NAT 前' : source.capture_stage === 'post_nat' ? 'NAT 后' : '接口流量'}</span>
                {source.risk ? <span className="net-text-warn">默认路由</span> : null}
              </span>
            ))}
          </div>
        ) : null}

        <div className="qvm-form-divider">目标空交换机</div>
        <Select
          multiple
          value={targetSwitchIds}
          placeholder="选择一个或多个接收镜像流量的空交换机"
          style={{ width: '100%' }}
          onChange={(value) => setTargetSwitchIds(((value as Array<number | string>) || []).map(Number))}
        >
          {options.targets.map((item) => (
            <Select.Option key={item.switch_id} value={item.switch_id}>
              <div className="net-port-mirror-option">
                <span>{item.switch_name}</span>
                <span className="qvm-mono">{item.bridge} · {item.vm_count} 个网口</span>
              </div>
            </Select.Option>
          ))}
        </Select>
        {targets.length > 0 ? (
          <div className="net-port-mirror-hint">
            将创建 {sourceInterfaces.length * targetSwitchIds.length} 条源到目标连接；
            {targets.map((target) => (
              <span key={target.switch_id} className="qvm-mono">{target.switch_name}/{target.bridge}</span>
            ))}
            中的全部虚拟机网口均可收到对应镜像副本。
          </div>
        ) : null}

        <div className="qvm-form-divider">镜像方向</div>
        <RadioGroup
          type="button"
          value={direction}
          options={directionOptions}
          onChange={(event) => setDirection(event.target.value as PortMirrorDirection)}
        />
      </div>
    </Modal>
  )
}
