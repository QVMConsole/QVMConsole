/**
 * 网络概览 Tab（仅管理员）
 * - 顶部统计卡：OVS 状态 / 网桥 / 端口数 / 内网 CIDR
 * - 操作：检测 / 修复 / 创建桥接网桥
 * - 基础状态 + 服务状态信息卡
 * - 宿主机网桥表、物理网卡表、OVS 端口表
 */
import { Button, Collapse, Switch, Table, Tag, Tooltip } from '@douyinfe/semi-ui'
import {
  IconBranch,
  IconCheckCircleStroked,
  IconDesktop,
  IconDelete,
  IconEditStroked,
  IconGlobeStroke,
  IconLink,
  IconPlus,
  IconRefresh,
  IconLock,
  IconSafeStroked,
  IconUnlock,
  IconWrench,
} from '@douyinfe/semi-icons'
import type { ColumnProps } from '@douyinfe/semi-ui/lib/es/table'
import type {
  OvsPort,
  OvsPortList,
  OvsStatus,
  PortSecurityPort,
  PortSecurityPreflight,
  PortSecurityStatus,
} from '@/api/ovs'
import type { HostInterface, NetworkBridge } from '@/api/network'
import { bridgeModeText, yesNo } from '../utils'

interface OverviewTabProps {
  status: OvsStatus | null
  ports: OvsPortList | null
  bridges: NetworkBridge[]
  hostInterfaces: HostInterface[]
  checking: boolean
  repairing: boolean
  portSecurity: PortSecurityStatus | null
  portSecurityPreflight: PortSecurityPreflight | null
  portSecurityAction: string
  onCheck: () => void
  onRepair: () => void
  onCreateBridge: () => void
  onDeleteBridge: (row: NetworkBridge) => void
  onConfigInterface: (name: string) => void
  onPortSecurityPreflight: () => void
  onPortSecurityToggle: (enabled: boolean) => void
  onPortSecurityReconcile: () => void
  onPortSecurityPortAction: (port: string, release: boolean) => void
}

/** 布尔状态 Tag（绿/红） */
function BoolTag({ ok }: { ok?: boolean }) {
  return (
    <Tag size="small" color={ok ? 'green' : 'red'}>
      {yesNo(ok)}
    </Tag>
  )
}

