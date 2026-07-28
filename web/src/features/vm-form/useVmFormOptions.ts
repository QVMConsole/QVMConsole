/**
 * 虚拟机表单选项数据加载（ISO / 模板 / 存储池 / VPC / 磁盘文件 / 直通设备 / 宿主机信息）
 * 创建向导与编辑表单共用，内部做一次性加载与缓存。
 */
import { useCallback, useRef, useState } from 'react'
import { getAllISOs, getVMStorageTargets, type IsoItem, type VmStorageTarget } from '@/api/infra'
import {
  getOSVariants,
  getPassthroughDevices,
  type OsVariantItem,
  type PassthroughDevice,
} from '@/api/vm'
import { getStorageFiles, getUserISOs, type StorageFileItem, type UserIsoItem } from '@/api/storage'
import { getTemplateList, type TemplateItem } from '@/api/template'
import { getVPCSecurityGroups, getVPCSwitches, type VpcSecurityGroup, type VpcSwitch } from '@/api/vpc'
import {
  getCPUAffinityPresets,
  getHostCPUCores,
  getPublicSystemInfo,
  getSettings,
  type CpuAffinityPreset,
} from '@/api/settings'
import { getPublicSettings } from '@/api/settings'

export interface UseVmFormOptionsParams {
  isAdmin: boolean
}

