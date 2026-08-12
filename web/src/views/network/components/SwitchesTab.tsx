/**
 * 交换机 Tab（管理员 + 普通用户）
 * - 顶部配额摘要：下行/上行月流量、下行/上行带宽
 * - 表格：名称 / VLAN / 目标网桥 / 桥接安全 / 子网 / 网关 / 流量进度 / 带宽 / 限速状态 / 操作
 * - 搜索（名称、子网）+ 客户端分页（100 条/页）
 */
import { useEffect, useMemo, useState } from 'react'
import { Button, Dropdown, Input, Pagination, Progress, Table, Tag, Tooltip } from '@douyinfe/semi-ui'
import {
  IconBranch,
  IconDelete,
  IconDownloadStroked,
  IconEditStroked,
  IconEyeOpenedStroked,
  IconMore,
  IconPlus,
  IconRefresh,
  IconSearch,
  IconSend,
  IconUpload,
} from '@douyinfe/semi-icons'
import type { ColumnProps } from '@douyinfe/semi-ui/lib/es/table'
import type { VpcQuota, VpcSwitch } from '@/api/vpc'
import { switchBandwidthText, switchTrafficPercent, switchTrafficText } from '../utils'

const PAGE_SIZE = 100

interface SwitchesTabProps {
  isAdmin: boolean
  switches: VpcSwitch[]
  quota: VpcQuota | null
  loading: boolean
  onCreate: () => void
  onEdit: (row: VpcSwitch) => void
  onDelete: (row: VpcSwitch) => void
  onResetTraffic: (row: VpcSwitch) => void
  onViewVMs: (row: VpcSwitch) => void
  onMigrate: (row: VpcSwitch) => void
  operatingSwitchIds: Set<number>
}

/** 配额摘要卡 */
function QuotaCard({
  icon,
  label,
  remaining,
  max,
  allocated,
  unit,
  iconClass,
}: {
  icon: React.ReactNode
  label: string
  remaining: number | undefined
  max: number | undefined
  allocated: number | undefined
  unit: string
  iconClass: string
}) {
  const unlimited = !max || max === -1 || remaining === -1
  return (
    <div className="net-stat-card">
      <div className={`net-stat-icon ${iconClass}`}>{icon}</div>
      <div>
        <div className="net-stat-label">{label}</div>
        <div className="net-stat-value">{unlimited ? '不限' : `${remaining ?? 0} ${unit}`}</div>
        <div className="net-stat-sub">已分配 {allocated || 0} {unit}</div>
      </div>
    </div>
  )
}

/** 月流量单元格（文案 + 进度条） */
function TrafficCell({ row, direction }: { row: VpcSwitch; direction: 'down' | 'up' }) {
  const quota = direction === 'down' ? row.traffic_down_gb : row.traffic_up_gb
  const limited = direction === 'down' ? row.is_limited_down : row.is_limited_up
  return (
    <div className="net-traffic-cell">
      <span className="net-traffic-text">{switchTrafficText(row, direction)}</span>
      {quota > 0 && (
        <Progress
          percent={switchTrafficPercent(row, direction)}
          showInfo={false}
          stroke={limited ? 'var(--semi-color-danger)' : 'var(--semi-color-primary)'}
          strokeWidth={6}
        />
      )}
    </div>
  )
}

