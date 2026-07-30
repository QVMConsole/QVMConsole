/**
 * 文件列表表格：iso / share / disk 三个类别共用
 * - iso：文件名 / 系统类型 / 大小 / 上传时间 / 删除
 * - share：文件名 / 大小 / 上传时间 / 下载 + 删除
 * - disk：文件名 / 大小 / 时间 / 下载 + 删除
 * 行内操作使用纯图标 + Tooltip 模式
 */
import { useMemo } from 'react'
import { Button, Table, Tag, Tooltip } from '@douyinfe/semi-ui'
import { IconDelete, IconDownload, IconLink, IconRefresh, IconUpload } from '@douyinfe/semi-icons'
import type { StorageFileItem, StorageCategory } from '@/api/storage'

interface FileTableProps {
  category: StorageCategory
  files: StorageFileItem[]
  loading: boolean
  readonly?: boolean
  /** 点击上传按钮 */
  onUpload: () => void
  /** 刷新文件列表 */
  onRefresh: () => void
  /** 挂载到虚拟机（仅 iso / share） */
  onMount?: () => void
  /** 下载文件 */
  onDownload: (row: StorageFileItem) => void
  /** 删除文件 */
  onDelete: (row: StorageFileItem) => void
}

/** 各类别允许的文件扩展名（accept 属性） */
const ACCEPT_MAP: Record<StorageCategory, string> = {
  iso: '.iso',
  share: '',
  disk: '.qcow2,.raw,.vmdk,.vhd,.vhdx,.img,.vfd,.ova,.ovf,.mf',
}

/** 各类别的上传按钮文字 */
const UPLOAD_LABEL_MAP: Record<StorageCategory, string> = {
  iso: '上传 ISO',
  share: '上传文件',
  disk: '上传磁盘文件',
}

/** 各类别的空状态文字 */
const EMPTY_LABEL_MAP: Record<StorageCategory, string> = {
  iso: '暂无 ISO 文件',
  share: '暂无共享文件',
  disk: '暂无磁盘文件',
}

export default function FileTable({
  category,
  files,
  loading,
  readonly,
  onUpload,
  onRefresh,
  onMount,
  onDownload,
  onDelete,
}: FileTableProps) {
  const columns = useMemo(() => {
    const cols: Array<Record<string, unknown>> = [
      {
        title: '文件名',
        dataIndex: 'name',
        ellipsis: true,
      },
    ]

    // ISO 类别多一列「系统类型」
    if (category === 'iso') {
      cols.push({
        title: '系统类型',
        dataIndex: 'os_type',
        width: 110,
        render: (v: string | undefined) =>
          v ? (
            <Tag size="small" color={v === 'windows' ? 'orange' : 'green'}>
              {v === 'windows' ? 'Windows' : 'Linux'}
            </Tag>
          ) : (
            <span style={{ color: 'var(--qvm-text-2)' }}>-</span>
          ),
      })
    }

    cols.push(
      { title: '大小', dataIndex: 'size_text', width: 110 },
      {
        title: category === 'disk' ? '时间' : '上传时间',
        dataIndex: 'mod_time',
        width: 170,
      },
      {
        title: '操作',
        dataIndex: '_actions',
        width: category === 'iso' ? 70 : 110,
        align: 'center' as const,
        render: (_: unknown, row: StorageFileItem) => (
          <div className="mst-act-cell">
            {category !== 'iso' && (
              <Tooltip content="下载" position="top">
                <span className="mst-act-ic info" onClick={() => onDownload(row)}>
                  <IconDownload />
                </span>
              </Tooltip>
            )}
            <Tooltip content="删除" position="top">
              <span className="mst-act-ic danger" onClick={() => onDelete(row)}>
                <IconDelete />
              </span>
            </Tooltip>
          </div>
        ),
      },
    )

    return cols
  }, [category, onDownload, onDelete])

  return (
    <div>
      {/* 工具栏 */}
      <div className="mst-toolbar">
        <Button
          type="primary"
          icon={<IconUpload />}
          onClick={onUpload}
          disabled={readonly}
        >
          {UPLOAD_LABEL_MAP[category]}
        </Button>
        <Button icon={<IconRefresh />} onClick={onRefresh}>
          刷新
        </Button>
        {onMount && (category === 'iso' || category === 'share') && (
          <Button type="primary" theme="light" icon={<IconLink />} onClick={onMount}>
            挂载到虚拟机
          </Button>
        )}
      </div>

      {/* disk 类别提示 */}
      {category === 'disk' && (
        <div className="mst-cat-tip">
          此目录存放虚拟机导出的磁盘、OVA/OVF 虚拟机包和配套清单（支持 .qcow2、.raw、.vmdk、.vhd、.vhdx、.img、.vfd、.ova、.ovf、.mf）
        </div>
      )}

      {/* 文件表格 */}
      <Table
        columns={columns}
        dataSource={files}
        rowKey="name"
        loading={loading}
        pagination={false}
        size="small"
        empty={EMPTY_LABEL_MAP[category]}
      />
    </div>
  )
}

export { ACCEPT_MAP }
