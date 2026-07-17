import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import Knowledge from '@/views/admin/Knowledge.vue'
import { setLocale } from '@/i18n'

const adminApi = vi.hoisted(() => ({
  createKnowledge: vi.fn(),
  deleteKnowledge: vi.fn(),
  getKnowledgeList: vi.fn(),
  updateKnowledge: vi.fn()
}))

vi.mock('@/api/admin', () => ({
  default: adminApi,
  ...adminApi
}))

describe('Admin Knowledge', () => {
  beforeEach(async () => {
    vi.resetAllMocks()
    vi.spyOn(console, 'error').mockImplementation(() => {})
    await setLocale('en')
  })

  afterEach(() => {
    vi.restoreAllMocks()
  })

  it('renders articles from legacy and panel envelope payloads', async () => {
    adminApi.getKnowledgeList
      .mockResolvedValueOnce({
        data: [
          {
            id: 1,
            category: '公告',
            title: 'Legacy Notice',
            body: 'Legacy article body',
            sort: 1,
            show: 1,
            updated_at: 1783526400
          }
        ]
      })
      .mockResolvedValueOnce({
        code: 0,
        msg: '操作成功',
        data: [
          {
            id: 2,
            category: 'tutorial',
            title: 'Panel Guide',
            body: 'Panel article body',
            sort: 2,
            show: 0,
            updated_at: 1786118400
          }
        ],
        ts: 1783526400000
      })

    const wrapper = mount(Knowledge)
    await flushPromises()

    expect(wrapper.text()).toContain('Legacy Notice')
    expect(wrapper.text()).toContain('Legacy article body')
    expect(wrapper.text()).toContain('Announcement')
    expect(wrapper.text()).toContain('Visible')

    await wrapper.vm.load()
    await flushPromises()

    expect(wrapper.text()).toContain('Panel Guide')
    expect(wrapper.text()).toContain('Panel article body')
    expect(wrapper.text()).toContain('Tutorial')
    expect(wrapper.text()).toContain('Hidden')
    expect(wrapper.text()).not.toContain('Legacy Notice')

    wrapper.unmount()
  })
})