export default function SwitchesTab({
  isAdmin,
  switches,
  quota,
  loading,
  onCreate,
  onEdit,
  onDelete,
  onResetTraffic,
  onViewVMs,
  onMigrate,
  operatingSwitchIds,
}: SwitchesTabProps) {
  const [searchName, setSearchName] = useState('')
  const [searchSubnet, setSearchSubnet] = useState('')
  const [page, setPage] = useState(1)

  const filtered = useMemo(() => {
    let data = switches
    if (searchName) {
      const q = searchName.toLowerCase()
      data = data.filter((s) => s.name.toLowerCase().includes(q))
    }
    if (searchSubnet) {
      const q = searchSubnet.toLowerCase()
      data = data.filter((s) => (s.cidr || '').toLowerCase().includes(q))
    }
    return data
  }, [switches, searchName, searchSubnet])

  useEffect(() => {
    setPage(1)
  }, [searchName, searchSubnet])

  const paged = useMemo(
    () => filtered.slice((page - 1) * PAGE_SIZE, page * PAGE_SIZE),
    [filtered, page],
  )

  const columns: ColumnProps<VpcSwitch>[] = [
    ...(isAdmin
      ? [{ title: '所属用户', dataIndex: 'username', width: 110 } as ColumnProps<VpcSwitch>]
      : []),
    {
      title: '名称',
      dataIndex: 'name',
      render: (text, row) => (
        <div className="net-name-cell">
          <IconBranch />
          <span>{text}</span>
          {row.is_system && <Tag size="small">系统</Tag>}
        </div>
      ),
    },
    {
      title: '运行模式',
      dataIndex: 'uplink_mode',
      width: 145,
      render: (_text, row) => {
        if (row.is_system) return <Tag size="small">系统基础网络</Tag>
        if (row.dhcp_enabled) return <Tag size="small" color="blue">内置 DHCP/NAT</Tag>
        if (row.uplink_if) return <Tag size="small" color="orange">物理直通</Tag>
        return <Tag size="small" color="green">空交换机</Tag>
      },
    },
    ...(isAdmin
      ? [
          {
            title: '上行链路',
            dataIndex: 'uplink_if',
            width: 120,
            render: (text: string, row: VpcSwitch) => (
              <span className="qvm-mono">{row.is_system ? '系统出口' : text || '未绑定'}</span>
            ),
          } as ColumnProps<VpcSwitch>,
          {
            title: '桥接 VLAN',
            dataIndex: 'bridge_vlan_id',
            width: 90,
            align: 'center',
            render: (_text: unknown, row: VpcSwitch) =>
              !row.dhcp_enabled && !!row.uplink_if
                ? row.bridge_vlan_id > 0
                  ? row.bridge_vlan_id
                  : '不打标签'
                : '—',
          } as ColumnProps<VpcSwitch>,
          {
            title: '桥接安全',
            dataIndex: 'allow_promiscuous',
            width: 200,
            render: (_text: unknown, row: VpcSwitch) =>
              !row.dhcp_enabled && !!row.uplink_if ? (
                <div className="net-security-tags">
                  <Tag size="small" color={row.allow_promiscuous ? 'orange' : 'green'}>
                    混杂{row.allow_promiscuous ? '允许' : '拒绝'}
                  </Tag>
                  <Tag size="small" color={row.allow_mac_change ? 'orange' : 'green'}>
                    MAC{row.allow_mac_change ? '允许' : '拒绝'}
                  </Tag>
                  <Tag size="small" color={row.allow_forged_transmits ? 'orange' : 'green'}>
                    伪传输{row.allow_forged_transmits ? '允许' : '拒绝'}
                  </Tag>
                </div>
              ) : (
                '—'
              ),
          } as ColumnProps<VpcSwitch>,
        ]
      : []),
    {
      title: '子网',
      dataIndex: 'cidr',
      render: (_text, row) => (
        <span className="qvm-mono">
          {row.dhcp_enabled || row.is_system ? row.cidr || '系统配置' : row.uplink_if ? '上级网络分配' : '纯二层'}
        </span>
      ),
    },
    {
      title: '网关',
      dataIndex: 'gateway_ip',
      render: (text, row) => <span className="qvm-mono">{row.dhcp_enabled || row.is_system ? text || '—' : '—'}</span>,
    },
    {
      title: '月下行流量',
      dataIndex: 'traffic_down_gb',
      width: 170,
      render: (_text, row) => <TrafficCell row={row} direction="down" />,
    },
    {
      title: '月上行流量',
      dataIndex: 'traffic_up_gb',
      width: 170,
      render: (_text, row) => <TrafficCell row={row} direction="up" />,
    },
    {
      title: '下行带宽',
      dataIndex: 'bandwidth_down_mbps',
      width: 110,
      align: 'center',
      render: (_text, row) => (
        <span className={row.is_limited_down ? 'net-text-danger' : ''}>
          {switchBandwidthText(row.effective_bandwidth_down_mbps || row.bandwidth_down_mbps)}
        </span>
      ),
    },
    {
      title: '上行带宽',
      dataIndex: 'bandwidth_up_mbps',
      width: 110,
      align: 'center',
      render: (_text, row) => (
        <span className={row.is_limited_up ? 'net-text-danger' : ''}>
          {switchBandwidthText(row.effective_bandwidth_up_mbps || row.bandwidth_up_mbps)}
        </span>
      ),
    },
    {
      title: '限速状态',
      dataIndex: 'is_limited_down',
      width: 90,
      align: 'center',
      render: (_text, row) => (
        <Tag size="small" color={row.is_limited_down || row.is_limited_up ? 'red' : 'green'}>
          {row.is_limited_down || row.is_limited_up ? '已限速' : '正常'}
        </Tag>
      ),
    },
    {
      title: '操作',
      dataIndex: 'actions',
      width: 92,
      render: (_text, row) => {
        const operating = operatingSwitchIds.has(row.id)
        return (
          <div className="qvm-act-cell" onClick={(event) => event.stopPropagation()}>
            <Tooltip content="查看虚拟机" position="top">
              <span className="qvm-act-ic" onClick={() => onViewVMs(row)}>
                <IconEyeOpenedStroked />
              </span>
            </Tooltip>
            <Dropdown
              trigger="click"
              position="bottomRight"
              clickToHide
              render={
                <Dropdown.Menu>
                  <Dropdown.Item icon={<IconEditStroked />} disabled={row.is_system || operating} onClick={() => onEdit(row)}>
                    编辑
                  </Dropdown.Item>
                  {isAdmin && row.legacy_migration_required && (
                    <Dropdown.Item icon={<IconSend />} disabled={operating} onClick={() => onMigrate(row)}>
                      迁移为空交换机
                    </Dropdown.Item>
                  )}
                  {isAdmin && (
                    <Dropdown.Item icon={<IconRefresh />} disabled={row.is_system || operating} onClick={() => onResetTraffic(row)}>
                      重置流量
                    </Dropdown.Item>
                  )}
                  <Dropdown.Divider />
                  <Dropdown.Item icon={<IconDelete />} type="danger" disabled={row.is_system || operating} onClick={() => onDelete(row)}>
                    删除
                  </Dropdown.Item>
                </Dropdown.Menu>
              }
            >
              <span className={`qvm-act-ic more ${row.is_system ? 'disabled' : ''}`}>
                {operating ? <IconRefresh spin /> : <IconMore />}
              </span>
            </Dropdown>
          </div>
        )
      },
    },
  ]

  return (
    <div>
      {/* 配额摘要 */}
      {quota && (
        <div className="net-stat-grid">
          <QuotaCard
            icon={<IconDownloadStroked />}
            label="下行流量"
            remaining={quota.remaining_traffic_down}
            max={quota.max_traffic_down}
            allocated={quota.allocated_traffic_down}
            unit="GB"
            iconClass="down"
          />
          <QuotaCard
            icon={<IconUpload />}
            label="上行流量"
            remaining={quota.remaining_traffic_up}
            max={quota.max_traffic_up}
            allocated={quota.allocated_traffic_up}
            unit="GB"
            iconClass="up"
          />
          <QuotaCard
            icon={<IconBranch />}
            label="下行带宽"
            remaining={quota.remaining_bandwidth_down}
            max={quota.max_bandwidth_down}
            allocated={quota.allocated_bandwidth_down}
            unit="Mbps"
            iconClass="down-bandwidth"
          />
          <QuotaCard
            icon={<IconBranch />}
            label="上行带宽"
            remaining={quota.remaining_bandwidth_up}
            max={quota.max_bandwidth_up}
            allocated={quota.allocated_bandwidth_up}
            unit="Mbps"
            iconClass="up-bandwidth"
          />
        </div>
      )}

      {/* 工具栏 */}
      <div className="net-toolbar">
        <div className="net-toolbar-left">
          <span className="net-table-title">交换机列表</span>
          <Tag size="small">{switches.length} 个</Tag>
        </div>
        <div className="net-toolbar-right">
          <Button type="primary" theme="light" icon={<IconPlus />} onClick={onCreate}>
            创建交换机
          </Button>
        </div>
      </div>

      {/* 筛选 */}
      <div className="net-filter-bar">
        <Input
          prefix={<IconSearch />}
          placeholder="搜索名称"
          value={searchName}
          onChange={setSearchName}
          showClear
          style={{ width: 180 }}
        />
        <Input
          prefix={<IconSearch />}
          placeholder="搜索子网"
          value={searchSubnet}
          onChange={setSearchSubnet}
          showClear
          style={{ width: 200 }}
        />
      </div>

      <div className="net-table-card">
        <Table<VpcSwitch>
          rowKey="id"
          columns={columns}
          dataSource={paged}
          loading={loading}
          pagination={false}
          size="small"
          empty="暂无交换机"
        />
        {filtered.length > PAGE_SIZE && (
          <div className="net-pagination">
            <Pagination
              total={filtered.length}
              pageSize={PAGE_SIZE}
              currentPage={page}
              onPageChange={setPage}
              showTotal
            />
          </div>
        )}
      </div>
    </div>
  )
}
