import { describe, expect, it } from 'vitest'
import { render, screen, waitFor, within } from '@testing-library/vue'
import userEvent from '@testing-library/user-event'
import { createMemoryHistory, createRouter } from 'vue-router'
import ForwardSuiteNav from '@/components/admin/ForwardSuiteNav.vue'

const paths = [
  '/admin/forward/setup', '/admin/forward', '/admin/forward/tunnel', '/admin/forward/limit',
  '/admin/forward/ansible-machines', '/admin/forward/local', '/admin/forward/nodes', '/admin/forward/nodex',
  '/admin/forward/agents', '/admin/forward/observability'
]

async function renderNav(startPath = '/admin/forward') {
  const router = createRouter({
    history: createMemoryHistory(),
    routes: paths.map(path => ({ path, component: { template: '<div />' } }))
  })
  await router.push(startPath)
  await router.isReady()
  render(ForwardSuiteNav, { global: { plugins: [router] } })
  return router
}

describe('ForwardSuiteNav.vue', () => {
  it('keeps the flux-panel sub-navigation: five links and a 更多 menu with the runtime tools', async () => {
    const user = userEvent.setup()
    await renderNav()
    const nav = screen.getByRole('navigation', { name: 'Forward suite' })
    const links = within(nav).getAllByRole('link')
    expect(links.map(link => link.getAttribute('href'))).toEqual([
      '/admin/forward/setup', '/admin/forward', '/admin/forward/tunnel', '/admin/forward/limit', '/admin/forward/nodes'
    ])
    expect(within(nav).getByRole('link', { name: 'Setup Wizard' }).getAttribute('title')).toBeTruthy()

    await user.click(within(nav).getByRole('button', { name: 'More forwarding tools' }))
    const menu = await screen.findByRole('menu')
    expect(within(menu).getAllByRole('menuitem').map(item => item.getAttribute('href'))).toEqual([
      '/admin/forward/ansible-machines', '/admin/forward/local', '/admin/forward/nodex', '/admin/forward/agents', '/admin/forward/observability'
    ])
    expect(within(menu).getByRole('menuitem', { name: /Ansible Machines/ }).textContent).toContain('Stateless execution machines')
  })

  it('marks only the current page, not the /admin/forward prefix', async () => {
    await renderNav('/admin/forward/tunnel')
    const nav = screen.getByRole('navigation', { name: 'Forward suite' })
    const current = within(nav).getAllByRole('link').filter(link => link.getAttribute('aria-current') === 'page')
    expect(current.map(link => link.getAttribute('href'))).toEqual(['/admin/forward/tunnel'])
    expect(current[0].classList.contains('is-active')).toBe(true)
  })

  it('names the current runtime tool on the 更多 button and in its menu', async () => {
    const user = userEvent.setup()
    await renderNav('/admin/forward/nodex')
    const nav = screen.getByRole('navigation', { name: 'Forward suite' })
    const more = within(nav).getByRole('button', { name: 'More forwarding tools: NodeX Runtime' })
    expect(more.classList.contains('is-active')).toBe(true)
    expect(within(nav).getAllByRole('link').some(link => link.getAttribute('aria-current') === 'page')).toBe(false)
    await user.click(more)
    const item = await screen.findByRole('menuitem', { name: /NodeX Runtime/ })
    expect(item.getAttribute('aria-current')).toBe('page')
  })

  it('opens a runtime tool from the keyboard', async () => {
    const user = userEvent.setup()
    const router = await renderNav()
    const more = screen.getByRole('button', { name: 'More forwarding tools' })
    more.focus()
    await user.keyboard('{Enter}')
    await screen.findByRole('menu')
    await user.keyboard('{ArrowDown}')
    await waitFor(() => expect(document.activeElement.getAttribute('href')).toBe('/admin/forward/local'))
    await user.keyboard('{Enter}')
    await waitFor(() => expect(router.currentRoute.value.path).toBe('/admin/forward/local'))
  })

  it('groups 流量转发 / 隧道 / 限速 as the segmented control (UI U7)', async () => {
    await renderNav('/admin/forward/limit')
    const nav = screen.getByRole('navigation', { name: 'Forward suite' })
    const segments = nav.querySelector('.forward-suite-segments')
    expect([...segments.querySelectorAll('a')].map(link => link.getAttribute('href'))).toEqual([
      '/admin/forward', '/admin/forward/tunnel', '/admin/forward/limit'
    ])
    expect(segments.querySelector('[aria-current="page"]').getAttribute('href')).toBe('/admin/forward/limit')
  })
})

