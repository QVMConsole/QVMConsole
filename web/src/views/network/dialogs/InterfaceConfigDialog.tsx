/**
 * 接口 IP/DNS 配置对话框（仅管理员）
 * - 展示接口当前 IPv4/IPv6/网关/DNS，编辑后保存或一键清除
 * - 物理网卡已加入网桥时提示改为在网桥上配置；不可配置接口禁用表单
 */
import { useEffect, useState } from 'react'
import { Banner, Button, Divider, Input, Modal, Spin, Tag, TextArea, Toast } from '@douyinfe/semi-ui'
import { getInterfaceConfig, setInterfaceConfig } from '@/api/network'
import { confirmModal } from '@/utils/confirm'
import { useMountModalLifecycle } from '@/hooks/useMountModalLifecycle'

interface InterfaceConfigDialogProps {
  name: string
  onClose: () => void
  onSaved: () => void
}

interface IfaceFormState {
  type: string
  bridge_name: string
  configurable: boolean
  reason: string
  managed_bridge: boolean
  current_addrs: string[]
  current_gateway: string
  current_dns: string[]
  current_addrs6: string[]
  current_gateway6: string
  addrs: string
  gateway: string
  dns: string
  addrs6: string
  gateway6: string
}

const EMPTY_STATE: IfaceFormState = {
  type: '',
  bridge_name: '',
  configurable: false,
  reason: '',
  managed_bridge: false,
  current_addrs: [],
  current_gateway: '',
  current_dns: [],
  current_addrs6: [],
  current_gateway6: '',
  addrs: '',
  gateway: '',
  dns: '',
  addrs6: '',
  gateway6: '',
}

