/**
 * 页面标签页栏（多对象快速切换，置于顶部导航栏内）
 * - 工作台为固定标签，其余标签随路由访问注册
 * - 支持关闭单个标签
 */
import { useLocation, useNavigate } from 'react-router'
import { IconGridRectangle, IconClose } from '@douyinfe/semi-icons'
import { usePageTabsStore } from '@/stores/pageTabs'

export default function PageTabsBar() {
  const navigate = useNavigate()
  const location = useLocation()
  const tabs = usePageTabsStore((s) => s.tabs)
  const closeTab = usePageTabsStore((s) => s.closeTab)

  const handleTabClick = (key: string) => {
    if (key !== location.pathname) {
      navigate(key)
    }
  }

  const handleClose = (key: string, e: React.MouseEvent) => {
    e.stopPropagation()
    const redirect = closeTab(key, location.pathname)
    if (redirect) navigate(redirect)
  }

  return (
    <div className="qvm-tabs-bar">
      {tabs.map((tab) => {
        const on = location.pathname === tab.key
        return (
          <div
            key={tab.key}
            className={`qvm-pg-tab ${on ? 'on' : ''} ${tab.pinned ? 'pin' : ''}`}
            onClick={() => handleTabClick(tab.key)}
          >
            {tab.pinned ? <IconGridRectangle size="small" /> : tab.dot ? (
              <span className={`qvm-dot ${tab.dot}`} />
            ) : null}
            {tab.title}
            {!tab.pinned && (
              <span className="qvm-tab-x" onClick={(e) => handleClose(tab.key, e)}>
                <IconClose size="extra-small" />
              </span>
            )}
          </div>
        )
      })}
    </div>
  )
}
