/**
 * 端口转发面板
 * - 规则列表（协议/宿主机端口/完整访问地址/目标/区域限制/操作）
 * - 添加 / 编辑 / 删除 / 批量删除
 * - 首次开通引导（未固定 IP 时显示）
 */
import { useCallback, useEffect, useMemo, useState } from 'react'
import {
  Button,
  Card,
  Input,
  Modal,
  Select,
  Switch,
  Table,
  Tag,
  Toast,
  Tooltip,
} from '@douyinfe/semi-ui'
import { IconCopy, IconPlus } from '@douyinfe/semi-icons'
import type { ColumnProps } from '@douyinfe/semi-ui/lib/es/table'
import {
  addPortForward,
  batchDeletePortForward,
  bindStaticIP,
  deletePortForward,
  getPortForwardList,
  setPortForwardFirewall,
  updatePortForward,
  type PortForwardRule,
} from '@/api/network'
import { copyTextWithFallback } from '@/utils/clipboard'
import { confirmModal } from '@/utils/confirm'
import { useUserStore } from '@/stores/user'
import type { NetworkSharedData } from './NetworkTab'

interface PortForwardPanelProps {
  vmName: string
  shared: NetworkSharedData
}

interface ForwardFormState {
  id: number | null
  vm_ip: string
  host_port: string
  vm_port: string
  protocol: string
}

const EMPTY_FORM: ForwardFormState = { id: null, vm_ip: '', host_port: '', vm_port: '', protocol: 'tcp' }

