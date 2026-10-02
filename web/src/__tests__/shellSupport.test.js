import { beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import { defineComponent, h } from 'vue'
import { render, screen, waitFor } from '@testing-library/vue'
import { createMemoryHistory, createRouter } from 'vue-router'
import { initTheme, useTheme } from '@/composables/useTheme'
import { useRouteIntent } from '@/composables/useRouteIntent'
import AboutDialog from '@/components/shell/AboutDialog.vue'
import { formatVersion, readSystemInfo } from '@/utils/systemInfo'

const mockGetSystemInfo = vi.hoisted(() => vi.fn())
vi.mock('@/api/admin', () => ({
  getSystemInfo: (...args) => mockGetSystemInfo(...args)
}))

describe('useTheme preference', () => {
  beforeEach(() => {
    vi.stubGlobal('matchMedia', vi.fn(query => ({ matches: query.includes('dark'), addEventListener: vi.fn(), removeEventListener: vi.fn() })))
  })

  it('follows the system until a theme is picked, and again after "system"', () => {
    initTheme()
    const { themePreference, currentTheme, setThemePreference } = useTheme()
    expect(themePreference.value).toBe('system')
    expect(currentTheme.value).toBe('dark')

    setThemePreference('light')
    expect(themePreference.value).toBe('light')
    expect(document.documentElement.getAttribute('data-theme')).toBe('light')
    expect(localStorage.getItem('v2board-theme')).toBe('light')

    setThemePreference('system')
    expect(themePreference.value).toBe('system')
    expect(localStorage.getItem('v2board-theme')).toBeNull()
    expect(currentTheme.value).toBe('dark')
  })
})

describe('useRouteIntent', () => {
  it('reads one-shot query values and removes them after mount', async () => {
    const router = createRouter({ history: createMemoryHistory(), routes: [{ path: '/:rest(.*)*', component: { template: '<div />' } }] })
    await router.push('/admin/users?email=lin%40example.test&create=1&page=2')
    await router.isReady()
    let intent = null
    const Probe = defineComponent({
      setup() {
        intent = useRouteIntent(['email', 'create'])
        return () => h('div')
      }
    })
    mount(Probe, { global: { plugins: [router] } })
    await flushPromises()
    expect(intent).toEqual({ email: 'lin@example.test', create: '1' })
    expect(router.currentRoute.value.fullPath).toBe('/admin/users?page=2')
  })

  it('passes later instructions for the open page to the callback, and leaves others alone', async () => {
    const router = createRouter({ history: createMemoryHistory(), routes: [{ path: '/:rest(.*)*', component: { template: '<div />' } }] })
    await router.push('/admin/users')
    await router.isReady()
    const later = vi.fn()
    let intent = null
    const Probe = defineComponent({
      setup() {
        intent = useRouteIntent(['email', 'create'], later)
        return () => h('div')
      }
    })
    mount(Probe, { global: { plugins: [router] } })
    expect(intent).toEqual({})
    await router.push('/admin/users?email=a%40b.test')
    await flushPromises()
    expect(later).toHaveBeenCalledWith({ email: 'a@b.test' })
    expect(router.currentRoute.value.fullPath).toBe('/admin/users')
    // Another page's instruction (the palette's 添加节点 while on users).
    await router.push('/admin/nodes?create=1')
    await flushPromises()
    expect(later).toHaveBeenCalledTimes(1)
    expect(router.currentRoute.value.fullPath).toBe('/admin/nodes?create=1')
  })

  it('is empty without a router or without the keys', () => {
    let intent = null
    const Probe = defineComponent({
      setup() {
        intent = useRouteIntent(['create'])
        return () => h('div')
      }
    })
    mount(Probe)
    expect(intent).toEqual({})
  })
})

describe('AboutDialog.vue (version moved out of the sidebar)', () => {
  beforeEach(() => {
    mockGetSystemInfo.mockReset()
  })

  it('reads the version and build when opened', async () => {
    mockGetSystemInfo.mockResolvedValue({ code: 0, msg: 'ok', data: { version: '4.1.0', build_code: '202610020001', build_time: '2026-10-02T00:00:00Z', commit: 'abc123' } })
    render(AboutDialog, { props: { open: true } })
    await screen.findByRole('dialog', { name: 'About AnixOps Control' })
    await waitFor(() => expect(screen.getByText('v4.1.0 #202610020001')).toBeTruthy())
    expect(screen.getByText('abc123')).toBeTruthy()
    expect(mockGetSystemInfo).toHaveBeenCalledTimes(1)
  })

  it('says so when the version cannot be read', async () => {
    mockGetSystemInfo.mockRejectedValue(new Error('offline'))
    render(AboutDialog, { props: { open: true } })
    expect(await screen.findByText('Version information is not available right now.')).toBeTruthy()
  })

  it('does not ask the backend while closed', () => {
    render(AboutDialog, { props: { open: false } })
    expect(mockGetSystemInfo).not.toHaveBeenCalled()
  })

  it('formats versions from both response shapes', () => {
    expect(readSystemInfo({ data: { version: '2.1.0' } })).toEqual({ version: '2.1.0' })
    expect(readSystemInfo({ code: 0, data: { version: '2.2.0' } })).toEqual({ version: '2.2.0' })
    expect(formatVersion('2.1.0', '202607090001')).toBe('v2.1.0 #202607090001')
    expect(formatVersion('2.1.0#7', 'x')).toBe('v2.1.0#7')
    expect(formatVersion('', 'x')).toBe('')
  })
})
