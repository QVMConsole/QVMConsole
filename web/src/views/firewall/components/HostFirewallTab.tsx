/**
 * 宿主机防火墙 Tab（UFW 入站管控）
 * - 状态横幅：启用/关闭 + 开启/关闭操作
 * - 左侧运行状态卡：UFW 可用性、默认策略、SSH/面板端口、Docker 兼容说明
 * - 右侧规则表：端口/协议/动作/备注筛选，保护行禁止编辑删除
 */
import { useMemo, useState } from 'react'
import { Button, Col, Input, Row, Select, Table, Tag, Tooltip } from '@douyinfe/semi-ui'
import {
  IconAlertTriangle,
  IconClose,
  IconDelete,
  IconEditStroked,
  IconInfoCircle,
  IconList,
  IconPlay,
  IconPlus,
  IconSearch,
  IconSettingStroked,
  IconTickCircle,
  IconVideo,
} from '@douyinfe/semi-icons'
import type { ColumnProps } from '@douyinfe/semi-ui/lib/es/table'
import type { HostFirewallRule, HostFirewallStatus } from '@/api/firewall'
import { formatRulePort } from '../utils'

interface HostFirewallTabProps {
  hostStatus: HostFirewallStatus | null
  loading: boolean
  /** 开启按钮预览请求中 */
  enableLoading: boolean
  onEnable: () => void
  onDisable: () => void
  onAddVncDefault: () => void
  onEditRule: (row?: HostFirewallRule) => void
  onDeleteRule: (row: HostFirewallRule) => void
}

