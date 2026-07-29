import { readFile, readdir, writeFile, mkdir, access } from 'node:fs/promises'
import path from 'node:path'
import { fileURLToPath } from 'node:url'

const webRoot = path.resolve(path.dirname(fileURLToPath(import.meta.url)), '..')
const projectRoot = path.resolve(webRoot, '..')
const routerFile = path.join(projectRoot, 'server', 'router', 'router.go')
const handlerDir = path.join(projectRoot, 'server', 'handler')
const outputFile = path.join(webRoot, 'src', 'views', 'api-docs', 'generated', 'endpoints.json')

async function exists(file) {
  try {
    await access(file)
    return true
  } catch {
    return false
  }
}

if (!(await exists(routerFile))) {
  if (await exists(outputFile)) {
    process.stderr.write('未找到后端路由源码，沿用已生成的接口清单。\n')
    process.exit(0)
  }
  throw new Error('未找到后端路由源码和历史接口清单')
}

const highRiskByHandler = new Map()
for (const name of await readdir(handlerDir)) {
  if (!name.endsWith('.go')) continue
  const source = await readFile(path.join(handlerDir, name), 'utf8')
  const functions = [...source.matchAll(/func\s+(\w+)\s*\([^)]*\)\s*\{/g)]
  for (let index = 0; index < functions.length; index++) {
    const current = functions[index]
    const start = current.index ?? 0
    const end = functions[index + 1]?.index ?? source.length
    const body = source.slice(start, end)
    const risk = body.match(/requireHighRiskVerification\s*\(\s*c\s*,\s*"([^"]+)"/)
    if (risk) highRiskByHandler.set(current[1], risk[1])
  }
}

const routerSource = await readFile(routerFile, 'utf8')
const groups = new Map([
  ['api', { prefix: '', auth: 'public', admin: false, elasticOnly: false, vmAccess: false }],
])

const mergeMiddleware = (base, text) => ({
  ...base,
  auth: text.includes('JWTTokenTypeMiddleware') || text.includes('TokenTypeMiddleware') ? 'jwt-only' : text.includes('AuthMiddleware') ? 'jwt' : base.auth,
  admin: base.admin || text.includes('AdminMiddleware'),
  elasticOnly: base.elasticOnly || text.includes('ElasticCloudOnlyMiddleware'),
  vmAccess: base.vmAccess || text.includes('VMAccessMiddleware'),
})

const endpoints = []
for (const rawLine of routerSource.split(/\r?\n/)) {
  const line = rawLine.trim()
  const groupMatch = line.match(/^(\w+)\s*:=\s*(\w+)\.Group\("([^"]*)"([^)]*)\)/)
  if (groupMatch) {
    const [, name, parent, suffix, middleware] = groupMatch
    const parentMeta = groups.get(parent) || groups.get('api')
    groups.set(name, mergeMiddleware({ ...parentMeta, prefix: `${parentMeta.prefix}${suffix}` }, middleware))
    if (name === 'authorized') groups.set(name, { ...groups.get(name), auth: 'jwt' })
    continue
  }
  const useMatch = line.match(/^(\w+)\.Use\((.*)\)/)
  if (useMatch && groups.has(useMatch[1])) {
    groups.set(useMatch[1], mergeMiddleware(groups.get(useMatch[1]), useMatch[2]))
    continue
  }
  const routeMatch = line.match(/^(\w+)\.(GET|POST|PUT|PATCH|DELETE)\("([^"]*)"\s*,\s*(.*)\)/)
  if (!routeMatch) continue
  const [, groupName, method, suffix, args] = routeMatch
  const group = groups.get(groupName)
  if (!group) continue
  const handlerMatches = [...args.matchAll(/handler\.(\w+)/g)]
  const handler = handlerMatches.at(-1)?.[1] || ''
  const comment = rawLine.includes('//') ? rawLine.slice(rawLine.lastIndexOf('//') + 2).trim() : ''
  const meta = mergeMiddleware(group, args)
  endpoints.push({
    method,
    path: `${group.prefix}${suffix}`.replace(/\/+/g, '/').replace(/^\/api(?=\/)/, ''),
    handler,
    auth: meta.auth,
    admin: meta.admin,
    elasticOnly: meta.elasticOnly,
    vmAccess: meta.vmAccess,
    highRisk: highRiskByHandler.get(handler) || '',
    comment,
  })
}

endpoints.sort((left, right) => `${left.path} ${left.method}`.localeCompare(`${right.path} ${right.method}`))
await mkdir(path.dirname(outputFile), { recursive: true })
await writeFile(outputFile, `${JSON.stringify({ generated_at: new Date().toISOString(), count: endpoints.length, endpoints }, null, 2)}\n`, 'utf8')
process.stdout.write(`已生成 ${endpoints.length} 个 API 端点。\n`)
