/**
 * 挂载管理表格：列出所有 VM 的 9p VirtFS 挂载，支持查看挂载命令 / 卸载
 * 迁移自旧前端 views/storage/index.vue 的挂载管理 Tab
 */
import { useMemo } from 'react'
import { Button, Table, Tag, Tooltip } from '@douyinfe/semi-ui'
import { IconCodeStroked, IconRefresh, IconStop } from '@douyinfe/semi-icons'
import type { VmMountItem } from '@/api/storage'

interface MountTableProps {
  mounts: VmMountItem[]
  loading: boolean
  /** 刷新挂载列表 */
  onRefresh: () => void
  /** 查看挂载命令说明 */
  onShowHelp: (tag: string, readonly: boolean) => void
  /** 卸载 */
  onUnmount: (row: VmMountItem) => void
}

export default function MountTable({ mounts, loading, onRefresh, onShowHelp, onUnmount }: MountTableProps) {
  const columns = useMemo(
    () => [
      { title: '虚拟机', dataIndex: 'vm_name', width: 160 },
      { title: '挂载标签', dataIndex: 'tag', width: 200, ellipsis: true },
      { title: '源目录', dataIndex: 'source', ellipsis: true },
      {
        title: '访问模式',
        dataIndex: 'access_mode',
        width: 90,
        render: (v: string) => (
          <Tag size="small" color={v === 'readonly' ? 'blue' : 'orange'}>
            {v === 'readonly' ? '只读' : '读写'}
          </Tag>
        ),
      },
      {
        title: '操作',
        dataIndex: '_actions',
        width: 110,
        align: 'center' as const,
        render: (_: unknown, row: VmMountItem) => (
          <div className="mst-act-cell">
            <Tooltip content="挂载命令" position="top">
              <span
                className="mst-act-ic info"
                onClick={() => onShowHelp(row.tag, row.access_mode === 'readonly')}
              >
                <IconCodeStroked />
              </span>
            </Tooltip>
            <Tooltip content="卸载" position="top">
              <span className="mst-act-ic danger" onClick={() => onUnmount(row)}>
                <IconStop />
              </span>
            </Tooltip>
          </div>
        ),
      },
    ],
    [onShowHelp, onUnmount],
  )

  return (
    <div>
      <div className="mst-toolbar">
        <Button icon={<IconRefresh />} onClick={onRefresh}>
          刷新
        </Button>
      </div>
      <Table
        columns={columns}
        dataSource={mounts}
        rowKey={(r) => `${r?.vm_name}:${r?.tag}`}
        loading={loading}
        pagination={false}
        size="small"
        empty="暂无挂载"
      />
    </div>
  )
}
