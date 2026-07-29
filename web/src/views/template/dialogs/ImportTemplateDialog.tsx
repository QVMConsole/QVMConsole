/**
 * 导入模板包弹窗：上传文件（分片上传/秒传/断点续传）或主机绝对路径 → 解析预览 → 确认导入
 * 迁移自旧前端 views/template/index.vue 的导入对话框
 */
import { useRef, useState } from 'react'
import { Banner, Button, Input, Modal, Progress, Radio, Table, Tag, Toast } from '@douyinfe/semi-ui'
import { IconClose, IconUpload } from '@douyinfe/semi-icons'
import {
  confirmImportTemplate,
  previewImportTemplate,
  templateUploadCancel,
  templateUploadChunk,
  templateUploadComplete,
  templateUploadInit,
  type ImportTemplatePreview,
  type ImportPreviewNode,
} from '@/api/template'
import { ChunkUploader } from '@/utils/chunkUploader'
import { templateGroupLabel } from '@/utils/templateCategory'
import { useMountModalLifecycle } from '@/hooks/useMountModalLifecycle'

interface ImportTemplateDialogProps {
  onClose: () => void
  /** 确认导入成功后回调（用于刷新列表） */
  onImported: () => void
}

type ImportMode = 'upload' | 'source_path'

/** 上传阶段：哈希计算 / 分片上传 */
type UploadPhase = 'hash' | 'upload'

const ACCEPT_EXT = ['.tar.gz', '.tgz']

