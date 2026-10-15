import { defineConfig } from 'vite'
import react from '@vitejs/plugin-react'

// 开发服务器代理 /api → 本地后端（catcube serve）。
export default defineConfig({
  plugins: [react()],
  server: {
    port: 5173,
    proxy: {
      '/api': { target: 'http://127.0.0.1:8080', changeOrigin: true },
    },
  },
})
