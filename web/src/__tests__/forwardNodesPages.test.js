// 转发节点 (UI U7): the NodeX nodes list, the node and Ansible machine detail
// pages, the run-mode switch and their routes. API calls, scopes and
// payloads are the ones the pages used before the redesign.
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { render, screen, waitFor, within } from '@testing-library/vue'
import userEvent from '@testing-library/user-event'
import { routeLocationKey, routerKey } from 'vue-router'
import ForwardNodes from '@/views/admin/ForwardNodes.vue'
import ForwardNodeDetail from '@/views/admin/forward-nodes/ForwardNodeDetail.vue'
import AnsibleMachineDetail from '@/views/admin/forward-nodes/AnsibleMachineDetail.vue'
import ForwardNodesModeNav from '@/views/admin/forward-nodes/ForwardNodesModeNav.vue'
import UiHost from '@/ui/UiHost.vue'
import { ADMIN_MENU, activeMenuItem, buildAdminMenu, showsForwardSuiteNav } from '@/navigation/menu'
import { toastMessages } from './helpers/feedback'

const api = vi.hoisted(() => ({
  checkForwardNode: vi.fn(),
  createForwardNode: vi.fn(),
  createForwardRule: vi.fn(),
  deleteForwardNode: vi.fn(),
  deleteForwardRule: vi.fn(),
  getForwardNode: vi.fn(),
  getForwardNodes: vi.fn(),
  getForwardRule: vi.fn(),
  getForwardRules: vi.fn(),
  getForwardStats: vi.fn(),
  syncForwardNodeStats: vi.fn(),
  testForwardConnection: vi.fn(),
  toggleForwardNode: vi.fn(),
  toggleForwardRule: vi.fn(),
  updateForwardNode: vi.fn(),
  updateForwardRule: vi.fn(),
  checkAnsibleMachine: vi.fn(),
  createAnsibleMachine: vi.fn(),
  deleteAnsibleMachine: vi.fn(),
  getAnsibleMachine: vi.fn(),
  getAnsibleMachines: vi.fn(),
  syncAnsibleMachineStats: vi.fn(),
  toggleAnsibleMachine: vi.fn(),
  updateAnsibleMachine: vi.fn()
}))

vi.mock('@/api/admin', () => api)

const relay = { id: 5, name: 'hk-relay-01', type: 'relay', host: '203.0.113.5', port: 8443, api_port: 18080, api_token: '********', enabled: true, status: 1, latency: 12, current_conn: 3 }

function fakeRouter(query = {}) {
  const route = { path: '/admin/forward/nodes', query, hash: '' }
  return {
    route,
    router: { push: vi.fn(), replace: vi.fn(async () => {}) }
  }
}

function renderWith(Page, { props = {}, query = {} } = {}) {
  const { router, route } = fakeRouter(query)
  const view = render({
    components: { Page, UiHost },
    setup: () => ({ props }),
    template: '<div><Page v-bind="props" /><UiHost /></div>'
  }, {
    global: {
      provide: { [routerKey]: router, [routeLocationKey]: route },
      stubs: { RouterLink: { props: ['to'], template: '<a :href="String(to)"><slot /></a>' } }
    }
  })
  return { ...view, router }
}

beforeEach(() => {
  for (const fn of Object.values(api)) fn.mockReset()
  api.getForwardStats.mockResolvedValue({ code: 0, data: { relay_nodes: 1, exit_nodes: 0, online_relay: 1, online_exit: 0, total_upload: 1024, total_download: 2048 } })
  api.getForwardNodes.mockResolvedValue({ code: 0, data: { list: [relay], total: 1 } })
  api.getForwardRules.mockResolvedValue({ code: 0, data: { list: [], total: 0 } })
  api.getForwardNode.mockResolvedValue({ code: 0, data: relay })
  api.getAnsibleMachine.mockResolvedValue({ data: { data: { id: 7, name: 'relay-exec-07', host: '198.51.100.7', port: 22, enabled: true, status: 0, weight: 2 } } })
})

afterEach(() => {
  vi.restoreAllMocks()
})

