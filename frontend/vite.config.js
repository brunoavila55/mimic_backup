import { defineConfig } from 'vite'
import react from '@vitejs/plugin-react'

export default defineConfig({
  plugins: [react()],
  server: {
    port: 5173,
    proxy: {
      '/api': { target: 'http://localhost:3000', changeOrigin: false },
      '/static': { target: 'http://localhost:3000', changeOrigin: false },
      '/nodes/import': { target: 'http://localhost:3000', changeOrigin: false },
      '/nodes/export': { target: 'http://localhost:3000', changeOrigin: false },
      '/settings': { target: 'http://localhost:3000', changeOrigin: false },
    },
  },
  build: { outDir: 'dist', emptyOutDir: true },
  test: { environment: 'jsdom', setupFiles: './src/test/setup.js' },
})
