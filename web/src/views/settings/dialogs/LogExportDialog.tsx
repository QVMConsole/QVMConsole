/**
 * 日志导出选择对话框：从日志文件列表中挑选后打包为 ZIP 下载
 */
import { useEffect, useState } from 'react'
import { Button, Modal, Table, Tag, Toast } from '@douyinfe/semi-ui'
import type { ColumnProps } from '@douyinfe/semi-ui/lib/es/table'
import { exportLogs, type LogFileItem } from '@/api/settings'
import { formatFileSize } from '@/utils/format'
import { downloadBlob, timestampFilename } from '@/utils/download'
import { categoryTagColor } from '../logUtils'

interface LogExportDialogProps {
  visible: boolean
  files: LogFileItem[]
  /** 打开时默认选中的文件名 */
  initialSelected: string[]
  onClose: () => void
}

export default function LogExportDialog({
  visible,
  files,
  initialSelected,
  onClose,
}: LogExportDialogProps) {
  const [selected, setSelected] = useState<string[]>([])
  const [exporting, setExporting] = useState(false)

  useEffect(() => {
    if (visible) setSelected(initialSelected)
  }, [visible, initialSelected])

  const handleExport = async () => {
    if (selected.length === 0) {
      Toast.warning('请选择要导出的日志文件')
      return
    }
    setExporting(true)
    try {
      const res = await exportLogs({ files: selected })
      downloadBlob(res.data, timestampFilename('qvmconsole-logs', 'zip'))
      Toast.success('日志导出成功')
      onClose()
    } catch {
      // 请求层已统一提示
    } finally {
      setExporting(false)
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
  ]

  return (
    <Modal
      title="选择要导出的日志文件"
      visible={visible}
      onCancel={onClose}
      width={680}
      footer={
        <div className="stg-export-footer">
          <Button
            onClick={() =>
              setSelected(selected.length === files.length ? [] : files.map((f) => f.name))
            }
          >
            {selected.length === files.length ? '取消全选' : '全选'}
          </Button>
          <div className="stg-export-footer-actions">
            <Button onClick={onClose}>取消</Button>
            <Button
              type="primary"
              theme="solid"
              loading={exporting}
              disabled={selected.length === 0}
              onClick={() => void handleExport()}
            >
              导出为 ZIP
            </Button>
          </div>
        </div>
      }
    >
      <div className="stg-plain-tip" style={{ marginBottom: 10 }}>
        已选中 {selected.length} 个文件，将打包为单个 ZIP 文件导出
      </div>
      <Table<LogFileItem>
        rowKey="name"
        columns={columns}
        dataSource={files}
        size="small"
        pagination={false}
        style={{ maxHeight: 400, overflowY: 'auto' }}
        rowSelection={{
          selectedRowKeys: selected,
          onChange: (keys) => setSelected((keys || []).map(String)),
        }}
      />
    </Modal>
  )
}
