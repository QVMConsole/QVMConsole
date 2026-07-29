/**
 * 404 页面
 */
import { Empty, Button } from '@douyinfe/semi-ui'
import { IllustrationNotFound, IllustrationNotFoundDark } from '@douyinfe/semi-illustrations'
import { useNavigate } from 'react-router'
import { useTheme } from '@/hooks/useTheme'

export default function NotFound() {
  const navigate = useNavigate()
  const { isDark } = useTheme()

  return (
    <div style={{ display: 'flex', justifyContent: 'center', paddingTop: 80 }}>
      <Empty
        image={isDark ? <IllustrationNotFoundDark /> : <IllustrationNotFound />}
        title="页面不存在"
        description="您访问的页面不存在或尚未迁移完成"
      >
        <Button type="primary" theme="solid" onClick={() => navigate('/')}>
          返回首页
        </Button>
      </Empty>
    </div>
  )
}
