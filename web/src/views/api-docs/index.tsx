/**
 * API 文档页
 * 接口清单由构建脚本从后端源码自动生成（generated/endpoints.json），
 * 中文文案来自 endpointDescriptions.ts 补充描述；后端新增接口重新构建即自动同步。
 * 支持关键字搜索、模块筛选、只看二次验证接口。
 */
import { useMemo, useState } from 'react'
import { Button, Card, Checkbox, Collapse, Empty, Input, Select, Tag, Toast } from '@douyinfe/semi-ui'
import { IconCopy, IconSearch } from '@douyinfe/semi-icons'
import { IllustrationNoResult, IllustrationNoResultDark } from '@douyinfe/semi-illustrations'
import { copyTextWithFallback } from '@/utils/clipboard'
import EndpointDetail from './EndpointDetail'
import { buildDocGroups, type DocEndpoint, type GeneratedEndpoint } from './docUtils'
import generatedData from './generated/endpoints.json'
import './api-docs.css'

const apiBase = `${window.location.origin}/api`

const curlExample = `curl -H "X-API-Key-ID: kvm_id_xxx" \\
  -H "X-API-Key: kvm_sk_xxx" \\
  "${apiBase}/vm/list"`

const responseExample = `{
  "code": 200,
  "message": "ok",
  "data": {}
}`

const highRiskExample = `curl -X POST "${apiBase}/auth/high-risk/verify" \\
  -H "X-API-Key-ID: kvm_id_xxx" \\
  -H "X-API-Key: kvm_sk_xxx" \\
  -H "Content-Type: application/json" \\
  -d '{"method":"totp","code":"123456","operation":"delete_vm"}'`

/** 请求方法标签配色 */
function methodColor(method: string): 'green' | 'blue' | 'orange' | 'red' | 'grey' {
  const map: Record<string, 'green' | 'blue' | 'orange' | 'red'> = {
    GET: 'green',
    POST: 'blue',
    PUT: 'orange',
    PATCH: 'orange',
    DELETE: 'red',
  }
  return map[method] || 'grey'
}

/** 关键字匹配 */
function matchEndpoint(ep: DocEndpoint, keyword: string): boolean {
  if (!keyword) return true
  return [
    ep.method,
    ep.path,
    ep.summary,
    ep.handler,
    ep.body,
    ep.response,
    ep.highRisk,
    ...ep.pathParams,
    ...ep.query,
    ...ep.notes,
  ]
    .join(' ')
    .toLowerCase()
    .includes(keyword)
}

const allGroups = buildDocGroups(generatedData.endpoints as GeneratedEndpoint[])

