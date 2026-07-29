/**
 * 添加/编辑节点对话框
 * - 编辑时 API Key 与 root 密码留空表示不修改
 * - root 密码接入统一密码泄露检测（本地弱密码 + HIBP k-匿名），泄露时警示确认后可继续保存
 */
import { useState } from 'react'
import { Input, InputNumber, Modal, Toast } from '@douyinfe/semi-ui'
import {
  createHostNode,
  updateHostNode,
  type HostNodeItem,
  type HostNodePayload,
} from '@/api/node'
import TextSwitch from '@/features/vm-form/sections/TextSwitch'
import { checkPasswordBreachAsync, validatePassword } from '@/utils/validate'
import { confirmModal } from '@/utils/confirm'
import { useMountModalLifecycle } from '@/hooks/useMountModalLifecycle'

interface NodeDialogProps {
  row?: HostNodeItem
  onClose: () => void
  onSaved: () => void
}

export default function NodeDialog({ row, onClose, onSaved }: NodeDialogProps) {
  const { modalVisible, requestClose, afterModalClose } = useMountModalLifecycle(onClose)
  const editing = !!row
  const [submitting, setSubmitting] = useState(false)
  const [form, setForm] = useState({
    name: row?.name || '',
    api_base_url: row?.api_base_url || '',
    api_key_id: row?.api_key_id || '',
    api_key: '',
    ssh_host: row?.ssh_host || '',
    ssh_port: row?.ssh_port || 22,
    ssh_user: row?.ssh_user || 'root',
    ssh_password: '',
    enabled: row ? row.enabled : true,
  })

  const patch = (p: Partial<typeof form>) => setForm((f) => ({ ...f, ...p }))

  /** 提交前校验必填项（编辑时密钥类字段留空跳过） */
  const validateForm = (): boolean => {
    if (!form.name.trim()) {
      Toast.warning('请输入节点名称')
      return false
    }
    if (!form.api_base_url.trim()) {
      Toast.warning('请输入面板 API 地址')
      return false
    }
    if (!form.api_key_id.trim()) {
      Toast.warning('请输入目标面板 API ID')
      return false
    }
    if (!editing && !form.api_key) {
      Toast.warning('请输入目标面板 API Key')
      return false
    }
    if (!form.ssh_host.trim()) {
      Toast.warning('请输入 SSH 地址')
      return false
    }
    if (!form.ssh_user.trim()) {
      Toast.warning('请输入 SSH 用户')
      return false
    }
    if (!editing && !form.ssh_password) {
      Toast.warning('请输入 root 密码')
      return false
    }
    return true
  }

  /**
   * root 密码泄露检测：属于既有服务器凭据，检测到泄露时不强制阻断，
   * 弹出危险确认提示用户尽快更换后仍可继续保存。
   */
  const confirmPasswordSafety = async (): Promise<boolean> => {
    if (!form.ssh_password) return true
    const local = validatePassword(form.ssh_password)
    const breach = local.valid ? await checkPasswordBreachAsync(form.ssh_password) : null
    const breached = !local.valid || (!!breach && breach.enabled && breach.breached)
    if (!breached) return true
    return confirmModal({
      title: 'root 密码存在泄露风险',
      content:
        '安全检测发现该 root 密码已出现在公开泄露数据库或常见弱密码库中，建议尽快登录目标节点更换。是否仍要继续保存？',
      okText: '仍要保存',
      danger: true,
    })
  }

  const handleSubmit = async () => {
    if (!validateForm()) return
    setSubmitting(true)
    try {
      if (!(await confirmPasswordSafety())) return
      const payload: HostNodePayload = {
        name: form.name.trim(),
        api_base_url: form.api_base_url.trim(),
        api_key_id: form.api_key_id.trim(),
        ssh_host: form.ssh_host.trim(),
        ssh_port: form.ssh_port,
        ssh_user: form.ssh_user.trim(),
        enabled: form.enabled,
      }
      // 编辑时留空表示不修改，创建时必填（前面已校验）
      if (form.api_key) payload.api_key = form.api_key
      if (form.ssh_password) payload.ssh_password = form.ssh_password
      if (editing && row) {
        await updateHostNode(row.id, payload)
        Toast.success('节点已更新')
      } else {
        await createHostNode(payload)
        Toast.success('节点已创建')
      }
      onSaved()
      requestClose()
    } catch {
      // 请求层已统一提示
    } finally {
      setSubmitting(false)
    }
  }

  return (
    <Modal
      title={editing ? '编辑节点' : '添加节点'}
      visible={modalVisible}
      afterClose={afterModalClose}
      onCancel={requestClose}
      onOk={() => void handleSubmit()}
      okText="保存"
      cancelText="取消"
      confirmLoading={submitting}
      width={560}
      closeOnEsc
    >
      <div className="qvm-form-item">
        <div className="qvm-form-label required">节点名称</div>
        <Input value={form.name} onChange={(v) => patch({ name: v })} placeholder="例如 kvm-node-2" />
      </div>
      <div className="qvm-form-item">
        <div className="qvm-form-label required">面板 API 地址</div>
        <Input
          value={form.api_base_url}
          onChange={(v) => patch({ api_base_url: v })}
          placeholder="例如 http://192.168.11.19:8080"
        />
      </div>
      <div className="qvm-form-item">
        <div className="qvm-form-label required">API ID</div>
        <Input
          value={form.api_key_id}
          onChange={(v) => patch({ api_key_id: v })}
          placeholder="目标面板管理员 API ID"
        />
      </div>
      <div className="qvm-form-item">
        <div className={`qvm-form-label${editing ? '' : ' required'}`}>API Key</div>
        <Input
          mode="password"
          value={form.api_key}
          onChange={(v) => patch({ api_key: v })}
          placeholder={editing ? '留空表示不修改' : '目标面板管理员 API Key'}
        />
      </div>
      <div className="qvm-form-item">
        <div className="qvm-form-label required">SSH 地址</div>
        <Input
          value={form.ssh_host}
          onChange={(v) => patch({ ssh_host: v })}
          placeholder="例如 192.168.11.19"
        />
      </div>
      <div className="node-form-row">
        <div className="qvm-form-item">
          <div className="qvm-form-label">SSH 端口</div>
          <InputNumber
            value={form.ssh_port}
            onChange={(v) => patch({ ssh_port: Number(v) || 22 })}
            min={1}
            max={65535}
            style={{ width: '100%' }}
          />
        </div>
        <div className="qvm-form-item">
          <div className="qvm-form-label required">SSH 用户</div>
          <Input value={form.ssh_user} onChange={(v) => patch({ ssh_user: v })} placeholder="root" />
        </div>
      </div>
      <div className="qvm-form-item">
        <div className={`qvm-form-label${editing ? '' : ' required'}`}>root 密码</div>
        <Input
          mode="password"
          value={form.ssh_password}
          onChange={(v) => patch({ ssh_password: v })}
          placeholder={editing ? '留空表示不修改' : '目标节点 root 密码'}
        />
        <div className="qvm-form-tip">保存时将进行密码泄露检测，凭据加密存储于面板数据库</div>
      </div>
      <div className="qvm-form-item">
        <div className="qvm-form-label">启用节点</div>
        <TextSwitch
          checked={form.enabled}
          onChange={(v) => patch({ enabled: v })}
          checkedText="开"
          uncheckedText="关"
        />
      </div>
    </Modal>
  )
}
