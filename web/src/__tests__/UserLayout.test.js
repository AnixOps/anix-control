import { beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import { createMemoryHistory, createRouter } from 'vue-router'
import { createPinia, setActivePinia } from 'pinia'
import { useUserStore } from '@/stores/user'
import UserLayout from '@/layouts/UserLayout.vue'
import { setEdition } from '@/composables/useEdition'

function stubNarrow(matches) {
  vi.stubGlobal('matchMedia', vi.fn(() => ({ matches, addEventListener: vi.fn(), removeEventListener: vi.fn() })))
}

async function mountLayout(path = '/user/dashboard') {
  const router = createRouter({
    history: createMemoryHistory(),
    routes: [{ path: '/:rest(.*)*', component: { template: '<div class="test-page">page</div>' } }]
  })
  await router.push(path)
  await router.isReady()
  const wrapper = mount(UserLayout, { global: { plugins: [router] } })
  await flushPromises()
  return { wrapper, router }
}

function links(wrapper, selector) {
  return wrapper.findAll(`${selector} a[data-nav-item]`).map(link => link.attributes('href'))
}

describe('UserLayout.vue', () => {
  beforeEach(() => {
    setActivePinia(createPinia())
    const userStore = useUserStore()
    userStore.getUserInfo = vi.fn()
    userStore.login('token-123', { id: 7, email: 'lin@example.test', is_admin: false })
    stubNarrow(false)
  })

  it('has a header with the lockup, a named navigation and the main landmark', async () => {
    const { wrapper } = await mountLayout()
    expect(wrapper.find('header.user-bar .brand-lockup').exists()).toBe(true)
    expect(wrapper.get('nav[data-user-nav="top"]').attributes('aria-label')).toBe('Main navigation')
    const main = wrapper.get('main#app-main-content')
    expect(main.attributes('tabindex')).toBe('-1')
    expect(main.find('.test-page').exists()).toBe(true)
    // Theme and language moved into the account menu.
    expect(wrapper.find('.locale-switcher').exists()).toBe(false)
    expect(wrapper.find('.theme-toggle').exists()).toBe(false)
    expect(wrapper.find('header [data-account-menu-trigger]').attributes('aria-label')).toBe('Account menu: lin@example.test')
  })

  it('shows 概览 / 订阅 / 帮助中心 / 工单 / 账户 and marks the current page', async () => {
    setEdition('community')
    const { wrapper } = await mountLayout('/user/subscribe')
    expect(links(wrapper, 'nav[data-user-nav="top"]')).toEqual([
      '/user/dashboard', '/user/subscribe', '/user/knowledge', '/user/tickets', '/user/account'
    ])
    const current = wrapper.findAll('nav[data-user-nav="top"] [aria-current="page"]')
    expect(current.map(link => link.attributes('href'))).toEqual(['/user/subscribe'])
    expect(wrapper.find('nav[data-user-nav="tabs"]').exists()).toBe(false)
  })

  it('adds plans and orders in the commercial edition', async () => {
    setEdition('commercial')
    const { wrapper } = await mountLayout()
    expect(links(wrapper, 'nav[data-user-nav="top"]')).toEqual([
      '/user/dashboard', '/user/subscribe', '/user/knowledge', '/user/tickets', '/user/plans', '/user/orders', '/user/account'
    ])
  })

  it('uses a bottom tab bar below 834 px and keeps the lockup and avatar on top', async () => {
    stubNarrow(true)
    setEdition('commercial')
    const { wrapper } = await mountLayout('/user/tickets')
    expect(wrapper.find('nav[data-user-nav="top"]').exists()).toBe(false)
    const tabs = wrapper.get('nav[data-user-nav="tabs"]')
    expect(tabs.attributes('aria-label')).toBe('Main navigation')
    expect(links(wrapper, 'nav[data-user-nav="tabs"]')).toEqual([
      '/user/dashboard', '/user/subscribe', '/user/knowledge', '/user/tickets', '/user/account'
    ])
    expect(tabs.find('[aria-current="page"]').attributes('href')).toBe('/user/tickets')
    expect(tabs.findAll('svg')).toHaveLength(5)
    expect(tabs.text()).toContain('Help')
    expect(wrapper.find('header [data-account-menu-trigger]').exists()).toBe(true)
  })

  it('fetches the profile on mount', async () => {
    await mountLayout()
    expect(useUserStore().getUserInfo).toHaveBeenCalledTimes(1)
  })
})
