/**
 * API 文档页工具函数
 * 负责合并「自动生成接口清单 + 人工补充描述」、字段提取与 curl 示例构建。
 */
import {
  endpointDescriptions,
  moduleGroups,
  fallbackGroupName,
  fallbackGroupDescription,
} from './endpointDescriptions'
import { fieldDescriptions, ignoredFieldTokens } from './fieldDictionary'

/** 自动生成的接口条目（endpoints.json 结构） */
export interface GeneratedEndpoint {
  method: string
  path: string
  handler: string
  auth: 'public' | 'api' | 'api-bootstrap' | 'jwt' | 'login'
  admin: boolean
  elasticOnly: boolean
  vmAccess: boolean
  highRisk: string
  comment: string
}

/** 合并后的展示条目 */
export interface DocEndpoint extends GeneratedEndpoint {
  key: string
  summary: string
  /** 是否有人工补充文案 */
  documented: boolean
  body: string
  response: string
  query: string[]
  notes: string[]
  requiredFields: string[]
  highRiskNote: string
  pathParams: string[]
}

/** 分组结果 */
export interface DocGroup {
  name: string
  description: string
  endpoints: DocEndpoint[]
}

/** 认证方式中文标签 */
export function authLabel(auth: GeneratedEndpoint['auth']): string {
  const map: Record<GeneratedEndpoint['auth'], string> = {
    public: '公开',
    api: 'JWT / API Key',
    'api-bootstrap': 'JWT(access/bootstrap) / API Key',
    jwt: '仅 JWT（access/bootstrap token）',
    login: '仅登录阶段 token',
  }
  return map[auth] || auth
}

/** 按认证方式给出请求头示例 */
export function authHeaders(auth: GeneratedEndpoint['auth']): string[] {
  const json = 'Content-Type: application/json'
  if (auth === 'public') return []
  if (auth === 'jwt') return ['Authorization: Bearer <access/bootstrap token>', json]
  if (auth === 'login') return ['Authorization: Bearer <login token>', json]
  return ['X-API-Key-ID: <API_ID>', 'X-API-Key: <API_KEY>', json]
}

/** 从路径中提取路径参数（:name 片段） */
function extractPathParams(path: string): string[] {
  return (path.match(/:(\w+)/g) || []).map((p) => p.slice(1))
}

/** handler 名转可读摘要兜底（无人工文案时展示） */
function fallbackSummary(handler: string): string {
  return handler
}

/** 合并生成数据与补充描述，并按模块分组 */
export function buildDocGroups(endpoints: GeneratedEndpoint[]): DocGroup[] {
  const groupMap = new Map<string, DocEndpoint[]>()

  for (const ep of endpoints) {
    const key = `${ep.method} ${ep.path}`
    const desc = endpointDescriptions[key]
    const doc: DocEndpoint = {
      ...ep,
      key,
      summary: desc?.summary || ep.comment || fallbackSummary(ep.handler),
      documented: Boolean(desc?.summary),
      body: desc?.body || '无',
      response: desc?.response || '统一返回 { code, message, data }，data 为对应资源或任务信息。',
      query: desc?.query || [],
      notes: desc?.notes || [],
      requiredFields: desc?.requiredFields || [],
      highRiskNote: desc?.highRiskNote || '',
      pathParams: extractPathParams(ep.path),
    }
    const group =
      moduleGroups.find((g) =>
        g.prefixes.some((p) => doc.path === p || doc.path.startsWith(`${p}/`)),
      )?.name || fallbackGroupName
    if (!groupMap.has(group)) groupMap.set(group, [])
    groupMap.get(group)!.push(doc)
  }

  // 按 moduleGroups 定义顺序输出，兜底组放最后
  const result: DocGroup[] = []
  for (const g of moduleGroups) {
    const eps = groupMap.get(g.name)
    if (eps?.length) result.push({ name: g.name, description: g.description, endpoints: eps })
  }
  const rest = groupMap.get(fallbackGroupName)
  if (rest?.length) {
    result.push({ name: fallbackGroupName, description: fallbackGroupDescription, endpoints: rest })
  }
  return result
}

// ==================== 字段解释表 ====================

export interface FieldRow {
  name: string
  location: string
  required?: string
  description: string
}

const normalizeParamName = (raw: string) =>
  String(raw || '')
    .replace(/^:/, '')
    .replace(/\(.+?\)/g, '')
    .replace(/（.+?）/g, '')
    .replace(/<|>/g, '')
    .trim()

const describeField = (name: string) => {
  const normalized = normalizeParamName(name)
  return (
    fieldDescriptions[normalized] ||
    '业务字段，按该接口请求体说明填写；后端会按当前资源状态、权限和配额进行校验。'
  )
}

