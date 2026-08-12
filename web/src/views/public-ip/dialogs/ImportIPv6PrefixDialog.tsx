/**
 * IPv6 前缀导入对话框
 * - 从 IPv6 默认路由或指定上联网卡检测公网前缀
 * - 将前缀展开为可逐台 VM 绑定的 /128 路由资源
 */
import { useEffect, useState } from 'react'
import { Banner, Button, Input, InputNumber, Modal, Select, TextArea, Toast } from '@douyinfe/semi-ui'
import { IconRefresh } from '@douyinfe/semi-icons'
import {
  discoverPublicIPv6Prefixes,
  importPublicIPv6Prefix,
  type PublicIPv6PrefixInfo,
} from '@/api/publicIp'
import { useMountModalLifecycle } from '@/hooks/useMountModalLifecycle'

interface ImportIPv6PrefixDialogProps {
  suggestedCount: number
  onClose: () => void
  onSaved: () => void
}

export default function ImportIPv6PrefixDialog({
  suggestedCount,
  onClose,
  onSaved,
}: ImportIPv6PrefixDialogProps) {
  const { modalVisible, requestClose, afterModalClose } = useMountModalLifecycle(onClose)
  const [uplinkIF, setUplinkIF] = useState('')
  const [prefix, setPrefix] = useState('')
  const [prefixes, setPrefixes] = useState<PublicIPv6PrefixInfo[]>([])
  const [count, setCount] = useState(Math.max(1, suggestedCount))
  const [remark, setRemark] = useState('')
  const [detecting, setDetecting] = useState(false)
  const [submitting, setSubmitting] = useState(false)

  const detect = async (specifiedIF = uplinkIF) => {
    setDetecting(true)
    try {
      const res = await discoverPublicIPv6Prefixes(specifiedIF.trim() || undefined)
      const items = res.data || []
      setPrefixes(items)
      if (items.length > 0) {
        setUplinkIF(items[0].uplink_if)
        setPrefix(items[0].prefix)
      }
    } catch {
      setPrefixes([])
      setPrefix('')
    } finally {
      setDetecting(false)
    }
  }

  useEffect(() => {
    void detect('')
    // 首次挂载按 IPv6 默认路由自动检测一次。
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [])

  const submit = async () => {
    if (!uplinkIF.trim() || !prefix) {
      Toast.warning('请先检测并选择公网 IPv6 前缀')
      return
    }
    setSubmitting(true)
    try {
      const res = await importPublicIPv6Prefix({
        uplink_if: uplinkIF.trim(),
        prefix,
        count,
        remark: remark.trim(),
      })
      Toast.success(`已导入 ${res.data?.created?.length || count} 个公网 IPv6 地址`)
      onSaved()
      requestClose()
    } catch {
      // 请求层已提示
    } finally {
      setSubmitting(false)
    }
  }

  return (
    <Modal
      title="导入公网 IPv6 前缀"
      visible={modalVisible}
      afterClose={afterModalClose}
      onCancel={requestClose}
      onOk={() => void submit()}
      okText="导入地址"
      cancelText="取消"
      confirmLoading={submitting}
      width={620}
      closeOnEsc
    >
      <Banner
        type="info"
        closeIcon={null}
        description="系统会为每台 VM 生成独立 /128 公网 IPv6，通过 Proxy NDP 与精确路由转发，不使用 NAT66。动态前缀变化时会保留主机位自动同步。"
        style={{ marginBottom: 14 }}
      />
      <div className="qvm-form-item">
        <div className="qvm-form-label required">上联网卡</div>
        <div className="pip-inline-field">
          <Input
            value={uplinkIF}
            onChange={setUplinkIF}
            placeholder="留空按 IPv6 默认路由检测，例如 enp6s0"
          />
          <Button
            icon={<IconRefresh spin={detecting} />}
            loading={detecting}
            onClick={() => void detect()}
          >
            检测前缀
          </Button>
        </div>
      </div>
      <div className="qvm-form-item">
        <div className="qvm-form-label required">公网 IPv6 前缀</div>
        <Select
          value={prefix}
          onChange={(value) => setPrefix(value as string)}
          placeholder="请先检测上联网卡"
          style={{ width: '100%' }}
          optionList={prefixes.map((item) => ({
            value: item.prefix,
            label: `${item.prefix}（本机 ${item.address}${item.gateway ? `，网关 ${item.gateway}` : ''}）`,
          }))}
        />
      </div>
      <div className="qvm-form-item">
        <div className="qvm-form-label required">生成地址数量</div>
        <InputNumber
          value={count}
          onChange={(value) => setCount(Number(value) || 1)}
          min={1}
          max={4096}
          style={{ width: '100%' }}
        />
        <div className="qvm-form-tip">默认按当前 VM 数量生成，可随时再次导入补充地址。</div>
      </div>
      <div className="qvm-form-item">
        <div className="qvm-form-label">备注</div>
        <TextArea
          rows={2}
          value={remark}
          onChange={setRemark}
          placeholder="例如：运营商动态 IPv6 前缀"
        />
      </div>
    </Modal>
  )
}
