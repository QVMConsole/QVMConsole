/**
 * 应用根组件
 * - Semi ConfigProvider（中文语言包）
 * - 启动时同步公开站点设置
 * - 挂载全局高风险验证弹窗
 */
import { useEffect } from 'react'
import { RouterProvider } from 'react-router'
import { ConfigProvider } from '@douyinfe/semi-ui'
import zh_CN from '@douyinfe/semi-ui/lib/es/locale/source/zh_CN'
import { router } from '@/router'
import { useTheme } from '@/hooks/useTheme'
import { syncPublicSiteInfo } from '@/config/site'
import HighRiskChallengeModal from '@/components/business/HighRiskChallengeModal'

export default function App() {
  // 初始化主题（刷新页面时应用存储的主题模式）
  useTheme()

  // 启动时同步站点标题等公开设置
  useEffect(() => {
    void syncPublicSiteInfo()
  }, [])

  return (
    <ConfigProvider locale={zh_CN}>
      <RouterProvider router={router} />
      <HighRiskChallengeModal />
    </ConfigProvider>
  )
}