/** 从请求体/返回描述文本中提取字段名 */
function extractFieldsFromText(text: string): string[] {
  if (!text || text === '无') return []
  const matches = String(text).match(/[A-Za-z][A-Za-z0-9_]*|[a-z]+_[a-z0-9_]+/g) || []
  const seen = new Set<string>()
  return matches
    .map(normalizeParamName)
    .filter((name) => {
      if (!name || ignoredFieldTokens.has(name) || ignoredFieldTokens.has(name.toUpperCase())) return false
      if (name.length < 2 && name !== 'id') return false
      if (seen.has(name)) return false
      seen.add(name)
      return /[a-z_]/.test(name)
    })
}

/** 请求参数解释表数据 */
export function requestFields(ep: DocEndpoint): FieldRow[] {
  const fields: FieldRow[] = []
  ep.pathParams.forEach((name) =>
    fields.push({ name: normalizeParamName(name), location: 'Path', required: '是', description: describeField(name) }),
  )
  ep.query.forEach((name) =>
    fields.push({ name: normalizeParamName(name), location: 'Query', required: '否', description: describeField(name) }),
  )
  const bodyLocation = ep.body.startsWith('FormData') ? 'FormData' : 'Body'
  extractFieldsFromText(ep.body).forEach((name) => {
    fields.push({
      name,
      location: bodyLocation,
      required: ep.requiredFields.includes(name) ? '是' : '否',
      description: describeField(name),
    })
  })
  if (!fields.length) {
    fields.push({ name: '无', location: '-', required: '-', description: '该接口不需要额外请求参数。' })
  }
  return fields
}

/** 返回参数解释表数据 */
export function responseFields(ep: DocEndpoint): FieldRow[] {
  const fields: FieldRow[] = [
    { name: 'code', location: 'Root', description: fieldDescriptions.code },
    { name: 'message', location: 'Root', description: fieldDescriptions.message },
    { name: 'data', location: 'Root', description: ep.response || '接口业务数据。' },
  ]
  extractFieldsFromText(ep.response).forEach((name) => {
    if (!['code', 'message', 'data'].includes(name)) {
      fields.push({ name, location: 'data', description: describeField(name) })
    }
  })
  if (ep.response.includes('文件流')) {
    fields.push({ name: 'binary', location: 'Response Body', description: '文件下载接口直接返回二进制文件流，不使用统一 JSON data。' })
  }
  if (ep.response.includes('text/event-stream')) {
    fields.push({ name: 'event', location: 'SSE', description: '服务端事件流数据，客户端需要按 EventSource/SSE 协议读取。' })
  }
  return fields
}

// ==================== curl 示例 ====================

/** 路径参数占位替换 */
const PATH_SAMPLE: Record<string, string> = {
  name: 'vm-name',
  username: 'username',
  filename: 'file-name',
  rule_key: 'rule-key',
  vm_name: 'vm-name',
  vmName: 'vm-name',
  task_id: '1',
  snap: 'snapshot-name',
  tag: 'tag',
  dev: 'vda',
  category: 'iso',
  id: '1',
  order: '1',
}

/** 简单示例请求体 */
function sampleBody(ep: DocEndpoint): string {
  if (ep.body === '无' || ep.body.startsWith('FormData')) return ''
  if (!['POST', 'PUT', 'PATCH', 'DELETE'].includes(ep.method)) return ''
  if (ep.path === '/vm/create' || ep.path === '/self/vm/create') {
    const isoDir = ep.path === '/vm/create' ? '/var/lib/libvirt/images/ISO' : '/mnt/user/iso'
    return `{"name":"demo","vcpu":2,"ram":4096,"disk_size":40,"disk_format":"qcow2","disk_bus":"virtio","os_variant":"generic","iso_path":"${isoDir}/example.iso","nic_model":"virtio","os_type":"linux","machine_type":"q35","boot_type":"bios","video_model":"virtio","switch_id":1,"security_group_id":1,"storage_pool_id":"default"}`
  }
  if (ep.body.includes('action(start')) return '{"action":"start"}'
  if (ep.body.includes('enabled')) return '{"enabled":true}'
  if (ep.body.includes('password')) return '{"password":"StrongPassword123!"}'
  if (ep.body.includes('security_group_id')) return '{"security_group_id":1}'
  if (ep.body.includes('size_gb')) return '{"size_gb":20}'
  if (ep.body.includes('profile')) return '{"profile":"balanced"}'
  if (ep.body.includes('email')) return '{"email":"user@example.com"}'
  if (ep.body.includes('username')) return '{"username":"test"}'
  if (ep.body.includes('xml')) return '{"xml":"<domain>...</domain>"}'
  return '{"example":"请按请求体说明填写"}'
}

/** 构建 curl 示例 */
export function buildCurl(ep: DocEndpoint, apiBase: string): string {
  const url = ep.path.replace(/:(\w+)/g, (_, p: string) => PATH_SAMPLE[p] || '1')
  const lines = [`curl -X ${ep.method} "${apiBase}${url}"`]
  authHeaders(ep.auth).forEach((header) => lines.push(`  -H "${header}"`))
  const body = sampleBody(ep)
  if (body) lines.push(`  -d '${body}'`)
  return lines.join(' \\\n')
}
