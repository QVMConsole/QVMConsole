import axios from 'axios'
import request from '@/utils/request'

// 获取虚拟机列表
export function getVmList(params) {
  return request({
    url: '/vm/list',
    method: 'get',
    params
  })
}

// 获取虚拟机详情
export function getVmDetail(name) {
  return request({
    url: `/vm/${name}`,
    method: 'get'
  })
}

// 获取虚拟机 IP
export function getVmIP(name) {
  return request({
    url: `/vm/${name}/ip`,
    method: 'get'
  })
}

// 获取虚拟机 PCIe 热插槽使用信息
export function getVmPCIEInfo(name) {
  return request({
    url: `/vm/${name}/pcie-info`,
    method: 'get'
  })
}

// 创建虚拟机详情 SSE 连接（实时推送）
export function createVmDetailSSE(name, token) {
  const baseUrl = import.meta.env.VITE_APP_BASE_API || '/api'
  return new EventSource(`${baseUrl}/vm/${name}/sse?token=${token}`)
}

// 编辑虚拟机配置
export function updateVm(name, data) {
  return request({
    url: `/vm/${name}`,
    method: 'put',
    data
  })
}

// 获取虚拟机持久化 XML
export function getVmXML(name) {
  return request({
    url: `/vm/${name}/xml`,
    method: 'get'
  })
}

// 保存虚拟机持久化 XML
export function updateVmXML(name, data) {
  return request({
    url: `/vm/${name}/xml`,
    method: 'put',
    data
  })
}

// 更新虚拟机速率限制
export function updateVMBandwidth(name, data) {
  return request({
    url: `/vm/${name}`,
    method: 'put',
    data
  })
}

// 删除虚拟机
export function deleteVm(name, data = {}) {
  return request({
    url: `/vm/${name}`,
    method: 'delete',
    data
  })
}

// 获取虚拟机的 qcow2 磁盘列表（删除确认用）
export function getVmQcow2Disks(name) {
  return request({
    url: `/vm/${name}/qcow2-disks`,
    method: 'get'
  })
}

// 获取虚拟机定时任务
export function getVmSchedules(name) {
  return request({
    url: `/vm/${name}/schedules`,
    method: 'get'
  })
}

// 创建虚拟机定时任务
export function createVmSchedule(name, data) {
  return request({
    url: `/vm/${name}/schedules`,
    method: 'post',
    data
  })
}

// 更新虚拟机定时任务
export function updateVmSchedule(name, id, data) {
  return request({
    url: `/vm/${name}/schedules/${id}`,
    method: 'put',
    data
  })
}

// 删除虚拟机定时任务
export function deleteVmSchedule(name, id) {
  return request({
    url: `/vm/${name}/schedules/${id}`,
    method: 'delete'
  })
}

// 操作虚拟机 (start/shutdown/destroy/reboot/reset)
export function operateVm(name, action) {
  return request({
    url: `/vm/${name}/operate`,
    method: 'post',
    data: { action }
  })
}

// 获取虚拟机资源使用
export function getVmStats(name, params = {}) {
  return request({
    url: `/vm/${name}/stats`,
    method: 'get',
    params
  })
}

// 获取虚拟机资源历史记录（按日期范围）
export function getVmStatsHistory(name, start, end) {
  return request({
    url: `/vm/${name}/stats/history`,
    method: 'get',
    params: { start, end }
  })
}

// 获取虚拟机 OVS 网络运行状态
export function getVMNetworkStatus(name) {
  return request({
    url: `/vm/${name}/network/status`,
    method: 'get'
  })
}

export function getVMNetworkDiagnostics(name) {
  return request({
    url: `/vm/${name}/network/diagnostics`,
    method: 'get'
  })
}

export function startVMNetworkCapture(name, data) {
  return request({
    url: `/vm/${name}/network/capture`,
    method: 'post',
    data
  })
}

// 链式克隆
export function cloneVm(data) {
  return request({
    url: '/vm/clone',
    method: 'post',
    data
  })
}

// 原生链式克隆
export function linkedCloneVm(data) {
  return request({
    url: '/vm/linked-clone',
    method: 'post',
    data
  })
}

// 批量克隆
export function batchCloneVm(data) {
  return request({
    url: '/vm/batch-clone',
    method: 'post',
    data
  })
}

// 重装系统
export function reinstallVm(name, data) {
  return request({
    url: `/vm/${name}/reinstall`,
    method: 'post',
    data
  })
}

