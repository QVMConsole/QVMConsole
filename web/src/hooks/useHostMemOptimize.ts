/**
 * 宿主机内存优化特性状态 Hook（管理员概览页用）
 * - 读取系统设置中开启的 KSM / zRAM 状态，用于内存使用率卡片统一展示
 * - 定时轮询刷新（zRAM 用量变化）；KSM 实时节省量由宿主机 SSE 页计数计算
 */
import { useEffect, useState } from 'react'
import {
  getHostKSMStatus,
  getHostZRAMStatus,
  type KSMStatus,
  type ZRAMStatus,
} from '@/api/settings'

/** 轮询间隔（ms）：zRAM 用量非高频变化，60s 刷新一次足够 */
const REFRESH_INTERVAL_MS = 60 * 1000

export function useHostMemOptimize() {
  const [ksm, setKsm] = useState<KSMStatus | null>(null)
  const [zram, setZram] = useState<ZRAMStatus | null>(null)

  useEffect(() => {
    let mounted = true

    const refresh = () => {
      void getHostKSMStatus()
        .then((res) => {
          if (mounted) setKsm(res.data || null)
        })
        .catch(() => undefined)
      void getHostZRAMStatus()
        .then((res) => {
          if (mounted) setZram(res.data || null)
        })
        .catch(() => undefined)
    }

    refresh()
    const timer = setInterval(refresh, REFRESH_INTERVAL_MS)
    return () => {
      mounted = false
      clearInterval(timer)
    }
  }, [])

  return {
    /** KSM 已在系统设置中开启且宿主机支持 */
    ksmEnabled: !!ksm?.supported && !!ksm?.enabled,
    /** zRAM 已在系统设置中开启且宿主机支持 */
    zramEnabled: !!zram?.supported && !!zram?.enabled,
    /** zRAM 逻辑容量（MB） */
    zramSizeMB: zram?.runtime_config?.size_mb ?? 0,
    /** zRAM 已用（MB） */
    zramUsedMB: zram?.runtime_config?.used_mb ?? 0,
    /** zRAM 压缩算法 */
    zramAlgorithm: zram?.runtime_config?.algorithm || '',
  }
}
