import { describe, it, expect, beforeEach, vi } from 'vitest'
import { mount, flushPromises } from '@vue/test-utils'
import Knowledge from '@/views/user/Knowledge.vue'

const mockGetKnowledgeList = vi.fn()

vi.mock('@/api/user', () => ({
  getKnowledgeList: (...args) => mockGetKnowledgeList(...args),
}))

describe('User Knowledge flow', () => {
  beforeEach(() => {
    mockGetKnowledgeList.mockReset()
  })

  it('loads articles and renders category tabs', async () => {
    mockGetKnowledgeList.mockResolvedValue({
      data: [
        {
          id: 1,
          title: 'Billing overview',
          body: 'Use this to track payments',
          category: 'Billing',
          updated_at: 1710000000,
        },
        {
          id: 2,
          title: 'Connectivity tips',
          body: 'Check Firewall and DNS',
          category: 'Networking',
          updated_at: 1710001000,
        },
      ],
    })

    const wrapper = mount(Knowledge)
    await flushPromises()

    expect(mockGetKnowledgeList).toHaveBeenCalledTimes(1)
    expect(wrapper.findAll('.article-card')).toHaveLength(2)

    const tabs = wrapper.findAll('.category-tabs .tab-item')
    expect(tabs.map(tab => tab.text())).toEqual(
      expect.arrayContaining(['全部', 'Billing', 'Networking'])
    )

    // Filter to Networking category, only its articles remain
    await tabs.find(tab => tab.text() === 'Networking').trigger('click')
    await flushPromises()

    const visibleCards = wrapper.findAll('.article-card')
    expect(visibleCards).toHaveLength(1)
    expect(visibleCards[0].find('.article-title').text()).toBe('Connectivity tips')
  })

  it('opens detail modal when clicking an article', async () => {
    mockGetKnowledgeList.mockResolvedValue({
      data: [
        {
          id: 3,
          title: 'Subscription security',
          body: 'Use strong passwords and MFA',
          category: 'Security',
          updated_at: 1710002000,
        },
      ],
    })

    const wrapper = mount(Knowledge)
    await flushPromises()

    expect(wrapper.findAll('.article-card')).toHaveLength(1)

    await wrapper.find('.article-card').trigger('click')
    await flushPromises()

    expect(wrapper.find('.article-detail-modal').exists()).toBe(true)
    expect(wrapper.find('.article-detail-modal h3').text()).toBe('Subscription security')
    expect(wrapper.find('.markdown-body').text()).toContain('Use strong passwords and MFA')
  })
})
