/**
 * 存储管理 Tab：用户存储维护（存储回收 fstrim + fallocate --dig-holes）
 */
import { useState } from 'react'
import { Banner, Button, Toast } from '@douyinfe/semi-ui'
import { IconFolder, IconRefresh } from '@douyinfe/semi-icons'
import { trimUserStorage, type TrimStorageResult } from '@/api/settings'
import { confirmModal } from '@/utils/confirm'
import { formatFileSize } from '@/utils/format'
import { SectionHead, SettingRow } from './SettingRow'

export default function StorageMaintainTab() {
  const [trimming, setTrimming] = useState(false)
  const [trimResult, setTrimResult] = useState<TrimStorageResult | null>(null)

  const handleTrim = async () => {
    const ok = await confirmModal({
      title: '存储回收',
      content: '确定要执行用户存储回收吗？此操作会回收稀疏文件中的未使用空间，不影响已有数据。',
      okText: '确定执行',
    })
    if (!ok) return
    setTrimming(true)
    setTrimResult(null)
    try {
      const res = await trimUserStorage()
      if (res.data) {
        setTrimResult(res.data)
        Toast.success(res.message || '存储回收完成')
      }
    } catch {
      // 请求层已统一提示
    } finally {
      setTrimming(false)
    }
  }

  return (
    <div className="stg-tab-pane">
      <SectionHead icon={<IconFolder />} title="用户存储维护" />

      <SettingRow label="存储镜像文件">
        <span className="stg-mono-text">/var/lib/kvm-user-storage.img</span>
      </SettingRow>

      <SettingRow label="挂载点">
        <span className="stg-mono-text">/var/lib/kvm-user-storage</span>
      </SettingRow>

      <SettingRow
        label="存储回收"
        tip="执行 fstrim + fallocate --dig-holes 回收稀疏文件中的未使用空间，不影响已有数据"
      >
        <div className="stg-host-field">
          {trimResult && (
            <Banner
              type={trimResult.trimmed_bytes > 0 ? 'success' : 'info'}
              closeIcon={null}
              className="stg-banner"
              description={`回收前 ${formatFileSize(trimResult.before_blocks * 1024)} → 回收后 ${formatFileSize(trimResult.after_blocks * 1024)}（释放 ${trimResult.trimmed_human}）`}
            />
          )}
          <Button
            type="primary"
            theme="light"
            icon={<IconRefresh />}
            loading={trimming}
            onClick={() => void handleTrim()}
          >
            执行存储回收
          </Button>
        </div>
      </SettingRow>
    </div>
  )
}
