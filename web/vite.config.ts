import { defineConfig } from 'vite'
import react from '@vitejs/plugin-react'
import tailwindcss from '@tailwindcss/vite'

// 开发：vite dev server (5173) 代理 /api 到 `story serve` (7878)
// 发布：vite build 产物直接输出到 dist/，由 Go go:embed 打包
export default defineConfig({
  plugins: [react(), tailwindcss()],
  server: {
    proxy: {
      '/api': 'http://127.0.0.1:7878',
    },
  },
  build: {
    outDir: 'dist',
    emptyOutDir: true,
  },
})
