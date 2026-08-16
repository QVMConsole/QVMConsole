/**
 * 存储管理 Tab：用户存储维护（存储回收 fstrim + fallocate --dig-holes + 自动定时回收开关）
 */
import { useEffect, useState } from 'react'
import { Banner, Button, Toast } from '@douyinfe/semi-ui'
import { IconFolder, IconRefresh } from '@douyinfe/semi-icons'
import { trimUserStorage, updateSettings, type TrimStorageResult } from '@/api/settings'
import { getTaskDetail } from '@/api/task'
import { confirmModal } from '@/utils/confirm'
import { formatFileSize } from '@/utils/format'
import TextSwitch from '@/features/vm-form/sections/TextSwitch'
import { SectionHead, SettingRow } from './SettingRow'
import type { SettingsTabProps } from '../types'

export default function StorageMaintainTab({ form, patch }: SettingsTabProps) {
  const [trimming, setTrimming] = useState(false)
  const [autoSaving, setAutoSaving] = useState(false)
  const [trimResult, setTrimResult] = useState<TrimStorageResult | null>(null)
  const [trimTaskId, setTrimTaskId] = useState<number | null>(null)

  useEffect(() => {
    if (!trimTaskId) return
    let finished = false

    const pollTask = async () => {
      if (finished) return
      try {
        const res = await getTaskDetail(trimTaskId)
        const task = res.data
        if (task.status === 'pending' || task.status === 'running') return

        finished = true
        setTrimTaskId(null)
        setTrimming(false)
        if (task.status !== 'success') {
          Toast.error(`${task.message || '存储回收任务执行失败'}，可前往任务中心查看详情`)
          return
        }
        let result: TrimStorageResult & { skipped?: boolean }
        try {
          result = JSON.parse(task.result || '{}') as TrimStorageResult & { skipped?: boolean }
        } catch {
          Toast.error('存储回收任务结果格式异常，可前往任务中心查看详情')
          return
        }
        if (result.skipped) {
          Toast.warning('用户存储文件系统未挂载，本次回收已跳过')
          return
        }
        setTrimResult(result)
        Toast.success('存储回收完成')
      } catch {
        // 临时轮询失败时保留任务状态，下次继续获取
      }
    }

    void pollTask()
    const timer = window.setInterval(() => void pollTask(), 2000)
    return () => {
      finished = true
      window.clearInterval(timer)
    }
  }, [trimTaskId])

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
      setTrimTaskId(res.data.task.id)
      Toast.success(res.message || '存储回收任务已提交')
    } catch {
      // 请求层已统一提示
      setTrimming(false)
    }
  }

  /** 自动定时回收开关：存储管理 Tab 为独立操作区，切换后即时保存 */
  const handleAutoTrimChange = async (checked: boolean) => {
    patch({ scheduled_storage_trim_enabled: checked })
    setAutoSaving(true)
    try {
      const res = await updateSettings({ scheduled_storage_trim_enabled: checked })
      Toast.success(res.message || (checked ? '已开启自动定时回收' : '已关闭自动定时回收'))
    } catch {
      // 保存失败时回滚开关状态（请求层已统一提示）
      patch({ scheduled_storage_trim_enabled: !checked })
    } finally {
      setAutoSaving(false)
    }
  }

  return (
    <div className="stg-tab-pane">
      <SectionHead icon={<IconFolder />} title="用户存储维护" />

      <SettingRow label="存储镜像文件">
        <span className="stg-mono-text">{trimResult?.image_path || '按当前 loop 挂载与 fstab 自动识别'}</span>
      </SettingRow>

      <SettingRow label="挂载点">
        <span className="stg-mono-text">{trimResult?.mount_point || '/var/lib/kvm-user-storage'}</span>
      </SettingRow>

      <SettingRow
        label="自动定时回收"
        tip="默认开启，每天凌晨 2:00 自动执行用户存储回收（fstrim + fallocate --dig-holes），执行结果记录在调度事件中心"
      >
        <TextSwitch
          checked={form.scheduled_storage_trim_enabled}
          onChange={(v) => void handleAutoTrimChange(v)}
          disabled={autoSaving}
        />
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