export default function ApiDocsPage() {
  const [keyword, setKeyword] = useState('')
  const [activeGroup, setActiveGroup] = useState('')
  const [onlyHighRisk, setOnlyHighRisk] = useState(false)

  const visibleGroups = useMemo(() => {
    const key = keyword.trim().toLowerCase()
    return allGroups
      .filter((g) => !activeGroup || g.name === activeGroup)
      .map((g) => ({
        ...g,
        endpoints: g.endpoints.filter((ep) => (!onlyHighRisk || ep.highRisk) && matchEndpoint(ep, key)),
      }))
      .filter((g) => g.endpoints.length)
  }, [keyword, activeGroup, onlyHighRisk])

  const totalVisible = useMemo(
    () => visibleGroups.reduce((sum, g) => sum + g.endpoints.length, 0),
    [visibleGroups],
  )

  const handleCopyAuth = async () => {
    try {
      await copyTextWithFallback(curlExample)
      Toast.success('已复制')
    } catch (err) {
      Toast.error(err instanceof Error ? err.message : '复制失败')
    }
  }

  return (
    <div className="api-docs-page">
      <div className="api-docs-header">
        <div>
          <h2>接口文档</h2>
          <p className="api-docs-sub">
            外部程序可使用账户安全设置中生成的 API ID 和 API Key 调用兼容接口。接口清单在构建时从后端源码自动生成。
          </p>
        </div>
        <div className="api-docs-actions">
          <Tag size="large" type="ghost">
            {totalVisible} 个接口
          </Tag>
          <Button theme="solid" icon={<IconCopy />} onClick={() => void handleCopyAuth()}>
            复制认证示例
          </Button>
        </div>
      </div>

      <Card className="api-doc-section" title="认证与响应">
        <div className="api-intro-grid">
          <div>
            <p>推荐使用独立请求头传递凭证，避免将 Key 放入 URL。登录、安全初始化、邮箱和 2FA 流程不接受 API Key。</p>
            <pre className="api-code-block">{curlExample}</pre>
          </div>
          <div>
            <p>接口返回统一 JSON 结构。code 为 200 或 0 表示成功，其他值表示失败。</p>
            <pre className="api-code-block">{responseExample}</pre>
          </div>
        </div>
      </Card>

      <Card className="api-doc-section" title="接口检索">
        <div className="api-filters">
          <Input
            prefix={<IconSearch />}
            placeholder="搜索路径、说明、请求字段"
            value={keyword}
            onChange={setKeyword}
            showClear
            className="api-filter-input"
          />
          <Select
            placeholder="选择模块"
            value={activeGroup || undefined}
            onChange={(v) => setActiveGroup((v as string) || '')}
            showClear
            className="api-filter-select"
            optionList={[
              { label: '全部模块', value: '' },
              ...allGroups.map((g) => ({ label: `${g.name}（${g.endpoints.length}）`, value: g.name })),
            ]}
          />
          <Checkbox checked={onlyHighRisk} onChange={(e) => setOnlyHighRisk(Boolean(e.target.checked))}>
            只看二次验证
          </Checkbox>
        </div>
      </Card>

      {visibleGroups.map((group) => (
        <Card
          key={group.name}
          className="api-doc-section"
          title={
            <div className="api-group-title">
              <span>{group.name}</span>
              <Tag size="small" type="ghost">
                {group.endpoints.length} 个
              </Tag>
            </div>
          }
        >
          <p className="api-group-desc">{group.description}</p>
          <Collapse keepDOM={false}>
            {group.endpoints.map((ep) => (
              <Collapse.Panel
                key={ep.key}
                itemKey={ep.key}
                header={
                  <div className="api-endpoint-title">
                    <Tag color={methodColor(ep.method)} size="small" className="api-method-tag">
                      {ep.method}
                    </Tag>
                    <code>{ep.path}</code>
                    <span className="api-endpoint-summary">{ep.summary}</span>
                    {ep.highRisk && (
                      <Tag color="orange" size="small">
                        二次验证
                      </Tag>
                    )}
                    {ep.admin && (
                      <Tag color="purple" size="small">
                        管理员
                      </Tag>
                    )}
                    {ep.elasticOnly && (
                      <Tag color="cyan" size="small">
                        轻量云不可用
                      </Tag>
                    )}
                    {!ep.documented && (
                      <Tag color="grey" size="small" type="ghost">
                        待补充文案
                      </Tag>
                    )}
                  </div>
                }
              >
                <EndpointDetail endpoint={ep} apiBase={apiBase} />
              </Collapse.Panel>
            ))}
          </Collapse>
        </Card>
      ))}

      {!visibleGroups.length && (
        <Card className="api-doc-section">
          <Empty
            image={<IllustrationNoResult style={{ width: 140, height: 140 }} />}
            darkModeImage={<IllustrationNoResultDark style={{ width: 140, height: 140 }} />}
            description="没有匹配的接口，请调整筛选条件"
          />
        </Card>
      )}

      <Card className="api-doc-section" title="高风险操作">
        <p className="api-group-desc">
          删除虚拟机、重置密码、修改防火墙等接口仍会要求二次验证。API 调用收到 428 后，先完成
          /api/auth/high-risk/verify，再在原请求携带 X-High-Risk-Token。
        </p>
        <pre className="api-code-block">{highRiskExample}</pre>
      </Card>
    </div>
  )
}
