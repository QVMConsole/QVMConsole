/**
 * 编辑模式设备管理（磁盘 / 光驱 / 软盘 / 引导设备）
 * 仅编辑表单（EditVmForm）使用，负责设备列表加载与纯数据操作；
 * 交互弹窗由 Section / dialogs 组件负责，保持本文件无 JSX。
 */
import { useCallback, useState } from 'react'
import { Toast } from '@douyinfe/semi-ui'
import {
  attachDisk,
  adminImportDiskForVM,
  changeCDROM,
  changeDiskBus,
  changeFloppy,
  ejectCDROM,
  ejectFloppy,
  getDiskList,
  removeCDROM,
  removeDisk,
  removeFloppy,
  resizeDisk,
  mountGuestDisk,
  retryGuestDiskGrow,
  type VmDiskItem,
  type GuestMountPayload,
} from '@/api/vm'
import type { EditBootDevice } from './types'

/** 编辑模式媒体设备（光驱/软盘） */
export interface MediaDevice {
  device: string
  path: string
}

export function useVmEditDevices(vmName: string) {
  const [editDisks, setEditDisks] = useState<VmDiskItem[]>([])
  const [editCdroms, setEditCdroms] = useState<MediaDevice[]>([])
  const [editFloppys, setEditFloppys] = useState<MediaDevice[]>([])
  const [editBootDevices, setEditBootDevices] = useState<EditBootDevice[]>([])

  /** 刷新磁盘列表（分离普通磁盘 / 光驱 / 软盘），返回普通磁盘供快照 */
  const refreshEditDisks = useCallback(async (): Promise<VmDiskItem[]> => {
    if (!vmName) return []
    try {
      const res = await getDiskList(vmName)
      const all = res.data || []
      const normalDisks: VmDiskItem[] = []
      const cdroms: MediaDevice[] = []
      const floppys: MediaDevice[] = []
      for (const d of all) {
        const isFloppy =
          d.device_type === 'floppy' ||
          (d.path && (d.path.endsWith('.img') || d.path.endsWith('.vfd') || d.path.endsWith('.flp')))
        const isCdrom = d.device_type === 'cdrom' || (d.path && d.path.endsWith('.iso'))
        if (isFloppy) {
          floppys.push({ device: d.device, path: d.path && d.path !== '-' ? d.path : '' })
        } else if (isCdrom) {
          cdroms.push({ device: d.device, path: d.path && d.path !== '-' ? d.path : '' })
        } else {
          normalDisks.push(d)
        }
      }
      setEditCdroms(cdroms)
      setEditFloppys(floppys)
      setEditDisks(normalDisks)
      return normalDisks
    } catch {
      return []
    }
  }, [vmName])

  // ==================== 磁盘操作 ====================

  /** 磁盘扩容（仅扩大），新容量校验由调用方完成 */
  const resizeDiskAction = useCallback(
    async (dev: string, sizeGB: number, autoGrowPartition = false) => {
      await resizeDisk(vmName, dev, sizeGB, autoGrowPartition)
      Toast.success(autoGrowPartition ? `磁盘 ${dev} 扩容任务已提交` : `磁盘 ${dev} 扩容成功`)
      await refreshEditDisks()
    },
    [vmName, refreshEditDisks],
  )

  /** 删除磁盘（deleteFile=true 连文件删除；transfer=true 转移到我的存储） */
  const removeDiskAction = useCallback(
    async (dev: string, deleteFile: boolean, transfer: boolean) => {
      await removeDisk(vmName, dev, deleteFile, transfer)
      Toast.success(
        transfer
          ? `磁盘 ${dev} 已卸载并转移到「我的存储-虚拟磁盘」`
          : `磁盘 ${dev} 已删除（含文件）`,
      )
      await refreshEditDisks()
    },
    [vmName, refreshEditDisks],
  )

  /** 修改磁盘驱动类型（失败自动回滚 UI） */
  const changeDiskBusAction = useCallback(
    async (dev: string, bus: string) => {
      try {
        await changeDiskBus(vmName, dev, bus)
        Toast.success(`磁盘 ${dev} 驱动已修改为 ${bus.toUpperCase()}`)
        await refreshEditDisks()
      } catch {
        await refreshEditDisks()
      }
    },
    [vmName, refreshEditDisks],
  )

  /** 挂载我的存储中的磁盘文件 */
  const attachDiskAction = useCallback(
    async (path: string, bus: string, guestMount?: GuestMountPayload) => {
      await attachDisk(vmName, path, bus, guestMount)
      Toast.success(guestMount?.enabled ? '磁盘关联与自动挂载任务已提交' : '磁盘挂载成功')
      await refreshEditDisks()
    },
    [vmName, refreshEditDisks],
  )

  /** 管理员绝对路径导入磁盘（异步任务） */
  const adminImportDiskAction = useCallback(
    async (data: {
      disk_path: string
      disk_source_type: string
      storage_pool_id?: string
      copy_disk?: boolean
      bus?: string
      guest_mount?: GuestMountPayload
    }) => {
      await adminImportDiskForVM(vmName, data)
      Toast.success('导入磁盘任务已提交，请在任务中心查看进度')
      await refreshEditDisks()
    },
    [vmName, refreshEditDisks],
  )

  const guestMountDiskAction = useCallback(
    async (dev: string, guestMount: GuestMountPayload, existingDisk = true) => {
      await mountGuestDisk(vmName, dev, { guest_mount: guestMount, existing_disk: existingDisk })
      Toast.success(`磁盘 ${dev} 来宾挂载任务已提交`)
    },
    [vmName],
  )

  const guestGrowDiskAction = useCallback(
    async (dev: string) => {
      await retryGuestDiskGrow(vmName, dev)
      Toast.success(`磁盘 ${dev} 来宾扩容重试任务已提交`)
    },
    [vmName],
  )

  // ==================== 光驱操作 ====================

  /** 插入光驱（替换已有设备）或新增光驱（forceNew） */
  const insertCDROMAction = useCallback(
    async (isoPath: string, device = '', forceNew = false) => {
      await changeCDROM(vmName, { iso_path: isoPath, device, force_new: forceNew || undefined })
      Toast.success(forceNew ? '光驱已添加' : '光盘已插入')
      await refreshEditDisks()
    },
    [vmName, refreshEditDisks],
  )

  const ejectCDROMAction = useCallback(
    async (device: string) => {
      await ejectCDROM(vmName, device)
      Toast.success('光盘已弹出')
      await refreshEditDisks()
    },
    [vmName, refreshEditDisks],
  )

  const removeCDROMAction = useCallback(
    async (device: string) => {
      await removeCDROM(vmName, device)
      Toast.success('光驱已移除')
      await refreshEditDisks()
    },
    [vmName, refreshEditDisks],
  )

  // ==================== 软盘操作 ====================

  const insertFloppyAction = useCallback(
    async (imagePath: string, device = '', forceNew = false) => {
      await changeFloppy(vmName, { image_path: imagePath, device, force_new: forceNew || undefined })
      Toast.success(forceNew ? '软盘已添加' : '软盘已插入')
      await refreshEditDisks()
    },
    [vmName, refreshEditDisks],
  )

  const ejectFloppyAction = useCallback(
    async (device: string) => {
      await ejectFloppy(vmName, device)
      Toast.success('软盘已弹出')
      await refreshEditDisks()
    },
    [vmName, refreshEditDisks],
  )

  const removeFloppyAction = useCallback(
    async (device: string) => {
      await removeFloppy(vmName, device)
      Toast.success('软盘已移除')
      await refreshEditDisks()
    },
    [vmName, refreshEditDisks],
  )

  return {
    editDisks,
    setEditDisks,
    editCdroms,
    editFloppys,
    editBootDevices,
    setEditBootDevices,
    refreshEditDisks,
    resizeDiskAction,
    removeDiskAction,
    changeDiskBusAction,
    attachDiskAction,
    adminImportDiskAction,
    guestMountDiskAction,
    guestGrowDiskAction,
    insertCDROMAction,
    ejectCDROMAction,
    removeCDROMAction,
    insertFloppyAction,
    ejectFloppyAction,
    removeFloppyAction,
  }
}

export type VmEditDevices = ReturnType<typeof useVmEditDevices>
