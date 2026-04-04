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
})