export default function OverviewTab({
  status,
  ports,
  bridges,
  hostInterfaces,
  checking,
  repairing,
  portSecurity,
  portSecurityPreflight,
  portSecurityAction,
  onCheck,
  onRepair,
  onCreateBridge,
  onDeleteBridge,
  onConfigInterface,
  onPortSecurityPreflight,
  onPortSecurityToggle,
  onPortSecurityReconcile,
  onPortSecurityPortAction,
}: OverviewTabProps) {
  const healthy = !!status?.healthy
  const portCount = ports?.ports?.length || 0

  // ==================== 网桥表列 ====================
  const bridgeColumns: ColumnProps<NetworkBridge>[] = [
    {
      title: '网桥',
      dataIndex: 'name',
      render: (text) => <span className="qvm-mono">{text}</span>,
    },
    {
      title: '类型',
      dataIndex: 'mode',
      width: 100,
      render: (text) => (
        <Tag size="small" color={text === 'bridge' ? 'orange' : 'green'}>
          {bridgeModeText(text)}
        </Tag>
      ),
    },
    {
      title: '物理网卡',
      dataIndex: 'uplink_if',
      render: (text) => <span className="qvm-mono">{text || '—'}</span>,
    },
    {
      title: '状态',
      dataIndex: 'active',
      width: 80,
      align: 'center',
      render: (_text, row) => (
        <Tag size="small" color={row.exists && row.active ? 'green' : 'red'}>
          {row.exists && row.active ? '正常' : '异常'}
        </Tag>
      ),
    },
    {
      title: '交换机',
      dataIndex: 'switch_count',
      width: 80,
      align: 'center',
      render: (text) => text || 0,
    },
    {
      title: 'IP / DNS',
      dataIndex: 'host_addrs',
      render: (_text, row) =>
        row.host_addrs || row.host_dns ? (
          <div>
            {row.host_addrs && (
              <div className="qvm-mono">IP: {row.host_addrs.replace(/\n/g, ', ')}</div>
            )}
            {row.host_dns && (
              <div className="qvm-mono net-text-muted">DNS: {row.host_dns}</div>
            )}
          </div>
        ) : (
          <span className="net-text-muted">—</span>
        ),
    },
    {
      title: '操作',
      dataIndex: 'actions',
      width: 160,
      render: (_text, row) => (
        <div className="net-row-actions">
          {row.migrate_host_ip && !row.is_default && (
            <Tooltip content="配置 IP" position="top">
              <span className="qvm-act-ic" onClick={() => onConfigInterface(row.name)}>
                <IconEditStroked />
              </span>
            </Tooltip>
          )}
          {!row.is_default ? (
            <Tooltip content="删除网桥" position="top">
              <span className="qvm-act-ic danger" onClick={() => onDeleteBridge(row)}>
                <IconDelete />
              </span>
            </Tooltip>
          ) : (
            <span className="net-text-muted">—</span>
          )}
        </div>
      ),
    },
  ]

  // ==================== 物理网卡表列 ====================
  const ifaceColumns: ColumnProps<HostInterface>[] = [
    {
      title: '名称',
      dataIndex: 'name',
      render: (text) => <span className="qvm-mono">{text}</span>,
    },
    { title: '状态', dataIndex: 'state', width: 80 },
    {
      title: 'MAC',
      dataIndex: 'mac',
      render: (text) => <span className="qvm-mono">{text}</span>,
    },
    {
      title: 'IP',
      dataIndex: 'addresses',
      render: (_text, row) =>
        row.addresses?.length ? (
          <span className="qvm-mono">{row.addresses.join(', ')}</span>
        ) : (
          <span className="net-text-muted">—</span>
        ),
    },
    {
      title: '默认路由',
      dataIndex: 'default_route',
      width: 90,
      align: 'center',
      render: (text) => (
        <Tag size="small" color={text ? 'orange' : 'grey'}>
          {yesNo(!!text)}
        </Tag>
      ),
    },
    {
      title: 'OVS 网桥',
      dataIndex: 'ovs_bridge',
      render: (_text, row) => (
        <span className="qvm-mono">{row.ovs_bridge || row.managed_bridge || '—'}</span>
      ),
    },
    {
      title: '风险提示',
      dataIndex: 'risk',
      render: (text) => text || <span className="net-text-muted">—</span>,
    },
    {
      title: '操作',
      dataIndex: 'actions',
      width: 100,
      render: (_text, row) =>
        !row.ovs_port && !row.managed_bridge ? (
          <Tooltip content="配置 IP" position="top">
            <span className="qvm-act-ic" onClick={() => onConfigInterface(row.name)}>
              <IconEditStroked />
            </span>
          </Tooltip>
        ) : (
          <span className="net-text-muted">—</span>
        ),
    },
  ]

  // ==================== OVS 端口表列 ====================
  const portColumns: ColumnProps<OvsPort>[] = [
    {
      title: '端口名称',
      dataIndex: 'name',
      render: (text) => <span className="qvm-mono">{text}</span>,
    },
    { title: 'ofport', dataIndex: 'ofport', width: 80 },
    {
      title: '类型',
      dataIndex: 'type',
      width: 90,
      render: (text) => (
        <Tag size="small" color={text === 'internal' ? 'grey' : 'blue'}>
          {text}
        </Tag>
      ),
    },
    {
      title: '关联 VM',
      dataIndex: 'vm_name',
      render: (text) => text || '—',
    },
    {
      title: 'IP 地址',
      dataIndex: 'ip',
      render: (text) => <span className="qvm-mono">{text || '—'}</span>,
    },
    {
      title: '异常信息',
      dataIndex: 'issues',
      render: (_text, row) =>
        row.issues?.length ? (
          <div className="net-security-tags">
            {row.issues.map((issue) => (
              <Tag key={issue} size="small" color="orange">
                {issue}
              </Tag>
            ))}
          </div>
        ) : (
          <span className="net-text-muted">—</span>
        ),
    },
  ]

  const securityPortColumns: ColumnProps<PortSecurityPort>[] = [
    {
      title: '虚拟机 / 网卡',
      dataIndex: 'vm_name',
      render: (_text, row) => (
        <div>
          <div>{row.vm_name || '残留端口'}</div>
          <div className="qvm-mono net-text-muted">#{row.interface_order + 1} · {row.mac || '—'}</div>
        </div>
      ),
    },
    {
      title: 'OVS 端口',
      dataIndex: 'port',
      render: (_text, row) => <span className="qvm-mono">{row.bridge} / {row.port} ({row.ofport})</span>,
    },
    {
      title: '策略',
      dataIndex: 'mode',
      width: 110,
      render: (_text, row) => {
        const labels = { strict: '严格', compatible: '兼容', quarantined: '隔离', disabled: '关闭' }
        const colors = { strict: 'green', compatible: 'orange', quarantined: 'red', disabled: 'grey' } as const
        return <Tag size="small" color={colors[row.mode]}>{labels[row.mode]}</Tag>
      },
    },
    {
      title: '允许地址',
      dataIndex: 'allowed_ipv4_addresses',
      render: (_text, row) => (
        <div className="qvm-mono net-port-addresses">
          <div>IPv4: {row.allowed_ipv4_addresses?.join(', ') || (row.mode === 'compatible' ? '兼容未知地址' : '仅 DHCP')}</div>
          {row.ipv6_enabled && <div>IPv6: {row.allowed_ipv6_addresses?.join(', ') || '—'}</div>}
        </div>
      ),
    },
    {
      title: '速率 / 丢弃',
      dataIndex: 'drop_packets',
      render: (_text, row) => (
        <div className="qvm-mono net-port-addresses">
          <div>{row.policing_kpps} kpps / {row.policing_burst_kpackets} kpackets</div>
          <div>身份 {row.drop_packets || 0} · 邻居 {row.neighbor_drop_packets || 0} · 广播 {row.broadcast_drop_packets || 0}</div>
        </div>
      ),
    },
    {
      title: '操作',
      dataIndex: 'actions',
      width: 70,
      align: 'center',
      render: (_text, row) => {
        const release = row.isolated
        const pending = portSecurityAction === `${release ? 'release' : 'isolate'}:${row.port}`
        return (
          <Tooltip content={release ? '释放端口' : '隔离端口'} position="top">
            <span
              className={`qvm-act-ic ${release ? '' : 'danger'} ${pending ? 'disabled' : ''}`}
              onClick={() => !pending && onPortSecurityPortAction(row.port, release)}
            >
              {pending ? <IconRefresh spin /> : release ? <IconUnlock /> : <IconLock />}
            </span>
          </Tooltip>
        )
      },
    },
  ]

  return (
    <div>
      <div className="net-card net-port-security-card">
        <div className="net-card-header">
          <IconSafeStroked />
          <span>端口安全防护</span>
          <div className="net-card-extra">
            <Tag size="small" color={portSecurity?.healthy ? 'green' : 'orange'}>
              {portSecurity?.healthy ? '状态正常' : '需要处理'}
            </Tag>
            <Switch
              checked={!!portSecurity?.enabled}
              checkedText="开"
              uncheckedText="关"
              loading={portSecurityAction === 'enable' || portSecurityAction === 'disable'}
              onChange={onPortSecurityToggle}
            />
          </div>
        </div>
        <div className="net-card-body">
          <div className="net-port-security-summary">
            <div>
              <div className="net-port-security-title">
                {portSecurity?.enabled ? '身份校验、ARP/ND 与广播抑制正在运行' : '防护默认关闭，当前网络行为保持不变'}
              </div>
              <div className="net-text-muted">
                已应用 {portSecurity?.applied_ports || 0} 个端口，兼容保护 {portSecurity?.compatible_ports || 0} 个，隔离 {portSecurity?.isolated_ports || 0} 个
                {portSecurity?.last_reconciled ? ` · 最近协调 ${new Date(portSecurity.last_reconciled).toLocaleString()}` : ''}
              </div>
            </div>
            <div className="net-toolbar-left">
              <Button loading={portSecurityAction === 'preflight'} onClick={onPortSecurityPreflight}>预检</Button>
              <Button
                icon={<IconRefresh />}
                loading={portSecurityAction === 'reconcile'}
                disabled={!portSecurity?.enabled}
                onClick={onPortSecurityReconcile}
              >协调</Button>
            </div>
          </div>
          {portSecurityPreflight && (
            <div className={`net-preflight ${portSecurityPreflight.ready ? 'ready' : 'blocked'}`}>
              <div className="net-preflight-head">
                <strong>{portSecurityPreflight.ready ? '预检通过' : '预检发现阻断项'}</strong>
                <span>{portSecurityPreflight.capabilities?.length || 0} 个网桥 · {portSecurityPreflight.ports?.length || 0} 个活动端口</span>
              </div>
              {(portSecurityPreflight.issues?.length || 0) > 0 && (
                <div className="net-preflight-issues">
                  {(portSecurityPreflight.issues || []).map((issue, index) => (
                    <div key={`${issue.code}-${issue.vm_name}-${issue.port}-${index}`}>
                      <Tag size="small" color={issue.blocking ? 'red' : 'orange'}>{issue.blocking ? '阻断' : '提示'}</Tag>
                      <span>{issue.vm_name ? `${issue.vm_name} / 网卡 ${(issue.interface_order || 0) + 1}：` : issue.bridge ? `${issue.bridge}：` : ''}{issue.message}</span>
                    </div>
                  ))}
                </div>
              )}
            </div>
          )}
        </div>
        {portSecurity?.ports?.length ? (
          <Collapse className="net-port-security-collapse" keepDOM={false}>
            <Collapse.Panel
              itemKey="protected-ports"
              header={(
                <span className="net-port-security-collapse-title">
                  <span>端口策略明细</span>
                  <Tag size="small" type="ghost">{portSecurity.ports.length} 个</Tag>
                </span>
              )}
            >
              <Table<PortSecurityPort>
                rowKey="port"
                columns={securityPortColumns}
                dataSource={portSecurity.ports}
                pagination={false}
                size="small"
              />
            </Collapse.Panel>
          </Collapse>
        ) : null}
      </div>

      {/* 统计卡 */}
      <div className="net-stat-grid">
        <div className={`net-stat-card ${healthy ? 'healthy' : 'warning'}`}>
          <div className="net-stat-icon">
            {healthy ? <IconCheckCircleStroked /> : <IconWrench />}
          </div>
          <div>
            <div className="net-stat-label">OVS 状态</div>
            <div className="net-stat-value">{healthy ? '运行正常' : '需要关注'}</div>
          </div>
        </div>
        <div className="net-stat-card">
          <div className="net-stat-icon">
            <IconBranch />
          </div>
          <div>
            <div className="net-stat-label">网桥</div>
            <div className="net-stat-value qvm-mono">{status?.bridge || '-'}</div>
          </div>
        </div>
        <div className="net-stat-card">
          <div className="net-stat-icon">
            <IconDesktop />
          </div>
          <div>
            <div className="net-stat-label">端口数</div>
            <div className="net-stat-value">{portCount}</div>
          </div>
        </div>
        <div className="net-stat-card">
          <div className="net-stat-icon">
            <IconGlobeStroke />
          </div>
          <div>
            <div className="net-stat-label">内网 CIDR</div>
            <div className="net-stat-value qvm-mono">{status?.subnet_cidr || '-'}</div>
          </div>
        </div>
      </div>

      {/* 操作栏 */}
      <div className="net-toolbar">
        <div className="net-toolbar-left">
          <Button icon={<IconRefresh />} loading={checking} onClick={onCheck}>
            检测
          </Button>
          <Button type="warning" theme="light" icon={<IconWrench />} loading={repairing} onClick={onRepair}>
            修复
          </Button>
          <Button type="primary" theme="light" icon={<IconPlus />} onClick={onCreateBridge}>
            创建桥接网桥
          </Button>
        </div>
      </div>

      {/* 基础状态 / 服务状态 */}
      <div className="net-info-grid">
        <div className="net-card">
          <div className="net-card-header">
            <IconLink />
            <span>基础状态</span>
          </div>
          <div className="net-card-body">
            <div className="net-info-list">
              <div className="net-info-item">
                <span className="net-info-label">网桥</span>
                <span className="net-info-value qvm-mono">{status?.bridge || '-'}</span>
              </div>
              <div className="net-info-item">
                <span className="net-info-label">网关 IP</span>
                <span className="net-info-value qvm-mono">{status?.gateway_ip || '-'}</span>
              </div>
              <div className="net-info-item">
                <span className="net-info-label">内网 CIDR</span>
                <span className="net-info-value qvm-mono">{status?.subnet_cidr || '-'}</span>
              </div>
              <div className="net-info-item">
                <span className="net-info-label">出口网卡</span>
                <span className="net-info-value qvm-mono">{status?.uplink || '未检测到'}</span>
              </div>
              <div className="net-info-item">
                <span className="net-info-label">ip_forward</span>
                <BoolTag ok={status?.ip_forward_enabled} />
              </div>
              <div className="net-info-item">
                <span className="net-info-label">NAT</span>
                <BoolTag ok={status?.nat_rule?.exists} />
              </div>
            </div>
          </div>
        </div>

        <div className="net-card">
          <div className="net-card-header">
            <IconWrench />
            <span>服务状态</span>
          </div>
          <div className="net-card-body">
            <div className="net-info-list">
              <div className="net-info-item">
                <span className="net-info-label">
                  {status?.openvswitch_service?.name || 'openvswitch-switch'}
                </span>
                <Tag size="small" color={status?.openvswitch_service?.active ? 'green' : 'red'}>
                  {status?.openvswitch_service?.state || '-'}
                </Tag>
              </div>
              <div className="net-info-item">
                <span className="net-info-label">OVS dnsmasq</span>
                <Tag size="small" color={status?.dnsmasq_service?.active ? 'green' : 'red'}>
                  {status?.dnsmasq_service?.state || '-'}
                </Tag>
              </div>
              <div className="net-info-item">
                <span className="net-info-label">出站 FORWARD</span>
                <BoolTag ok={status?.forward_out_rule?.exists} />
              </div>
              <div className="net-info-item">
                <span className="net-info-label">回程 FORWARD</span>
                <BoolTag ok={status?.forward_return_rule?.exists} />
              </div>
            </div>
          </div>
        </div>
      </div>

      {/* 宿主机网桥 */}
      <div className="net-card">
        <div className="net-card-header">
          <IconBranch />
          <span>宿主机网桥</span>
          <div className="net-card-extra">
            <Tag size="small">{bridges.length} 个网桥</Tag>
          </div>
        </div>
        <Table<NetworkBridge>
          rowKey="name"
          columns={bridgeColumns}
          dataSource={bridges}
          pagination={false}
          size="small"
          empty="暂无网桥"
        />
      </div>

      {/* 物理网卡 */}
      <div className="net-card">
        <div className="net-card-header">
          <IconDesktop />
          <span>物理网卡</span>
          <div className="net-card-extra">
            <Tag size="small">{hostInterfaces.length} 张网卡</Tag>
          </div>
        </div>
        <Table<HostInterface>
          rowKey="name"
          columns={ifaceColumns}
          dataSource={hostInterfaces}
          pagination={false}
          size="small"
          empty="暂无物理网卡"
        />
      </div>

      {/* OVS 端口列表 */}
      <div className="net-card">
        <div className="net-card-header">
          <IconLink />
          <span>OVS 端口列表</span>
          <div className="net-card-extra">
            <Tag size="small">{portCount} 个端口</Tag>
          </div>
        </div>
        <Table<OvsPort>
          rowKey="name"
          columns={portColumns}
          dataSource={ports?.ports || []}
          pagination={false}
          size="small"
          empty="暂无端口"
        />
      </div>
    </div>
  )
}
