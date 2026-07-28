/**
 * 日志管理 Tab：日志归档设置 / 磁盘占用 / 文件列表（多选删除、导出 ZIP）
 */
import { useEffect, useState } from 'react'
import { Button, Empty, InputNumber, Table, Tag, Toast } from '@douyinfe/semi-ui'
import { IconDelete, IconDownload, IconFolder, IconPulse, IconRefresh } from '@douyinfe/semi-icons'
import type { ColumnProps } from '@douyinfe/semi-ui/lib/es/table'
import { deleteLogs, getLogStatus, type LogFileItem, type LogStatus } from '@/api/settings'
import { formatFileSize } from '@/utils/format'
import { confirmModal } from '@/utils/confirm'
import { SectionHead, SettingRow } from './SettingRow'
import LogExportDialog from '../dialogs/LogExportDialog'
import { categoryTagColor } from '../logUtils'
import type { SettingsTabProps } from '../types'

export default function LogTab({ form, patch }: SettingsTabProps) {
  const [status, setStatus] = useState<LogStatus>({
    total_size: 0,
    total_size_human: '0 B',
    files: [],
  })
  const [loading, setLoading] = useState(false)
  const [deleting, setDeleting] = useState(false)
  const [selected, setSelected] = useState<string[]>([])
  const [exportVisible, setExportVisible] = useState(false)

  const fetchStatus = async () => {
    setLoading(true)
    try {
      const res = await getLogStatus()
      setStatus({
        total_size: res.data?.total_size || 0,
        total_size_human: res.data?.total_size_human || '0 B',
        files: res.data?.files || [],
        categories: res.data?.categories || [],
      })
      setSelected([])
    } catch {
      // 请求层已统一提示
    } finally {
      setLoading(false)
    }
  }

  useEffect(() => {
    void fetchStatus()
  }, [])

  const handleDelete = async () => {
    if (selected.length === 0) {
      Toast.warning('请先选择要删除的日志文件')
      return
    }
    const ok = await confirmModal({
      title: '删除日志',
      content: `确定要删除选中的 ${selected.length} 个日志文件吗？此操作不可恢复。`,
      okText: '确定删除',
      danger: true,
    })
    if (!ok) return
    setDeleting(true)
    try {
      const res = await deleteLogs({ files: selected })
      Toast.success(res.message || '日志文件已删除')
      await fetchStatus()
    } catch {
      // 请求层已统一提示
    } finally {
      setDeleting(false)
    }
  }

  const columns: ColumnProps<LogFileItem>[] = [
    {
      title: '文件名',
      dataIndex: 'name',
      render: (text, row) => (
        <span className="stg-log-name">
          {text}
          {row.is_today && (
            <Tag size="small" color="green">
              今日日志
            </Tag>
          )}
        </span>
      ),
    },
    {
      title: '类型',
      dataIndex: 'category',
      width: 90,
      render: (text) => (
        <Tag size="small" color={categoryTagColor(text)}>
          {text}
        </Tag>
      ),
    },
    {
      title: '大小',
      dataIndex: 'size',
      width: 100,
      render: (size) => formatFileSize(size),
    },
    { title: '修改时间', dataIndex: 'mod_time', width: 170 },
  ]

  return (
    <div className="stg-tab-pane stg-tab-pane-wide">
      <SectionHead icon={<IconFolder />} title="日志归档设置" />

      <SettingRow
        label="日志最大备份数"
        tip="设置日志文件的最大归档数量，0 表示不限制（仅靠保留天数控制）。超过限制后最旧的归档将被自动删除 | 环境变量: KVM_LOG_MAX_BACKUPS"
      >
        <InputNumber
          value={form.log_max_backups}
          onNumberChange={(v) => patch({ log_max_backups: v })}
          min={0}
          max={10000}
          style={{ width: 260 }}
        />
      </SettingRow>

      <SectionHead icon={<IconPulse />} title="日志磁盘占用" />

      <SettingRow label="总占用大小">
        <div className="stg-host-row">
          <Tag size="large" color="orange">
            {status.total_size_human || '加载中...'}
          </Tag>
          <Button
            size="small"
            icon={<IconRefresh />}
            loading={loading}
            onClick={() => void fetchStatus()}
          >
            刷新
          </Button>
        </div>
      </SettingRow>

      <div className="stg-log-table">
        {!loading && status.files.length === 0 ? (
          <Empty description="暂无日志文件" style={{ padding: '24px 0' }} />
        ) : (
          <Table<LogFileItem>
            rowKey="name"
            columns={columns}
            dataSource={status.files}
            loading={loading}
            size="small"
            pagination={false}
            rowSelection={{
              selectedRowKeys: selected,
              onChange: (keys) => setSelected((keys || []).map(String)),
            }}
          />
        )}
      </div>

      <div className="stg-log-actions">
        <Button
          type="danger"
          theme="light"
          icon={<IconDelete />}
          loading={deleting}
          disabled={selected.length === 0}
          onClick={() => void handleDelete()}
        >
          一键删除
        </Button>
        <Button
          type="primary"
          theme="light"
          icon={<IconDownload />}
          disabled={selected.length === 0}
          onClick={() => setExportVisible(true)}
        >
          一键导出
        </Button>
        <Button
          onClick={() =>
            setSelected(
              selected.length === status.files.length ? [] : status.files.map((f) => f.name),
            )
          }
        >
          {selected.length === status.files.length && status.files.length > 0
            ? '取消全选'
            : '全选'}
        </Button>
      </div>

      <LogExportDialog
        visible={exportVisible}
        files={status.files}
        initialSelected={selected}
        onClose={() => setExportVisible(false)}
      />
    </div>
  )
}
