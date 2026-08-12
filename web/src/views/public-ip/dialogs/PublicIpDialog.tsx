/**
 * 新增/编辑公网 IP 对话框
 * - 支持模式多选（1:1 NAT / 经典网络-路由 / 经典网络-桥接）
 * - 已绑定的公网 IP 不允许修改 IP 地址与状态（后端强制）
 */
import { useState } from 'react'
import { Checkbox, Input, Modal, Radio, Select, TextArea, Toast } from '@douyinfe/semi-ui'
import {
  createPublicIP,
  batchCreatePublicIPs,
  updatePublicIP,
  type PublicIpItem,
  type PublicIpMode,
} from '@/api/publicIp'
import { ALL_PUBLIC_IP_MODES, publicIpModeLabel } from '../utils'
import { useMountModalLifecycle } from '@/hooks/useMountModalLifecycle'

interface PublicIpDialogProps {
  row?: PublicIpItem
  onClose: () => void
  onSaved: () => void
}

export default function PublicIpDialog({ row, onClose, onSaved }: PublicIpDialogProps) {
  const { modalVisible, requestClose, afterModalClose } = useMountModalLifecycle(onClose)
  const editing = !!row
  const bound = !!row?.binding
  const [submitting, setSubmitting] = useState(false)
  const [batchMode, setBatchMode] = useState(false)
  const [form, setForm] = useState({
    ip: row?.ip || '',
    batchIPs: '',
    cidr: row?.cidr || '',
    gateway: row?.gateway || '',
    uplink_if: row?.uplink_if || '',
    modes: (row?.modes?.length ? row.modes : ALL_PUBLIC_IP_MODES) as PublicIpMode[],
    status: row?.status === 'reserved' ? 'reserved' : 'free',
    remark: row?.remark || '',
  })
	const isIPv6 = form.ip.includes(':')
	const availableModes = isIPv6
	  ? ALL_PUBLIC_IP_MODES.filter((mode) => mode !== 'nat')
	  : ALL_PUBLIC_IP_MODES

  const patch = (p: Partial<typeof form>) => setForm((f) => ({ ...f, ...p }))
	const patchIP = (ip: string) => {
	  const nextIsIPv6 = ip.includes(':')
	  const modes = nextIsIPv6
		? form.modes.filter((mode) => mode !== 'nat')
		: form.modes
	  patch({ ip, modes: modes.length > 0 ? modes : ['classic_route'] })
	}

  const handleSubmit = async () => {
    if (form.modes.length === 0) {
      Toast.warning('请至少选择一种支持模式')
      return
    }
    // 批量新增：一行一个公网 IP，下方字段对整批共用
    if (!editing && batchMode) {
      const ips = Array.from(
        new Set(
          form.batchIPs
            .split('\n')
            .map((line) => line.trim())
            .filter(Boolean),
        ),
      )
      if (ips.length === 0) {
        Toast.warning('请输入至少一个公网 IP')
        return
      }
      setSubmitting(true)
      try {
        const res = await batchCreatePublicIPs({
          ips,
          cidr: form.cidr.trim(),
          gateway: form.gateway.trim(),
          uplink_if: form.uplink_if.trim(),
          supported_modes: form.modes.join(','),
          status: form.status,
          remark: form.remark,
        })
        const { created, skipped, failed } = res.data
        let message = `成功新增 ${created} 个公网 IP`
        if (skipped > 0) message += `，跳过 ${skipped} 个`
        if (failed > 0) message += `，失败 ${failed} 个`
        Toast.success(message)
        onSaved()
        requestClose()
      } catch {
        // 请求层已提示
      } finally {
        setSubmitting(false)
      }
      return
    }
    // 单条新增/编辑
    if (!form.ip.trim()) {
      Toast.warning('请输入公网 IP')
      return
    }
    setSubmitting(true)
    try {
      const payload = {
        ip: form.ip.trim(),
        cidr: form.cidr.trim(),
        gateway: form.gateway.trim(),
        uplink_if: form.uplink_if.trim(),
        supported_modes: form.modes.join(','),
        status: form.status,
        remark: form.remark,
      }
      if (editing && row) {
        await updatePublicIP(row.id, payload)
      } else {
        await createPublicIP(payload)
      }
      Toast.success('公网 IP 已保存')
      onSaved()
      requestClose()
    } catch {
      // 请求层已提示
    } finally {
      setSubmitting(false)
    }
  }

  return (
    <Modal
      title={editing ? '编辑公网 IP' : '新增公网 IP'}
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
        <div
          className="qvm-form-label required"
          style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between' }}
        >
          <span>公网 IP</span>
          {!editing && (
            <Radio.Group
              type="button"
              value={batchMode ? 'batch' : 'single'}
              onChange={(e) => setBatchMode(e.target.value === 'batch')}
            >
              <Radio value="single">单条</Radio>
              <Radio value="batch">批量</Radio>
            </Radio.Group>
          )}
        </div>
        {!editing && batchMode ? (
          <TextArea
            value={form.batchIPs}
            onChange={(v) => patch({ batchIPs: v })}
            rows={6}
            placeholder={'一行一个公网 IP，例如\n2408:871a:b000:2:0:1:0:50\n2408:871a:b000:2:0:1:0:51'}
          />
        ) : (
          <Input
            value={form.ip}
            onChange={patchIP}
            disabled={bound || !!row?.auto_ipv6}
            placeholder="例如 203.0.113.10 或 2001:db8::10"
          />
        )}
        {!editing && batchMode && (
          <div className="qvm-form-tip">
            批量导入时，下方 CIDR、网关、出口网卡、支持模式等字段对所有 IP 共用；重复或已存在的 IP 会自动跳过
          </div>
        )}
        {bound && <div className="qvm-form-tip warn">公网 IP 已绑定，不能修改 IP 地址</div>}
        {row?.auto_ipv6 && <div className="qvm-form-tip">动态 IPv6 地址由来源前缀自动同步</div>}
      </div>
      <div className="qvm-form-item">
        <div className="qvm-form-label">CIDR/掩码</div>
        <Input
          value={form.cidr}
          onChange={(v) => patch({ cidr: v })}
		  disabled={!!row?.auto_ipv6}
		  placeholder="例如 203.0.113.0/29 或 2001:db8::/64"
        />
      </div>
      <div className="qvm-form-item">
        <div className="qvm-form-label">网关</div>
        <Input
          value={form.gateway}
          onChange={(v) => patch({ gateway: v })}
          placeholder="经典网络给 VM 使用的网关"
        />
      </div>
      <div className="qvm-form-item">
        <div className="qvm-form-label">出口网卡</div>
        <Input
          value={form.uplink_if}
          onChange={(v) => patch({ uplink_if: v })}
		  disabled={!!row?.auto_ipv6}
          placeholder="留空时自动检测默认出口"
        />
      </div>
      <div className="qvm-form-item">
        <div className="qvm-form-label required">支持模式</div>
        <Checkbox.Group
          value={form.modes}
          onChange={(v) => patch({ modes: v as PublicIpMode[] })}
        >
		  {availableModes.map((mode) => (
            <Checkbox key={mode} value={mode}>
              {publicIpModeLabel(mode)}
            </Checkbox>
          ))}
        </Checkbox.Group>
		{(isIPv6 || (!editing && batchMode)) && (
          <div className="qvm-form-tip">IPv6 使用路由或桥接转发，不使用 NAT66</div>
        )}
      </div>
      <div className="qvm-form-item">
        <div className="qvm-form-label">状态</div>
        <Select
          value={form.status}
          onChange={(v) => patch({ status: v as string })}
          disabled={bound}
          style={{ width: '100%' }}
          optionList={[
            { label: '空闲', value: 'free' },
            { label: '保留', value: 'reserved' },
          ]}
        />
        {bound && <div className="qvm-form-tip">已绑定状态下由系统维护，不可手动调整</div>}
      </div>
      <div className="qvm-form-item">
        <div className="qvm-form-label">备注</div>
        <TextArea
          rows={2}
          value={form.remark}
          onChange={(v) => patch({ remark: v })}
          placeholder="请输入备注信息"
        />
      </div>
    </Modal>
  )
}
