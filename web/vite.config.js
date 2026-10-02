import { defineConfig } from 'vite'
import vue from '@vitejs/plugin-vue'
import path from 'path'
import brandIcons from './scripts/vite-brand-icons.mjs'
import chunkGraph from './scripts/vite-chunk-graph.mjs'

function pad(value) {
  return String(value).padStart(2, '0')
}

function formatBuildCode(date) {
  return [
    String(date.getFullYear()).slice(-2),
    pad(date.getMonth() + 1),
    pad(date.getDate()),
    pad(date.getHours()),
    pad(date.getMinutes())
  ].join('')
}

const appBuildDate = new Date()
const appBuildCode = process.env.VITE_APP_BUILD_CODE || formatBuildCode(appBuildDate)
const appBuildTime = process.env.VITE_APP_BUILD_TIME || appBuildDate.toISOString()

// Vendor chunks (rolldown code-splitting groups). App code is left to the
// automatic splitting, so each route chunk carries the API modules and
// messages it uses and the sign-in page loads none of the admin code. A group
// also takes the dependencies of what it captures, so higher priorities run
// first: Vue before the libraries built on it.
const nodeModule = (pattern) => new RegExp(`[\\\\/]node_modules[\\\\/](${pattern})[\\\\/]`)

export const chunkGroups = [
  // Vue itself is split over @vue/* packages (runtime-core, reactivity, ...).
  { name: 'vue-vendor', test: nodeModule('vue|@vue|pinia|vue-demi'), priority: 60 },
  { name: 'router', test: nodeModule('vue-router'), priority: 50 },
  { name: 'i18n', test: nodeModule('vue-i18n|@intlify'), priority: 50 },
  { name: 'axios', test: nodeModule('axios'), priority: 50 },
  // Heavy visualization libraries load only with the pages that draw charts
  // and topology graphs.
  { name: 'echarts', test: nodeModule('echarts|zrender'), priority: 40 },
  { name: 'g6', test: nodeModule('@antv'), priority: 40 },
  // The QR encoder loads only when a page draws a code (UiQrCode imports it
  // on first use).
  { name: 'qr', test: nodeModule('uqr'), priority: 40 }
  // Reka UI and its helpers have no group on purpose: automatic splitting
  // shares each primitive only between the routes that render it, so the
  // sign-in page does not load the dialogs, menus and tables of the admin
  // console (one ui-vendor chunk cost it 43 KB gzip).
]

// The group a module lands in (for tests), or undefined for automatic chunks.
export function chunkGroupFor(id) {
  return [...chunkGroups].sort((a, b) => b.priority - a.priority).find((group) => group.test.test(id))?.name
}

export default defineConfig({
  plugins: [
    vue(),
    brandIcons({
      designDir: path.resolve(__dirname, './src/design'),
      name: 'AnixOps Control',
      shortName: 'AnixOps',
      description: 'AnixOps Control manages subscriptions, payments, nodes, forwarding topology, and agent runtime operations.'
    }),
    // Read by `npm run bundle:budget` (scripts/check-bundle-budget.mjs).
    chunkGraph({
      root: __dirname,
      outFile: path.resolve(__dirname, process.env.BUNDLE_GRAPH || './bundle-reports/chunk-graph.json')
    })
  ],
  publicDir: false,
  define: {
    'import.meta.env.VITE_APP_BUILD_CODE': JSON.stringify(appBuildCode),
    'import.meta.env.VITE_APP_BUILD_TIME': JSON.stringify(appBuildTime),
    // vue-i18n runs in Composition API mode only (src/i18n.js, legacy: false):
    // drop the legacy VueI18n API from the bundle. <i18n-t> stays installed.
    __VUE_I18N_LEGACY_API__: 'false',
    __VUE_I18N_FULL_INSTALL__: 'true',
    __INTLIFY_PROD_DEVTOOLS__: 'false'
  },
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
    rolldownOptions: {
      output: {
        codeSplitting: { groups: chunkGroups }
      }
    }
  }
})
