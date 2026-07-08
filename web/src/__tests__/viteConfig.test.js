import { describe, expect, it } from 'vitest'
import { manualChunks } from '../../vite.config'

describe('vite manual chunking', () => {
  it('keeps heavy visualization dependencies in dedicated chunks', () => {
    expect(manualChunks('/repo/web/node_modules/echarts/lib/echarts.js')).toBe('echarts')
    expect(manualChunks('/repo/web/node_modules/zrender/lib/core/util.js')).toBe('echarts')
    expect(manualChunks('/repo/web/node_modules/@antv/g6/esm/index.js')).toBe('g6')
  })

  it('keeps app infrastructure chunks stable for browser caching', () => {
    expect(manualChunks('/repo/web/src/api/admin.js')).toBe('api')
    expect(manualChunks('/repo/web/node_modules/vue-router/dist/vue-router.mjs')).toBe('router')
    expect(manualChunks('/repo/web/node_modules/vue-i18n/dist/vue-i18n.mjs')).toBe('i18n')
    expect(manualChunks('/repo/web/node_modules/axios/index.js')).toBe('network')
    expect(manualChunks('/repo/web/src/locales/zh-CN.js')).toBe('locale-zh-CN')
    expect(manualChunks('/repo/web/src/views/admin/Forward.vue')).toBeUndefined()
  })
})
