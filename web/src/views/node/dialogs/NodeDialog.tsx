/**
 * 添加/编辑节点对话框
 * - 编辑时 API Key 与 root 密码留空表示不修改
 * - 节点 SSH 用户固定为 root，避免迁移写入存储目录或调用 libvirt/OVS 时权限不足
 * - SSH 认证方式二选一：
 *   - 密码认证：root 密码接入统一密码泄露检测（本地弱密码 + HIBP k-匿名），泄露时警示确认后可继续保存
 *   - SSH 密钥认证：面板不保存密钥，由用户自行在系统中配置免密登录，面板仅做连通性检测
 * - 点击「保存」时后端会先探测节点连接：探测通过才真正保存并关闭弹窗；
 *   探测失败不保存，弹窗说明失败原因并保留表单，必须解决问题后才能再次保存
 */
import { useState } from 'react'
import type { AxiosError } from 'axios'
import { Button, Input, InputNumber, Modal, Radio, RadioGroup, Toast } from '@douyinfe/semi-ui'
import {
  createHostNode,
  updateHostNode,
  type HostNodeItem,
  type HostNodePayload,
} from '@/api/node'
import type { ApiResponse } from '@/types/api'
import TextSwitch from '@/features/vm-form/sections/TextSwitch'
import { checkPasswordBreachAsync, validatePassword } from '@/utils/validate'
import { confirmModal } from '@/utils/confirm'
import { useMountModalLifecycle } from '@/hooks/useMountModalLifecycle'

/** 密钥认证默认使用的本机私钥路径（与后端 DefaultNodeSSHKeyPath 对齐） */
const DEFAULT_SSH_KEY_PATH = '/root/.ssh/id_ed25519'

interface NodeDialogProps {
  row?: HostNodeItem
  onClose: () => void
  onSaved: () => void
}

