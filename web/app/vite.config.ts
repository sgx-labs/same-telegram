import { defineConfig } from 'vite'
import react from '@vitejs/plugin-react'

export default defineConfig({
  plugins: [react()],
  build: {
    outDir: 'dist',
    // Single-page app — inline small assets for fewer requests
    assetsInlineLimit: 8192,
    // Target modern browsers (Telegram WebView is WKWebView/Chrome)
    target: 'es2020',
  },
  css: {
    modules: {
      // Generate shorter class names in production
      generateScopedName: '[hash:base64:6]',
    },
  },
  server: {
    // Proxy WebSocket to workspace server during development
    proxy: {
      '/ws': {
        target: 'ws://localhost:8080',
        ws: true,
      },
      '/health': {
        target: 'http://localhost:8080',
      },
    },
  },
})
