/**
 * 我的存储页（用户存储池管理）
 * - 未初始化：空状态 + 开通按钮
 * - 已初始化：配额概览 + 未完成上传恢复 + 文件管理 Tabs（ISO/共享/磁盘/挂载）
 * - 分片上传（断点续传 + 秒传），上传前预检查配额
 * - 删除文件为高风险操作（428 二次验证由请求层自动处理）
 * - 9p VirtFS 挂载到 Linux 虚拟机
 * 迁移自旧前端 views/storage/index.vue
 */
import { useCallback, useEffect, useRef, useState } from 'react'
import { useSearchParams } from 'react-router'
import { Button, Spin, Tabs, Toast } from '@douyinfe/semi-ui'
import {
  IconDisc,
  IconFolder,
  IconLink,
  IconRefresh,
} from '@douyinfe/semi-icons'
import {
  deleteStorageFile,
  getPendingUploads,
  getStorageDownloadUrl,
  getStorageFiles,
  getStorageInfo,
  getUserMounts,
  initStorage,
  storageUploadCancel,
  storageUploadChunk,
  storageUploadComplete,
  storageUploadInit,
  unmountStorage,
  type PendingUploadItem,
  type StorageCategory,
  type StorageFileItem,
  type UserStorageInfo,
  type VmMountItem,
} from '@/api/storage'
import { ChunkUploader } from '@/utils/chunkUploader'
import { confirmModal } from '@/utils/confirm'
import QuotaCard from './components/QuotaCard'
import PendingUploads from './components/PendingUploads'
import FileTable, { ACCEPT_MAP } from './components/FileTable'
import MountTable from './components/MountTable'
import UploadDialog, { type UploadStatus } from './dialogs/UploadDialog'
import MountDialog from './dialogs/MountDialog'
import MountHelpDialog from './dialogs/MountHelpDialog'
import './my-storage.css'

/** Tab key 类型 */
type TabKey = StorageCategory | 'mounts'

/** 弹窗状态 */
type DialogState =
  | { type: 'mount'; defaultCategory: string }
  | { type: 'mountHelp'; tag: string; readonly: boolean }
  | null