// 获取快照列表
export function getSnapshots(name) {
  return request({
    url: `/vm/${name}/snapshots`,
    method: 'get'
  })
}

// 创建快照
export function createSnapshot(name, data) {
  return request({
    url: `/vm/${name}/snapshot`,
    method: 'post',
    data
  })
}

// 恢复快照
export function revertSnapshot(vmName, snapName) {
  return request({
    url: `/vm/${vmName}/snapshot/${snapName}/revert`,
    method: 'post'
  })
}

// 删除快照
export function deleteSnapshot(vmName, snapName) {
  return request({
    url: `/vm/${vmName}/snapshot/${snapName}`,
    method: 'delete'
  })
}

// 删除全部快照
export function deleteAllSnapshots(vmName) {
  return request({
    url: `/vm/${vmName}/snapshots`,
    method: 'delete'
  })
}

// 获取 VNC 状态
export function getVncStatus(name) {
  return request({
    url: `/vm/${name}/vnc/status`,
    method: 'get'
  })
}

// 开启 VNC
export function enableVnc(name, password = '') {
  return request({
    url: `/vm/${name}/vnc/enable`,
    method: 'post',
    data: { password }
  })
}

// 关闭 VNC
export function disableVnc(name) {
  return request({
    url: `/vm/${name}/vnc/disable`,
    method: 'post'
  })
}

// 修改 VNC 密码
export function changeVncPassword(name, password) {
  return request({
    url: `/vm/${name}/vnc/passwd`,
    method: 'post',
    data: { password }
  })
}

// 切换 VNC 对外暴露
export function exposeVnc(name, expose) {
  return request({
    url: `/vm/${name}/vnc/expose`,
    method: 'post',
    data: { expose }
  })
}

// ==================== SPICE（外部客户端直连，提供 .vv 下载） ====================

// 获取 SPICE 状态
export function getSpiceStatus(name) {
  return request({
    url: `/vm/${name}/spice/status`,
    method: 'get'
  })
}

// 获取 SPICE 连接信息（host:port + 密码）
export function getSpiceConnInfo(name) {
  return request({
    url: `/vm/${name}/spice/info`,
    method: 'get'
  })
}

// 开启 SPICE
export function enableSpice(name, password = '') {
  return request({
    url: `/vm/${name}/spice/enable`,
    method: 'post',
    data: { password }
  })
}

// 关闭 SPICE
export function disableSpice(name) {
  return request({
    url: `/vm/${name}/spice/disable`,
    method: 'post'
  })
}

// 修改 SPICE 密码
export function changeSpicePassword(name, password) {
  return request({
    url: `/vm/${name}/spice/passwd`,
    method: 'post',
    data: { password }
  })
}

// 切换 SPICE 对外暴露（联动宿主防火墙端口）
export function exposeSpice(name, expose) {
  return request({
    url: `/vm/${name}/spice/expose`,
    method: 'post',
    data: { expose }
  })
}

// 下载 SPICE .vv 连接文件（返回 blob，使用原始 axios 避免被 JSON 拦截器干扰）
export async function downloadSpiceVV(name, deleteFile = true) {
  const baseURL = import.meta.env.VITE_APP_BASE_API || '/api'
  const { useUserStore } = await import('@/store/user')
  const userStore = useUserStore()
  const headers = {}
  if (userStore.token) {
    headers.Authorization = `Bearer ${userStore.token}`
  }
  const response = await axios({
    url: baseURL + `/vm/${name}/spice/vv?delete=${deleteFile ? 1 : 0}`,
    method: 'get',
    responseType: 'blob',
    headers
  })
  return response.data
}

// 获取 QEMU Monitor 状态
export function getVMMonitorStatus(name) {
  return request({
    url: `/vm/${name}/monitor/status`,
    method: 'get'
  })
}

// 执行 QEMU Monitor 命令
export function executeVMMonitorCommand(name, command) {
  return request({
    url: `/vm/${name}/monitor/command`,
    method: 'post',
    data: { command }
  })
}

// 获取模板列表
export function getTemplateList() {
  return request({
    url: '/template/list',
    method: 'get'
  })
}

// 制作模板
export function prepareTemplate(data) {
  return request({
    url: '/template/prepare',
    method: 'post',
    data
  })
}

// 兼容导入模板
export function importTemplate(formData, onUploadProgress) {
  return request({
    url: '/template/import',
    method: 'post',
    data: formData,
    timeout: 0, // 大文件上传不超时
    maxContentLength: Infinity,
    maxBodyLength: Infinity,
    onUploadProgress
  })
}

