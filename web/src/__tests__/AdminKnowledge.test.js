import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import Knowledge from '@/views/admin/Knowledge.vue'
import { setLocale } from '@/i18n'
import { render, screen, waitFor, within } from '@testing-library/vue'
import userEvent from '@testing-library/user-event'
import UiHost from '@/ui/UiHost.vue'
import { toastMessages, toasts } from './helpers/feedback'

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

    expect(wrapper.find('table').exists()).toBe(true)
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

  describe('dialogs and feedback', () => {
    const Harness = { components: { Knowledge, UiHost }, template: '<div><Knowledge /><UiHost /></div>' }
    const article = { id: 5, category: 'faq', title: 'Reset password', body: 'Steps', sort: 1, show: 1, updated_at: 1783526400 }

    async function renderPage(list = [article]) {
      adminApi.getKnowledgeList.mockResolvedValue({ data: list })
      render(Harness)
      await screen.findByText(list[0]?.title || 'No articles yet')
    }

    async function rowAction(user, name, title = 'Reset password') {
      const trigger = screen.getByRole('button', { name: `Actions for ${title}` })
      await user.click(trigger)
      await user.click(within(await screen.findByRole('menu')).getByRole('menuitem', { name }))
      return trigger
    }

    it('validates the article dialog inline and publishes with a toast', async () => {
      const user = userEvent.setup()
      adminApi.createKnowledge.mockResolvedValue({ code: 0 })
      await renderPage()
      const opener = screen.getAllByRole('button', { name: 'New article' })[0]
      await user.click(opener)
      const dialog = await screen.findByRole('dialog', { name: 'New article' })
      await user.click(within(dialog).getByRole('button', { name: 'Publish' }))
      expect(within(dialog).getByRole('alert').textContent).toBe('Add a title and the article text')
      expect(adminApi.createKnowledge).not.toHaveBeenCalled()

      await user.keyboard('{Escape}')
      await waitFor(() => expect(screen.queryByRole('dialog')).toBeNull())
      await waitFor(() => expect(document.activeElement).toBe(opener))

      await user.click(opener)
      const again = await screen.findByRole('dialog')
      await user.type(within(again).getByLabelText(/Title/), 'New guide')
      await user.type(within(again).getByLabelText(/Content/), 'Body')
      expect(within(again).getByText('New guide', { selector: '.editor__preview-title' })).toBeTruthy()
      await user.click(within(again).getByRole('button', { name: 'Publish' }))
      await waitFor(() => expect(screen.queryByRole('dialog')).toBeNull())
      expect(adminApi.createKnowledge).toHaveBeenCalledWith(expect.objectContaining({ title: 'New guide', body: 'Body' }))
      expect(toastMessages('success')).toEqual(['Article published'])
    })

    it('keeps a failed save in the dialog', async () => {
      const user = userEvent.setup()
      adminApi.updateKnowledge.mockRejectedValue(new Error('conflict'))
      await renderPage()
      await rowAction(user, 'Edit')
      const dialog = await screen.findByRole('dialog', { name: 'Edit article' })
      await user.click(within(dialog).getByRole('button', { name: 'Save' }))
      expect((await within(dialog).findByRole('alert')).textContent).toBe('conflict')
      expect(toasts()).toHaveLength(0)
    })

    it('asks before deleting an article; Cancel keeps it', async () => {
      const user = userEvent.setup()
      adminApi.deleteKnowledge.mockRejectedValueOnce(new Error('locked')).mockResolvedValueOnce({ code: 0 })
      await renderPage()
      await rowAction(user, 'Delete article…')
      let confirm = await screen.findByRole('alertdialog', { name: 'Delete article "Reset password"?' })
      await user.click(within(confirm).getByRole('button', { name: 'Cancel' }))
      await waitFor(() => expect(screen.queryByRole('alertdialog')).toBeNull())
      expect(adminApi.deleteKnowledge).not.toHaveBeenCalled()

      await rowAction(user, 'Delete article…')
      confirm = await screen.findByRole('alertdialog')
      await user.click(within(confirm).getByRole('button', { name: 'Delete article' }))
      expect((await within(confirm).findByRole('alert')).textContent).toContain('locked')
      await user.click(within(confirm).getByRole('button', { name: 'Delete article' }))
      await waitFor(() => expect(screen.queryByRole('alertdialog')).toBeNull())
      expect(adminApi.deleteKnowledge).toHaveBeenCalledWith(5)
      expect(toastMessages('success')).toEqual(['Article "Reset password" deleted'])
    })

    it('opens the editor from a row and previews Markdown as elements, never HTML', async () => {
      const user = userEvent.setup()
      await renderPage([{ ...article, body: '## Steps\n\n- Open **Account**\n- Tap <img src=x onerror=alert(1)>' }])
      const row = screen.getByText('Reset password').closest('tr')
      row.focus()
      await user.keyboard('{Enter}')
      const dialog = await screen.findByRole('dialog', { name: 'Edit article' })
      const preview = dialog.querySelector('[data-test="knowledge-preview"]')
      expect(within(preview).getByRole('heading', { level: 3, name: 'Steps' })).toBeTruthy()
      expect(within(preview).getByText('Account').tagName).toBe('STRONG')
      expect(preview.querySelector('img')).toBeNull()
      expect(preview.textContent).toContain('<img src=x onerror=alert(1)>')
    })

    it('hides an article from the row menu with the same update and offers undo', async () => {
      const user = userEvent.setup()
      adminApi.updateKnowledge.mockResolvedValue({ code: 0 })
      await renderPage()
      await rowAction(user, 'Hide from Help Center')
      await waitFor(() => expect(adminApi.updateKnowledge).toHaveBeenCalledWith(5, { title: 'Reset password', category: '常见问题', body: 'Steps', sort: 1, show: 0 }))
      await waitFor(() => expect(toastMessages('success')).toEqual(['“Reset password” is hidden']))
    })

    it('filters by category and search, and shows empty and error states', async () => {
      const user = userEvent.setup()
      const other = { id: 6, category: '教程', title: 'Install Clash', body: 'Download', sort: 2, show: 1, updated_at: 1783526400 }
      await renderPage([article, other])
      await user.click(within(screen.getByRole('group', { name: 'Filter by category' })).getByRole('button', { name: 'Tutorial 1' }))
      expect(screen.queryByText('Reset password')).toBeNull()
      expect(screen.getByText('Install Clash')).toBeTruthy()
      await user.type(screen.getByRole('searchbox', { name: 'Search articles' }), 'zzz')
      expect(screen.getByRole('heading', { name: 'No results' })).toBeTruthy()
      await user.click(screen.getByRole('button', { name: 'Clear filters' }))
      expect(screen.getByText('Reset password')).toBeTruthy()
    })

    it('shows a load error with retry, then the empty state', async () => {
      const user = userEvent.setup()
      adminApi.getKnowledgeList.mockRejectedValueOnce(new Error('bad gateway')).mockResolvedValueOnce({ data: [] })
      render(Harness)
      const alert = await screen.findByRole('alert')
      expect(alert.textContent).toContain('Articles didn’t load')
      await user.click(within(alert).getByRole('button', { name: 'Try again' }))
      expect(await screen.findByRole('heading', { name: 'No articles yet' })).toBeTruthy()
    })
  })
})
