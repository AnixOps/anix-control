// Measuring logic of the performance budget gate (scripts/check-bundle-budget.mjs).
import { describe, expect, it } from 'vitest'
import {
  KB,
  chunkForModule,
  evaluateBudget,
  lazyChunks,
  routeFiles,
  staticClosure,
  sumSizes
} from '../../scripts/bundle-budget-lib.mjs'

// entry -> vue (static), Login and Admin (dynamic); Admin -> shared (static)
// and the admin messages (dynamic, awaited by the router).
const graph = {
  chunks: {
    'assets/index.js': { isEntry: true, module: 'src/main.js', modules: ['src/main.js'], imports: ['assets/vue.js'], dynamicImports: ['assets/Login.js', 'assets/Admin.js', 'assets/zh.js'], css: ['assets/index.css'] },
    'assets/vue.js': { name: 'vue-vendor', module: null, modules: [], imports: [], dynamicImports: [], css: [] },
    'assets/axios.js': { name: 'axios', module: null, modules: [], imports: [], dynamicImports: [], css: [] },
    'assets/zh.js': { isDynamicEntry: true, module: 'src/locales/zh-CN.js', modules: ['src/locales/zh-CN.js'], imports: [], dynamicImports: [], css: [] },
    'assets/Login.js': { isDynamicEntry: true, module: 'src/views/Login.vue', modules: ['src/views/Login.vue'], imports: ['assets/vue.js', 'assets/index.js'], dynamicImports: ['assets/axios.js'], css: ['assets/Login.css'] },
    'assets/Admin.js': { isDynamicEntry: true, module: 'src/views/Admin.vue', modules: ['src/views/Admin.vue'], imports: ['assets/shared.js'], dynamicImports: [], css: [] },
    'assets/shared.js': { module: null, modules: ['src/ui/UiCard.vue', 'src/api/admin.js'], imports: ['assets/vue.js'], dynamicImports: [], css: ['assets/shared.css'] }
  }
}

const sizes = {
  'assets/index.js': 10 * KB,
  'assets/index.css': 2 * KB,
  'assets/vue.js': 40 * KB,
  'assets/axios.js': 18 * KB,
  'assets/zh.js': 8 * KB,
  'assets/Login.js': 4 * KB,
  'assets/Login.css': 1 * KB,
  'assets/Admin.js': 6 * KB,
  'assets/shared.js': 5 * KB,
  'assets/shared.css': 1 * KB
}

describe('bundle budget measuring', () => {
  it('follows static imports and CSS, not dynamic imports', () => {
    expect([...staticClosure(graph, ['assets/Login.js'])].sort()).toEqual([
      'assets/Login.css', 'assets/Login.js', 'assets/index.css', 'assets/index.js', 'assets/vue.js'
    ])
  })

  it('finds a module by facade, by bundled module, or a vendor chunk by name', () => {
    expect(chunkForModule(graph, 'src/views/Login.vue')).toBe('assets/Login.js')
    expect(chunkForModule(graph, 'src/api/admin.js')).toBe('assets/shared.js')
    expect(chunkForModule(graph, 'chunk:axios')).toBe('assets/axios.js')
    expect(() => chunkForModule(graph, 'src/missing.js')).toThrow(/no chunk/)
  })

  it('adds the entry, the boot modules and the route chunks', () => {
    const files = routeFiles(graph, { boot: ['src/locales/zh-CN.js'], modules: ['src/views/Admin.vue'] })
    expect(sumSizes(files, sizes)).toBe((10 + 2 + 40 + 8 + 6 + 5 + 1) * KB)
    expect(files.has('assets/Login.js')).toBe(false)
  })

  it('ranks lazy chunks with their CSS and skips excluded vendors', () => {
    const chunks = lazyChunks(graph, sizes, { exclude: ['^assets/vue'] })
    expect(chunks[0]).toEqual({ file: 'assets/axios.js', module: null, gzip: 18 * KB })
    expect(chunks.find((c) => c.file === 'assets/shared.js').gzip).toBe(6 * KB)
    expect(chunks.some((c) => c.file === 'assets/vue.js' || c.file === 'assets/index.js')).toBe(false)
  })

  it('passes within budget and reports what is over', () => {
    const budget = {
      boot: ['src/locales/zh-CN.js'],
      routes: { login: { modules: ['src/views/Login.vue'], after: ['chunk:axios'], maxGzipKB: 65, maxTotalGzipKB: 90 } },
      chunks: { maxGzipKB: 20, exclude: ['^assets/vue'] }
    }
    const ok = evaluateBudget(graph, sizes, budget)
    expect(ok.failures).toEqual([])
    expect(ok.routes[0]).toMatchObject({ name: 'login', initial: 65 * KB, total: 83 * KB, deferred: ['assets/axios.js'] })

    budget.routes.login.maxGzipKB = 64
    budget.routes.login.maxTotalGzipKB = 80
    budget.chunks.maxGzipKB = 10
    const over = evaluateBudget(graph, sizes, budget)
    expect(over.failures).toEqual([
      'route login: initial 65.0 KB gzip > 64 KB',
      'route login: total 83.0 KB gzip > 80 KB',
      'chunk assets/axios.js: 18.0 KB gzip > 10 KB'
    ])
  })
})