export default function MyStoragePage() {
  const [searchParams, setSearchParams] = useSearchParams()

  // ==================== 存储信息 ====================
  const [storageInfo, setStorageInfo] = useState<UserStorageInfo>({ initialized: false })
  const [infoLoaded, setInfoLoaded] = useState(false)
  const [initLoading, setInitLoading] = useState(false)

  // ==================== Tab 与文件列表 ====================
  const tabParam = (searchParams.get('tab') || 'iso') as TabKey
  const [activeTab, setActiveTab] = useState<TabKey>(tabParam)
  const [isoFiles, setIsoFiles] = useState<StorageFileItem[]>([])
  const [shareFiles, setShareFiles] = useState<StorageFileItem[]>([])
  const [diskFiles, setDiskFiles] = useState<StorageFileItem[]>([])
  const [filesLoading, setFilesLoading] = useState(false)

  // ==================== 挂载管理 ====================
  const [mountList, setMountList] = useState<VmMountItem[]>([])
  const [mountsLoading, setMountsLoading] = useState(false)

  // ==================== 未完成上传 ====================
  const [pendingUploads, setPendingUploads] = useState<PendingUploadItem[]>([])
  const resumeFileInputRef = useRef<HTMLInputElement>(null)
  const resumeCategoryRef = useRef<StorageCategory>('iso')
  // 上传文件 input（每个 tab 共用）
  const uploadFileInputRef = useRef<HTMLInputElement>(null)
  const uploadCategoryRef = useRef<StorageCategory>('iso')

  // ==================== 上传进度 ====================
  const [uploadVisible, setUploadVisible] = useState(false)
  const [uploadFileName, setUploadFileName] = useState('')
  const [uploadProgress, setUploadProgress] = useState(0)
  const [uploadStatus, setUploadStatus] = useState<UploadStatus>('hashing')
  const [uploadPaused, setUploadPaused] = useState(false)
  const uploaderRef = useRef<ChunkUploader | null>(null)

  // ==================== 弹窗 ====================
  const [dialog, setDialog] = useState<DialogState>(null)
  const pendingMountHelpRef = useRef<{ tag: string; readonly: boolean } | null>(null)

  // ==================== 数据加载 ====================

  const loadStorageInfo = useCallback(async () => {
    try {
      const res = await getStorageInfo()
      setStorageInfo(res.data || { initialized: false })
    } catch (err) {
      console.error('获取存储信息失败', err)
    } finally {
      setInfoLoaded(true)
    }
  }, [])

  const loadPendingUploads = useCallback(async () => {
    try {
      const res = await getPendingUploads()
      setPendingUploads(res.data || [])
    } catch (err) {
      console.error('获取未完成上传失败', err)
    }
  }, [])

  const loadFiles = useCallback(async (category: TabKey) => {
    if (category === 'mounts') return
    setFilesLoading(true)
    try {
      const res = await getStorageFiles(category)
      const list = res.data || []
      if (category === 'iso') setIsoFiles(list)
      else if (category === 'disk') setDiskFiles(list)
      else setShareFiles(list)
    } catch (err) {
      console.error('获取文件列表失败', err)
    } finally {
      setFilesLoading(false)
    }
  }, [])

  const loadMounts = useCallback(async () => {
    setMountsLoading(true)
    try {
      const res = await getUserMounts()
      setMountList(res.data || [])
    } catch (err) {
      console.error('获取挂载列表失败', err)
    } finally {
      setMountsLoading(false)
    }
  }, [])

  // 初始化加载
  useEffect(() => {
    void loadStorageInfo()
    void loadPendingUploads()
  }, [loadStorageInfo, loadPendingUploads])

  // 存储信息加载完成后加载当前 Tab 数据
  useEffect(() => {
    if (infoLoaded && storageInfo.initialized) {
      if (activeTab === 'mounts') void loadMounts()
      else void loadFiles(activeTab)
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [infoLoaded, storageInfo.initialized])

  // ==================== Tab 切换 ====================
  const handleTabChange = useCallback(
    (key: string) => {
      const tab = key as TabKey
      setActiveTab(tab)
      setSearchParams((prev) => {
        prev.set('tab', tab)
        return prev
      }, { replace: true })
      if (tab === 'mounts') void loadMounts()
      else void loadFiles(tab)
    },
    [loadFiles, loadMounts, setSearchParams],
  )

  // ==================== 初始化存储池 ====================
  const handleInit = useCallback(async () => {
    setInitLoading(true)
    try {
      await initStorage()
      Toast.success('存储池初始化成功')
      await loadStorageInfo()
      void loadFiles(activeTab)
    } catch (err) {
      console.error('初始化存储池失败', err)
    } finally {
      setInitLoading(false)
    }
  }, [loadStorageInfo, loadFiles, activeTab])

  // ==================== 文件上传 ====================

  /** 触发文件选择 */
  const triggerUpload = useCallback((category: StorageCategory) => {
    uploadCategoryRef.current = category
    const input = uploadFileInputRef.current
    if (input) {
      input.accept = ACCEPT_MAP[category]
      input.value = ''
      input.click()
    }
  }, [])

  /** 执行分片上传 */
  const doUpload = useCallback(
    async (file: File, category: StorageCategory) => {
      // ISO 文件类型检查
      if (category === 'iso' && !file.name.toLowerCase().endsWith('.iso')) {
        Toast.warning('ISO 类别仅支持 .iso 文件')
        return
      }

      // 上传前预检查配额
      try {
        const infoRes = await getStorageInfo()
        const info: UserStorageInfo = infoRes.data || { initialized: false }
        if (info.readonly) {
          Toast.error('存储空间已满，请先删除部分文件')
          return
        }
        if (info.max_bytes && info.max_bytes > 0 && (info.used_bytes || 0) + file.size > info.max_bytes) {
          const remaining = info.max_bytes - (info.used_bytes || 0)
          const remainMB = Math.max(0, remaining / 1024 / 1024).toFixed(1)
          const fileMB = (file.size / 1024 / 1024).toFixed(1)
          Toast.error(`存储空间不足，剩余 ${remainMB} MB，文件大小 ${fileMB} MB`)
          return
        }
        setStorageInfo(info)
      } catch {
        // 预检查失败不阻止上传，由后端兜底
      }

      // 打开上传弹窗
      setUploadFileName(file.name)
      setUploadProgress(0)
      setUploadStatus('hashing')
      setUploadPaused(false)
      setUploadVisible(true)

      const uploader = new ChunkUploader({
        init: storageUploadInit,
        chunk: storageUploadChunk,
        complete: storageUploadComplete,
      })
      uploaderRef.current = uploader

      try {
        await uploader.upload(file, { category }, {
          // 哈希阶段占 0–8%，上传阶段占 8–100%：进度条始终单调爬升，
          // 既不在「校验中」卡 0%，也不在进入上传时从 100% 回跳到 0%
          onHashProgress: (ratio) => {
            setUploadStatus('hashing')
            setUploadProgress(Math.round(ratio * 8))
          },
          onUploadProgress: (ratio) => {
            setUploadStatus('uploading')
            setUploadProgress(8 + Math.round(ratio * 92))
          },
        })
        setUploadProgress(100)
        setUploadStatus('done')
        Toast.success('文件上传成功')
        setUploadVisible(false)
        void loadFiles(category)
        void loadStorageInfo()
        void loadPendingUploads()
      } catch (err) {
        if (uploader.sessionKey) {
          storageUploadCancel(uploader.sessionKey).catch(() => {})
        }
        if ((err as Error)?.message !== '上传已取消') {
          Toast.error('上传失败：' + ((err as Error)?.message || '请重试'))
        } else {
          Toast.info('已取消上传')
        }
        setUploadVisible(false)
      } finally {
        uploaderRef.current = null
      }
    },
    [loadFiles, loadStorageInfo, loadPendingUploads],
  )

  /** 文件选择回调（新上传） */
  const handleUploadFileChange = useCallback(
    (e: React.ChangeEvent<HTMLInputElement>) => {
      const file = e.target.files?.[0]
      if (file) void doUpload(file, uploadCategoryRef.current)
    },
    [doUpload],
  )

  /** 暂停 / 继续 */
  const handleTogglePause = useCallback(() => {
    const uploader = uploaderRef.current
    if (!uploader) return
    if (uploadPaused) {
      uploader.resume()
      setUploadPaused(false)
    } else {
      uploader.pause()
      setUploadPaused(true)
    }
  }, [uploadPaused])

  /** 取消上传 */
  const handleCancelUpload = useCallback(() => {
    uploaderRef.current?.cancel()
  }, [])

  // ==================== 未完成上传恢复 ====================

  const handleResumePending = useCallback((item: PendingUploadItem) => {
    resumeCategoryRef.current = (item.category || 'iso') as StorageCategory
    const input = resumeFileInputRef.current
    if (input) {
      const accept =
        item.category === 'iso'
          ? '.iso'
          : item.category === 'disk'
            ? '.qcow2,.raw,.vmdk,.vhd,.vhdx,.img,.vfd,.ova,.ovf,.mf'
            : ''
      input.accept = accept
      input.value = ''
      input.click()
    }
  }, [])

  const handleResumeFileChange = useCallback(
    (e: React.ChangeEvent<HTMLInputElement>) => {
      const file = e.target.files?.[0]
      if (file) {
        void doUpload(file, resumeCategoryRef.current).finally(() => void loadPendingUploads())
      }
    },
    [doUpload, loadPendingUploads],
  )

  const handleCancelPending = useCallback(
    async (item: PendingUploadItem) => {
      const ok = await confirmModal({
        title: '取消上传',
        content: `取消上传「${item.file_name}」并删除已传部分？`,
      })
      if (!ok) return
      try {
        await storageUploadCancel(item.session_key)
        Toast.success('已取消')
        void loadPendingUploads()
        void loadStorageInfo()
      } catch (err) {
        console.error('取消上传失败', err)
      }
    },
    [loadPendingUploads, loadStorageInfo],
  )

  // ==================== 文件操作 ====================

  const handleDelete = useCallback(
    async (row: StorageFileItem, category: StorageCategory) => {
      const ok = await confirmModal({
        title: '删除文件',
        content: `确定删除文件 ${row.name}？`,
        danger: true,
      })
      if (!ok) return
      try {
        await deleteStorageFile(category, row.name)
        Toast.success('文件已删除')
        void loadFiles(category)
        void loadStorageInfo()
      } catch (err) {
        console.error('删除文件失败', err)
      }
    },
    [loadFiles, loadStorageInfo],
  )

  const handleDownload = useCallback((row: StorageFileItem, category: StorageCategory) => {
    const url = getStorageDownloadUrl(category, row.name)
    window.open(url, '_blank')
  }, [])

  // ==================== 挂载操作 ====================

  const handleUnmount = useCallback(
    async (row: VmMountItem) => {
      const ok = await confirmModal({
        title: '卸载存储池',
        content: `确定从虚拟机 ${row.vm_name} 卸载挂载 "${row.tag}"？`,
      })
      if (!ok) return
      try {
        await unmountStorage(row.vm_name, row.tag)
        Toast.success('已卸载')
        void loadMounts()
      } catch (err) {
        console.error('卸载失败', err)
      }
    },
    [loadMounts],
  )

  // ==================== 渲染 ====================

  // 加载中
  if (!infoLoaded) {
    return (
      <div className="mst-page">
        <div style={{ display: 'flex', justifyContent: 'center', paddingTop: 120 }}>
          <Spin size="large" />
        </div>
      </div>
    )
  }

  // 未初始化
  if (!storageInfo.initialized) {
    return (
      <div className="mst-page">
        <div className="mst-page-header qvm-fade-up">
          <div>
            <h2>
              <IconFolder style={{ marginRight: 8, color: 'var(--qvm-acc-ink)' }} />
              我的存储
            </h2>
            <p className="mst-page-sub">管理个人存储池中的文件，上传到虚拟机使用</p>
          </div>
        </div>
        <div className="mst-init-card qvm-fade-up" style={{ '--qvm-delay': '100ms' } as React.CSSProperties}>
          <div className="mst-init-icon">
            <IconFolder />
          </div>
          <div className="mst-init-title">存储池尚未初始化</div>
          <div className="mst-init-desc">
            开通存储池后，您可以上传 ISO 镜像、共享文件和虚拟磁盘，并将它们挂载到虚拟机中使用。
          </div>
          <Button type="primary" size="large" loading={initLoading} onClick={() => void handleInit()}>
            开通存储池
          </Button>
        </div>
      </div>
    )
  }

  // 已初始化
  return (
    <div className="mst-page">
      {/* 页头 */}
      <div className="mst-page-header qvm-fade-up">
        <div>
          <h2>
            <IconFolder style={{ marginRight: 8, color: 'var(--qvm-acc-ink)' }} />
            我的存储
          </h2>
          <p className="mst-page-sub">管理个人存储池中的文件，上传到虚拟机使用</p>
        </div>
        <div className="mst-header-actions">
          <Button icon={<IconRefresh />} onClick={() => { void loadStorageInfo(); void loadPendingUploads(); if (activeTab === 'mounts') void loadMounts(); else void loadFiles(activeTab) }}>
            刷新
          </Button>
        </div>
      </div>

      {/* 未完成上传恢复 */}
      <PendingUploads
        items={pendingUploads}
        onResume={handleResumePending}
        onCancel={(item) => void handleCancelPending(item)}
      />

      {/* 配额概览 */}
      <QuotaCard info={storageInfo} />

      {/* 文件管理 Tabs */}
      <div className="mst-files-card qvm-fade-up" style={{ '--qvm-delay': '120ms' } as React.CSSProperties}>
        <Tabs activeKey={activeTab} onChange={handleTabChange} type="line">
          <Tabs.TabPane tab="ISO 镜像" itemKey="iso" icon={<IconDisc />}>
            <FileTable
              category="iso"
              files={isoFiles}
              loading={filesLoading}
              readonly={storageInfo.readonly}
              onUpload={() => triggerUpload('iso')}
              onRefresh={() => void loadFiles('iso')}
              onMount={() => setDialog({ type: 'mount', defaultCategory: 'iso' })}
              onDownload={(row) => handleDownload(row, 'iso')}
              onDelete={(row) => void handleDelete(row, 'iso')}
            />
          </Tabs.TabPane>
          <Tabs.TabPane tab="文件共享" itemKey="share" icon={<IconFolder />}>
            <FileTable
              category="share"
              files={shareFiles}
              loading={filesLoading}
              readonly={storageInfo.readonly}
              onUpload={() => triggerUpload('share')}
              onRefresh={() => void loadFiles('share')}
              onMount={() => setDialog({ type: 'mount', defaultCategory: 'share' })}
              onDownload={(row) => handleDownload(row, 'share')}
              onDelete={(row) => void handleDelete(row, 'share')}
            />
          </Tabs.TabPane>
          <Tabs.TabPane tab="虚拟磁盘" itemKey="disk" icon={<IconDisc />}>
            <FileTable
              category="disk"
              files={diskFiles}
              loading={filesLoading}
              readonly={storageInfo.readonly}
              onUpload={() => triggerUpload('disk')}
              onRefresh={() => void loadFiles('disk')}
              onDownload={(row) => handleDownload(row, 'disk')}
              onDelete={(row) => void handleDelete(row, 'disk')}
            />
          </Tabs.TabPane>
          <Tabs.TabPane tab="挂载管理" itemKey="mounts" icon={<IconLink />}>
            <MountTable
              mounts={mountList}
              loading={mountsLoading}
              onRefresh={() => void loadMounts()}
              onShowHelp={(tag, ro) => setDialog({ type: 'mountHelp', tag, readonly: ro })}
              onUnmount={(row) => void handleUnmount(row)}
            />
          </Tabs.TabPane>
        </Tabs>
      </div>

      {/* 隐藏的文件选择 input（新上传） */}
      <input
        ref={uploadFileInputRef}
        type="file"
        style={{ display: 'none' }}
        onChange={handleUploadFileChange}
      />
      {/* 隐藏的文件选择 input（恢复上传） */}
      <input
        ref={resumeFileInputRef}
        type="file"
        style={{ display: 'none' }}
        onChange={handleResumeFileChange}
      />

      {/* 上传进度弹窗 */}
      <UploadDialog
        visible={uploadVisible}
        fileName={uploadFileName}
        progress={uploadProgress}
        status={uploadStatus}
        paused={uploadPaused}
        uploader={uploaderRef.current}
        onTogglePause={handleTogglePause}
        onCancel={handleCancelUpload}
        onClose={() => setUploadVisible(false)}
      />

      {/* 挂载到虚拟机弹窗 */}
      {dialog?.type === 'mount' && (
        <MountDialog
          defaultCategory={dialog.defaultCategory}
          onClose={() => {
            const help = pendingMountHelpRef.current
            pendingMountHelpRef.current = null
            setDialog(help ? { type: 'mountHelp', ...help } : null)
          }}
          onMounted={(tag, readonly) => {
            pendingMountHelpRef.current = { tag, readonly }
          }}
        />
      )}

      {/* 挂载命令说明弹窗 */}
      {dialog?.type === 'mountHelp' && (
        <MountHelpDialog
          tag={dialog.tag}
          readonly={dialog.readonly}
          onClose={() => setDialog(null)}
        />
      )}
    </div>
  )
}
