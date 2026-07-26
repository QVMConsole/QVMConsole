import { defineConfig } from 'vite'
import react from '@vitejs/plugin-react'
import { fileURLToPath, URL } from 'node:url'

// https://vite.dev/config/
export default defineConfig({
  plugins: [react()],
  build: {
    rolldownOptions: {
      output: {
        // 依赖分包：降低首屏单 chunk 体积
        codeSplitting: {
          groups: [
            { name: 'react-vendor', test: /node_modules[\\/](react|react-dom|react-router|scheduler)/ },
            { name: 'semi-vendor', test: /node_modules[\\/]@douyinfe/ },
            { name: 'echarts-vendor', test: /node_modules[\\/](echarts|zrender)/ },
          ],
        },
      },
    },
  },
  resolve: {
    alias: {
      '@': fileURLToPath(new URL('./src', import.meta.url)),
    },
  },
  server: {
    port: 5173,
    host: '0.0.0.0',
    proxy: {
      '/api': {
        target: 'http://localhost:8080',
        changeOrigin: true,
        ws: true, // 支持 WebSocket（noVNC 需要）
        timeout: 0, // 大文件上传不超时（0=无限制）
        proxyTimeout: 0, // 代理到后端的连接不超时
        // 不 rewrite，后端路由就是 /api 前缀
      },
    },
  },
})
