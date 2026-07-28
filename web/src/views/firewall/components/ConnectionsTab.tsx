/**
 * 连接管理 Tab
 * - 预览/关闭已建立的 TCP 连接
 * - 非防火墙端口：仅关闭本地端口不在 UFW 允许规则内的连接
 * - 全部连接：包含 SSH 与面板连接，当前会话可能立即断开（高风险）
 */
import { Button, Table, Tag } from '@douyinfe/semi-ui'
import {
  IconAlertTriangle,
  IconClose,
  IconEyeOpenedStroked,
  IconLink,
  IconList,
  IconStop,
} from '@douyinfe/semi-icons'
import type { ColumnProps } from '@douyinfe/semi-ui/lib/es/table'
import type { HostFirewallConnection, HostFirewallConnectionPreview } from '@/api/firewall'

interface ConnectionsTabProps {
  preview: HostFirewallConnectionPreview | null
  /** 预览请求中的模式（用于按钮 loading） */
  previewing: string | null
  onPreview: (mode: 'non_firewall' | 'all') => void
  onCloseConnections: (mode: 'non_firewall' | 'all') => void
}

export default function ConnectionsTab({
  preview,
  previewing,
  onPreview,
  onCloseConnections,
}: ConnectionsTabProps) {
  const columns: ColumnProps<HostFirewallConnection>[] = [
    {
      title: '协议',
      dataIndex: 'protocol',
      width: 90,
      align: 'center',
      render: (text) => <Tag size="small">{(text || '').toUpperCase()}</Tag>,
    },
    {
      title: '本地地址',
      dataIndex: 'local_ip',
      render: (_text, row) => (
        <span className="qvm-mono">
          {row.local_ip}:{row.local_port}
        </span>
      ),
    },
    {
      title: '对端地址',
      dataIndex: 'peer_ip',
      render: (_text, row) => (
        <span className="qvm-mono">
          {row.peer_ip}:{row.peer_port}
        </span>
      ),
    },
    {
      title: '防火墙端口',
      dataIndex: 'allowed_port',
      width: 120,
      align: 'center',
      render: (text) => (
        <Tag size="small" color={text ? 'green' : 'orange'}>
          {text ? '已放行' : '未放行'}
        </Tag>
      ),
    },
  ]

  return (
    <div className="fw-tab-pane">
      {/* 高风险提示 */}
      <div className="fw-banner warning">
        <div className="fw-banner-icon">
          <IconAlertTriangle />
        </div>
        <div className="fw-banner-body">
          <div className="fw-banner-title">高风险操作</div>
          <div className="fw-banner-desc">
            关闭全部连接会断开 SSH 和面板连接，当前会话可能立即断开
          </div>
        </div>
      </div>

      {/* 连接控制 */}
      <div className="fw-card">
        <div className="fw-card-header">
          <IconLink />
          <span>连接控制</span>
        </div>
        <div className="fw-conn-groups">
          <div className="fw-conn-group">
            <div className="fw-conn-label">非防火墙端口</div>
            <div className="fw-conn-desc">关闭本地端口不在 UFW 允许规则内的 TCP 已建立连接</div>
            <div className="fw-conn-actions">
              <Button
                icon={<IconEyeOpenedStroked />}
                loading={previewing === 'non_firewall'}
                onClick={() => onPreview('non_firewall')}
              >
                预览连接
              </Button>
              <Button
                type="warning"
                theme="light"
                icon={<IconClose />}
                onClick={() => onCloseConnections('non_firewall')}
              >
                关闭连接
              </Button>
            </div>
          </div>
          <div className="fw-conn-divider" />
          <div className="fw-conn-group">
            <div className="fw-conn-label">全部连接</div>
            <div className="fw-conn-desc">关闭所有 TCP 已建立连接，包括 SSH 和面板</div>
            <div className="fw-conn-actions">
              <Button
                icon={<IconEyeOpenedStroked />}
                loading={previewing === 'all'}
                onClick={() => onPreview('all')}
              >
                预览连接
              </Button>
              <Button
                type="danger"
                theme="light"
                icon={<IconStop />}
                onClick={() => onCloseConnections('all')}
              >
                关闭全部
              </Button>
            </div>
          </div>
        </div>
      </div>

      {/* 连接预览 */}
      {preview && (
        <div className="fw-card" style={{ marginTop: 16 }}>
          <div className="fw-card-header">
            <IconList />
            <span>连接预览</span>
            <Tag size="small">{preview.connections?.length || 0} 个连接</Tag>
          </div>
          {preview.warning && <div className="fw-conn-warning">{preview.warning}</div>}
          <Table<HostFirewallConnection>
            rowKey={(row) => `${row?.local_ip}:${row?.local_port}-${row?.peer_ip}:${row?.peer_port}`}
            columns={columns}
            dataSource={preview.connections || []}
            pagination={false}
            size="small"
            empty="暂无已建立连接"
            scroll={{ y: 360 }}
          />
        </div>
      )}
    </div>
  )
}
