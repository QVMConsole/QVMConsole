/**
 * 绑定/迁移公网 IP 对话框
 * - 用户 → 虚拟机 联动过滤（虚拟机归属来自用户列表的 vms 字段）
 * - 绑定模式限定为该公网 IP 支持的模式
 * - 提交前可预览将下发的 nftables/路由规则与配置提示
 * - 提交后由任务队列异步应用（高风险 428 二次验证由请求层处理）
 */
import { useMemo, useState } from 'react'
import { Banner, Button, Input, Modal, Select, Toast } from '@douyinfe/semi-ui'
import { IconSearch } from '@douyinfe/semi-icons'
import {
  bindPublicIP,
  migratePublicIP,
  previewPublicIP,
  type PublicIpItem,
  type PublicIpMode,
  type PublicIpPreview,
} from '@/api/publicIp'
import { publicIpModeLabel, publicIpTaskToast } from '../utils'
import { useMountModalLifecycle } from '@/hooks/useMountModalLifecycle'

/** 虚拟机选项（含归属用户） */
export interface BindVmOption {
  name: string
  username: string
}

interface BindPublicIpDialogProps {
  row: PublicIpItem
  action: 'bind' | 'migrate'
  users: string[]
  vms: BindVmOption[]
  onClose: () => void
  onSubmitted: () => void
}

export default function BindPublicIpDialog({
  row,
  action,
  users,
  vms,
  onClose,
  onSubmitted,
}: BindPublicIpDialogProps) {
  const { modalVisible, requestClose, afterModalClose } = useMountModalLifecycle(onClose)
  const binding = row.binding
	const isIPv6 = row.address_family === 'ipv6' || row.ip.includes(':')
  const supportedModes = useMemo(
	() => {
	  const modes = row.modes?.length ? row.modes : (['nat'] as PublicIpMode[])
	  return isIPv6 ? modes.filter((mode) => mode !== 'nat') : modes
	},
	[row.modes, isIPv6],
  )
  const [form, setForm] = useState({
    username: binding?.username || '',
    vm_name: binding?.vm_name || '',
    vm_private_ip: binding?.vm_private_ip || '',
    mode: (binding?.mode || supportedModes[0] || 'nat') as PublicIpMode,
  })
  const [preview, setPreview] = useState<PublicIpPreview | null>(null)
  const [previewLoading, setPreviewLoading] = useState(false)
  const [submitting, setSubmitting] = useState(false)

  const patch = (p: Partial<typeof form>) => setForm((f) => ({ ...f, ...p }))

  /** 按所选用户过滤虚拟机；未选用户时展示全部 */
  const filteredVMs = useMemo(() => {
    if (!form.username) return vms
    return vms.filter((vm) => vm.username === form.username || !vm.username)
  }, [vms, form.username])

  const handlePreview = async () => {
    setPreviewLoading(true)
    try {
      const res = await previewPublicIP(row.id, { ...form })
      setPreview(res.data || null)
    } catch {
      // 请求层已提示
    } finally {
      setPreviewLoading(false)
    }
  }

  const handleSubmit = async () => {
    if (!form.vm_name) {
      Toast.warning('请选择虚拟机')
      return
    }
    setSubmitting(true)
    try {
      const api = action === 'migrate' ? migratePublicIP : bindPublicIP
      const res = await api(row.id, { ...form })
      Toast.success(
        publicIpTaskToast(
          action === 'migrate' ? '迁移任务已提交' : '绑定任务已提交',
          res.data?.task_id,
        ),
      )
      onSubmitted()
      requestClose()
    } catch {
      // 请求层已提示
    } finally {
      setSubmitting(false)
    }
  }

  return (
    <Modal
      title={action === 'migrate' ? '迁移公网 IP' : '绑定公网 IP'}
      visible={modalVisible}
      afterClose={afterModalClose}
      onCancel={requestClose}
      width={640}
      closeOnEsc
      footer={
        <>
          <Button onClick={requestClose}>取消</Button>
          <Button
            type="primary"
            loading={submitting}
            onClick={() => void handleSubmit()}
          >
            {action === 'migrate' ? '提交迁移任务' : '提交绑定任务'}
          </Button>
        </>
      }
    >
      <Banner
        type="info"
        closeIcon={null}
        description={`公网 IP：${row.ip}`}
        style={{ marginBottom: 14 }}
      />
      <div className="qvm-form-item">
        <div className="qvm-form-label">用户</div>
        <Select
          value={form.username}
          onChange={(v) => patch({ username: v as string, vm_name: '' })}
          filter
          placeholder="选择用户"
          style={{ width: '100%' }}
          optionList={users.map((u) => ({ label: u, value: u }))}
        />
      </div>
      <div className="qvm-form-item">
        <div className="qvm-form-label required">虚拟机</div>
        <Select
          value={form.vm_name}
          onChange={(v) => patch({ vm_name: v as string })}
          filter
          placeholder="选择虚拟机"
          style={{ width: '100%' }}
          optionList={filteredVMs.map((vm) => ({ label: vm.name, value: vm.name }))}
        />
      </div>
      <div className="qvm-form-item">
        <div className="qvm-form-label">绑定模式</div>
        <Select
          value={form.mode}
          onChange={(v) => patch({ mode: v as PublicIpMode })}
          style={{ width: '100%' }}
          optionList={supportedModes.map((mode) => ({
            label: publicIpModeLabel(mode),
            value: mode,
          }))}
        />
      </div>
	  {!isIPv6 && <div className="qvm-form-item">
        <div className="qvm-form-label">VM 私网 IP</div>
        <Input
          value={form.vm_private_ip}
          onChange={(v) => patch({ vm_private_ip: v })}
          placeholder="NAT 必填，留空时后端自动解析或静态绑定"
        />
	  </div>}
	  {isIPv6 && (
		<Banner
		  type="warning"
		  closeIcon={null}
		  description="绑定后请按规则预览中的提示，在 VM 主网卡配置该公网 IPv6 /128 与链路本地默认网关。"
		  style={{ marginBottom: 14 }}
		/>
	  )}
      <div className="qvm-form-item">
        <Button
          icon={<IconSearch />}
          loading={previewLoading}
          onClick={() => void handlePreview()}
        >
          预览规则
        </Button>
      </div>

      {preview && (
        <div className="pip-preview-panel">
          <div className="pip-preview-title">规则预览</div>
          <pre className="pip-preview-commands">{(preview.commands || []).join('\n')}</pre>
          {preview.config_hint && (
            <>
              <div className="pip-preview-title">配置提示</div>
              <p className="pip-preview-hint">{preview.config_hint}</p>
            </>
          )}
          {(preview.warnings || []).map((item) => (
            <Banner
              key={item}
              type="warning"
              closeIcon={null}
              description={item}
              style={{ marginTop: 8 }}
            />
          ))}
        </div>
      )}
    </Modal>
  )
}
