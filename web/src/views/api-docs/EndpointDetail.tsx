/**
 * 单个接口的展开详情
 * 展示认证/请求头/参数说明、请求与返回字段解释表、curl 示例。
 */
import { Button, Descriptions, Table, Tag, Toast } from '@douyinfe/semi-ui'
import { IconCopy } from '@douyinfe/semi-icons'
import type { ColumnProps } from '@douyinfe/semi-ui/lib/es/table'
import { copyTextWithFallback } from '@/utils/clipboard'
import {
  authHeaders,
  authLabel,
  buildCurl,
  requestFields,
  responseFields,
  type DocEndpoint,
  type FieldRow,
} from './docUtils'

const requestColumns: ColumnProps<FieldRow>[] = [
  { title: '参数', dataIndex: 'name', width: 200 },
  { title: '位置', dataIndex: 'location', width: 100 },
  { title: '必填', dataIndex: 'required', width: 70 },
  { title: '说明', dataIndex: 'description' },
]

const responseColumns: ColumnProps<FieldRow>[] = [
  { title: '字段', dataIndex: 'name', width: 200 },
  { title: '位置', dataIndex: 'location', width: 120 },
  { title: '说明', dataIndex: 'description' },
]

async function copyText(text: string) {
  try {
    await copyTextWithFallback(text)
    Toast.success('已复制')
  } catch (err) {
    Toast.error(err instanceof Error ? err.message : '复制失败')
  }
}

export default function EndpointDetail({ endpoint, apiBase }: { endpoint: DocEndpoint; apiBase: string }) {
  const headers = authHeaders(endpoint.auth)
  const curl = buildCurl(endpoint, apiBase)

  return (
    <div className="api-endpoint-detail">
      <Descriptions
        align="left"
        data={[
          { key: '认证', value: authLabel(endpoint.auth) },
          {
            key: '请求头',
            value: headers.length ? (
              <span className="api-header-list">
                {headers.map((h) => (
                  <code key={h}>{h}</code>
                ))}
              </span>
            ) : (
              '无'
            ),
          },
          { key: '路径参数', value: endpoint.pathParams.length ? endpoint.pathParams.join(', ') : '无' },
          { key: '查询参数', value: endpoint.query.length ? endpoint.query.join(', ') : '无' },
          { key: '请求体', value: endpoint.body },
          { key: '返回', value: endpoint.response },
          ...(endpoint.highRisk
            ? [
                {
                  key: '二次验证',
                  value: endpoint.highRiskNote || `操作标识 ${endpoint.highRisk}，428 后携带 X-High-Risk-Token 重试`,
                },
              ]
            : []),
          ...(endpoint.notes.length
            ? [
                {
                  key: '备注',
                  value: (
                    <span className="api-note-list">
                      {endpoint.notes.map((note) => (
                        <Tag key={note} size="small" color="grey" type="ghost">
                          {note}
                        </Tag>
                      ))}
                    </span>
                  ),
                },
              ]
            : []),
        ]}
      />

      <div className="api-field-section">
        <h4>请求参数解释</h4>
        <Table
          columns={requestColumns}
          dataSource={requestFields(endpoint)}
          rowKey={(r) => `${r?.location}-${r?.name}`}
          pagination={false}
          size="small"
          bordered
        />
      </div>

      <div className="api-field-section">
        <h4>返回参数解释</h4>
        <Table
          columns={responseColumns}
          dataSource={responseFields(endpoint)}
          rowKey={(r) => `${r?.location}-${r?.name}`}
          pagination={false}
          size="small"
          bordered
        />
      </div>

      <div className="api-example-row">
        <pre className="api-code-block">{curl}</pre>
        <Button icon={<IconCopy />} onClick={() => void copyText(curl)}>
          复制 curl
        </Button>
      </div>
    </div>
  )
}
