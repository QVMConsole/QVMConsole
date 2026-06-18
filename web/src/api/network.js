import request from '@/utils/request'

// 获取静态 IP 列表
export function getStaticIPList() {
  return request({ url: '/network/static-ip/list', method: 'get' })
}

// 绑定静态 IP
export function bindStaticIP(data) {
  return request({ url: '/network/static-ip/bind', method: 'post', data })
}

// 解绑静态 IP
export function unbindStaticIP(data) {
  return request({ url: '/network/static-ip/unbind', method: 'post', data })
}

// 获取端口转发列表
export function getPortForwardList() {
  return request({ url: '/network/port-forward/list', method: 'get' })
}

export function getHostInterfaces() {
  return request({ url: '/network/host/interfaces', method: 'get' })
}

export function getNetworkBridges() {
  return request({ url: '/network/bridges', method: 'get' })
}

export function createNetworkBridge(data) {
  return request({ url: '/network/bridges', method: 'post', data })
}

export function deleteNetworkBridge(id, name = '') {
  const params = name ? `?name=${encodeURIComponent(name)}` : ''
  return request({ url: `/network/bridges/${id}${params}`, method: 'delete' })
}

// 获取接口 IP/DNS 配置
export function getInterfaceConfig(name) {
  return request({ url: `/network/interfaces/${encodeURIComponent(name)}/config`, method: 'get' })
}

// 设置接口 IP/DNS 配置
export function setInterfaceConfig(name, data) {
  return request({ url: `/network/interfaces/${encodeURIComponent(name)}/config`, method: 'put', data })
}

export function getPublicIPs() {
  return request({ url: '/network/public-ips', method: 'get' })
}

export function createPublicIP(data) {
  return request({ url: '/network/public-ips', method: 'post', data })
}

export function updatePublicIP(id, data) {
  return request({ url: `/network/public-ips/${id}`, method: 'put', data })
}

export function deletePublicIP(id) {
  return request({ url: `/network/public-ips/${id}`, method: 'delete' })
}

export function previewPublicIP(id, data) {
  return request({ url: `/network/public-ips/${id}/preview`, method: 'post', data })
}

export function bindPublicIP(id, data) {
  return request({ url: `/network/public-ips/${id}/bind`, method: 'post', data })
}

export function unbindPublicIP(id) {
  return request({ url: `/network/public-ips/${id}/unbind`, method: 'post' })
}

export function migratePublicIP(id, data) {
  return request({ url: `/network/public-ips/${id}/migrate`, method: 'post', data })
}

export function applyPublicIPRules() {
  return request({ url: '/network/public-ips/apply', method: 'post' })
}

// 添加端口转发
export function addPortForward(data) {
  return request({ url: '/network/port-forward/add', method: 'post', data })
}

// 编辑端口转发
export function updatePortForward(id, data) {
  return request({ url: `/network/port-forward/${id}`, method: 'put', data })
}

// 自动添加端口转发
export function autoAddPortForward(data) {
  return request({ url: '/network/port-forward/auto-add', method: 'post', data })
}

// 删除端口转发
export function deletePortForward(id) {
  return request({ url: `/network/port-forward/${id}`, method: 'delete' })
}

export function deletePortForwardByRuleKey(ruleKey) {
  return request({ url: `/network/port-forward/by-key/${encodeURIComponent(ruleKey)}`, method: 'delete' })
}

// 批量删除端口转发
export function batchDeletePortForward(data) {
  return request({ url: '/network/port-forward/batch-delete', method: 'post', data })
}

// 保存端口转发规则
export function savePortForwardRules() {
  return request({ url: '/network/port-forward/save', method: 'post' })
}

// 获取端口转发手动 IP 映射
export function getPortForwardIPs(vmName) {
  return request({ url: '/network/port-forward/ip-mapping', method: 'get', params: { vm_name: vmName } })
}

// 添加端口转发手动 IP 映射
export function addPortForwardIP(data) {
  return request({ url: '/network/port-forward/ip-mapping', method: 'post', data })
}

// 删除端口转发手动 IP 映射
export function deletePortForwardIP(id) {
  return request({ url: `/network/port-forward/ip-mapping/${id}`, method: 'delete' })
}

// 设置端口转发是否豁免入站区域限制
export function setPortForwardFirewall(data) {
  return request({ url: '/firewall/port-forward', method: 'put', data })
}

export function runPortForwardHTTPProbe(data = {}) {
  return request({ url: '/network/port-forward/probe/run', method: 'post', data })
}

export function getPortForwardWhitelistList() {
  return request({ url: '/network/port-forward/whitelist', method: 'get' })
}

export function addPortForwardUserWhitelist(data) {
  return request({ url: '/network/port-forward/whitelist/user', method: 'post', data })
}

export function deletePortForwardUserWhitelist(username) {
  return request({ url: `/network/port-forward/whitelist/user/${encodeURIComponent(username)}`, method: 'delete' })
}

export function addPortForwardVMWhitelist(data) {
  return request({ url: '/network/port-forward/whitelist/vm', method: 'post', data })
}

export function deletePortForwardVMWhitelist(vmName) {
  return request({ url: `/network/port-forward/whitelist/vm/${encodeURIComponent(vmName)}`, method: 'delete' })
}

export function getPortForwardWhitelistSummary(vmName) {
  return request({ url: '/network/port-forward/whitelist/summary', method: 'get', params: { vm_name: vmName } })
}

export function getNetworkCaptureSession(taskId) {
  return request({ url: `/network/captures/${taskId}`, method: 'get', silent: true })
}

export function getNetworkCaptureDownloadUrl(taskId) {
  const token = localStorage.getItem('token')
  const baseUrl = import.meta.env.VITE_APP_BASE_API || '/api'
  return `${baseUrl}/network/captures/${taskId}/download?token=${encodeURIComponent(token || '')}`
}

export function deleteNetworkCapture(taskId) {
  return request({ url: `/network/captures/${taskId}`, method: 'delete' })
}
