/**
 * 任务详情抽屉（底部任务栏与任务中心页共用）
 * - 展示任务基础信息、参数与执行结果 JSON
 * - 执行结果中包含 download_path / extra_downloads 时提供下载按钮
 */
import { Button, Descriptions, SideSheet, Tag } from '@douyinfe/semi-ui'
import { IconDownload } from '@douyinfe/semi-icons'
import { taskStatusText, taskTypeColor, taskTypeText } from '@/stores/task'
import type { TaskItem } from '@/api/task'
import { getTemplateExportDownloadUrl } from '@/api/template'
import { formatDateTime } from '@/utils/format'

interface DownloadLink {
  label: string
  path: string
}

/** 从任务结果 JSON 中提取可下载文件链接 */
function extractDownloadLinks(resultStr?: string): DownloadLink[] {
  if (!resultStr) return []
  try {
    const parsed = JSON.parse(resultStr) as {
      download_path?: string
      file_name?: string
      extra_downloads?: Array<{ download_path?: string; label?: string; file_name?: string }>
    }
    const links: DownloadLink[] = []
    if (parsed.download_path) {
      links.push({
        label: parsed.file_name ? `下载 ${parsed.file_name}` : '下载文件',
        path: parsed.download_path,
      })
    }
    if (Array.isArray(parsed.extra_downloads)) {
      parsed.extra_downloads.forEach((item) => {
        if (item?.download_path) {
          links.push({
            label: item.label || (item.file_name ? `下载 ${item.file_name}` : '下载附加文件'),
            path: item.download_path,
          })
        }
      })
    }
    return links
  } catch {
    return []
  }
}

/** JSON 字符串美化 */
function formatJSON(jsonStr: string): string {
  try {
    return JSON.stringify(JSON.parse(jsonStr), null, 2)
  } catch {
    return jsonStr
  }
}

/** JSON 预览块统一样式 */
const jsonPreStyle: React.CSSProperties = {
  margin: 0,
  padding: 12,
  borderRadius: 10,
  fontSize: 11,
  lineHeight: 1.6,
  maxHeight: 220,
  overflow: 'auto',
  background: 'var(--qvm-hover-bg)',
  border: '1px solid var(--qvm-stroke)',
  color: 'var(--qvm-text-1)',
}

interface TaskDetailSheetProps {
  task: TaskItem | null
  visible: boolean
  onClose: () => void
}

export default function TaskDetailSheet({ task, visible, onClose }: TaskDetailSheetProps) {
  const downloadLinks = extractDownloadLinks(task?.result)

  const handleDownload = (link: DownloadLink) => {
    const url = getTemplateExportDownloadUrl(link.path)
    if (url) window.open(url, '_blank')
  }

  return (
    <SideSheet title="任务详情" visible={visible} onCancel={onClose} width={480}>
      {task && (
        <div>
          <Descriptions align="left" size="medium">
            <Descriptions.Item itemKey="任务 ID">{task.id}</Descriptions.Item>
            <Descriptions.Item itemKey="任务类型">
              <Tag
                style={{
                  color: taskTypeColor(task.type).color,
                  backgroundColor: taskTypeColor(task.type).bg,
                  border: `1px solid ${taskTypeColor(task.type).border}`,
                }}
              >
                {taskTypeText(task.type)}
              </Tag>
            </Descriptions.Item>
            <Descriptions.Item itemKey="状态">{taskStatusText(task.status)}</Descriptions.Item>
            <Descriptions.Item itemKey="进度">{task.progress || 0}%</Descriptions.Item>
            <Descriptions.Item itemKey="状态消息">{task.message || '-'}</Descriptions.Item>
            <Descriptions.Item itemKey="创建人">{task.created_by || '-'}</Descriptions.Item>
            <Descriptions.Item itemKey="创建时间">{formatDateTime(task.created_at)}</Descriptions.Item>
            <Descriptions.Item itemKey="更新时间">{formatDateTime(task.updated_at)}</Descriptions.Item>
          </Descriptions>

          {task.params && (
            <div style={{ marginTop: 16 }}>
              <h4 style={{ margin: '0 0 8px', color: 'var(--qvm-text-0)' }}>任务参数</h4>
              <pre className="qvm-num" style={jsonPreStyle}>
                {formatJSON(task.params)}
              </pre>
            </div>
          )}

          {task.result && (
            <div style={{ marginTop: 16 }}>
              <h4 style={{ margin: '0 0 8px', color: 'var(--qvm-text-0)' }}>执行结果</h4>
              <pre className="qvm-num" style={jsonPreStyle}>
                {formatJSON(task.result)}
              </pre>
            </div>
          )}

          {downloadLinks.length > 0 && (
            <div style={{ marginTop: 16 }}>
              <h4 style={{ margin: '0 0 8px', color: 'var(--qvm-text-0)' }}>结果下载</h4>
              <div style={{ display: 'flex', gap: 8, flexWrap: 'wrap' }}>
                {downloadLinks.map((link) => (
                  <Button
                    key={link.path}
                    theme="light"
                    type="primary"
                    icon={<IconDownload />}
                    onClick={() => handleDownload(link)}
                  >
                    {link.label}
                  </Button>
                ))}
              </div>
            </div>
          )}
        </div>
      )}
    </SideSheet>
  )
}
