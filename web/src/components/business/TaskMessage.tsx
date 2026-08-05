import { Tooltip } from '@douyinfe/semi-ui'

const PRIMARY_GPU_PASSTHROUGH_ERROR_MARKER = '当前宿主机活动的帧缓冲控制台'
const PRIMARY_GPU_PASSTHROUGH_HELP_URL =
  'https://qvmcdocs.xiaozhuhouses.asia/docs/install/common-questions/known-errors#%E6%A0%B8%E6%98%BE%E7%9B%B4%E9%80%9A%E5%87%BA%E7%8E%B0%E6%8B%92%E7%BB%9D%E6%93%8D%E4%BD%9C'

interface TaskMessageProps {
  message?: string
  truncate?: boolean
}

/** 任务状态消息；已知的核显直通保护错误附带排障文档链接。 */
export default function TaskMessage({ message, truncate = false }: TaskMessageProps) {
  if (!message) return <>-</>

  const showPrimaryGpuHelp = message.includes(PRIMARY_GPU_PASSTHROUGH_ERROR_MARKER)

  const messageContent = truncate ? (
    <span style={{ minWidth: 0, flex: 1 }}>
      <Tooltip content={message} position="top">
        <span
          style={{
            display: 'block',
            overflow: 'hidden',
            textOverflow: 'ellipsis',
            whiteSpace: 'nowrap',
          }}
        >
          {message}
        </span>
      </Tooltip>
    </span>
  ) : (
    <span style={{ overflowWrap: 'anywhere', whiteSpace: 'pre-wrap' }}>{message}</span>
  )

  if (!showPrimaryGpuHelp) return messageContent

  return (
    <span
      style={{
        display: 'inline-flex',
        alignItems: 'baseline',
        gap: 6,
        maxWidth: '100%',
        minWidth: 0,
        width: truncate ? '100%' : undefined,
      }}
    >
      {messageContent}
      <a
        href={PRIMARY_GPU_PASSTHROUGH_HELP_URL}
        target="_blank"
        rel="noopener noreferrer"
        style={{ color: 'var(--qvm-acc-ink)', flex: 'none', textDecoration: 'underline' }}
      >
        详情
      </a>
    </span>
  )
}
