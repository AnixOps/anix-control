import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { render, screen, waitFor, within } from '@testing-library/vue'
import userEvent from '@testing-library/user-event'
import { createMemoryHistory, createRouter } from 'vue-router'
import { createPinia, setActivePinia } from 'pinia'
import i18n from '@/i18n'
import { buildAdminMenu } from '@/navigation/menu'
import { useTheme } from '@/composables/useTheme'
import CommandPalette from '@/components/shell/CommandPalette.vue'
import { isPaletteShortcut, usePalette } from '@/components/shell/usePalette'

const mockGetUserList = vi.hoisted(() => vi.fn())
vi.mock('@/api/admin', () => ({
  getUserList: (...args) => mockGetUserList(...args)
}))

const t = (...args) => i18n.global.t(...args)

async function renderPalette({ edition = 'community' } = {}) {
  const router = createRouter({
    history: createMemoryHistory(),
    routes: [{ path: '/:rest(.*)*', component: { template: '<div />' } }]
  })
  await router.push('/admin/dashboard')
  await router.isReady()
  const groups = buildAdminMenu({ t, editionAllows: required => required === edition })
  render(CommandPalette, { props: { groups }, global: { plugins: [router] } })
  return router
}

async function openPalette() {
  usePalette().openPalette()
  const dialog = await screen.findByRole('dialog', { name: 'Search or jump to' })
  const input = within(dialog).getByRole('combobox', { name: 'Search or jump to' })
  await waitFor(() => expect(document.activeElement).toBe(input))
  return { dialog, input }
}

function optionNames(dialog) {
  return within(dialog).queryAllByRole('option').map(option => option.querySelector('.palette__item-label').textContent.trim())
}

describe('CommandPalette.vue', () => {
  beforeEach(() => {
    setActivePinia(createPinia())
    mockGetUserList.mockReset()
    mockGetUserList.mockResolvedValue({ code: 0, data: { list: [{ id: 7, email: 'lin@example.test' }], total: 1 } })
  })

  afterEach(() => {
    usePalette().closePalette()
  })

  it('is a labelled modal dialog around a combobox that controls a listbox', async () => {
    await renderPalette()
    const { dialog, input } = await openPalette()
    expect(dialog.getAttribute('aria-modal')).toBe('true')
    expect(input.getAttribute('aria-expanded')).toBe('true')
    expect(input.getAttribute('aria-autocomplete')).toBe('list')
    const listbox = document.getElementById(input.getAttribute('aria-controls'))
    expect(listbox.getAttribute('role')).toBe('listbox')
    // Actions first, then every page the admin can see.
    const groups = within(dialog).getAllByRole('group').map(group => group.getAttribute('data-palette-group'))
    expect(groups).toEqual(['actions', 'pages'])
    expect(optionNames(dialog)).toEqual(expect.arrayContaining(['Add a node', 'Add a user', 'Forward setup wizard', 'Dashboard', 'Tunnels', 'Account']))
    // The community edition has no commercial pages.
    expect(optionNames(dialog)).not.toContain('Orders')
  })

  it('filters as you type, moves with the arrows and opens with Enter', async () => {
    const user = userEvent.setup()
    const router = await renderPalette()
    const { dialog, input } = await openPalette()
    await user.type(input, 'tunnel')
    await waitFor(() => expect(optionNames(dialog)).toEqual(['Tunnels']))
    await waitFor(() => expect(input.getAttribute('aria-activedescendant')).toBeTruthy())
    await user.keyboard('{Enter}')
    await waitFor(() => expect(router.currentRoute.value.fullPath).toBe('/admin/forward/tunnel'))
    expect(usePalette().open.value).toBe(false)
  })

  it('moves the highlight with ↓ and runs a quick action', async () => {
    const user = userEvent.setup()
    const router = await renderPalette()
    const { dialog, input } = await openPalette()
    await user.type(input, 'add')
    await waitFor(() => expect(optionNames(dialog)).toEqual(['Add a node', 'Add a user']))
    const first = input.getAttribute('aria-activedescendant')
    await user.keyboard('{ArrowDown}')
    const second = input.getAttribute('aria-activedescendant')
    expect(second).not.toBe(first)
    expect(document.getElementById(second).textContent).toContain('Add a user')
    await user.keyboard('{Enter}')
    await waitFor(() => expect(router.currentRoute.value.fullPath).toBe('/admin/users?create=1'))
  })

  it('finds users by email through the user list filter', async () => {
    const user = userEvent.setup()
    const router = await renderPalette()
    const { dialog, input } = await openPalette()
    await user.type(input, 'lin@')
    const option = await within(dialog).findByRole('option', { name: /lin@example\.test/ })
    expect(mockGetUserList).toHaveBeenLastCalledWith({ email: 'lin@', page: 1, page_size: 5 })
    expect(within(dialog).getByRole('group', { name: 'Users' })).toBeTruthy()
    await user.click(option)
    await waitFor(() => expect(router.currentRoute.value.path).toBe('/admin/users'))
    expect(router.currentRoute.value.query).toEqual({ email: 'lin@example.test' })
  })

  it('does not search users for a single character, and says when nothing matches', async () => {
    const user = userEvent.setup()
    await renderPalette()
    const { dialog, input } = await openPalette()
    await user.type(input, 'q')
    await new Promise(resolve => setTimeout(resolve, 350))
    expect(mockGetUserList).not.toHaveBeenCalled()
    mockGetUserList.mockResolvedValue({ code: 0, data: { list: [], total: 0 } })
    await user.type(input, 'xzq')
    await waitFor(() => expect(within(dialog).getByRole('status').textContent).toBe('No results'))
    expect(optionNames(dialog)).toEqual([])
  })

  it('switches the appearance from an action', async () => {
    const user = userEvent.setup()
    await renderPalette()
    const { dialog, input } = await openPalette()
    await user.type(input, 'dark')
    await user.click(await within(dialog).findByRole('option', { name: 'Use dark appearance' }))
    expect(useTheme().themePreference.value).toBe('dark')
    document.documentElement.removeAttribute('data-theme')
  })

  it('closes with Escape', async () => {
    const user = userEvent.setup()
    await renderPalette()
    await openPalette()
    await user.keyboard('{Escape}')
    await waitFor(() => expect(screen.queryByRole('dialog')).toBeNull())
    expect(usePalette().open.value).toBe(false)
  })

  it('recognises ⌘K and Ctrl+K only', () => {
    expect(isPaletteShortcut({ key: 'k', metaKey: true })).toBe(true)
    expect(isPaletteShortcut({ key: 'K', ctrlKey: true })).toBe(true)
    expect(isPaletteShortcut({ key: 'k' })).toBe(false)
    expect(isPaletteShortcut({ key: 'k', ctrlKey: true, altKey: true })).toBe(false)
    expect(isPaletteShortcut({ key: 'j', metaKey: true })).toBe(false)
  })
})
