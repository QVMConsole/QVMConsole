/**
 * ACL Tab（仅管理员）
 * - 预览 VPC nftables 规则（代码块 + 复制）
 * - 应用 ACL（重建防火墙规则，高风险二次确认）
 */
import { Button, Spin } from '@douyinfe/semi-ui'
import { IconCheckList, IconCopy, IconFile, IconRefresh } from '@douyinfe/semi-icons'

interface AclTabProps {
  preview: string
  loading: boolean
  applying: boolean
  onRefresh: () => void
  onApply: () => void
  onCopy: () => void
}

export default function AclTab({ preview, loading, applying, onRefresh, onApply, onCopy }: AclTabProps) {
  return (
    <div>
      <div className="net-toolbar">
        <div className="net-toolbar-left">
          <span className="net-table-title">VPC ACL 规则预览</span>
        </div>
        <div className="net-toolbar-right">
          <Button icon={<IconRefresh />} loading={loading} onClick={onRefresh}>
            刷新预览
          </Button>
          <Button
            type="warning"
            theme="light"
            icon={<IconCheckList />}
            loading={applying}
            onClick={onApply}
          >
            应用 ACL
          </Button>
        </div>
      </div>

      <div className="net-code-block">
        <div className="net-code-header">
          <IconFile />
          <span>nftables 规则</span>
          <Button size="small" theme="borderless" icon={<IconCopy />} onClick={onCopy} disabled={!preview}>
            复制
          </Button>
        </div>
        <Spin spinning={loading} size="large">
          <pre className="net-code-body">
            <code>{preview || '点击「刷新预览」加载规则'}</code>
          </pre>
        </Spin>
      </div>
    </div>
  )
}
