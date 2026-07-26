import { StrictMode } from 'react'
import { createRoot } from 'react-dom/client'
import App from './App'
import 'nprogress/nprogress.css'
import '@/assets/styles/index.css'

createRoot(document.getElementById('root')!).render(
  <StrictMode>
    <App />
  </StrictMode>,
)
