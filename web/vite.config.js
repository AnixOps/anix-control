import { defineConfig } from 'vite'
import vue from '@vitejs/plugin-vue'
import path from 'path'

function manualChunks(id) {
  if (id.includes('/src/locales/en.js')) {
    return 'locale-en'
  }

  if (id.includes('/src/locales/zh-CN.js')) {
    return 'locale-zh-CN'
  }

  if (id.includes('/src/api/')) {
    return 'api'
  }

  if (!id.includes('node_modules')) {
    return undefined
  }

  if (id.includes('vue-i18n')) {
    return 'i18n'
  }

  if (id.includes('vue-router')) {
    return 'router'
  }

  if (id.includes('pinia') || id.includes('/vue/')) {
    return 'vue-vendor'
  }

  if (id.includes('axios')) {
    return 'network'
  }

  // Heavy visualization libs are dynamically imported only by the Observability
  // route; keep them out of the main vendor chunk so they load on demand.
  if (id.includes('echarts') || id.includes('zrender')) {
    return 'echarts'
  }

  if (id.includes('@antv')) {
    return 'g6'
  }

  return 'vendor'
}

export default defineConfig({
  plugins: [vue()],
  publicDir: false,
  resolve: {
    alias: {
      '@': path.resolve(__dirname, './src')
    }
  },
  server: {
    port: 5173,
    proxy: {
      '/api': {
        target: 'http://localhost:8080',
        changeOrigin: true
      }
    }
  },
  build: {
    outDir: './public',
    emptyOutDir: true,
    rollupOptions: {
      output: {
        manualChunks
      }
    }
  }
})
