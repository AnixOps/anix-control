import { describe, expect, it } from 'vitest'
import { chunkGroupFor } from '../../vite.config'

describe('vite code-splitting groups', () => {
  it('keeps heavy visualization dependencies in dedicated chunks', () => {
    expect(chunkGroupFor('/repo/web/node_modules/echarts/lib/echarts.js')).toBe('echarts')
    expect(chunkGroupFor('/repo/web/node_modules/zrender/lib/core/util.js')).toBe('echarts')
    expect(chunkGroupFor('/repo/web/node_modules/@antv/g6/esm/index.js')).toBe('g6')
  })

  it('keeps the QR encoder in its own on-demand chunk', () => {
    expect(chunkGroupFor('/repo/web/node_modules/uqr/dist/index.mjs')).toBe('qr')
  })

  it('keeps framework chunks stable for browser caching', () => {
    expect(chunkGroupFor('/repo/web/node_modules/vue/dist/vue.runtime.esm-bundler.js')).toBe('vue-vendor')
    expect(chunkGroupFor('/repo/web/node_modules/@vue/runtime-core/dist/runtime-core.esm-bundler.js')).toBe('vue-vendor')
    expect(chunkGroupFor('/repo/web/node_modules/pinia/dist/pinia.mjs')).toBe('vue-vendor')
    expect(chunkGroupFor('/repo/web/node_modules/vue-router/dist/vue-router.mjs')).toBe('router')
    expect(chunkGroupFor('/repo/web/node_modules/vue-i18n/dist/vue-i18n.mjs')).toBe('i18n')
    expect(chunkGroupFor('/repo/web/node_modules/axios/index.js')).toBe('axios')
  })

  it('leaves app code, locales and the component library to automatic splitting', () => {
    // API modules and message groups go with the routes that use them, and
    // Reka UI primitives are shared only by the routes that render them.
    expect(chunkGroupFor('/repo/web/src/api/admin.js')).toBeUndefined()
    expect(chunkGroupFor('/repo/web/src/locales/zh-CN.js')).toBeUndefined()
    expect(chunkGroupFor('/repo/web/src/views/admin/Forward.vue')).toBeUndefined()
    expect(chunkGroupFor('/repo/web/node_modules/reka-ui/dist/Dialog/DialogRoot.js')).toBeUndefined()
  })
})
