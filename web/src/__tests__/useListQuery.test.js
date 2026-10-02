import { afterEach, describe, expect, it, vi } from 'vitest'
import { defineComponent, h } from 'vue'
import { flushPromises, mount } from '@vue/test-utils'
import { createMemoryHistory, createRouter } from 'vue-router'
import { useListQuery } from '@/composables/useListQuery'

const Blank = { render: () => null }

async function setup(url, options) {
  const router = createRouter({
    history: createMemoryHistory(),
    routes: [{ path: '/admin/list', component: Blank }, { path: '/admin/other', component: Blank }]
  })
  await router.push(url)
  let api
  const Probe = defineComponent({ setup() { api = useListQuery(options); return () => h('div') } })
  const wrapper = mount(Probe, { global: { plugins: [router] } })
  return { router, api, wrapper }
}

let wrapper
afterEach(() => wrapper?.unmount())

describe('useListQuery', () => {
  it('reads strings, allowed values and the page from the query', async () => {
    const ctx = await setup('/admin/list?q=hk&status=bogus&type=2&page=3')
    wrapper = ctx.wrapper
    expect(ctx.api.read('q')).toBe('hk')
    expect(ctx.api.read('status', { values: ['open', 'closed'] })).toBe('')
    expect(ctx.api.read('type', { values: ['1', '2'] })).toBe('2')
    expect(ctx.api.read('missing')).toBe('')
    expect(ctx.api.readPage()).toBe(3)
  })

  it('reads a bad or first page as 1', async () => {
    const ctx = await setup('/admin/list?page=abc')
    wrapper = ctx.wrapper
    expect(ctx.api.readPage()).toBe(1)
  })

  it('replaces the entry, drops defaults and keeps other keys', async () => {
    const ctx = await setup('/admin/list?keep=1&page=4')
    wrapper = ctx.wrapper
    const push = vi.spyOn(ctx.router, 'push')
    const replace = vi.spyOn(ctx.router, 'replace')
    ctx.api.write({ q: 'tokyo', status: '', page: 1 })
    await flushPromises()
    expect(ctx.router.currentRoute.value.query).toEqual({ keep: '1', q: 'tokyo' })
    ctx.api.write({ q: 'tokyo', status: 'open', page: 2 })
    await flushPromises()
    expect(ctx.router.currentRoute.value.query).toEqual({ keep: '1', q: 'tokyo', status: 'open', page: '2' })
    expect(replace).toHaveBeenCalledTimes(2)
    expect(push).not.toHaveBeenCalled()
    // Writing the same state again does not navigate.
    ctx.api.write({ q: 'tokyo', status: 'open', page: 2 })
    expect(replace).toHaveBeenCalledTimes(2)
  })

  it('drops consumed one-shot keys', async () => {
    const ctx = await setup('/admin/list?email=a%40b.c&create=1', { omit: ['email', 'create'] })
    wrapper = ctx.wrapper
    ctx.api.write({ q: 'a@b.c' })
    await flushPromises()
    expect(ctx.router.currentRoute.value.query).toEqual({ q: 'a@b.c' })
  })

  it('does not rewrite another page after the list was left', async () => {
    const ctx = await setup('/admin/list')
    wrapper = ctx.wrapper
    await ctx.router.push('/admin/other?x=1')
    ctx.api.write({ q: 'late' })
    await flushPromises()
    expect(ctx.router.currentRoute.value.fullPath).toBe('/admin/other?x=1')
  })

  it('reads empty values and writes nothing without a router', () => {
    let api
    wrapper = mount(defineComponent({ setup() { api = useListQuery(); return () => h('div') } }))
    expect(api.read('q')).toBe('')
    expect(api.readPage()).toBe(1)
    expect(() => api.write({ q: 'x' })).not.toThrow()
  })
})
