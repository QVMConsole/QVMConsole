/**
 * 批量绑定公网 IP 对话框
 * - 选中多个未绑定的公网 IP 后，统一选择用户/虚拟机/模式
 * - IPv4 NAT 模式可填写统一私网 IP（留空时后端按 VM 自动解析）
 * - IPv6 不支持 1:1 NAT，地址族混合时自动剔除 NAT 选项
 * - 提交后由任务队列异步应用（高风险 428 二次验证由请求层处理）
 */
import { useMemo, useState } from 'react'
import { Banner, Button, Input, Modal, Select, Tag, Toast } from '@douyinfe/semi-ui'
import { batchBindPublicIPs, type PublicIpItem, type PublicIpMode } from '@/api/publicIp'
import { publicIpModeLabel, publicIpTaskToast } from '../utils'
import { useMountModalLifecycle } from '@/hooks/useMountModalLifecycle'
import type { BindVmOption } from './BindPublicIpDialog'

interface BatchBindPublicIpDialogProps {
  rows: PublicIpItem[]
  users: string[]
  vms: BindVmOption[]
  onClose: () => void
  onSubmitted: () => void
}

export default function BatchBindPublicIpDialog({
  rows,
  users,
  vms,
  onClose,
  onSubmitted,
}: BatchBindPublicIpDialogProps) {
  const { modalVisible, requestClose, afterModalClose } = useMountModalLifecycle(onClose)
  // 地址族判定：全 IPv6 / 全 IPv4 / 混合
  const familyInfo = useMemo(() => {
    let hasV4 = false
    let hasV6 = false
    for (const row of rows) {
      if (row.address_family === 'ipv6' || row.ip.includes(':')) {
        hasV6 = true
      } else {
        hasV4 = true
      }
    }
    return { hasV4, hasV6, isAllV6: hasV6 && !hasV4, isAllV4: hasV4 && !hasV6 }
  }, [rows])

  // 可选模式：IPv6 不支持 NAT；混合时也剔除 NAT
  const supportedModes = useMemo<PublicIpMode[]>(() => {
    if (familyInfo.hasV6) {
      return ['classic_route', 'classic_bridge']
    }
    return ['nat', 'classic_route', 'classic_bridge']
  }, [familyInfo.hasV6])

  const [form, setForm] = useState({
    username: '',
    vm_name: '',
    vm_private_ip: '',
    mode: (supportedModes[0] || 'classic_route') as PublicIpMode,
  })
  const [submitting, setSubmitting] = useState(false)

  const patch = (p: Partial<typeof form>) => setForm((f) => ({ ...f, ...p }))

  // 按所选用户过滤虚拟机
  const filteredVMs = useMemo(() => {
    if (!form.username) return vms
    return vms.filter((vm) => vm.username === form.username || !vm.username)
  }, [vms, form.username])

  const showPrivateIP = form.mode === 'nat' && !familyInfo.hasV6

  const handleSubmit = async () => {
    if (!form.vm_name) {
      Toast.warning('请选择虚拟机')
      return
    }
    setSubmitting(true)
    try {
      const items = rows.map((row) => ({
        id: row.id,
        payload: {
          username: form.username || undefined,
          vm_name: form.vm_name,
          vm_private_ip: showPrivateIP ? form.vm_private_ip || undefined : undefined,
          mode: form.mode,
        },
      }))
      const res = await batchBindPublicIPs(items)
      Toast.success(publicIpTaskToast('批量绑定任务已提交', res.data?.task_id))
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
      title={`批量绑定公网 IP（${rows.length} 条）`}
      visible={modalVisible}
      afterClose={afterModalClose}
      onCancel={requestClose}
      width={680}
      closeOnEsc
      footer={
        <>
          <Button onClick={requestClose}>取消</Button>
          <Button
            type="primary"
            loading={submitting}
            onClick={() => void handleSubmit()}
          >
            提交批量绑定任务
          </Button>
        </>
      }
    >
      <Banner
        type="info"
        closeIcon={null}
        description={`将把选中的 ${rows.length} 条公网 IP 统一绑定到同一虚拟机。已绑定的会被后端跳过并标记失败。`}
        style={{ marginBottom: 14 }}
      />
      <div className="qvm-form-item">
        <div className="qvm-form-label">待绑定公网 IP</div>
        <div className="pip-batch-ip-list">
          {rows.map((row) => (
            <Tag key={row.id} size="small" color={row.address_family === 'ipv6' || row.ip.includes(':') ? 'purple' : 'cyan'}>
              {row.ip}
            </Tag>
          ))}
        </div>
      </div>
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
        {familyInfo.hasV6 && (
          <div className="qvm-form-tip">
            选中项含 IPv6 地址，已自动剔除 1:1 NAT 模式（IPv6 使用路由模式）。
          </div>
        )}
      </div>
      {showPrivateIP && (
        <div className="qvm-form-item">
          <div className="qvm-form-label">VM 私网 IP</div>
          <Input
            value={form.vm_private_ip}
            onChange={(v) => patch({ vm_private_ip: v })}
            placeholder="留空时后端按所选 VM 自动解析或静态绑定"
          />
        </div>
      )}
      {familyInfo.hasV6 && (
        <Banner
          type="warning"
          closeIcon={null}
          description="IPv6 绑定后请按规则预览中的提示，在 VM 主网卡配置该公网 IPv6 /128 与链路本地默认网关。"
          style={{ marginBottom: 14 }}
        />
      )}
    </Modal>
  )
}
