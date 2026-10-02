import { beforeEach, describe, expect, it, vi } from 'vitest'
import { createPinia, setActivePinia } from 'pinia'
import { render, screen } from '@testing-library/vue'
import { createMemoryHistory, createRouter, createWebHistory } from 'vue-router'
import router from '@/router'
import { useUserStore } from '@/stores/user'
import StatusPage from '@/views/StatusPage.vue'
import { scrollBehavior, waitUntilReachable } from '@/router/scroll'

vi.mock('@/api/kernel', () => ({
  getKernelExtensions: vi.fn(async () => [])
}))

vi.mock('@/api/user', () => ({
  getProfile: vi.fn(async () => ({ data: {} }))
}))

function leaf(route) {
  return route.matched[route.matched.length - 1]
}

describe('404, 无权限 and account routes', () => {
  beforeEach(async () => {
    localStorage.clear()
    setActivePinia(createPinia())
    if (router.currentRoute.value.path !== '/login') {
      await router.replace('/login')
    }
  })

  it('serves 404 for an unknown path outside the shells, keeping the URL', async () => {
    await router.push('/nowhere/at/all?x=1')
    const route = router.currentRoute.value
    expect(route.name).toBe('not-found')
    expect(route.fullPath).toBe('/nowhere/at/all?x=1')
    expect(leaf(route).props.default).toEqual({ kind: 'not-found', standalone: true })
    expect(route.meta.titleKey).toBe('shell.status.notFound.title')
  })

  it('serves 404 inside the admin and user shells', async () => {
    useUserStore().login('admin-token', { id: 1, is_admin: true, permissions: [] })
    await router.push('/admin/no-such-page')
    let route = router.currentRoute.value
    expect(route.path).toBe('/admin/no-such-page')
    expect(route.matched[0].path).toBe('/admin')
    expect(leaf(route).props.default).toEqual({ kind: 'not-found' })

    useUserStore().login('user-token', { id: 2, is_admin: false, permissions: [] })
    await router.push('/user/no-such-page')
    route = router.currentRoute.value
    expect(route.matched[0].path).toBe('/user')
    expect(leaf(route).props.default).toEqual({ kind: 'not-found' })
  })

  it('shows 无权限 at the admin URL a signed-in user opened', async () => {
    useUserStore().login('user-token', { id: 2, is_admin: false, permissions: [] })
    await router.push('/admin/users?page=2')
    const route = router.currentRoute.value
    expect(route.name).toBe('forbidden')
    expect(route.fullPath).toBe('/admin/users?page=2')
    expect(leaf(route).props.default).toEqual({ kind: 'forbidden', standalone: true })
  })

  it('still sends signed-out visitors to the login page', async () => {
    await router.push('/admin/users')
    expect(router.currentRoute.value.path).toBe('/login')
    await router.push('/user/account')
    expect(router.currentRoute.value.path).toBe('/login')
  })

  it('opens the shells at their first page', async () => {
    useUserStore().login('admin-token', { id: 1, is_admin: true, permissions: [] })
    await router.push('/admin')
    expect(router.currentRoute.value.path).toBe('/admin/dashboard')
  })

  it('has an account page in both shells', async () => {
    useUserStore().login('user-token', { id: 2, is_admin: false, permissions: [] })
    await router.push('/user/account')
    expect(router.currentRoute.value.path).toBe('/user/account')
    expect(router.currentRoute.value.meta.titleKey).toBe('shell.accountPage.title')

    useUserStore().login('admin-token', { id: 1, is_admin: true, permissions: [] })
    await router.push('/admin/account')
    expect(router.currentRoute.value.path).toBe('/admin/account')
  })

  it('keeps the seven legacy forward redirects', () => {
    const redirects = {
      '/admin/forward/tunnels': '/admin/forward/tunnel',
      '/admin/forward/limits': '/admin/forward/limit',
      '/admin/forward/ansible': '/admin/forward/ansible-machines',
      '/admin/local': '/admin/forward/local',
      '/admin/nodex': '/admin/forward/nodex',
      '/admin/tunnel': '/admin/forward/tunnel',
      '/admin/limit': '/admin/forward/limit'
    }
    const records = router.getRoutes()
    for (const [from, to] of Object.entries(redirects)) {
      expect(records.find(record => record.path === from)?.redirect).toBe(to)
    }
  })
})

describe('StatusPage.vue', () => {
  async function renderStatus(props, { admin = false, signedIn = true, from = '' } = {}) {
    setActivePinia(createPinia())
    if (signedIn) useUserStore().login('token', { id: 1, is_admin: admin })
    // The browser history records where a navigation came from (state.back).
    const memory = createRouter({ history: from ? createWebHistory() : createMemoryHistory(), routes: [{ path: '/:rest(.*)*', component: { template: '<div />' } }] })
    if (from) await memory.push(from)
    await memory.push('/somewhere')
    await memory.isReady()
    return render(StatusPage, { props, global: { plugins: [memory] } })
  }

  it('is the page H1 with a way home (EmptyState)', async () => {
    await renderStatus({ kind: 'not-found' }, { admin: true })
    expect(screen.getByRole('heading', { level: 1, name: 'Page not found' })).toBeTruthy()
    expect(screen.getByRole('link', { name: 'Go to home' }).getAttribute('href')).toBe('/admin/dashboard')
    expect(screen.getByText('Error 404')).toBeTruthy()
    // No in-app history: no "Go back".
    expect(screen.queryByRole('button', { name: 'Go back' })).toBeNull()
  })

  it('brings its own lockup and main landmark when standalone', async () => {
    const { container } = await renderStatus({ kind: 'forbidden', standalone: true })
    expect(screen.getByRole('heading', { level: 1, name: 'You don’t have access to this page' })).toBeTruthy()
    expect(container.querySelector('main#app-main-content')).not.toBeNull()
    expect(container.querySelector('.brand-lockup')).not.toBeNull()
    expect(screen.getByRole('link', { name: 'Go to home' }).getAttribute('href')).toBe('/user/dashboard')
  })

  it('offers "Go back" after an in-app navigation', async () => {
    await renderStatus({ kind: 'not-found' }, { from: '/admin/users' })
    expect(screen.getByRole('button', { name: 'Go back' })).toBeTruthy()
  })

  it('sends signed-out visitors to the login page', async () => {
    await renderStatus({ kind: 'not-found', standalone: true }, { signedIn: false })
    expect(screen.getByRole('link', { name: 'Go to home' }).getAttribute('href')).toBe('/login')
  })
})

describe('router/scroll.js', () => {
  it('starts new pages at the top without the smooth-scroll animation', async () => {
    expect(await scrollBehavior({ path: '/admin/users', hash: '' }, { path: '/admin/nodes' }, null)).toEqual({ left: 0, top: 0, behavior: 'instant' })
  })

  it('keeps the position when only the query changes', async () => {
    expect(await scrollBehavior({ path: '/admin/users', hash: '' }, { path: '/admin/users' }, null)).toBe(false)
  })

  it('restores the saved position on back and forward', async () => {
    expect(await scrollBehavior({ path: '/admin/users', hash: '' }, { path: '/admin/nodes' }, { left: 0, top: 0 })).toEqual({ left: 0, top: 0, behavior: 'instant' })
  })

  it('waits until the page is tall enough, but not forever', async () => {
    let now = 0
    const frames = []
    const done = vi.fn()
    waitUntilReachable(100000, { timeout: 50, now: () => now, frame: callback => frames.push(callback) }).then(done)
    expect(frames).toHaveLength(1)
    now = 60
    frames.shift()()
    await Promise.resolve()
    expect(done).toHaveBeenCalled()
  })
})