export default function NodeDialog({ row, onClose, onSaved }: NodeDialogProps) {
  const { modalVisible, requestClose, afterModalClose } = useMountModalLifecycle(onClose)
  const editing = !!row
  const [submitting, setSubmitting] = useState(false)
  /** 保存前连接探测失败的原因（非空时展示错误弹窗，表单保留待修改） */
  const [probeError, setProbeError] = useState('')
  const [form, setForm] = useState({
    name: row?.name || '',
    api_base_url: row?.api_base_url || '',
    api_key_id: row?.api_key_id || '',
    api_key: '',
    ssh_host: row?.ssh_host || '',
    ssh_port: row?.ssh_port || 22,
    ssh_user: 'root',
    ssh_password: '',
    ssh_key_auth: row ? row.ssh_key_auth : false,
    ssh_key_path: row?.ssh_key_path || '',
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
    if (form.ssh_user.trim() !== 'root') {
      Toast.warning('节点 SSH 用户必须为 root')
      return false
    }
    if (!form.ssh_key_auth) {
      // 密码认证：创建时必须输入密码；编辑时若原节点是密钥认证，切换为密码也必须输入
      if (!editing && !form.ssh_password) {
        Toast.warning('请输入 root 密码')
        return false
      }
      if (editing && row?.ssh_key_auth && !form.ssh_password) {
        Toast.warning('SSH 认证方式为「密码」时，必须输入目标节点 root 密码')
        return false
      }
    }
    if (editing && row?.ssh_user !== 'root' && !form.ssh_password) {
      Toast.warning('该节点原 SSH 用户不是 root，请输入目标节点 root 密码后保存')
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
    setProbeError('')
    setSubmitting(true)
    try {
      if (!(await confirmPasswordSafety())) return
      const payload: HostNodePayload = {
        name: form.name.trim(),
        api_base_url: form.api_base_url.trim(),
        api_key_id: form.api_key_id.trim(),
        ssh_host: form.ssh_host.trim(),
        ssh_port: form.ssh_port,
        ssh_user: 'root',
        ssh_key_auth: form.ssh_key_auth,
        ssh_key_path: form.ssh_key_path.trim() || undefined,
        enabled: form.enabled,
      }
      // 编辑时留空表示不修改，创建时必填（前面已校验）
      if (form.api_key) payload.api_key = form.api_key
      if (form.ssh_password) payload.ssh_password = form.ssh_password
      if (editing && row) {
        await updateHostNode(row.id, payload)
        Toast.success('节点已更新，连接正常')
      } else {
        await createHostNode(payload)
        Toast.success('节点已创建，连接正常')
      }
      onSaved()
      requestClose()
    } catch (e) {
      // 后端在写入前已完成连接探测：失败时节点未保存，展示原因并保留表单，
      // 必须修复连接问题后才能再次点击保存成功
      const respData = (e as AxiosError<ApiResponse<HostNodeItem>>).response?.data
      setProbeError(
        respData?.data?.last_probe_message ||
          respData?.message ||
          '节点连接探测失败，请检查配置后重试',
      )
    } finally {
      setSubmitting(false)
    }
  }

  /** 密码必填标记（仅密码认证且当前没有可用密码时显示） */
  const passwordRequired = !form.ssh_key_auth && (!editing || !!row?.ssh_key_auth)

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
          <Input value={form.ssh_user} disabled placeholder="root" />
          <div className="qvm-form-tip">迁移虚拟机需要写入目标存储并调用 libvirt/OVS，SSH 用户固定为 root</div>
        </div>
      </div>
      <div className="qvm-form-item">
        <div className="qvm-form-label">SSH 认证方式</div>
        <RadioGroup
          type="button"
          value={form.ssh_key_auth ? 'key' : 'password'}
          onChange={(e) => patch({ ssh_key_auth: e.target.value === 'key' })}
        >
          <Radio value="password">密码认证</Radio>
          <Radio value="key">SSH 密钥认证</Radio>
        </RadioGroup>
        {form.ssh_key_auth && (
          <div className="qvm-form-tip">
            面板不保存密钥：请自行将面板所在系统的公钥配置到目标节点 root 的 ~/.ssh/authorized_keys，保存后面板仅做免密连通性检测
          </div>
        )}
      </div>
      {form.ssh_key_auth ? (
        <div className="qvm-form-item">
          <div className="qvm-form-label">密钥路径（可选）</div>
          <Input
            value={form.ssh_key_path}
            onChange={(v) => patch({ ssh_key_path: v })}
            placeholder={`留空使用默认 ${DEFAULT_SSH_KEY_PATH}`}
          />
          <div className="qvm-form-tip">
            仅填写面板所在系统的本机私钥绝对路径，例如 /root/.ssh/id_rsa；留空使用默认迁移密钥 {DEFAULT_SSH_KEY_PATH}
          </div>
        </div>
      ) : (
        <div className="qvm-form-item">
          <div className={`qvm-form-label${passwordRequired ? ' required' : ''}`}>root 密码</div>
          <Input
            mode="password"
            value={form.ssh_password}
            onChange={(v) => patch({ ssh_password: v })}
            placeholder={!editing || row?.ssh_key_auth ? '目标节点 root 密码' : '留空表示不修改'}
          />
          <div className="qvm-form-tip">保存时将进行密码泄露检测，凭据加密存储于面板数据库</div>
        </div>
      )}
      <div className="qvm-form-item">
        <div className="qvm-form-label">启用节点</div>
        <TextSwitch
          checked={form.enabled}
          onChange={(v) => patch({ enabled: v })}
          checkedText="开"
          uncheckedText="关"
        />
      </div>

      {/* 保存前连接探测失败提示：节点未保存，返回修改后需重新保存 */}
      <Modal
        title="节点连接未通过，暂未保存"
        visible={!!probeError}
        onCancel={() => setProbeError('')}
        footer={
          <Button type="primary" onClick={() => setProbeError('')}>
            返回修改
          </Button>
        }
        closeOnEsc
      >
        <div>节点未保存。请先解决以下连接问题，再次点击「保存」待探测通过后即可成功：</div>
        <div className="node-probe-err">{probeError}</div>
        {form.ssh_key_auth && (
          <div className="node-probe-tip">
            已选择 SSH 密钥认证：请确认面板所在系统的公钥已加入目标节点 root 的 ~/.ssh/authorized_keys。
          </div>
        )}
      </Modal>
    </Modal>
  )
}