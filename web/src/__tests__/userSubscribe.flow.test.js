import { describe, it, expect, beforeEach, afterEach, vi } from 'vitest'
import { mount, flushPromises } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import { useUserStore } from '@/stores/user'
import Subscribe from '@/views/user/Subscribe.vue'

const mockGetSubscription = vi.fn()
const mockFetch = vi.fn()
const mockAlert = vi.fn()
const mockWriteText = vi.fn()

vi.mock('@/api/user', () => ({
  getSubscription: (...args) => mockGetSubscription(...args),
}))

describe('User Subscribe flow', () => {
  beforeEach(() => {
    localStorage.clear()
    setActivePinia(createPinia())
    const userStore = useUserStore()
    userStore.userInfo = { token: 'sub-token-1' }

    mockGetSubscription.mockReset()
    mockFetch.mockReset()
    mockAlert.mockReset()
    mockWriteText.mockReset()
    mockWriteText.mockResolvedValue()

    vi.stubGlobal('fetch', mockFetch)
    vi.stubGlobal('alert', mockAlert)
    Object.defineProperty(navigator, 'clipboard', {
      configurable: true,
      value: {
        writeText: mockWriteText,
      },
    })
  })

  afterEach(() => {
    vi.unstubAllGlobals()
  })

  it('loads subscription and refreshes cache', async () => {
    mockGetSubscription
      .mockResolvedValueOnce({
        data: {
          ExpireAt: 0,
          UsedTraffic: 1024,
          TotalTraffic: 2048,
        },
      })
      .mockResolvedValueOnce({
        code: 0,
        msg: '操作成功',
        ts: 1783536000000,
        data: {
          ExpireAt: 0,
          UsedTraffic: 2048,
          TotalTraffic: 4096,
        },
      })

    const wrapper = mount(Subscribe)
    await flushPromises()

    expect(mockGetSubscription).toHaveBeenNthCalledWith(1, false)

    await wrapper.find('.link-actions .btn.btn-primary').trigger('click')
    await flushPromises()

    expect(mockGetSubscription).toHaveBeenNthCalledWith(2, true)
    expect(mockAlert).toHaveBeenCalled()
  })

  it('previews subscription content and supports copy link', async () => {
    mockGetSubscription.mockResolvedValue({
      data: {
        ExpireAt: 0,
        UsedTraffic: 1024,
        TotalTraffic: 4096,
        subscribe_domains: ['sub-a.example.com', 'sub-b.example.com'],
        subscribe_path: '/s',
      },
    })
    mockFetch.mockResolvedValue({
      ok: true,
      text: async () => 'preview-subscription-content',
    })

    const wrapper = mount(Subscribe)
    await flushPromises()

    const firstRowButtons = wrapper.find('.link-item .link-row').findAll('button')
    await firstRowButtons[0].trigger('click')
    await flushPromises()
    expect(mockWriteText).toHaveBeenCalledWith(
      expect.stringContaining('/s/sub-token-1')
    )

    await firstRowButtons[1].trigger('click')
    await flushPromises()

    expect(mockFetch).toHaveBeenCalledWith(expect.stringContaining('/s/sub-token-1'))
    expect(wrapper.find('.preview-card textarea').element.value).toContain('preview-subscription-content')
  })

  it('switches the displayed subscription domain when multiple domains are configured', async () => {
    mockGetSubscription.mockResolvedValue({
      data: {
        ExpireAt: 0,
        UsedTraffic: 1024,
        TotalTraffic: 4096,
        subscribe_domains: ['sub-a.example.com', 'sub-b.example.com'],
        subscribe_path: '/s',
      },
    })

    const wrapper = mount(Subscribe)
    await flushPromises()

    const domainSelect = wrapper.find('#subscribe-domain')
    expect(domainSelect.exists()).toBe(true)

    await domainSelect.setValue('sub-b.example.com')
    const linkInput = wrapper.find('.link-item .link-row input')
    expect(linkInput.element.value).toContain('sub-b.example.com')
  })

  it('offers a native WireGuard profile and downloads its preview as a conf file', async () => {
    mockGetSubscription.mockResolvedValue({
      data: {
        ExpireAt: 0,
        UsedTraffic: 1024,
        TotalTraffic: 4096,
        subscribe_path: '/s',
      },
    })
    mockFetch.mockResolvedValue({
      ok: true,
      text: async () => '[Interface]\nPrivateKey = test',
    })
    const createObjectURL = vi.fn(() => 'blob:wireguard-preview')
    const revokeObjectURL = vi.fn()
    const click = vi.spyOn(HTMLAnchorElement.prototype, 'click').mockImplementation(() => {})
    vi.stubGlobal('URL', { createObjectURL, revokeObjectURL })

    const wrapper = mount(Subscribe)
    await flushPromises()

    const wireGuardRow = wrapper.findAll('.link-item').find((row) => row.text().includes('WireGuard'))
    expect(wireGuardRow).toBeDefined()
    expect(wireGuardRow.find('input').element.value).toContain('?type=wireguard')

    await wireGuardRow.findAll('button')[1].trigger('click')
    await flushPromises()
    expect(mockFetch).toHaveBeenCalledWith(expect.stringContaining('?type=wireguard'))

    await wrapper.findAll('.preview-controls .btn')[1].trigger('click')
    expect(createObjectURL).toHaveBeenCalled()
    expect(click).toHaveBeenCalled()
    expect(click.mock.instances[0].download).toBe('subscription.conf')
    click.mockRestore()
  })
})