describe('NodeX nodes list', () => {
  it('lists the NodeX scope with server filters and opens the detail page from a row', async () => {
    const user = userEvent.setup()
    const { router } = renderWith(ForwardNodes)
    const table = await screen.findByRole('table', { name: 'NodeX nodes' })
    expect(api.getForwardNodes).toHaveBeenCalledWith({ page: 1, page_size: 12, scope: 'nodex' })
    expect(within(table).getByText('203.0.113.5:8443')).toBeTruthy()
    expect(screen.getByText('Forward nodes', { selector: 'h1' })).toBeTruthy()

    await user.click(screen.getByRole('button', { name: 'Exit' }))
    expect(api.getForwardNodes).toHaveBeenLastCalledWith({ page: 1, page_size: 12, scope: 'nodex', type: 'exit' })
    await user.click(screen.getByRole('button', { name: 'Offline' }))
    expect(api.getForwardNodes).toHaveBeenLastCalledWith({ page: 1, page_size: 12, scope: 'nodex', type: 'exit', status: 0 })

    await user.click(within(table).getByText('hk-relay-01'))
    expect(router.push).toHaveBeenCalledWith('/admin/forward/nodes/5')
  })

  it('shows a load error with retry instead of an empty list', async () => {
    const user = userEvent.setup()
    api.getForwardNodes.mockRejectedValueOnce({ response: { data: { msg: 'database locked' } } })
    renderWith(ForwardNodes)
    const alert = await screen.findByRole('alert')
    expect(alert.textContent).toContain('Failed to load relay/exit nodes')
    expect(alert.textContent).toContain('database locked')
    await user.click(within(alert).getByRole('button', { name: 'Try again' }))
    expect(await screen.findByRole('table', { name: 'NodeX nodes' })).toBeTruthy()
  })

  it('checks a user ID filter before querying the legacy rules', async () => {
    const user = userEvent.setup()
    renderWith(ForwardNodes)
    await screen.findByRole('table', { name: 'NodeX nodes' })
    const field = screen.getByLabelText('Filter by user ID')
    await user.type(field, 'abc')
    await user.click(screen.getByRole('button', { name: 'Filter' }))
    expect(screen.getByText('User ID must be a positive integer')).toBeTruthy()
    await user.clear(field)
    await user.type(field, '42{Enter}')
    expect(api.getForwardRules).toHaveBeenLastCalledWith({ page: 1, page_size: 10, user_id: 42 })
  })
})

describe('NodeX node detail page', () => {
  it('loads the node in the NodeX scope and shows its sections without the token', async () => {
    const user = userEvent.setup()
    renderWith(ForwardNodeDetail, { props: { id: 5 } })
    expect(await screen.findByRole('heading', { level: 1, name: 'hk-relay-01' })).toBeTruthy()
    expect(api.getForwardNode).toHaveBeenCalledWith(5, { params: { scope: 'nodex' } })
    const tabs = screen.getByRole('tablist', { name: 'Node details' })
    expect(within(tabs).getAllByRole('tab').map(tab => tab.textContent.trim())).toEqual(['Overview', 'Configuration', 'Danger zone'])

    await user.click(within(tabs).getByRole('tab', { name: 'Configuration' }))
    const panel = screen.getByRole('tabpanel')
    expect(panel.textContent).toContain('18080')
    expect(panel.textContent).toContain('Set (hidden)')
    expect(panel.textContent).not.toContain('********')
  })

  it('runs a health check from the header and keeps the result', async () => {
    const user = userEvent.setup()
    api.checkForwardNode.mockResolvedValue({ code: 0, data: { status: 1, latency: 9 } })
    renderWith(ForwardNodeDetail, { props: { id: 5 } })
    await screen.findByRole('heading', { level: 1, name: 'hk-relay-01' })
    await user.click(screen.getByRole('button', { name: 'Health Check' }))
    await waitFor(() => expect(api.checkForwardNode).toHaveBeenCalledWith(5, { params: { scope: 'nodex' } }))
    expect((await screen.findByText('Latency 9 ms', { selector: '[data-test="node-detail-result"]' }))).toBeTruthy()
    expect(api.getForwardNode).toHaveBeenCalledTimes(2)
  })

  it('deletes the node from the danger zone after its name is typed, then returns to the list', async () => {
    const user = userEvent.setup()
    api.deleteForwardNode.mockResolvedValue({ code: 0 })
    const { router } = renderWith(ForwardNodeDetail, { props: { id: 5 } })
    await screen.findByRole('heading', { level: 1, name: 'hk-relay-01' })
    await user.click(screen.getByRole('tab', { name: 'Danger zone' }))
    expect(router.replace).toHaveBeenCalledWith({ path: '/admin/forward/nodes', query: { tab: 'danger' }, hash: '' })
    await user.click(screen.getByRole('button', { name: 'Delete forward node' }))
    const confirm = await screen.findByRole('alertdialog', { name: 'Delete forward node hk-relay-01?' })
    await user.type(within(confirm).getByRole('textbox'), 'hk-relay-01')
    await user.click(within(confirm).getByRole('button', { name: 'Delete forward node' }))
    await waitFor(() => expect(api.deleteForwardNode).toHaveBeenCalledWith(5, { params: { scope: 'nodex' } }))
    await waitFor(() => expect(router.push).toHaveBeenCalledWith('/admin/forward/nodes'))
    expect(toastMessages('success')).toHaveLength(1)
  })

  it('says when the node no longer exists', async () => {
    api.getForwardNode.mockRejectedValue({ response: { status: 404, data: { msg: 'record not found' } } })
    renderWith(ForwardNodeDetail, { props: { id: 99 } })
    expect(await screen.findByText('This node no longer exists. It may have been deleted.')).toBeTruthy()
    expect(screen.queryByRole('tablist')).toBeNull()
  })

  it('shows a load error with retry', async () => {
    const user = userEvent.setup()
    api.getForwardNode.mockRejectedValueOnce({ response: { status: 500, data: { msg: 'boom' } } })
    renderWith(ForwardNodeDetail, { props: { id: 5 } })
    const alert = await screen.findByRole('alert')
    expect(alert.textContent).toContain('Could not load the node')
    await user.click(within(alert).getByRole('button', { name: 'Try again' }))
    expect(await screen.findByRole('heading', { level: 1, name: 'hk-relay-01' })).toBeTruthy()
  })

  it('opens the section named in the URL', async () => {
    renderWith(ForwardNodeDetail, { props: { id: 5 }, query: { tab: 'config' } })
    await screen.findByRole('heading', { level: 1, name: 'hk-relay-01' })
    expect(screen.getByRole('tab', { name: 'Configuration' }).getAttribute('aria-selected')).toBe('true')
  })
})