export default function ImportTemplateDialog({ onClose, onImported }: ImportTemplateDialogProps) {
  const { modalVisible, requestClose, afterModalClose } = useMountModalLifecycle(onClose)
  const [importMode, setImportMode] = useState<ImportMode>('upload')
  const [sourcePath, setSourcePath] = useState('')
  const [file, setFile] = useState<File | null>(null)
  const [uploading, setUploading] = useState(false)
  const [phase, setPhase] = useState<UploadPhase>('upload')
  const [progress, setProgress] = useState(0)
  const [submitting, setSubmitting] = useState(false)
  const [confirming, setConfirming] = useState(false)
  const [preview, setPreview] = useState<ImportTemplatePreview | null>(null)
  // 分片上传产出的临时包路径，未导入而关闭时据此清理
  const [sessionKey, setSessionKey] = useState('')
  const fileInputRef = useRef<HTMLInputElement>(null)
  const uploaderRef = useRef<ChunkUploader | null>(null)

  const busy = submitting || confirming

  // ==================== 文件选择 ====================
  const handlePickFile = () => fileInputRef.current?.click()

  const handleFileChange = (e: React.ChangeEvent<HTMLInputElement>) => {
    const picked = e.target.files?.[0] || null
    e.target.value = '' // 允许重复选择同一文件
    if (!picked) return
    const lower = picked.name.toLowerCase()
    if (!ACCEPT_EXT.some((ext) => lower.endsWith(ext))) {
      Toast.warning('仅支持 .tar.gz / .tgz 模板链路包')
      return
    }
    setFile(picked)
    setPreview(null)
  }

  const handleClearFile = () => {
    setFile(null)
    setPreview(null)
  }

  // ==================== 关闭清理 ====================
  const doClose = () => {
    if (busy) return
    // 未确认导入时清理已上传的临时模板包，避免残留
    if (sessionKey) {
      setSessionKey('')
      templateUploadCancel(sessionKey).catch(() => {})
    }
    requestClose()
  }

  // ==================== 解析预览 ====================
  const handlePreview = async () => {
    let path = ''
    if (importMode === 'upload') {
      if (!file) {
        Toast.warning('请选择模板包')
        return
      }
      setSubmitting(true)
      setUploading(true)
      setProgress(0)
      try {
        const uploader = new ChunkUploader({
          init: templateUploadInit,
          chunk: templateUploadChunk,
          complete: templateUploadComplete,
        })
        uploaderRef.current = uploader
        const { sessionKey: key } = await uploader.upload(file, {}, {
          onHashProgress: (ratio) => {
            setPhase('hash')
            setProgress(Math.round(ratio * 100))
          },
          onUploadProgress: (ratio) => {
            setPhase('upload')
            setProgress(Math.round(ratio * 100))
          },
        })
        path = key
        setSessionKey(key) // 记录临时包路径，未导入而关闭时清理
      } catch (err) {
        console.error('上传模板包失败', err)
        setSubmitting(false)
        setUploading(false)
        uploaderRef.current = null
        return
      }
      setUploading(false)
      uploaderRef.current = null
    } else {
      path = sourcePath.trim()
      if (!path || !path.startsWith('/')) {
        Toast.warning('请输入宿主机上的绝对路径')
        return
      }
      setSubmitting(true)
    }

    try {
      const formData = new FormData()
      formData.append('source_path', path)
      const res = await previewImportTemplate(formData)
      setPreview(res.data?.preview || null)
      Toast.success(res.message || '模板包解析完成')
    } catch (err) {
      console.error('预览模板导入失败', err)
    } finally {
      setSubmitting(false)
    }
  }

  // ==================== 确认导入 ====================
  const handleConfirm = async () => {
    if (!preview?.token) {
      Toast.warning('请先解析模板包')
      return
    }
    setConfirming(true)
    try {
      const res = await confirmImportTemplate(preview.token)
      Toast.success(res.message || '模板导入任务已提交，请在任务中心查看进度')
      setSessionKey('') // 已导入，临时包交给导入任务，关闭时不再清理
      onImported()
      requestClose()
    } catch (err) {
      console.error('确认导入模板失败', err)
    } finally {
      setConfirming(false)
    }
  }

  // ==================== 预览表格 ====================
  const columns = [
    {
      title: '节点',
      dataIndex: 'name',
      render: (_: unknown, row: ImportPreviewNode) => (
        <div className="tpl-preview-node">
          <strong>{row.admin_name || row.name}</strong>
          <span>{row.name}</span>
        </div>
      ),
    },
    { title: '用户侧显示', dataIndex: 'display_name' },
    {
      title: '分类',
      dataIndex: 'type',
      render: (_: unknown, row: ImportPreviewNode) =>
        templateGroupLabel(row.type || '', row.category),
    },
    {
      title: '状态',
      dataIndex: 'exists',
      width: 100,
      render: (_: unknown, row: ImportPreviewNode) =>
        row.conflict_reason ? (
          <Tag color="red" size="small">冲突</Tag>
        ) : row.exists ? (
          <Tag color="blue" size="small">已存在</Tag>
        ) : (
          <Tag color="green" size="small">将导入</Tag>
        ),
    },
    {
      title: '用户可见',
      dataIndex: 'clone_visible',
      width: 90,
      render: (v: boolean) => (v ? '是' : '否'),
    },
    {
      title: '禁用',
      dataIndex: 'disabled',
      width: 80,
      render: (v: boolean) => (v ? '是' : '否'),
    },
    {
      title: 'MD5',
      dataIndex: 'md5',
      ellipsis: true,
    },
    {
      title: 'SHA256',
      dataIndex: 'sha256',
      ellipsis: true,
    },
    { title: '提示', dataIndex: 'conflict_reason' },
  ]

  return (
    <Modal
      title="导入模板包"
      visible={modalVisible}
      afterClose={afterModalClose}
      onCancel={doClose}
      width={860}
      maskClosable={false}
      closeOnEsc={!busy}
      footer={
        <>
          <Button onClick={doClose} disabled={busy}>
            取消
          </Button>
          <Button type="primary" loading={submitting} onClick={() => void handlePreview()}>
            解析预览
          </Button>
          <Button
            type="primary"
            disabled={!preview?.can_import}
            loading={confirming}
            onClick={() => void handleConfirm()}
          >
            确认导入
          </Button>
        </>
      }
    >
      <div className="qvm-form-item">
        <div className="qvm-form-label required">导入来源</div>
        <Radio.Group
          type="button"
          value={importMode}
          onChange={(e) => setImportMode(e.target.value as ImportMode)}
          options={[
            { value: 'upload', label: '上传文件' },
            { value: 'source_path', label: '主机绝对路径' },
          ]}
        />
        <div className="qvm-form-tip">仅支持新版 .tar.gz / .tgz 模板链路包</div>
      </div>

      {importMode === 'upload' ? (
        <div className="qvm-form-item">
          <div className="qvm-form-label required">模板包</div>
          <input
            ref={fileInputRef}
            type="file"
            accept=".tar.gz,.tgz"
            style={{ display: 'none' }}
            onChange={handleFileChange}
          />
          {file ? (
            <div className="tpl-picked-file">
              <span className="tpl-picked-name" title={file.name}>
                {file.name}
              </span>
              <Button
                size="small"
                theme="borderless"
                icon={<IconClose />}
                onClick={handleClearFile}
                disabled={busy}
              />
            </div>
          ) : (
            <Button icon={<IconUpload />} onClick={handlePickFile} disabled={busy}>
              选择文件
            </Button>
          )}
        </div>
      ) : (
        <div className="qvm-form-item">
          <div className="qvm-form-label required">主机路径</div>
          <Input
            value={sourcePath}
            onChange={setSourcePath}
            placeholder="/data/template/demo-template-export.tar.gz"
            disabled={busy}
          />
        </div>
      )}

      {uploading && (
        <div className="qvm-form-item">
          <div className="qvm-form-tip" style={{ marginBottom: 4 }}>
            {phase === 'hash' ? '正在计算文件哈希…' : '正在上传模板包…'}
          </div>
          <Progress percent={progress} showInfo strokeWidth={16} motion={false} />
        </div>
      )}

      {preview && (
        <div className="tpl-import-preview">
          <Banner
            type={preview.can_import ? 'success' : 'danger'}
            closeIcon={null}
            description={`${preview.message || ''}，模式：${preview.mode === 'update' ? '增量更新' : '新模板树'}`}
            style={{ marginBottom: 12 }}
          />
          <Table
            columns={columns}
            dataSource={preview.nodes || []}
            rowKey="name"
            size="small"
            bordered
            pagination={false}
            scroll={{ y: 320 }}
          />
        </div>
      )}
    </Modal>
  )
}