// 预览模板包导入
export function previewImportTemplate(formData, onUploadProgress) {
  return request({
    url: '/template/import/preview',
    method: 'post',
    data: formData,
    timeout: 0, // 大文件上传不超时
    maxContentLength: Infinity,
    maxBodyLength: Infinity,
    onUploadProgress
  })
}

// 确认模板包导入
export function confirmImportTemplate(token) {
  return request({
    url: '/template/import/confirm',
    method: 'post',
    data: { token }
  })
}

// 导出模板
export function exportTemplate(name, scope = 'node') {
  return request({
    url: `/template/${name}/export`,
    method: 'post',
    params: { scope }
  })
}

// 删除模板导出文件
export function deleteTemplateExport(name) {
  return request({
    url: `/template/${name}/export`,
    method: 'delete'
  })
}

// 获取模板导出文件下载地址
export function getTemplateExportDownloadUrl(path) {
  const token = localStorage.getItem('token')
  const baseUrl = import.meta.env.VITE_APP_BASE_API || '/api'
  if (!path) return ''
  return `${baseUrl}${path.startsWith('/api') ? path.slice(4) : path}?token=${encodeURIComponent(token)}`
}

// 删除模板
export function deleteTemplate(name, data = {}) {
  return request({
    url: `/template/${name}`,
    method: 'delete',
    data
  })
}

// 获取模板创建的虚拟机列表
export function getTemplateVMs(name) {
  return request({
    url: `/template/${name}/vms`,
    method: 'get'
  })
}

// 获取模板删除预览
export function getTemplateDeletePreview(name) {
  return request({
    url: `/template/${name}/delete-preview`,
    method: 'get'
  })
}

// 更新模板发布展示配置
export function updateTemplatePublish(name, data) {
  return request({
    url: `/template/${name}/publish`,
    method: 'put',
    data
  })
}

// 更新模板展示配置（兼容旧调用）
export function updateTemplateMeta(name, data) {
  return request({
    url: `/template/${name}/meta`,
    method: 'put',
    data
  })
}

// 普通方式创建虚拟机（不通过模板）
export function createVm(data) {
  return request({
    url: '/vm/create',
    method: 'post',
    data
  })
}

// 获取系统变体列表
export function getOSVariants() {
  return request({
    url: '/vm/os-variants',
    method: 'get'
  })
}

// 获取 ISO 列表
export function getISOList() {
  return request({
    url: '/vm/iso-list',
    method: 'get'
  })
}

// 管理员通过绝对路径导入磁盘创建虚拟机
export function adminImportDisk(data) {
  return request({
    url: '/vm/import-disk',
    method: 'post',
    data
  })
}

// 获取磁盘列表
export function getDiskList(name) {
  return request({
    url: `/vm/${name}/disks`,
    method: 'get'
  })
}

// 获取本机硬盘迁移选项
export function getDiskMigrationOptions(name) {
  return request({
    url: `/vm/${name}/disk-migration/options`,
    method: 'get'
  })
}

// 磁盘扩容
export function resizeDisk(name, dev, sizeGB) {
  return request({
    url: `/vm/${name}/disk/${dev}/resize`,
    method: 'post',
    data: { size_gb: sizeGB }
  })
}

// 删除/转移磁盘
export function removeDisk(name, dev, deleteFile = false, transfer = false) {
  return request({
    url: `/vm/${name}/disk/${dev}`,
    method: 'delete',
    data: { delete_file: deleteFile, transfer }
  })
}

// 修改磁盘驱动类型
export function changeDiskBus(name, dev, bus) {
  return request({
    url: `/vm/${name}/disk/${dev}/bus`,
    method: 'put',
    data: { bus }
  })
}

// 挂载已有磁盘文件
export function attachDisk(name, path, bus = 'virtio') {
  return request({
    url: `/vm/${name}/disk/attach`,
    method: 'post',
    data: { path, bus }
  })
}

// 管理员为已有虚拟机通过绝对路径导入磁盘
export function adminImportDiskForVM(name, data) {
  return request({
    url: `/vm/${name}/disk/import`,
    method: 'post',
    data
  })
}

// 提交本机硬盘迁移
export function migrateDisk(name, dev, data) {
  return request({
    url: `/vm/${name}/disk/${dev}/migrate`,
    method: 'post',
    data,
    timeout: 60000
  })
}

// ==================== CD/DVD 管理 ====================

