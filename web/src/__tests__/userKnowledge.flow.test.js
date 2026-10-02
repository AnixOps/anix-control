import { describe, it, expect, beforeEach, afterEach, vi } from 'vitest'
import { mount, flushPromises, enableAutoUnmount } from '@vue/test-utils'
import Knowledge from '@/views/user/Knowledge.vue'
import { inBody } from './helpers/feedback'

const mockGetKnowledgeList = vi.fn()

vi.mock('@/api/user', () => ({
  getKnowledgeList: (...args) => mockGetKnowledgeList(...args)
}))

enableAutoUnmount(afterEach)

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
          updated_at: 1710000000
        },
        {
          id: 2,
          title: 'Connectivity tips',
          body: 'Check Firewall and DNS',
          category: 'Networking',
          updated_at: 1710001000
        }
      ]
    })

    const wrapper = mount(Knowledge, { attachTo: document.body })
    await flushPromises()

    expect(mockGetKnowledgeList).toHaveBeenCalledTimes(1)
    expect(wrapper.findAll('.article-card')).toHaveLength(2)

    const tabs = wrapper.findAll('.category-tabs .tab-item')
    expect(tabs.map((tab) => tab.text())).toEqual(
      expect.arrayContaining(['All', 'Billing', 'Networking'])
    )

    await tabs.find((tab) => tab.text() === 'Networking').trigger('click')
    await flushPromises()

    const visibleCards = wrapper.findAll('.article-card')
    expect(visibleCards).toHaveLength(1)
    expect(visibleCards[0].find('.article-title').text()).toBe('Connectivity tips')
  })

  it('opens detail modal when clicking an article', async () => {
    mockGetKnowledgeList.mockResolvedValue({
      code: 0,
      msg: '操作成功',
      ts: 1783536000000,
      data: [
        {
          id: 3,
          title: 'Subscription security',
          body: 'Use strong passwords and MFA',
          category: 'Security',
          updated_at: 1710002000
        }
      ]
    })

    const wrapper = mount(Knowledge, { attachTo: document.body })
    await flushPromises()

    expect(wrapper.findAll('.article-card')).toHaveLength(1)

    await wrapper.find('.article-card').trigger('click')
    await flushPromises()

    const dialog = inBody('[role="dialog"]')
    expect(dialog.classes()).toContain('article-detail-dialog')
    expect(inBody('.article-detail-dialog h2').text()).toBe('Subscription security')
    expect(inBody('.markdown-body').text()).toContain('Use strong passwords and MFA')

    // Esc closes the article.
    await dialog.trigger('keydown', { key: 'Escape' })
    await flushPromises()
    expect(inBody('[role="dialog"]').exists()).toBe(false)
  })
})
