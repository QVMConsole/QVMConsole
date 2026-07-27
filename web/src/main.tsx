// React 19 适配器：必须在最顶部导入，保证 Semi 的 Toast/Modal/Tooltip 正常工作
import '@douyinfe/semi-ui/react19-adapter'
import { StrictMode } from 'react'
import { createRoot } from 'react-dom/client'
import App from './App'
import 'nprogress/nprogress.css'
import '@/assets/styles/index.css'
import '@/assets/styles/aurora.css'

createRoot(document.getElementById('root')!).render(
  <StrictMode>
    <App />
  </StrictMode>,
)