// 插入/更换 CD/DVD
export function changeCDROM(name, data) {
  return request({
    url: `/vm/${name}/cdrom`,
    method: 'post',
    data
  })
}

// 弹出 CD/DVD
export function ejectCDROM(name, device = '') {
  return request({
    url: `/vm/${name}/cdrom/eject`,
    method: 'post',
    params: device ? { device } : {}
  })
}

// 移除 CD/DVD 光驱
export function removeCDROM(name, device = '') {
  return request({
    url: `/vm/${name}/cdrom`,
    method: 'delete',
    params: device ? { device } : {}
  })
}

// ==================== 软盘管理 ====================

// 插入/更换软盘
export function changeFloppy(name, data) {
  return request({
    url: `/vm/${name}/floppy`,
    method: 'post',
    data
  })
}

// 弹出软盘
export function ejectFloppy(name, device = '') {
  return request({
    url: `/vm/${name}/floppy/eject`,
    method: 'post',
    params: device ? { device } : {}
  })
}

// 移除软盘设备
export function removeFloppy(name, device = '') {
  return request({
    url: `/vm/${name}/floppy`,
    method: 'delete',
    params: device ? { device } : {}
  })
}

// ==================== 救援系统 ====================

// 启动/关闭救援系统
export function rescueVm(name, action) {
  return request({
    url: `/vm/${name}/rescue`,
    method: 'post',
    data: { action }
  })
}

// 重置虚拟机登录密码
export function resetVmLinuxPassword(name, data) {
  return request({
    url: `/vm/${name}/password/reset`,
    method: 'post',
    data
  })
}

// 预览跨节点迁移
export function previewVmMigration(name, data) {
  return request({
    url: `/vm/${name}/migration/preview`,
    method: 'post',
    data,
    timeout: 300000
  })
}

// 提交跨节点迁移
export function migrateVm(name, data) {
  return request({
    url: `/vm/${name}/migrate`,
    method: 'post',
    data,
    timeout: 60000
  })
}

// ==================== 磁盘 IOPS 限制 ====================

// 获取磁盘 IOPS 限制
export function getDiskIOPS(name, dev) {
  return request({
    url: `/vm/${name}/disk/${dev}/iops`,
    method: 'get'
  })
}

// 设置磁盘 IOPS 限制（仅管理员）
export function setDiskIOPS(name, dev, data) {
  return request({
    url: `/vm/${name}/disk/${dev}/iops`,
    method: 'put',
    data
  })
}

// ==================== 硬件直通 ====================

// 获取可直通的 PCI 设备列表
export function getPassthroughDevices() {
  return request({
    url: '/host/passthrough',
    method: 'get'
  })
}

// 绑定 PCI 设备到 vfio-pci
export function bindPCIDevice(pciAddress) {
  return request({
    url: '/host/passthrough/bind',
    method: 'post',
    data: { pci_address: pciAddress }
  })
}

// 从 vfio-pci 解绑 PCI 设备
export function unbindPCIDevice(pciAddress) {
  return request({
    url: '/host/passthrough/unbind',
    method: 'post',
    data: { pci_address: pciAddress }
  })
}

// 获取虚拟机的直通设备
export function getVmPassthroughDevices(name) {
  return request({
    url: `/vm/${name}/passthrough`,
    method: 'get'
  })
}

// 添加 PCI 直通设备到虚拟机
export function attachPCIDeviceToVm(name, pciAddress) {
  return request({
    url: `/vm/${name}/passthrough`,
    method: 'post',
    data: { pci_address: pciAddress }
  })
}

// 从虚拟机移除 PCI 直通设备
export function detachPCIDeviceFromVm(name, pciAddress) {
  return request({
    url: `/vm/${name}/passthrough`,
    method: 'delete',
    data: { pci_address: pciAddress }
  })
}

// ==================== 虚拟机锁定管理 ====================

// 锁定虚拟机
export function lockVm(name) {
  return request({
    url: `/vm/${name}/lock`,
    method: 'post'
  })
}

// 解锁虚拟机（需要二次验证）
export function unlockVm(name) {
  return request({
    url: `/vm/${name}/unlock`,
    method: 'post'
  })
}

// 获取虚拟机锁定状态
export function getVMLockStatus(name) {
  return request({
    url: `/vm/${name}/lock`,
    method: 'get'
  })
}

// 转为独立虚拟机（脱离链式克隆 backing chain，仅管理员）
export function makeVMIndependent(name) {
  return request({
    url: `/vm/${name}/make-independent`,
    method: 'post'
  })
}
