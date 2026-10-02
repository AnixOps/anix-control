import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { render, screen, waitFor, within } from '@testing-library/vue'
import userEvent from '@testing-library/user-event'
import { createMemoryHistory, createRouter } from 'vue-router'
import { createPinia, setActivePinia } from 'pinia'
import { useUserStore } from '@/stores/user'
import { useTheme } from '@/composables/useTheme'
import i18n from '@/i18n'
import AccountMenu from '@/components/shell/AccountMenu.vue'

function stubNarrow(matches) {
  vi.stubGlobal('matchMedia', vi.fn(() => ({ matches, addEventListener: vi.fn(), removeEventListener: vi.fn() })))
}

async function renderMenu(props = {}) {
  const router = createRouter({
    history: createMemoryHistory(),
    routes: [{ path: '/:rest(.*)*', component: { template: '<div />' } }]
  })
  await router.push('/admin/dashboard')
  await router.isReady()
  const result = render(AccountMenu, {
    props: { variant: 'row', role: 'admin', accountPath: '/admin/account', showAbout: true, ...props },
    global: { plugins: [router] }
  })
  return { ...result, router }
}

describe('AccountMenu.vue', () => {
  beforeEach(() => {
    setActivePinia(createPinia())
    useUserStore().login('token', { id: 1, email: 'admin@example.test', is_admin: true })
    stubNarrow(false)
  })

  afterEach(() => {
    document.documentElement.removeAttribute('data-theme')
  })

  it('names the trigger and lists 账户, 外观, 语言, 关于 and 退出 in order', async () => {
    const user = userEvent.setup()
    await renderMenu()
    const trigger = screen.getByRole('button', { name: 'Account menu: Administrator' })
    expect(trigger.getAttribute('aria-haspopup')).toBe('menu')
    await user.click(trigger)
    const menu = await screen.findByRole('menu')
    const items = within(menu).getAllByRole('menuitem').map(item => item.querySelector('.shell-menu__label').textContent.trim())
    expect(items).toEqual(['Account', 'Appearance', 'Language', 'About', 'Sign out'])
    // The submenus show the current choice.
    expect(within(menu).getByRole('menuitem', { name: /Appearance/ }).textContent).toContain('System')
    expect(within(menu).getByRole('menuitem', { name: /Language/ }).textContent).toContain('English')
    expect(within(menu).getByText('admin@example.test')).toBeTruthy()
    expect(within(menu).getByRole('menuitem', { name: 'Account' }).getAttribute('href')).toBe('/admin/account')
  })

  it('is keyboard operable: Enter opens, arrows move, Esc closes and returns focus', async () => {
    const user = userEvent.setup()
    await renderMenu()
    const trigger = screen.getByRole('button', { name: /Account menu/ })
    trigger.focus()
    await user.keyboard('{Enter}')
    const menu = await screen.findByRole('menu')
    await waitFor(() => expect(document.activeElement?.textContent).toContain('Account'))
    await user.keyboard('{ArrowDown}')
    expect(document.activeElement.textContent).toContain('Appearance')
    await user.keyboard('{Escape}')
    await waitFor(() => expect(screen.queryByRole('menu')).toBeNull())
    expect(menu.isConnected).toBe(false)
    expect(document.activeElement).toBe(trigger)
  })

  it('switches the theme from the appearance submenu (system, light, dark)', async () => {
    const user = userEvent.setup()
    await renderMenu()
    await user.click(screen.getByRole('button', { name: /Account menu/ }))
    const appearance = await screen.findByRole('menuitem', { name: /Appearance/ })
    appearance.focus()
    await user.keyboard('{ArrowRight}')
    const dark = await screen.findByRole('menuitemradio', { name: 'Dark' })
    expect(screen.getByRole('menuitemradio', { name: 'System' }).getAttribute('aria-checked')).toBe('true')
    await user.click(dark)
    const { themePreference } = useTheme()
    expect(themePreference.value).toBe('dark')
    expect(document.documentElement.getAttribute('data-theme')).toBe('dark')
    expect(localStorage.setItem).toHaveBeenCalledWith('v2board-theme', 'dark')
  })

  it('switches the language from the language submenu', async () => {
    const user = userEvent.setup()
    await renderMenu()
    await user.click(screen.getByRole('button', { name: /Account menu/ }))
    const language = await screen.findByRole('menuitem', { name: /Language/ })
    language.focus()
    await user.keyboard('{ArrowRight}')
    await user.click(await screen.findByRole('menuitemradio', { name: '简体中文' }))
    await waitFor(() => expect(i18n.global.locale.value).toBe('zh-CN'))
  })

  it('emits about and signs out', async () => {
    const user = userEvent.setup()
    const { emitted, router } = await renderMenu()
    await user.click(screen.getByRole('button', { name: /Account menu/ }))
    await user.click(await screen.findByRole('menuitem', { name: 'About' }))
    expect(emitted().about).toHaveLength(1)

    const userStore = useUserStore()
    userStore.logout = vi.fn()
    await user.click(screen.getByRole('button', { name: /Account menu/ }))
    await user.click(await screen.findByRole('menuitem', { name: 'Sign out' }))
    expect(userStore.logout).toHaveBeenCalledTimes(1)
    await waitFor(() => expect(router.currentRoute.value.path).toBe('/login'))
  })

  it('has no 关于 for users and names them by email', async () => {
    const user = userEvent.setup()
    useUserStore().login('token', { id: 7, email: 'lin@example.test', is_admin: false })
    await renderMenu({ role: 'user', variant: 'avatar', showAbout: false, accountPath: '/user/account' })
    await user.click(screen.getByRole('button', { name: 'Account menu: lin@example.test' }))
    const menu = await screen.findByRole('menu')
    expect(within(menu).queryByRole('menuitem', { name: 'About' })).toBeNull()
    // The email is the name: not repeated underneath.
    expect(within(menu).getAllByText('lin@example.test')).toHaveLength(1)
  })

  it('puts the choices inline on phones, with extra page links', async () => {
    stubNarrow(true)
    const user = userEvent.setup()
    await renderMenu({ extraLinks: [{ id: 'plans', to: '/user/plans', label: 'Plans', icon: 'plans' }] })
    await user.click(screen.getByRole('button', { name: /Account menu/ }))
    const menu = await screen.findByRole('menu')
    expect(within(menu).queryByRole('menuitem', { name: /Appearance/ })).toBeNull()
    expect(within(menu).getAllByRole('menuitemradio').map(item => item.querySelector('.shell-menu__label').textContent.trim())).toEqual(['System', 'Light', 'Dark', '简体中文', 'English'])
    expect(within(menu).getByRole('menuitem', { name: 'Plans' }).getAttribute('href')).toBe('/user/plans')
  })
})
