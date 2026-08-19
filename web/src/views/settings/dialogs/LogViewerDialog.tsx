/**
 * 日志内容查看对话框：在线预览 .log 文件文本（读取文件末尾，支持向前分页加载更早记录）
 */
import { useMemo, useEffect, useRef, useState } from 'react'
import { Banner, Button, Empty, Modal, Spin, Toast } from '@douyinfe/semi-ui'
import { IconArrowUp, IconRefresh } from '@douyinfe/semi-icons'
import { readLogFile, type LogFileItem } from '@/api/settings'
import { formatFileSize } from '@/utils/format'

interface LogViewerDialogProps {
  visible: boolean
  /** 当前查看的日志文件（.log；.log.gz 由调用方拦截提示，不下发到此） */
  file: LogFileItem | null
  /** 请求日志详情（记录响应 JSON）开关当前状态，用于提示历史记录 */
  detailEnabled: boolean
  onClose: () => void
}

export default function LogViewerDialog({
  visible,
  file,
  detailEnabled,
  onClose,
}: LogViewerDialogProps) {
  const [content, setContent] = useState('')
  const [prevOffset, setPrevOffset] = useState(0)
  const [eof, setEof] = useState(false)
  const [loading, setLoading] = useState(false)
  const [loadingMore, setLoadingMore] = useState(false)
  const viewRef = useRef<HTMLDivElement>(null)

  /** 当前已加载内容中是否包含响应体记录（body= 字段） */
  const hasBodyRecords = useMemo(() => content.includes(' body='), [content])

  // 打开对话框时读取文件末尾
  useEffect(() => {
    if (!visible || !file) return
    if (file.name.endsWith('.gz')) {
      setContent('')
      setEof(true)
      Toast.warning('压缩归档日志不支持在线预览，请下载后查看')
      return
    }
    void loadTail()
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [visible, file])

  /** 重新读取文件末尾（刷新 / 首次打开） */
  const loadTail = async () => {
    if (!file) return
    setLoading(true)
    try {
      const res = await readLogFile({ file: file.name })
      const data = res.data
      setContent(data?.content || '')
      setPrevOffset(data?.prev_offset || 0)
      setEof(!!data?.eof)
      requestAnimationFrame(() => {
        const el = viewRef.current
        if (el) el.scrollTop = el.scrollHeight
      })
    } catch {
      // 请求层已统一提示
    } finally {
      setLoading(false)
    }
  }

  /** 加载更早的记录（向前分页，内容追加到顶部） */
  const loadMore = async () => {
    if (!file || eof || prevOffset <= 0 || loadingMore) return
    setLoadingMore(true)
    try {
      const res = await readLogFile({ file: file.name, offset: prevOffset })
      const data = res.data
      setContent((prev) => (data?.content || '') + prev)
      setPrevOffset(data?.prev_offset || 0)
      setEof(!!data?.eof)
    } catch {
      // 请求层已统一提示
    } finally {
      setLoadingMore(false)
    }
  }

  return (
    <Modal
      title={`查看日志内容${file ? `：${file.name}` : ''}`}
      visible={visible}
      onCancel={onClose}
      width={920}
      style={{ maxWidth: 'calc(100vw - 32px)' }}
      footer={
        <div className="stg-log-viewer-footer">
          <div className="stg-log-viewer-meta">
            {file ? (
              <>
                <span>大小：{formatFileSize(file.size)}</span>
                <span>修改时间：{file.mod_time}</span>
                {eof && <span className="stg-log-viewer-eof">已到文件头</span>}
              </>
            ) : null}
          </div>
          <div className="stg-log-viewer-actions">
            <Button
              icon={<IconArrowUp />}
              loading={loadingMore}
              disabled={eof || prevOffset <= 0}
              onClick={() => void loadMore()}
            >
              加载更早
            </Button>
            <Button icon={<IconRefresh />} loading={loading} onClick={() => void loadTail()}>
              刷新
            </Button>
          </div>
        </div>
      }
    >
      <div className="stg-log-viewer-tip">
        仅展示文件末尾的记录，可通过「加载更早」向前翻页；敏感字段（token、密码、密钥等）在记录时已脱敏。
      </div>
      {!detailEnabled && (
        <Banner
          type="info"
          closeIcon={null}
          className="stg-log-viewer-banner"
          description={
            hasBodyRecords
              ? '已关闭「记录响应 JSON」，本文件中的响应体记录（body=）为关闭前写入的历史日志；关闭后不再产生任何新记录，文件内容保持不变。'
              : '「记录响应 JSON」已关闭，本文件内容为开关关闭前的历史记录，之后不再产生新请求日志。'
          }
        />
      )}
      <Spin spinning={loading}>
        <div className="stg-log-viewer-body" ref={viewRef}>
          {content ? (
            <pre>{content}</pre>
          ) : !loading ? (
            <Empty
              description={file?.name.endsWith('.gz') ? '压缩归档日志不支持在线预览' : '暂无日志内容'}
              style={{ padding: '48px 0' }}
            />
          ) : null}
        </div>
      </Spin>
    </Modal>
  )
}