export default function HostFirewallTab({
  hostStatus,
  loading,
  enableLoading,
  onEnable,
  onDisable,
  onAddVncDefault,
  onEditRule,
  onDeleteRule,
}: HostFirewallTabProps) {
  // ==================== 筛选 ====================
  const [portSearch, setPortSearch] = useState('')
  const [protocolFilter, setProtocolFilter] = useState('')
  const [actionFilter, setActionFilter] = useState('')
  const [remarkSearch, setRemarkSearch] = useState('')

  const rules = useMemo(() => hostStatus?.rules || [], [hostStatus])

  const filteredRules = useMemo(() => {
    let data = rules
    if (portSearch) {
      data = data.filter((r) => {
        const start = r.port_start ? String(r.port_start) : ''
        const end = r.port_end ? String(r.port_end) : ''
        return start.includes(portSearch) || end.includes(portSearch)
      })
    }
    if (protocolFilter) {
      data = data.filter((r) => r.protocol === protocolFilter)
    }
    if (actionFilter) {
      data = data.filter((r) => r.action === actionFilter)
    }
    if (remarkSearch) {
      const q = remarkSearch.toLowerCase()
      data = data.filter((r) => (r.comment || '').toLowerCase().includes(q))
    }
    return data
  }, [rules, portSearch, protocolFilter, actionFilter, remarkSearch])

  // ==================== 表格 ====================
  const columns: ColumnProps<HostFirewallRule>[] = [
    {
      title: '动作',
      dataIndex: 'action',
      width: 90,
      align: 'center',
      render: (text) => (
        <Tag size="small" color={text === 'allow' ? 'green' : 'red'}>
          {text === 'allow' ? '允许' : '拒绝'}
        </Tag>
      ),
    },
    {
      title: '协议',
      dataIndex: 'protocol',
      width: 90,
      align: 'center',
      render: (text) => <span className="qvm-mono">{(text || '').toUpperCase()}</span>,
    },
    {
      title: '端口',
      dataIndex: 'port_start',
      width: 130,
      align: 'center',
      render: (_text, row) => <span className="qvm-mono">{formatRulePort(row)}</span>,
    },
    {
      title: '来源',
      dataIndex: 'source_cidr',
      render: (text) => <span className="qvm-mono">{text || 'any'}</span>,
    },
    {
      title: '备注',
      dataIndex: 'comment',
      render: (text) => (
        <Tooltip content={text || ''} position="top" showArrow={false}>
          <span className="fw-ellipsis">{text || '-'}</span>
        </Tooltip>
      ),
    },
    {
      title: '状态',
      dataIndex: 'protected',
      width: 140,
      align: 'center',
      render: (_text, row) => {
        if (row.protected) {
          return <Tag size="small" color="red">{row.protected_reason || '保护规则'}</Tag>
        }
        if (row.managed_by_panel) {
          return <Tag size="small" color="grey">面板管理</Tag>
        }
        return <span className="fw-muted">—</span>
      },
    },
    {
      title: '操作',
      dataIndex: 'actions',
      width: 80,
      align: 'center',
      render: (_text, row) => (
        <div className="fw-act-cell">
          <Tooltip content={row.protected ? '保护规则不可编辑' : '编辑'} position="top">
            <span
              className={`fw-act-ic edit${row.protected ? ' disabled' : ''}`}
              onClick={() => !row.protected && onEditRule(row)}
            >
              <IconEditStroked />
            </span>
          </Tooltip>
          <Tooltip content={row.protected ? '保护规则不可删除' : '删除'} position="top">
            <span
              className={`fw-act-ic delete${row.protected ? ' disabled' : ''}`}
              onClick={() => !row.protected && onDeleteRule(row)}
            >
              <IconDelete />
            </span>
          </Tooltip>
        </div>
      ),
    },
  ]

  return (
    <div className="fw-tab-pane">
      {/* 状态横幅 */}
      <div className={`fw-banner ${hostStatus?.active ? 'enabled' : 'disabled'}`}>
        <div className="fw-banner-icon">
          {hostStatus?.active ? <IconTickCircle /> : <IconAlertTriangle />}
        </div>
        <div className="fw-banner-body">
          <div className="fw-banner-title">
            {hostStatus?.active ? '宿主机防火墙已启用' : '宿主机防火墙已关闭'}
          </div>
          <div className="fw-banner-desc">
            {hostStatus?.active
              ? '防火墙规则正在保护宿主机入站流量'
              : '端口转发仍会写入 UFW 持久放通规则'}
          </div>
        </div>
        <div className="fw-banner-actions">
          <Button
            type="primary"
            theme="light"
            icon={<IconPlay />}
            loading={enableLoading}
            onClick={onEnable}
          >
            开启防火墙
          </Button>
          <Button type="warning" theme="light" icon={<IconClose />} onClick={onDisable}>
            关闭防火墙
          </Button>
        </div>
      </div>

      <Row gutter={[16, 16]}>
        {/* 运行状态 */}
        <Col xs={24} md={9} lg={8}>
          <div className="fw-card">
            <div className="fw-card-header">
              <IconSettingStroked />
              <span>运行状态</span>
            </div>
            <div className="fw-info-list">
              <div className="fw-info-item">
                <span className="fw-info-label">UFW</span>
                <Tag size="small" color={hostStatus?.ufw_available ? 'green' : 'red'}>
                  {hostStatus?.ufw_available ? '可用' : '不可用'}
                </Tag>
              </div>
              <div className="fw-info-item">
                <span className="fw-info-label">入站默认</span>
                <Tag size="small" color={hostStatus?.default_incoming === 'allow' ? 'red' : 'green'}>
                  {hostStatus?.default_incoming || '-'}
                </Tag>
              </div>
              <div className="fw-info-item">
                <span className="fw-info-label">出站默认</span>
                <Tag size="small" color={hostStatus?.default_outgoing === 'allow' ? 'green' : 'grey'}>
                  {hostStatus?.default_outgoing || '-'}
                </Tag>
              </div>
              <div className="fw-info-item">
                <span className="fw-info-label">转发默认</span>
                <Tag size="small" color={hostStatus?.default_routed === 'allow' ? 'orange' : 'green'}>
                  {hostStatus?.default_routed || '-'}
                </Tag>
              </div>
              <div className="fw-info-item">
                <span className="fw-info-label">SSH 端口</span>
                <span className="qvm-mono">{(hostStatus?.ssh_ports || []).join(', ') || '-'}</span>
              </div>
              <div className="fw-info-item">
                <span className="fw-info-label">面板端口</span>
                <span className="qvm-mono">{(hostStatus?.panel_ports || []).join(', ') || '-'}</span>
              </div>
            </div>
            {hostStatus?.docker_compatibility && (
              <div className="fw-card-footer">
                <IconInfoCircle />
                <span>{hostStatus.docker_compatibility}</span>
              </div>
            )}
          </div>
        </Col>

        {/* 宿主机规则 */}
        <Col xs={24} md={15} lg={16}>
          <div className="fw-card">
            <div className="fw-card-header">
              <IconList />
              <span>宿主机规则</span>
              <Tag size="small">{filteredRules.length} 条</Tag>
              <div className="fw-card-header-actions">
                <Button size="small" icon={<IconVideo />} onClick={onAddVncDefault}>
                  添加 VNC 5900-5999
                </Button>
                <Button
                  size="small"
                  type="primary"
                  theme="light"
                  icon={<IconPlus />}
                  onClick={() => onEditRule()}
                >
                  添加规则
                </Button>
              </div>
            </div>
            <div className="fw-filter-bar">
              <Input
                prefix={<IconSearch />}
                placeholder="搜索端口"
                value={portSearch}
                onChange={setPortSearch}
                showClear
                size="small"
                style={{ width: 130 }}
              />
              <Select
                value={protocolFilter}
                onChange={(v) => setProtocolFilter(v as string)}
                placeholder="协议筛选"
                showClear
                size="small"
                style={{ width: 120 }}
                optionList={[
                  { label: 'TCP', value: 'tcp' },
                  { label: 'UDP', value: 'udp' },
                ]}
              />
              <Select
                value={actionFilter}
                onChange={(v) => setActionFilter(v as string)}
                placeholder="动作筛选"
                showClear
                size="small"
                style={{ width: 120 }}
                optionList={[
                  { label: '允许', value: 'allow' },
                  { label: '拒绝', value: 'deny' },
                ]}
              />
              <Input
                prefix={<IconSearch />}
                placeholder="搜索备注"
                value={remarkSearch}
                onChange={setRemarkSearch}
                showClear
                size="small"
                style={{ width: 150 }}
              />
            </div>
            <Table<HostFirewallRule>
              rowKey="id"
              columns={columns}
              dataSource={filteredRules}
              loading={loading}
              pagination={false}
              size="small"
              empty="暂无防火墙规则"
              scroll={{ y: 460 }}
            />
          </div>
        </Col>
      </Row>
    </div>
  )
}