export default function PortForwardPanel({ vmName, shared }: PortForwardPanelProps) {
  const username = useUserStore((s) => s.username)
  const {
    isAdmin,
    isLightweight,
    isLightweightVM,
    vpcInfo,
    selfQuota,
    staticBindings,
    dhcpLeases,
    runtimeStatus,
    manualIPs,
    refreshStaticIPs,
    refreshRuntimeStatus,
    refreshVPCBinding,
    refreshSelfQuota,
    refreshManualIPs,
  } = shared

  const [rules, setRules] = useState<PortForwardRule[]>([])
  const [loading, setLoading] = useState(false)
  const [submitting, setSubmitting] = useState(false)
  const [selectedKeys, setSelectedKeys] = useState<string[]>([])
  const [dialogVisible, setDialogVisible] = useState(false)
  const [form, setForm] = useState<ForwardFormState>(EMPTY_FORM)
  const [opening, setOpening] = useState(false)
  const [showIntro, setShowIntro] = useState(false)

  // ============ 派生数据 ============
  const currentVmBindings = useMemo(
    () => staticBindings.filter((b) => b.vm_name === vmName),
    [staticBindings, vmName],
  )
  const currentVmDhcpLeases = useMemo(() => {
    const staticIPs = new Set(currentVmBindings.map((b) => b.ip))
    const staticMACs = new Set(currentVmBindings.map((b) => b.mac))
    return dhcpLeases.filter(
      (l) => l.vm_name === vmName && !staticIPs.has(l.ip) && !staticMACs.has(l.mac),
    )
  }, [dhcpLeases, currentVmBindings, vmName])

  /** 转发目标 IP 候选：静态绑定 + DHCP 租约 + 手动映射（去重） */
  const vmIPOptions = useMemo(() => {
    const options: { ip: string; source: string }[] = []
    const seen = new Set<string>()
    for (const b of staticBindings) {
      if (b.vm_name === vmName && !seen.has(b.ip)) {
        options.push({ ip: b.ip, source: '静态绑定' })
        seen.add(b.ip)
      }
    }
    for (const lease of dhcpLeases) {
      if (lease.vm_name === vmName && !seen.has(lease.ip)) {
        options.push({ ip: lease.ip, source: 'DHCP' })
        seen.add(lease.ip)
      }
    }
    for (const m of manualIPs) {
      if (!seen.has(m.ip)) {
        options.push({ ip: m.ip, source: '手动' })
        seen.add(m.ip)
      }
    }
    return options
  }, [staticBindings, dhcpLeases, manualIPs, vmName])

  /** 当前 VM 的转发规则（目标 IP 属于当前 VM） */
  const currentVmRules = useMemo(() => {
    const vmIPs = new Set(vmIPOptions.map((o) => o.ip))
    return rules.filter((r) => vmIPs.has(r.dest_ip))
  }, [rules, vmIPOptions])

  // 配额
  const lightweightQuota = vpcInfo?.lightweight_quota || null
  const quotaUsed =
    isLightweight || isLightweightVM
      ? lightweightQuota?.used_port_forwards || 0
      : selfQuota?.used_port_forwards || 0
  const quotaLimit =
    isLightweight || isLightweightVM
      ? lightweightQuota?.max_port_forwards || 0
      : selfQuota?.max_port_forwards || 0
  const quotaReached = !isAdmin && quotaLimit > 0 && quotaUsed >= quotaLimit
  const quotaVisible = isLightweight || isLightweightVM || !!selfQuota

  // 首次开通引导
  const introStorageKey = `vm-port-forward-intro-seen:${username || 'default'}:${vmName || 'default'}`
  useEffect(() => {
    if (currentVmBindings.length > 0) {
      setShowIntro(false)
      localStorage.setItem(introStorageKey, '1')
      return
    }
    setShowIntro(localStorage.getItem(introStorageKey) !== '1')
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [currentVmBindings.length, introStorageKey])

  // ============ 数据加载 ============
  const fetchRules = useCallback(async () => {
    setLoading(true)
    try {
      const res = await getPortForwardList()
      setRules(res.data || [])
    } catch {
      setRules([])
    } finally {
      setLoading(false)
    }
  }, [])

  useEffect(() => {
    void fetchRules()
  }, [fetchRules])

  const refreshQuotaAfterChange = useCallback(async () => {
    if (isLightweight || isLightweightVM) {
      await refreshVPCBinding()
    } else {
      await refreshSelfQuota()
    }
  }, [isLightweight, isLightweightVM, refreshVPCBinding, refreshSelfQuota])

  // ============ 开通引导 ============
  const preferredIP = (): string => {
    const staticIP = staticBindings.find((b) => b.vm_name === vmName)?.ip
    if (staticIP) return staticIP
    const runtimeIP = runtimeStatus?.interfaces?.find((i) => i.ip)?.ip
    if (runtimeIP) return runtimeIP
    return currentVmDhcpLeases[0]?.ip || ''
  }

  const handleOpenPanel = async () => {
    setOpening(true)
    try {
      const res = await bindStaticIP({ vm_name: vmName, ip: preferredIP() })
      Toast.success(res.message || '端口转发已开通，已固定当前 VM IP')
      setShowIntro(false)
      localStorage.setItem(introStorageKey, '1')
      await Promise.all([refreshStaticIPs(), refreshRuntimeStatus(), refreshVPCBinding()])
    } catch {
      // 请求层已提示
    } finally {
      setOpening(false)
    }
  }

  // ============ 添加 / 编辑 ============
  const openAdd = () => {
    const next = { ...EMPTY_FORM }
    if (vmIPOptions.length === 1) next.vm_ip = vmIPOptions[0].ip
    setForm(next)
    setDialogVisible(true)
  }

  const openEdit = (row: PortForwardRule) => {
    setForm({
      id: row.id,
      vm_ip: row.dest_ip || '',
      host_port: row.host_port || '',
      vm_port: row.dest_port || '',
      protocol: String(row.protocol || 'tcp').toLowerCase(),
    })
    setDialogVisible(true)
  }

  const handleSubmit = async () => {
    if (!form.vm_port) {
      Toast.warning('请输入虚拟机端口')
      return
    }
    if (!form.vm_ip) {
      Toast.warning('请选择或输入目标 IP')
      return
    }
    if (form.id !== null && !form.host_port) {
      Toast.warning('请输入宿主机端口')
      return
    }
    setSubmitting(true)
    try {
      if (form.id !== null) {
        await updatePortForward(form.id, {
          vm_name: vmName,
          vm_ip: form.vm_ip,
          host_port: form.host_port,
          vm_port: form.vm_port,
          protocol: form.protocol,
        })
        Toast.success('端口转发规则已更新')
      } else {
        const res = await addPortForward({
          vm_name: vmName,
          vm_ip: form.vm_ip,
          host_port: form.host_port,
          vm_port: form.vm_port,
          protocol: form.protocol,
        })
        Toast.success(res.message || '端口转发规则已添加')
      }
      setDialogVisible(false)
      void fetchRules()
      void refreshStaticIPs()
      void refreshManualIPs()
      void refreshQuotaAfterChange()
    } catch {
      // 请求层已提示
    } finally {
      setSubmitting(false)
    }
  }

  // ============ 删除 ============
  const handleDelete = async (row: PortForwardRule) => {
    const ok = await confirmModal({ title: '删除端口转发', content: '确定删除此转发规则？' })
    if (!ok) return
    try {
      await deletePortForward(row.id)
      Toast.success('规则已删除')
      void fetchRules()
      void refreshQuotaAfterChange()
    } catch {
      // 请求层已提示
    }
  }

  const handleBatchDelete = async () => {
    if (selectedKeys.length === 0) {
      Toast.warning('请先选择要删除的端口转发规则')
      return
    }
    const ok = await confirmModal({
      title: '批量删除',
      content: `确定批量删除已选中的 ${selectedKeys.length} 条端口转发规则？`,
    })
    if (!ok) return
    try {
      await batchDeletePortForward({ ids: selectedKeys.map(Number) })
      Toast.success('端口转发规则已批量删除')
      setSelectedKeys([])
      void fetchRules()
      void refreshQuotaAfterChange()
    } catch {
      // 请求层已提示
    }
  }

  // ============ 区域限制开关 ============
  const handleFirewallToggle = async (row: PortForwardRule, enabled: boolean) => {
    if (!row.firewall_key) return
    try {
      await setPortForwardFirewall({ key: row.firewall_key, exempt: !enabled })
      Toast.success(enabled ? '已继承入站区域限制' : '已豁免入站区域限制')
      void fetchRules()
    } catch {
      void fetchRules()
    }
  }

  const copyAccessAddress = async (value?: string) => {
    if (!value) {
      Toast.warning('没有可复制的完整访问地址')
      return
    }
    try {
      await copyTextWithFallback(value)
      Toast.success('完整访问地址已复制到剪贴板')
    } catch {
      Toast.warning('复制失败，请手动复制')
    }
  }

  // ============ 表格列 ============
  const columns: ColumnProps<PortForwardRule>[] = [
    { title: '#', dataIndex: 'id', width: 56 },
    {
      title: '协议',
      dataIndex: 'protocol',
      width: 70,
      render: (text) => <span className="qvm-mono">{String(text || '').toUpperCase()}</span>,
    },
    { title: '宿主机端口', dataIndex: 'host_port', width: 100, render: (text) => <span className="qvm-mono">{text}</span> },
    {
      title: '完整访问地址',
      dataIndex: 'access_address',
      render: (_text, row) => (
        <span className="qvm-forward-addr">
          <span className="qvm-mono qvm-ellipsis">{row.access_address || '-'}</span>
          <Tooltip content="复制完整访问地址" position="top">
            <Button
              size="small"
              theme="borderless"
              icon={<IconCopy size="small" />}
              disabled={!row.access_address}
              onClick={() => void copyAccessAddress(row.access_address)}
            />
          </Tooltip>
        </span>
      ),
    },
    { title: '目标 IP', dataIndex: 'dest_ip', width: 120, render: (text) => <span className="qvm-mono">{text}</span> },
    { title: '目标端口', dataIndex: 'dest_port', width: 90, render: (text) => <span className="qvm-mono">{text}</span> },
    ...(isAdmin
      ? [
          {
            title: '入站区域限制',
            dataIndex: 'region_filter_enabled',
            width: 110,
            render: (_text: unknown, row: PortForwardRule) => (
              <Switch
                checked={!!row.region_filter_enabled}
                size="small"
                onChange={(checked) => void handleFirewallToggle(row, checked)}
                checkedText="开"
                uncheckedText="关"
              />
            ),
          } as ColumnProps<PortForwardRule>,
        ]
      : []),
    {
      title: '操作',
      dataIndex: 'actions',
      width: 130,
      render: (_text, row) => (
        <div className="qvm-snap-actions">
          <Button size="small" theme="borderless" type="primary" onClick={() => openEdit(row)}>
            编辑
          </Button>
          <Button size="small" theme="borderless" type="danger" onClick={() => void handleDelete(row)}>
            删除
          </Button>
        </div>
      ),
    },
  ]

  return (
    <div className="qvm-forward-panel">
      <div className="qvm-tab-toolbar">
        <div className="qvm-tab-toolbar-left">
          <Button type="primary" size="small" icon={<IconPlus />} onClick={openAdd} disabled={quotaReached}>
            添加转发
          </Button>
          <Button
            type="danger"
            theme="light"
            size="small"
            disabled={selectedKeys.length === 0}
            onClick={() => void handleBatchDelete()}
          >
            批量删除
          </Button>
        </div>
        {quotaVisible && !isAdmin && (
          <Tag size="small" color={quotaReached ? 'red' : 'blue'}>
            端口转发已用 {quotaUsed} / {quotaLimit > 0 ? quotaLimit : '不限'}
          </Tag>
        )}
      </div>

      <div className={showIntro ? 'qvm-content-blurred' : ''}>
        <Table<PortForwardRule>
          rowKey="rule_key"
          columns={columns}
          dataSource={currentVmRules}
          loading={loading}
          pagination={false}
          size="small"
          empty="暂无端口转发规则"
          rowSelection={{
            selectedRowKeys: selectedKeys,
            onChange: (keys) => setSelectedKeys((keys || []) as string[]),
          }}
        />
      </div>

      {/* 首次开通引导 */}
      {showIntro && (
        <div className="qvm-intro-mask">
          <Card className="qvm-intro-card" title="端口转发说明">
            <p>端口转发用于从外网的访问流量转发到虚拟机，实现公网访问目的。</p>
            <p>开通端口转发时，系统将自动把当前虚拟机 IP 绑定为静态地址，避免转发目标在 DHCP 变化后失效。</p>
            <p>请确认您暴露到公网的服务已经完成必要的安全加固。</p>
            <div className="qvm-intro-actions">
              <Button type="primary" loading={opening} onClick={() => void handleOpenPanel()}>
                立即开通
              </Button>
            </div>
          </Card>
        </div>
      )}

      {/* 添加/编辑转发对话框 */}
      <Modal
        title={form.id !== null ? '编辑端口转发' : '添加端口转发'}
        visible={dialogVisible}
        onCancel={() => setDialogVisible(false)}
        onOk={() => void handleSubmit()}
        okText={form.id !== null ? '保存' : '确定'}
        cancelText="取消"
        confirmLoading={submitting}
        width={440}
        closeOnEsc
      >
        <div className="qvm-form-item">
          <div className="qvm-form-label">目标 IP</div>
          <Select
            style={{ width: '100%' }}
            placeholder="选择或输入目标 IP"
            filter
            allowCreate
            defaultActiveFirstOption
            value={form.vm_ip}
            onChange={(v) => setForm((f) => ({ ...f, vm_ip: String(v || '') }))}
            optionList={vmIPOptions.map((o) => ({ value: o.ip, label: `${o.ip}（${o.source}）` }))}
          />
        </div>
        <div className="qvm-form-item">
          <div className="qvm-form-label">宿主机端口</div>
          <Input
            value={form.host_port}
            onChange={(v) => setForm((f) => ({ ...f, host_port: v }))}
            placeholder={form.id !== null ? '如 10022' : '留空自动分配'}
          />
        </div>
        <div className="qvm-form-item">
          <div className="qvm-form-label">虚拟机端口</div>
          <Input
            value={form.vm_port}
            onChange={(v) => setForm((f) => ({ ...f, vm_port: v }))}
            placeholder="如 22"
          />
        </div>
        <div className="qvm-form-item">
          <div className="qvm-form-label">协议</div>
          <Select
            style={{ width: '100%' }}
            value={form.protocol}
            onChange={(v) => setForm((f) => ({ ...f, protocol: String(v) }))}
            optionList={
              form.id !== null
                ? [
                    { value: 'tcp', label: 'TCP' },
                    { value: 'udp', label: 'UDP' },
                  ]
                : [
                    { value: 'tcp', label: 'TCP' },
                    { value: 'udp', label: 'UDP' },
                    { value: 'both', label: 'TCP+UDP' },
                  ]
            }
          />
        </div>
      </Modal>
    </div>
  )
}
