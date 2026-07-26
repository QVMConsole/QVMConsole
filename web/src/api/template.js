import request from '@/utils/request'

export function prepareImportedLinuxTemplate(name) {
  return request({
    url: `/template/${encodeURIComponent(name)}/prepare-linux`,
    method: 'post'
  })
}

// ==================== 模板包分片上传 API ====================
// 模板包（.tar.gz/.tgz）通过分片上传落到导入临时目录，完成后走 preview/confirm（仍在 @/api/vm）。

// 初始化/恢复模板包上传会话（含秒传判断）
export function templateUploadInit(data) {
  return request({
    url: '/template/upload/init',
    method: 'post',
    data
  })
}

// 上传单个分片（multipart: file, session_key, index）
export function templateUploadChunk(formData) {
  return request({
    url: '/template/upload/chunk',
    method: 'post',
    data: formData,
    timeout: 0,
    maxContentLength: Infinity,
    maxBodyLength: Infinity
  })
}

// 全部分片到齐后完成校验，返回临时路径供 preview/confirm 使用
export function templateUploadComplete(data) {
  return request({
    url: '/template/upload/complete',
    method: 'post',
    data
  })
}

// 清理已上传的模板包临时文件（预览后未导入而关闭对话框时调用）
export function templateUploadCancel(path) {
  return request({
    url: '/template/upload',
    method: 'delete',
    params: { path }
  })
}
