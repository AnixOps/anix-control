import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import { createMemoryHistory, createRouter } from 'vue-router'
import Monitor from '@/views/admin/Monitor.vue'

const adminApi = vi.hoisted(() => ({
  getTrafficHourly: vi.fn(),
  getUserTrafficRanking: vi.fn(),
  getTrafficDestinations: vi.fn(),
  getDestinationSettings: vi.fn(),
  updateDestinationSettings: vi.fn(),
  getTrafficCategories: vi.fn(),
  classifyTrafficDestination: vi.fn(),
  createTrafficCategoryRule: vi.fn(),
  updateTrafficCategoryRule: vi.fn(),
  deleteTrafficCategoryRule: vi.fn(),
  createTrafficCategorySource: vi.fn(),
  updateTrafficCategorySource: vi.fn(),
  deleteTrafficCategorySource: vi.fn(),
  refreshTrafficCategorySource: vi.fn()
}))

vi.mock('@/api/admin', () => adminApi)
vi.mock('@/api/kernel', () => ({ getKernelNodeTraffic: vi.fn() }))
vi.mock('@/ui/internal/echarts.js', () => ({ init: vi.fn(() => ({ setOption: vi.fn(), setTheme: vi.fn(), resize: vi.fn(), dispose: vi.fn() })), registerTheme: vi.fn() }))

const ANSWER = {
  categories: ['video', 'music', 'messaging', 'social', 'gaming', 'ai', 'download', 'web', 'unknown'],
  builtin: [{ category: 'video', domains: ['youtube.com'], networks: [], protocols: [] }],
  rules: [{ id: 1, kind: 'domain', value: 'music.youtube.com', category: 'music', note: '' }],
  sources: [{ id: 2, name: 'netflix', url: 'https://example.test/netflix', category: 'video', enabled: true, refresh_hours: 24, entry_count: 12, fetched_at: '2026-11-01T00:00:00Z', fetch_error: '' }]
}

async function mountSection(path = '/admin/monitor/categories') {
  const router = createRouter({ history: createMemoryHistory(), routes: [{ path: '/admin/monitor/:section?', component: Monitor }] })
  await router.push(path)
  await router.isReady()
  const wrapper = mount(Monitor, { attachTo: document.body, global: { plugins: [router] } })
  await flushPromises()
  return { wrapper, router }
}

beforeEach(() => {
  vi.resetAllMocks()
  vi.spyOn(console, 'error').mockImplementation(() => {})
  adminApi.getTrafficCategories.mockResolvedValue(ANSWER)
})

afterEach(() => {
  vi.restoreAllMocks()
  document.body.innerHTML = ''
})

describe('流量与监控 · 流量分类', () => {
  it('lists the custom rules, the remote sources and the built-in rules', async () => {
    const { wrapper } = await mountSection()
    expect(wrapper.find('[data-monitor-categories]').exists()).toBe(true)
    expect(adminApi.getTrafficCategories).toHaveBeenCalledTimes(1)
    expect(wrapper.get('[data-category-rules]').text()).toContain('music.youtube.com')
    expect(wrapper.get('[data-category-sources]').text()).toContain('netflix')
    expect(wrapper.get('[data-category-builtin]').text()).toContain('1 domains')
  })

  it('classifies a destination by the current rules', async () => {
    adminApi.classifyTrafficDestination.mockResolvedValue({ domain: 'music.youtube.com', protocol: '', category: 'music' })
    const { wrapper } = await mountSection()
    await wrapper.get('input[data-category-test-domain]').setValue('music.youtube.com')
    await wrapper.get('[data-category-test] form').trigger('submit')
    await flushPromises()
    expect(adminApi.classifyTrafficDestination).toHaveBeenCalledWith('music.youtube.com', '')
    expect(wrapper.get('[data-category-test-result]').text()).toContain('Music')
  })

  it('adds a rule and reloads the list', async () => {
    adminApi.createTrafficCategoryRule.mockResolvedValue({ rule: { id: 3 } })
    const { wrapper } = await mountSection()
    await wrapper.get('[data-category-rule-add]').trigger('click')
    await flushPromises()
    const value = document.querySelector('input[data-category-rule-value]')
    value.value = 'example.com'
    value.dispatchEvent(new Event('input'))
    await flushPromises()
    document.querySelector('#category-rule-form').dispatchEvent(new Event('submit'))
    await flushPromises()
    expect(adminApi.createTrafficCategoryRule).toHaveBeenCalledWith({ kind: 'domain', value: 'example.com', category: 'video', note: '' })
    expect(adminApi.getTrafficCategories).toHaveBeenCalledTimes(2)
  })
})
