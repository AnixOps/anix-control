import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import { createMemoryHistory, createRouter } from 'vue-router'
import Monitor from '@/views/admin/Monitor.vue'

const adminApi = vi.hoisted(() => ({
  getTrafficHourly: vi.fn(),
  getUserTrafficRanking: vi.fn(),
  getTrafficDestinations: vi.fn(),
  getDestinationSettings: vi.fn(),
  updateDestinationSettings: vi.fn()
}))

vi.mock('@/api/admin', () => adminApi)
vi.mock('@/api/kernel', () => ({ getKernelNodeTraffic: vi.fn() }))
vi.mock('@/ui/internal/echarts.js', () => ({ init: vi.fn(() => ({ setOption: vi.fn(), setTheme: vi.fn(), resize: vi.fn(), dispose: vi.fn() })), registerTheme: vi.fn() }))

const SUMMARY = {
  upload: 10,
  download: 990,
  connections: 3,
  categories: [{ category: 'video', upload: 10, download: 990, connections: 3 }],
  domains: [{ domain: 'youtube.com', category: 'video', upload: 10, download: 990, connections: 3 }, { domain: '', category: 'unknown', upload: 0, download: 1, connections: 0 }],
  domain_count: 2,
  settings: { enabled: false, retention_days: 30 }
}

async function mountSection(path = '/admin/monitor/destinations') {
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
  adminApi.getUserTrafficRanking.mockResolvedValue({ data: [{ user_id: 7, email: 'seven@example.test', traffic: 5 }] })
  adminApi.getTrafficDestinations.mockResolvedValue(SUMMARY)
  adminApi.getDestinationSettings.mockResolvedValue({ enabled: false, retention_days: 30 })
})

afterEach(() => {
  vi.restoreAllMocks()
  document.body.innerHTML = ''
})

describe('流量与监控 · 访问统计', () => {
  it('loads the summary of all users and says the statistics are off', async () => {
    const { wrapper } = await mountSection()
    expect(wrapper.find('[data-monitor-destinations]').exists()).toBe(true)
    expect(adminApi.getTrafficDestinations).toHaveBeenCalledWith({ hours: 24, userId: 0, limit: 200 })
    expect(wrapper.find('[data-destination-off]').exists()).toBe(true)
    const categories = wrapper.get('[data-destination-categories]').text()
    expect(categories).toContain('Video')
    expect(categories).toContain('100.0%')
    expect(wrapper.text()).toContain('youtube.com')
    expect(wrapper.text()).toContain('Other (beyond the per-report limit)')
  })

  it('saves the switch and the retention', async () => {
    adminApi.updateDestinationSettings.mockResolvedValue({ enabled: true, retention_days: 60 })
    const { wrapper } = await mountSection()
    const save = wrapper.get('[data-destination-save]')
    expect(save.attributes('disabled')).toBeDefined()
    await wrapper.get('[data-destination-enabled]').trigger('click')
    const input = wrapper.get('input[data-destination-retention]')
    await input.setValue('60')
    await input.trigger('blur')
    await flushPromises()
    await wrapper.get('form').trigger('submit')
    await flushPromises()
    expect(adminApi.updateDestinationSettings).toHaveBeenCalledWith({ enabled: true, retention_days: 60 })
    expect(wrapper.find('[data-destination-off]').exists()).toBe(false)
  })
})
