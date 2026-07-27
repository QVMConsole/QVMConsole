/**
 * 安全组策略 Tab（管理员 + 普通用户）
 * - 表格：名称 / 类型 / 备注 / 规则数 / 操作；行展开显示规则列表
 * - 展开面板内联管理规则（添加/删除）
 * - 搜索（名称、类型）+ 客户端分页（100 条/页）
 */
import { useEffect, useMemo, useState } from 'react'
import { Button, Empty, Input, Pagination, Select, Table, Tag, Tooltip } from '@douyinfe/semi-ui'
import { IconLock, IconPlus, IconSearch } from '@douyinfe/semi-icons'
import type { ColumnProps } from '@douyinfe/semi-ui/lib/es/table'
import type { VpcSecurityGroup, VpcSecurityGroupRule } from '@/api/vpc'
import { directionText, portText, targetText } from '../utils'

const PAGE_SIZE = 100

interface SecurityGroupsTabProps {
  isAdmin: boolean
  groups: VpcSecurityGroup[]
  loading: boolean
  onCreate: () => void
  onEdit: (row: VpcSecurityGroup) => void
  onDelete: (row: VpcSecurityGroup) => void
  onAddRule: (group: VpcSecurityGroup) => void
  onDeleteRule: (rule: VpcSecurityGroupRule) => void
}

/** 展开面板：规则列表 */
function RulePanel({
  group,
  onAddRule,
  onDeleteRule,
}: {
  group: VpcSecurityGroup
  onAddRule: (group: VpcSecurityGroup) => void
  onDeleteRule: (rule: VpcSecurityGroupRule) => void
}) {
  const ruleColumns: ColumnProps<VpcSecurityGroupRule>[] = [
    {
      title: '方向',
      dataIndex: 'direction',
      width: 80,
      align: 'center',
      render: (text) => (
        <Tag size="small" color={text === 'ingress' ? 'green' : 'orange'}>
          {directionText(text)}
        </Tag>
      ),
    },
    {
      title: '协议',
      dataIndex: 'protocol',
      width: 80,
      align: 'center',
      render: (text) => <span className="qvm-mono">{String(text || '').toUpperCase()}</span>,
    },
    {
      title: '端口范围',
      dataIndex: 'port_start',
      width: 120,
      render: (_text, rule) => <span className="qvm-mono">{portText(rule)}</span>,
    },
    {
      title: '目标',
      dataIndex: 'target_value',
      render: (_text, rule) => <span className="qvm-mono">{targetText(rule)}</span>,
    },
    {
      title: '备注',
      dataIndex: 'remark',
      render: (text) => text || <span className="net-text-muted">—</span>,
    },
    {
      title: '操作',
      dataIndex: 'actions',
      width: 80,
      align: 'center',
      render: (_text, rule) => (
        <Button size="small" theme="borderless" type="danger" onClick={() => onDeleteRule(rule)}>
          删除
        </Button>
      ),
    },
  ]

  return (
    <div className="net-rule-panel">
      <div className="net-rule-header">
        <span className="net-rule-title">安全组规则（{group.name}）</span>
        <Button size="small" type="primary" icon={<IconPlus />} onClick={() => onAddRule(group)}>
          添加规则
        </Button>
      </div>
      {group.rules?.length ? (
        <Table<VpcSecurityGroupRule>
          rowKey="id"
          columns={ruleColumns}
          dataSource={group.rules}
          pagination={false}
          size="small"
        />
      ) : (
        <Empty description="暂无规则，点击上方按钮添加" />
      )}
    </div>
  )
}

export default function SecurityGroupsTab({
  isAdmin,
  groups,
  loading,
  onCreate,
  onEdit,
  onDelete,
  onAddRule,
  onDeleteRule,
}: SecurityGroupsTabProps) {
  const [searchName, setSearchName] = useState('')
  const [typeFilter, setTypeFilter] = useState<string>('')
  const [page, setPage] = useState(1)

  const filtered = useMemo(() => {
    let data = groups
    if (searchName) {
      const q = searchName.toLowerCase()
      data = data.filter((g) => g.name.toLowerCase().includes(q))
    }
    if (typeFilter !== '') {
      const wantDefault = typeFilter === 'default'
      data = data.filter((g) => g.is_default === wantDefault)
    }
    return data
  }, [groups, searchName, typeFilter])

  useEffect(() => {
    setPage(1)
  }, [searchName, typeFilter])

  const paged = useMemo(
    () => filtered.slice((page - 1) * PAGE_SIZE, page * PAGE_SIZE),
    [filtered, page],
  )

  const columns: ColumnProps<VpcSecurityGroup>[] = [
    ...(isAdmin
      ? [{ title: '所属用户', dataIndex: 'username', width: 110 } as ColumnProps<VpcSecurityGroup>]
      : []),
    {
      title: '名称',
      dataIndex: 'name',
      render: (text) => (
        <div className="net-name-cell">
          <IconLock />
          <span>{text}</span>
        </div>
      ),
    },
    {
      title: '类型',
      dataIndex: 'is_default',
      width: 90,
      align: 'center',
      render: (text) => (
        <Tag size="small" color={text ? 'green' : 'grey'}>
          {text ? '默认' : '自定义'}
        </Tag>
      ),
    },
    {
      title: '备注',
      dataIndex: 'remark',
      render: (text) =>
        text ? (
          <span className="net-text-muted">{text}</span>
        ) : (
          <span className="net-text-muted">—</span>
        ),
    },
    {
      title: '规则数',
      dataIndex: 'rules',
      width: 90,
      align: 'center',
      render: (_text, row) => <Tag size="small">{row.rules?.length || 0} 条</Tag>,
    },
    {
      title: '操作',
      dataIndex: 'actions',
      width: 150,
      render: (_text, row) => (
        <div className="net-row-actions">
          <Button size="small" theme="borderless" type="primary" onClick={() => onEdit(row)}>
            编辑
          </Button>
          <Tooltip content="默认安全组用于兜底策略，不能删除" disabled={!row.is_default}>
            <Button
              size="small"
              theme="borderless"
              type="danger"
              disabled={row.is_default}
              onClick={() => onDelete(row)}
            >
              {row.is_default ? '受保护' : '删除'}
            </Button>
          </Tooltip>
        </div>
      ),
    },
  ]

  return (
    <div>
      {/* 工具栏 */}
      <div className="net-toolbar">
        <div className="net-toolbar-left">
          <span className="net-table-title">安全组列表</span>
          <Tag size="small">{groups.length} 个</Tag>
        </div>
        <div className="net-toolbar-right">
          <Button type="primary" theme="light" icon={<IconPlus />} onClick={onCreate}>
            创建安全组
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
        <Select
          placeholder="类型筛选"
          value={typeFilter}
          onChange={(v) => setTypeFilter(String(v ?? ''))}
          showClear
          style={{ width: 140 }}
          optionList={[
            { value: 'default', label: '默认' },
            { value: 'custom', label: '自定义' },
          ]}
        />
      </div>

      <div className="net-table-card">
        <Table<VpcSecurityGroup>
          rowKey="id"
          columns={columns}
          dataSource={paged}
          loading={loading}
          pagination={false}
          size="small"
          empty="暂无安全组"
          expandedRowRender={(row) =>
            row ? (
              <RulePanel group={row} onAddRule={onAddRule} onDeleteRule={onDeleteRule} />
            ) : null
          }
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
