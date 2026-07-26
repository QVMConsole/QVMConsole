/**
 * 首页仪表盘（地基占位）
 * 完整版（资源统计、宿主机状态、监控图表等）随 dashboard 模块迭代迁移
 */
import { Card, Typography, Banner } from '@douyinfe/semi-ui'
import { useUserStore } from '@/stores/user'

export default function DashboardPage() {
  const username = useUserStore((s) => s.username)

  return (
    <div style={{ display: 'flex', flexDirection: 'column', gap: 16 }}>
      <Banner
        type="info"
        description="前端地基已就绪：React 19 + TypeScript + Semi Design。业务页面将按模块逐步迁移。"
        closeIcon={null}
      />
      <Card title="欢迎">
        <Typography.Paragraph>
          你好，{username || '用户'}！QVMConsole 前端重构正在进行中。
        </Typography.Paragraph>
      </Card>
    </div>
  )
}