export function useVmFormOptions({ isAdmin }: UseVmFormOptionsParams) {
  const [isoList, setIsoList] = useState<(IsoItem | UserIsoItem)[]>([])
  const [isoLoading, setIsoLoading] = useState(false)
  const [isoStorageDir, setIsoStorageDir] = useState('/var/lib/libvirt/images/ISO')
  const [osVariants, setOsVariants] = useState<OsVariantItem[]>([])
  const [templates, setTemplates] = useState<TemplateItem[]>([])
  const [vpcSwitches, setVpcSwitches] = useState<VpcSwitch[]>([])
  const [vpcSecurityGroups, setVpcSecurityGroups] = useState<VpcSecurityGroup[]>([])
  const [storageTargets, setStorageTargets] = useState<VmStorageTarget[]>([])
  const [diskFiles, setDiskFiles] = useState<StorageFileItem[]>([])
  const [diskFilesLoading, setDiskFilesLoading] = useState(false)
  const [passthroughDevices, setPassthroughDevices] = useState<PassthroughDevice[]>([])
  const [cpuAffinityPresets, setCpuAffinityPresets] = useState<CpuAffinityPreset[]>([])
  const [hostCores, setHostCores] = useState(0)
  const [hostArch, setHostArch] = useState('x86_64')
  const [spiceSupported, setSpiceSupported] = useState(true)
  const [spiceDefault, setSpiceDefault] = useState(false)

  const baseLoadedRef = useRef(false)

  /** 打开表单时一次性加载：宿主信息 / 系统设置 / 亲和性预设 / 公开设置
   * 返回最新加载结果（state 更新存在时序差，调用方应使用返回值） */
  const ensureBaseLoaded = useCallback(async (): Promise<{
    hostArch: string
    hostCores: number
    spiceSupported: boolean
    spiceDefault: boolean
  }> => {
    if (baseLoadedRef.current) {
      return { hostArch, hostCores, spiceSupported, spiceDefault }
    }
    baseLoadedRef.current = true
    const result = { hostArch: 'x86_64', hostCores: 0, spiceSupported: true, spiceDefault: false }
    const tasks: Promise<void>[] = []
    tasks.push(
      getHostCPUCores()
        .then((res) => {
          const cores = Number(res.data?.cores || 0)
          if (cores > 0) {
            result.hostCores = cores
            setHostCores(cores)
          }
        })
        .catch(() => undefined),
    )
    tasks.push(
      getPublicSystemInfo()
        .then((res) => {
          const archStr = (res.data?.arch || '').split(' ')[0].toLowerCase()
          if (['aarch64', 'x86_64', 'riscv64'].includes(archStr)) {
            result.hostArch = archStr
            setHostArch(archStr)
          }
          if (res.data?.qemu_spice !== undefined) {
            result.spiceSupported = !!res.data.qemu_spice
            setSpiceSupported(!!res.data.qemu_spice)
          }
        })
        .catch(() => undefined),
    )
    // 系统设置为管理员专属接口，普通用户调用会被 403 拦截并弹出全局错误提示
    if (isAdmin) {
      tasks.push(
        getSettings()
          .then((res) => {
            if (res.data?.iso_dir) setIsoStorageDir(res.data.iso_dir)
          })
          .catch(() => undefined),
      )
    }
    tasks.push(
      getCPUAffinityPresets()
        .then((res) => setCpuAffinityPresets(res.data || []))
        .catch(() => undefined),
    )
    tasks.push(
      getPublicSettings()
        .then((res) => {
          result.spiceDefault = !!res.data?.spice_enabled_by_default
          setSpiceDefault(!!res.data?.spice_enabled_by_default)
        })
        .catch(() => undefined),
    )
    await Promise.all(tasks)
    return result
  }, [isAdmin, hostArch, hostCores, spiceSupported, spiceDefault])

  /** ISO 列表（管理员聚合全部存储池，普通用户取自己的存储） */
  const loadISOs = useCallback(async () => {
    setIsoLoading(true)
    try {
      if (isAdmin) {
        const res = await getAllISOs()
        setIsoList(res.data || [])
      } else {
        const res = await getUserISOs()
        setIsoList(res.data || [])
      }
    } catch {
      // 列表加载失败不阻断表单
    } finally {
      setIsoLoading(false)
    }
  }, [isAdmin])

  /** 操作系统变体（仅加载一次） */
  const loadOSVariants = useCallback(async () => {
    if (osVariants.length > 0) return
    try {
      const res = await getOSVariants()
      setOsVariants(res.data || [])
    } catch {
      // 忽略
    }
  }, [osVariants.length])

  /** 模板列表（force 强制刷新） */
  const loadTemplates = useCallback(async (force = false): Promise<TemplateItem[]> => {
    if (!force && templates.length > 0) return templates
    try {
      const res = await getTemplateList()
      const list = res.data || []
      setTemplates(list)
      return list
    } catch {
      return templates
    }
  }, [templates])

  /** VPC 交换机与安全组 */
  const loadVPCOptions = useCallback(async (): Promise<{
    switches: VpcSwitch[]
    groups: VpcSecurityGroup[]
  }> => {
    try {
      const [switchRes, groupRes] = await Promise.all([getVPCSwitches(), getVPCSecurityGroups()])
      const switches = switchRes.data || []
      const groups = groupRes.data || []
      setVpcSwitches(switches)
      setVpcSecurityGroups(groups)
      return { switches, groups }
    } catch {
      return { switches: vpcSwitches, groups: vpcSecurityGroups }
    }
  }, [vpcSwitches, vpcSecurityGroups])

  /** 虚拟机磁盘存储位置 */
  const loadStorageTargets = useCallback(async (): Promise<VmStorageTarget[]> => {
    try {
      const res = await getVMStorageTargets()
      const list = res.data || []
      setStorageTargets(list)
      return list
    } catch {
      setStorageTargets([])
      return []
    }
  }, [])

  /** 我的存储中的虚拟磁盘文件（导入/软盘/挂载用） */
  const loadDiskFiles = useCallback(async () => {
    setDiskFilesLoading(true)
    try {
      const res = await getStorageFiles('disk')
      setDiskFiles(res.data || [])
    } catch {
      // 忽略
    } finally {
      setDiskFilesLoading(false)
    }
  }, [])

  /** 宿主机可直通 PCI 设备（仅管理员） */
  const loadPassthroughDevices = useCallback(async () => {
    try {
      const res = await getPassthroughDevices()
      setPassthroughDevices(res.data || [])
    } catch {
      setPassthroughDevices([])
    }
  }, [])

  return {
    isoList,
    isoLoading,
    isoStorageDir,
    osVariants,
    templates,
    vpcSwitches,
    vpcSecurityGroups,
    storageTargets,
    diskFiles,
    diskFilesLoading,
    passthroughDevices,
    cpuAffinityPresets,
    hostCores,
    hostArch,
    spiceSupported,
    spiceDefault,
    ensureBaseLoaded,
    loadISOs,
    loadOSVariants,
    loadTemplates,
    loadVPCOptions,
    loadStorageTargets,
    loadDiskFiles,
    loadPassthroughDevices,
  }
}

export type VmFormOptions = ReturnType<typeof useVmFormOptions>