describe('Ansible machine detail page', () => {
  it('loads the machine, checks it and toggles it', async () => {
    const user = userEvent.setup()
    api.checkAnsibleMachine.mockResolvedValue({ data: { data: { status: 0, error: 'dial tcp timeout' } } })
    api.toggleAnsibleMachine.mockResolvedValue({})
    renderWith(AnsibleMachineDetail, { props: { id: 7 } })
    expect(await screen.findByRole('heading', { level: 1, name: 'relay-exec-07' })).toBeTruthy()
    expect(api.getAnsibleMachine).toHaveBeenCalledWith(7)

    await user.click(screen.getByRole('button', { name: 'Health Check' }))
    await waitFor(() => expect(api.checkAnsibleMachine).toHaveBeenCalledWith(7))
    expect(await screen.findByText('dial tcp timeout')).toBeTruthy()

    await user.click(screen.getByRole('tab', { name: 'Danger zone' }))
    await user.click(screen.getByRole('button', { name: 'Disable' }))
    await waitFor(() => expect(api.toggleAnsibleMachine).toHaveBeenCalledWith(7, false))
  })

  it('deletes the machine after its name is typed and returns to the list', async () => {
    const user = userEvent.setup()
    api.deleteAnsibleMachine.mockResolvedValue({})
    const { router } = renderWith(AnsibleMachineDetail, { props: { id: 7 } })
    await screen.findByRole('heading', { level: 1, name: 'relay-exec-07' })
    await user.click(screen.getByRole('tab', { name: 'Danger zone' }))
    await user.click(screen.getByRole('button', { name: 'Delete machine' }))
    const confirm = await screen.findByRole('alertdialog', { name: 'Delete Ansible machine relay-exec-07?' })
    await user.type(within(confirm).getByRole('textbox'), 'relay-exec-07')
    await user.click(within(confirm).getByRole('button', { name: 'Delete machine' }))
    await waitFor(() => expect(api.deleteAnsibleMachine).toHaveBeenCalledWith(7))
    await waitFor(() => expect(router.push).toHaveBeenCalledWith('/admin/forward/ansible-machines'))
  })
})

describe('run-mode switch and routes', () => {
  it('links the four forward-node pages and marks the current one', () => {
    renderWith(ForwardNodesModeNav, { props: { current: 'local' } })
    const nav = screen.getByRole('navigation', { name: 'Run mode' })
    const links = within(nav).getAllByRole('link')
    expect(links.map(link => link.getAttribute('href'))).toEqual([
      '/admin/forward/nodes',
      '/admin/forward/ansible-machines',
      '/admin/forward/local',
      '/admin/forward/nodex',
      '/admin/forward/agents'
    ])
    expect(within(nav).getByRole('link', { name: 'Local Runtime' }).getAttribute('aria-current')).toBe('page')
    expect(links.filter(link => link.getAttribute('aria-current'))).toHaveLength(1)
  })

  it('leaves the forward suite navigation to the Flux control-plane pages', () => {
    for (const path of ['/admin/forward/nodes', '/admin/forward/nodes/5', '/admin/forward/ansible-machines', '/admin/forward/ansible-machines/7', '/admin/forward/local', '/admin/forward/nodex']) {
      expect(showsForwardSuiteNav(path)).toBe(false)
    }
    for (const path of ['/admin/forward', '/admin/forward/tunnel', '/admin/forward/limit', '/admin/forward/setup', '/admin/forward/agents', '/admin/forward/observability']) {
      expect(showsForwardSuiteNav(path)).toBe(true)
    }
    expect(showsForwardSuiteNav('/admin/forward/nodesx')).toBe(true)
    expect(showsForwardSuiteNav('/admin/users')).toBe(false)
  })

  it('keeps 转发节点 selected in the sidebar on the detail pages', () => {
    const groups = buildAdminMenu({ t: key => key, menu: ADMIN_MENU })
    expect(activeMenuItem(groups, '/admin/forward/nodes/5').item.id).toBe('forward-nodes')
    expect(activeMenuItem(groups, '/admin/forward/ansible-machines/7').item.id).toBe('forward-nodes')
  })
})