export default function InterfaceConfigDialog({ name, onClose, onSaved }: InterfaceConfigDialogProps) {
  const { modalVisible, requestClose, afterModalClose } = useMountModalLifecycle(onClose)
  const [loading, setLoading] = useState(true)
  const [submitting, setSubmitting] = useState(false)
  const [form, setForm] = useState<IfaceFormState>(EMPTY_STATE)

  useEffect(() => {
    setLoading(true)
    getInterfaceConfig(name)
      .then((res) => {
        const d = res.data
        setForm({
          type: d?.type || '',
          bridge_name: d?.bridge_name || '',
          configurable: d?.configurable || false,
          reason: d?.reason || '',
          managed_bridge: d?.managed_bridge || false,
          current_addrs: d?.addrs || [],
          current_gateway: d?.gateway || '',
          current_dns: d?.dns || [],
          current_addrs6: d?.addrs6 || [],
          current_gateway6: d?.gateway6 || '',
          addrs: (d?.addrs || []).join('\n'),
          gateway: d?.gateway || '',
          dns: (d?.dns || []).join(' '),
          addrs6: (d?.addrs6 || []).join('\n'),
          gateway6: d?.gateway6 || '',
        })
      })
      .catch(() => {
        // 请求层已提示，保持空表单
      })
      .finally(() => setLoading(false))
  }, [name])

  const handleSubmit = async () => {
    // IPv4 和 IPv6 至少配置一个
    if (!form.addrs.trim() && !form.addrs6.trim()) {
      Toast.warning('请至少填写一个 IPv4 或 IPv6 地址')
      return
    }
    setSubmitting(true)
    try {
      await setInterfaceConfig(name, {
        addrs: form.addrs,
        gateway: form.gateway,
        dns: form.dns,
        addrs6: form.addrs6,
        gateway6: form.gateway6,
        clear: false,
      })
      Toast.success('接口配置已更新')
      onSaved()
      requestClose()
    } catch {
      // 请求层已提示
    } finally {
      setSubmitting(false)
    }
  }

  const handleClear = async () => {
    const ok = await confirmModal({
      title: '清除配置',
      content: `确定清除 ${name} 上的所有静态 IP/DNS 配置？清除后该接口将不再有静态 IPv4 和 IPv6 地址。`,
      okText: '清除',
      danger: true,
    })
    if (!ok) return
    setSubmitting(true)
    try {
      await setInterfaceConfig(name, { clear: true })
      Toast.success('接口配置已清除')
      onSaved()
      requestClose()
    } catch {
      // 请求层已提示
    } finally {
      setSubmitting(false)
    }
  }

  const typeText = form.type === 'bridge' ? '网桥' : form.type === 'nic' ? '物理网卡' : '未知'

  return (
    <Modal
      title={`配置接口 IP/DNS — ${name}`}
      visible={modalVisible}
      afterClose={afterModalClose}
      onCancel={requestClose}
      width={600}
      closeOnEsc
      footer={
        <>
          <Button onClick={requestClose}>取消</Button>
          <Button
            type="danger"
            theme="light"
            loading={submitting}
            disabled={!form.configurable}
            onClick={() => void handleClear()}
          >
            清除配置
          </Button>
          <Button
            type="primary"
            loading={submitting}
            disabled={!form.configurable}
            onClick={() => void handleSubmit()}
          >
            保存
          </Button>
        </>
      }
    >
      <Spin spinning={loading} size="large">
        {form.bridge_name && (
          <Banner
            type="warning"
            closeIcon={null}
            description={`该网卡已加入网桥 ${form.bridge_name}，请在网桥上配置 IP`}
            style={{ marginBottom: 14 }}
          />
        )}
        {!form.configurable && !form.bridge_name && !loading && (
          <Banner
            type="info"
            closeIcon={null}
            description={form.reason || '该接口不支持配置 IP'}
            style={{ marginBottom: 14 }}
          />
        )}

        <div className="qvm-form-item">
          <div className="qvm-form-label">接口类型</div>
          <Tag size="small">{typeText}</Tag>
          {form.managed_bridge && (
            <Tag size="small" color="green" style={{ marginLeft: 8 }}>
              面板管理
            </Tag>
          )}
        </div>

        <div className="net-iface-current">
          <div className="net-iface-line">
            <span className="k">当前 IPv4</span>
            <span className="v qvm-mono">
              {form.current_addrs.length ? form.current_addrs.join(', ') : '无'}
            </span>
          </div>
          <div className="net-iface-line">
            <span className="k">IPv4 网关</span>
            <span className="v qvm-mono">{form.current_gateway || '—'}</span>
          </div>
          <div className="net-iface-line">
            <span className="k">当前 IPv6</span>
            <span className="v qvm-mono">
              {form.current_addrs6.length ? form.current_addrs6.join(', ') : '无'}
            </span>
          </div>
          <div className="net-iface-line">
            <span className="k">IPv6 网关</span>
            <span className="v qvm-mono">{form.current_gateway6 || '—'}</span>
          </div>
          <div className="net-iface-line">
            <span className="k">当前 DNS</span>
            <span className="v qvm-mono">
              {form.current_dns.length ? form.current_dns.join(', ') : '—'}
            </span>
          </div>
        </div>

        <div className="qvm-form-item">
          <div className="qvm-form-label">IPv4 地址</div>
          <TextArea
            rows={3}
            value={form.addrs}
            onChange={(v) => setForm((f) => ({ ...f, addrs: v }))}
            disabled={!form.configurable}
            placeholder="CIDR 格式，每行一个，如 192.168.1.10/24"
          />
          <div className="qvm-form-tip">每行一个 IPv4/CIDR，多个地址可换行填写</div>
        </div>
        <div className="qvm-form-item">
          <div className="qvm-form-label">IPv4 网关</div>
          <Input
            value={form.gateway}
            onChange={(v) => setForm((f) => ({ ...f, gateway: v }))}
            disabled={!form.configurable}
            placeholder="如 192.168.1.1"
          />
        </div>

        <Divider margin="12px">IPv6</Divider>

        <div className="qvm-form-item">
          <div className="qvm-form-label">IPv6 地址</div>
          <TextArea
            rows={3}
            value={form.addrs6}
            onChange={(v) => setForm((f) => ({ ...f, addrs6: v }))}
            disabled={!form.configurable}
            placeholder="CIDR 格式，每行一个，如 2001:db8::1/64"
          />
          <div className="qvm-form-tip">每行一个 IPv6/CIDR，多个地址可换行填写</div>
        </div>
        <div className="qvm-form-item">
          <div className="qvm-form-label">IPv6 网关</div>
          <Input
            value={form.gateway6}
            onChange={(v) => setForm((f) => ({ ...f, gateway6: v }))}
            disabled={!form.configurable}
            placeholder="如 fe80::1 或 2001:db8::1"
          />
        </div>

        <Divider margin="12px">DNS</Divider>

        <div className="qvm-form-item">
          <div className="qvm-form-label">DNS 服务器</div>
          <Input
            value={form.dns}
            onChange={(v) => setForm((f) => ({ ...f, dns: v }))}
            disabled={!form.configurable}
            placeholder="空格分隔，如 223.5.5.5 2402:4e00::"
          />
          <div className="qvm-form-tip">支持 IPv4 和 IPv6 DNS 地址，空格分隔</div>
        </div>
      </Spin>
    </Modal>
  )
}
