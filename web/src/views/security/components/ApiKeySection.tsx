/**
 * API 凭证区块
 * - 显示 API ID、脱敏 Key 标识和使用状态
 * - 支持生成、轮换、撤销；明文 API Key 仅在生成响应后保留在当前页面内存中
 */
import { useCallback, useEffect, useState } from 'react'
import { useNavigate } from 'react-router'
import { Banner, Button, Input, Tag, Toast, Tooltip } from '@douyinfe/semi-ui'
import { IconCopy, IconDelete } from '@douyinfe/semi-icons'
import { getAPIKeyInfo, revokeAPIKey, rotateAPIKey, type UserAPIKeyInfo } from '@/api/apiKey'
import { copyTextWithFallback } from '@/utils/clipboard'
import { confirmModal } from '@/utils/confirm'
import { formatDateTime } from '@/utils/format'

interface CopyActionProps {
  text: string
  label: string
}

function CopyAction({ text, label }: CopyActionProps) {
  const [copying, setCopying] = useState(false)

  const handleCopy = async () => {
    setCopying(true)
    try {
      await copyTextWithFallback(text)
      Toast.success(`${label}已复制`)
    } catch (err) {
      Toast.error(err instanceof Error ? err.message : '复制失败')
    } finally {
      setCopying(false)
    }
  }

  return (
    <Tooltip content={`复制${label}`} position="top">
      <span>
        <Button
          aria-label={`复制${label}`}
          disabled={!text}
          icon={<IconCopy />}
          loading={copying}
          size="small"
          theme="borderless"
          onClick={() => void handleCopy()}
        />
      </span>
    </Tooltip>
  )
}

export default function ApiKeySection() {
  const navigate = useNavigate()
  const [info, setInfo] = useState<UserAPIKeyInfo | null>(null)
  const [generatedKey, setGeneratedKey] = useState('')
  const [loading, setLoading] = useState(true)
  const [generating, setGenerating] = useState(false)
  const [revoking, setRevoking] = useState(false)

  const loadInfo = useCallback(async () => {
    setLoading(true)
    try {
      const res = await getAPIKeyInfo()
      setInfo(res.data || null)
    } catch {
      // 请求层已统一提示
    } finally {
      setLoading(false)
    }
  }, [])

  useEffect(() => {
    void loadInfo()
  }, [loadInfo])

  const handleRotate = async () => {
    const enabled = !!info?.enabled
    const confirmed = await confirmModal({
      title: enabled ? '重新生成 API 凭证' : '生成 API 凭证',
      content: enabled
        ? '重新生成后，当前 API Key 会立即失效。请确认外部程序已准备好更新凭证。'
        : '生成后请立即复制并保存 API Key，页面刷新或离开后无法再次查看明文。',
      okText: enabled ? '重新生成' : '生成',
      danger: enabled,
    })
    if (!confirmed) return

    setGenerating(true)
    try {
      const res = await rotateAPIKey()
      setInfo(res.data || null)
      setGeneratedKey(res.data?.api_key || '')
      Toast.success(res.message || 'API 凭证已生成')
    } catch {
      // 请求层已统一提示（428 高风险验证由请求层自动处理）
    } finally {
      setGenerating(false)
    }
  }

  const handleRevoke = async () => {
    const confirmed = await confirmModal({
      title: '撤销 API 凭证',
      content: '撤销后，使用当前 API Key 的外部程序将立即失去接口访问权限。',
      okText: '撤销',
      danger: true,
    })
    if (!confirmed) return

    setRevoking(true)
    try {
      const res = await revokeAPIKey()
      setGeneratedKey('')
      await loadInfo()
      Toast.success(res.message || 'API 凭证已撤销')
    } catch {
      // 请求层已统一提示（428 高风险验证由请求层自动处理）
    } finally {
      setRevoking(false)
    }
  }

  return (
    <div className="sec-tab-pane">
      <Banner
        type="warning"
        closeIcon={null}
        className="sec-banner"
        description="API Key 可供外部程序调用兼容接口，请仅保存在可信环境中。重新生成或撤销后，旧 Key 会立即失效。"
      />

      <div className="sec-row">
        <div className="sec-row-label">状态</div>
        <div className="sec-row-main">
          <Tag color={info?.enabled ? 'green' : 'grey'}>{info?.enabled ? '已启用' : '未生成'}</Tag>
        </div>
      </div>

      <div className="sec-row">
        <div className="sec-row-label">API ID</div>
        <div className="sec-row-main sec-input-action">
          <Input value={info?.api_key_id || '未生成'} disabled />
          <CopyAction text={info?.api_key_id || ''} label="API ID" />
        </div>
      </div>

      <div className="sec-row">
        <div className="sec-row-label">Key 标识</div>
        <div className="sec-row-main">
          <Input value={info?.key_prefix || '未生成'} disabled />
        </div>
      </div>

      <div className="sec-row">
        <div className="sec-row-label">创建时间</div>
        <div className="sec-row-main">
          <Input value={formatDateTime(info?.created_at)} disabled />
        </div>
      </div>

      <div className="sec-row">
        <div className="sec-row-label">最后使用</div>
        <div className="sec-row-main">
          <Input value={formatDateTime(info?.last_used_at)} disabled />
        </div>
      </div>

      {generatedKey && (
        <div className="sec-row">
          <div className="sec-row-label">API Key</div>
          <div className="sec-row-main sec-input-action">
            <Input mode="password" value={generatedKey} readonly />
            <CopyAction text={generatedKey} label="API Key" />
            <div className="sec-row-tip sec-api-key-tip">
              API Key 仅在本次生成后显示一次；刷新或离开安全中心后，无法再次查看明文。
            </div>
          </div>
        </div>
      )}

      <div className="sec-row">
        <div className="sec-row-label" />
        <div className="sec-row-main sec-actions">
          <Button type="primary" theme="solid" loading={generating || loading} onClick={() => void handleRotate()}>
            {info?.enabled ? '重新生成 Key 和 ID' : '生成 Key 和 ID'}
          </Button>
          <Button
            type="danger"
            theme="light"
            icon={<IconDelete />}
            disabled={!info?.enabled || loading}
            loading={revoking}
            onClick={() => void handleRevoke()}
          >
            撤销
          </Button>
          <Button theme="borderless" onClick={() => navigate('/api-docs')}>
            查看接口文档
          </Button>
        </div>
      </div>
    </div>
  )
}
