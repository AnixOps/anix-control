// Node list (UI U7, 5.0 node onboarding): the list template on UiDataTable,
// server search and status chips in the URL, rows that open the node page, the
// approval of nodes that joined a group requiring it, and the Agent columns.
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { enableAutoUnmount, flushPromises, mount } from '@vue/test-utils'
import { createMemoryHistory, createRouter } from 'vue-router'
import { screen, within } from '@testing-library/vue'
import { toastMessages } from './helpers/feedback'
import userEvent from '@testing-library/user-event'
import Nodes from '@/views/admin/Nodes.vue'

const adminApi = vi.hoisted(() => ({
  getNode: vi.fn(),
  getNodeStats: vi.fn(),
  getNodeLogs: vi.fn(),
  updateNode: vi.fn(),
  deleteNode: vi.fn(),
  syncNodeProtocol: vi.fn(),
  getNodeCredentials: vi.fn(),
  getNodeProtocols: vi.fn(),
  createNodeProtocol: vi.fn(),
  updateNodeProtocol: vi.fn(),
  deleteNodeProtocol: vi.fn(),
  getProtocolTemplates: vi.fn(),
  getSubscriptionGroups: vi.fn(),
  generateWireGuardKeypair: vi.fn()
}))

vi.mock('@/api/admin', () => adminApi)

// The node list (GET /api/v4/admin/nodes), the Agent connection of the nodes in
// view (GET /api/v4/kernel/agents/transports) and the node groups.
const kernelApi = vi.hoisted(() => ({
  getKernelAdminNodes: vi.fn(),
  getKernelAgentTransports: vi.fn(),
  listKernelNodeGroups: vi.fn(),
  approveKernelNode: vi.fn(),
  rejectKernelNode: vi.fn()
}))
vi.mock('@/api/kernel', async importOriginal => ({ ...(await importOriginal()), ...kernelApi }))

// The services summary of the nodes in view (the machine-telemetry package's
// GET /api/v3/plugins/machine-telemetry/nodes/services).
const telemetryApi = vi.hoisted(() => ({ getNodesServicesSummary: vi.fn() }))
vi.mock('@/api/machineTelemetry', async importOriginal => ({ ...(await importOriginal()), ...telemetryApi }))

const Stub = { template: '<div />' }

async function mountNodes(path = '/admin/nodes') {
  const router = createRouter({
    history: createMemoryHistory(),
    routes: [
      { path: '/admin/nodes', component: Nodes },
      { path: '/admin/nodes/:id', component: Stub }
    ]
  })
  await router.push(path)
  await router.isReady()
  const wrapper = mount(Nodes, { attachTo: document.body, global: { plugins: [router] } })
  await flushPromises()
  return { wrapper, router }
}

enableAutoUnmount(afterEach)

describe('Nodes.vue', () => {
  beforeEach(() => {
    localStorage.clear()
    vi.resetAllMocks()
    vi.spyOn(console, 'error').mockImplementation(() => {})
    kernelApi.getKernelAdminNodes.mockResolvedValue({ list: [], total: 0 })
    adminApi.getNodeStats.mockResolvedValue({ data: { total: 0, online: 0, offline: 0, pending: 0 } })
    adminApi.getNodeCredentials.mockResolvedValue({ data: { api_key: '' } })
    kernelApi.getKernelAgentTransports.mockResolvedValue({ nodes: [] })
    kernelApi.listKernelNodeGroups.mockResolvedValue({ groups: [] })
    telemetryApi.getNodesServicesSummary.mockResolvedValue([])
  })

  it('stops follow-up requests and shows the error state when the nodes do not load', async () => {
    kernelApi.getKernelAdminNodes.mockRejectedValueOnce(new Error('401'))

    const { wrapper } = await mountNodes()

    expect(kernelApi.getKernelAdminNodes).toHaveBeenCalledTimes(1)
    expect(adminApi.getNodeStats).not.toHaveBeenCalled()
    expect(wrapper.text()).toContain('Couldn’t load nodes')

    kernelApi.getKernelAdminNodes.mockResolvedValueOnce({ list: [{ id: 1, name: 'hk-01', host: 'hk.example' }], total: 1 })
    await wrapper.findAll('button').find(button => button.text() === 'Try again').trigger('click')
    await flushPromises()
    expect(wrapper.text()).toContain('hk-01')
    expect(adminApi.getNodeStats).toHaveBeenCalledTimes(1)
  })

  it('reads the list and the stats from legacy and panel envelopes', async () => {
    kernelApi.getKernelAdminNodes.mockResolvedValueOnce({
      code: 0,
      msg: '操作成功',
      data: { list: [{ id: 2, name: 'Envelope Node', host: 'env.example', protocols: [{ id: 1 }, { id: 2 }] }], total: 3 },
      ts: 1783526400000
    })
    adminApi.getNodeStats.mockResolvedValueOnce({ data: { total: 12, online: 7, offline: 4, pending: 1 } })

    const { wrapper } = await mountNodes()

    expect(wrapper.vm.nodes[0]).toMatchObject({ name: 'Envelope Node', address: 'env.example' })
    expect(wrapper.vm.pagination.total).toBe(3)
    expect(wrapper.find('[data-testid="node-stats"]').text()).toContain('12')
    expect(wrapper.find('tbody').text()).toContain('env.example')

    adminApi.getNodeStats.mockResolvedValueOnce({ code: 0, msg: '操作成功', data: { total: 21, online: 18, offline: 2, pending: 1 }, ts: 1 })
    await wrapper.vm.loadStats()
    expect(wrapper.vm.stats).toMatchObject({ total: 21, online: 18, offline: 2, pending: 1 })
  })

  it('searches and filters on the server and keeps the list state in the URL', async () => {
    const { wrapper, router } = await mountNodes('/admin/nodes?q=hk&status=offline&page=2')

    expect(kernelApi.getKernelAdminNodes).toHaveBeenLastCalledWith({ page: 2, page_size: 20, search: 'hk', status: 2 })

    await wrapper.findAll('[aria-pressed]').find(chip => chip.text() === 'Disabled').trigger('click')
    await flushPromises()
    expect(kernelApi.getKernelAdminNodes).toHaveBeenLastCalledWith({ page: 1, page_size: 20, search: 'hk', status: 3 })
    expect(router.currentRoute.value.query).toEqual({ q: 'hk', status: 'disabled' })
  })

  it('opens the node page from a row', async () => {
    kernelApi.getKernelAdminNodes.mockResolvedValue({ list: [{ id: 5, name: 'hk-01', host: 'hk.example', status: 1 }], total: 1 })
    const { wrapper, router } = await mountNodes()

    await wrapper.find('tbody tr').trigger('click')
    await flushPromises()
    expect(router.currentRoute.value.fullPath).toBe('/admin/nodes/5')
  })

  describe('sorting on the server (GET /admin/nodes sort and order)', () => {
    const rows = [{ id: 5, name: 'hk-01', host: 'hk.example', status: 1, protocols: [{ id: 1 }] }]

    it('sorts only the columns the API supports and goes back to the first page', async () => {
      kernelApi.getKernelAdminNodes.mockResolvedValue({ list: rows, total: 40 })
      const { wrapper, router } = await mountNodes('/admin/nodes?page=2')
      const header = label => wrapper.findAll('th').find(cell => cell.text().startsWith(label))
      // Name, address and last seen sort (so do the ID and the load, which
      // start hidden); the derived status, the protocol count, the version
      // and the Agent certificate do not. Load and Connection are off until
      // the table settings turn them on, to make room for the Agent version
      // and the last seen time.
      for (const label of ['Name', 'Address', 'Last seen']) expect(header(label).find('button').exists()).toBe(true)
      expect(header('Load')).toBeUndefined()
      expect(header('Connection')).toBeUndefined()
      for (const label of ['Protocols', 'Status', 'Certificate', 'Agent version']) expect(header(label).find('button').exists()).toBe(false)

      await header('Last seen').find('button').trigger('click')
      await flushPromises()
      // A date column starts newest first.
      expect(kernelApi.getKernelAdminNodes).toHaveBeenLastCalledWith({ page: 1, page_size: 20, sort: 'last_check_at', order: 'desc' })
      expect(router.currentRoute.value.query).toEqual({ sort: 'last_check_at', order: 'desc' })

      wrapper.vm.changeSort({ key: 'load', direction: 'desc' })
      await flushPromises()
      expect(kernelApi.getKernelAdminNodes).toHaveBeenLastCalledWith({ page: 1, page_size: 20, sort: 'cpu_usage', order: 'desc' })
      expect(router.currentRoute.value.query).toEqual({ sort: 'cpu_usage', order: 'desc' })

      await header('Address').find('button').trigger('click')
      await flushPromises()
      expect(kernelApi.getKernelAdminNodes).toHaveBeenLastCalledWith({ page: 1, page_size: 20, sort: 'host', order: 'asc' })
    })

    it('restores the sort from the URL, sends it with the filters, and drops one the API does not know', async () => {
      kernelApi.getKernelAdminNodes.mockResolvedValue({ list: rows, total: 1 })
      const { router } = await mountNodes('/admin/nodes?q=hk&sort=cpu_usage&order=asc&page=2')
      expect(kernelApi.getKernelAdminNodes).toHaveBeenLastCalledWith({ page: 2, page_size: 20, search: 'hk', sort: 'cpu_usage', order: 'asc' })
      expect(router.currentRoute.value.query).toEqual({ q: 'hk', sort: 'cpu_usage', order: 'asc', page: '2' })

      kernelApi.getKernelAdminNodes.mockClear()
      await mountNodes('/admin/nodes?sort=traffic&order=desc')
      expect(kernelApi.getKernelAdminNodes).toHaveBeenLastCalledWith({ page: 1, page_size: 20 })
    })
  })

  describe('Agent connection and certificate of the nodes in view', () => {
    // 连接 starts hidden (certificate, version and last seen take its room):
    // these tests turn it on in the table settings.
    beforeEach(() => {
      localStorage.setItem('anix.table.admin.nodes', JSON.stringify({
        hidden: ['id', 'nodeGroup', 'load', 'parent', 'traffic', 'monthlyQuota'], density: 'comfortable'
      }))
    })

    const nodes = [
      { id: 5, name: 'hk-01', host: 'hk.example', status: 1 },
      { id: 6, name: 'jp-02', host: 'jp.example', status: 1 },
      { id: 7, name: 'sg-03', host: 'sg.example', status: 2 }
    ]
    const cert = (extra = {}) => ({
      serial: 'ab12', issued_at: '2026-10-01T00:00:00Z', not_after: '2026-10-08T00:00:00Z', renew_after: '2026-10-05T16:00:00Z', revoked_at: null, ...extra
    })
    const inventory = {
      nodes: [
        { node: 'proxy-5', certificate_state: 'valid', last_certificate: cert({ renew_after: '2999-01-01T00:00:00Z', not_after: '2999-02-01T12:00:00Z' }), connection: { type: 'mtls_stream', transport: 'mtls-stream', last_seen_at: '2026-10-02T07:59:00Z' } },
        { node: 'proxy-6', certificate_state: 'revoked', last_certificate: cert({ revoked_at: '2026-10-03T12:00:00Z', revoke_reason: 'node disabled' }), connection: { type: 'legacy', transport: 'http-legacy', last_seen_at: '2026-10-02T07:59:00Z' } },
        { node: 'proxy-7', certificate_state: 'none', last_certificate: null, connection: { type: 'offline', last_seen_at: null } }
      ]
    }

    it('asks for the nodes of the page in one call after the list, and shows a chip and the certificate state', async () => {
      kernelApi.getKernelAdminNodes.mockResolvedValue({ list: nodes, total: 3 })
      kernelApi.getKernelAgentTransports.mockResolvedValue(inventory)
      const { wrapper } = await mountNodes()

      expect(kernelApi.getKernelAgentTransports).toHaveBeenCalledTimes(1)
      expect(kernelApi.getKernelAgentTransports).toHaveBeenCalledWith({ nodes: ['proxy-5', 'proxy-6', 'proxy-7'] })
      const chips = wrapper.findAll('[data-testid="node-connection"]')
      expect(chips.map(chip => chip.attributes('data-connection'))).toEqual(['mtls_stream', 'legacy', 'offline'])
      expect(chips.map(chip => chip.text())).toEqual(['mTLS stream', 'Legacy', 'Offline'])
      const certificates = wrapper.findAll('[data-testid="node-certificate"]')
      expect(certificates.map(cell => cell.attributes('data-certificate'))).toEqual(['valid', 'revoked', 'none'])
      expect(certificates[0].text()).toContain('Valid')
      expect(certificates[0].text()).toContain('until 2999-02-01')
      expect(certificates[1].text()).toContain('Revoked')
      expect(certificates[1].text()).toContain('revoked 2026-10-03')
      expect(certificates[2].text()).toBe('No certificate')
    })

    it('flags a valid certificate whose Agent did not renew in time', async () => {
      kernelApi.getKernelAdminNodes.mockResolvedValue({ list: [nodes[0]], total: 1 })
      kernelApi.getKernelAgentTransports.mockResolvedValue({ nodes: [{ node: 'proxy-5', certificate_state: 'valid', last_certificate: cert({ renew_after: '2000-01-01T00:00:00Z', not_after: '2999-02-01T12:00:00Z' }), connection: { type: 'apikey_stream' } }] })
      const { wrapper } = await mountNodes()
      expect(wrapper.find('[data-testid="node-certificate"]').text()).toContain('Renewal overdue')
    })

    it('does not hold the list up: rows show while the call is pending, and a failed call only says so', async () => {
      kernelApi.getKernelAdminNodes.mockResolvedValue({ list: nodes, total: 3 })
      let release
      kernelApi.getKernelAgentTransports.mockReturnValueOnce(new Promise((resolve) => { release = resolve }))
      const { wrapper } = await mountNodes()
      expect(wrapper.find('tbody').text()).toContain('hk-01')
      expect(wrapper.find('[data-testid="node-connection"]').exists()).toBe(false)
      release(inventory)
      await flushPromises()
      expect(wrapper.find('[data-testid="node-connection"]').exists()).toBe(true)

      kernelApi.getKernelAgentTransports.mockRejectedValueOnce(Object.assign(new Error('forbidden'), { response: { status: 403 } }))
      await wrapper.vm.loadNodes()
      await flushPromises()
      expect(wrapper.find('tbody').text()).toContain('hk-01')
      expect(wrapper.findAll('[data-testid="node-connection-unavailable"]')).toHaveLength(3)
      expect(wrapper.findAll('[data-testid="node-certificate-unavailable"]')).toHaveLength(3)
    })

    it('drops the answer for a page the administrator has already left', async () => {
      kernelApi.getKernelAdminNodes.mockResolvedValue({ list: nodes.slice(0, 1), total: 40 })
      let releaseFirst
      kernelApi.getKernelAgentTransports
        .mockReturnValueOnce(new Promise((resolve) => { releaseFirst = resolve }))
        .mockResolvedValueOnce({ nodes: [{ node: 'proxy-5', certificate_state: 'none', last_certificate: null, connection: { type: 'offline' } }] })
      const { wrapper } = await mountNodes()
      await wrapper.vm.loadNodes()
      await flushPromises()
      releaseFirst({ nodes: [{ node: 'proxy-5', certificate_state: 'valid', last_certificate: cert(), connection: { type: 'mtls_stream' } }] })
      await flushPromises()
      expect(wrapper.find('[data-testid="node-connection"]').attributes('data-connection')).toBe('offline')
    })

    it('asks for nothing when there are no nodes', async () => {
      await mountNodes()
      expect(kernelApi.getKernelAgentTransports).not.toHaveBeenCalled()
    })
  })

  describe('服务状态 of the nodes in view', () => {
    const nodes = [5, 6, 7, 8, 9, 10, 11, 12, 13].map(id => ({ id, name: `node-${id}`, host: `n${id}.example`, status: 1 }))
    const summary = (total, active, failed) => ({ total, active, failed, inactive: total - active - failed })
    const entry = (nodeID, extra = {}) => ({
      node_id: nodeID, enabled: true, reported: true, supported: true, unsupported_reason: '', stale: false,
      observed_at: '2026-10-02T07:59:00Z', summary: summary(3, 3, 0), ...extra
    })
    const answer = [
      entry(5),
      entry(6, { summary: summary(3, 1, 1) }),
      entry(7, { summary: summary(3, 2, 0) }),
      entry(8, { summary: summary(0, 0, 0) }),
      entry(9, { reported: false, supported: false, observed_at: null, summary: summary(0, 0, 0) }),
      entry(10, { supported: false, unsupported_reason: 'cgroup v1', summary: summary(0, 0, 0) }),
      entry(11, { stale: true }),
      entry(12, { enabled: false, reported: false, supported: false, observed_at: null, summary: summary(0, 0, 0) })
    ]
    const cells = wrapper => wrapper.findAll('tbody tr').map(row => row.find('[data-service-status]'))

    it('asks for the nodes of the page in one call after the list and shows a word for each state', async () => {
      kernelApi.getKernelAdminNodes.mockResolvedValue({ list: nodes, total: nodes.length })
      telemetryApi.getNodesServicesSummary.mockResolvedValue(answer)
      const { wrapper } = await mountNodes()

      expect(telemetryApi.getNodesServicesSummary).toHaveBeenCalledTimes(1)
      expect(telemetryApi.getNodesServicesSummary).toHaveBeenCalledWith([5, 6, 7, 8, 9, 10, 11, 12, 13])
      const header = wrapper.findAll('thead th').map(th => th.text())
      expect(header).toContain('Services')
      const shown = cells(wrapper)
      expect(shown.map(cell => cell.attributes('data-service-status'))).toEqual([
        'healthy', 'failed', 'partial', 'empty', 'waiting', 'unsupported', 'stale', 'none', 'none'
      ])
      expect(shown.map(cell => cell.text())).toEqual([
        '3/3 OK', '1 failed', '2/3 running', 'No services', 'Waiting', 'Unsupported', 'Stale',
        '—Service collection is off on this node', '—Service collection is off on this node'
      ])
      expect(shown[1].attributes('title')).toContain('3 services: 1 running, 1 failed')
      expect(shown[5].attributes('title')).toBe('Reason: cgroup v1')
    })

    it('never breaks the page: without the plugin host, the package or the permission the column shows a dash', async () => {
      kernelApi.getKernelAdminNodes.mockResolvedValue({ list: nodes.slice(0, 2), total: 2 })
      for (const status of [502, 403, 404]) {
        telemetryApi.getNodesServicesSummary.mockRejectedValueOnce(Object.assign(new Error(`request failed ${status}`), { response: { status } }))
        const { wrapper } = await mountNodes()
        expect(wrapper.find('tbody').text()).toContain('node-5')
        const shown = cells(wrapper)
        expect(shown.map(cell => cell.attributes('data-service-status'))).toEqual(['unavailable', 'unavailable'])
        expect(shown[0].text()).toContain('—')
        expect(shown[0].attributes('title')).toContain('Service status is unavailable')
        expect(wrapper.find('[data-testid="node-service-status"]').exists()).toBe(false)
        wrapper.unmount()
      }
    })

    it('does not hold the list up, and drops the answer for a page the administrator has already left', async () => {
      kernelApi.getKernelAdminNodes.mockResolvedValue({ list: nodes.slice(0, 1), total: 40 })
      let releaseFirst
      telemetryApi.getNodesServicesSummary
        .mockReturnValueOnce(new Promise((resolve) => { releaseFirst = resolve }))
        .mockResolvedValueOnce([entry(5, { summary: summary(2, 1, 1) })])
      const { wrapper } = await mountNodes()
      expect(wrapper.find('tbody').text()).toContain('node-5')
      expect(cells(wrapper)[0].attributes('data-service-status')).toBe('loading')
      await wrapper.vm.loadNodes()
      await flushPromises()
      releaseFirst([entry(5)])
      await flushPromises()
      expect(cells(wrapper)[0].attributes('data-service-status')).toBe('failed')
    })

    it('is a column of the table settings: it can be hidden, and stays hidden', async () => {
      kernelApi.getKernelAdminNodes.mockResolvedValue({ list: nodes.slice(0, 1), total: 1 })
      const user = userEvent.setup()
      const { wrapper } = await mountNodes()
      const headers = () => wrapper.findAll('thead th').map(th => th.text())
      expect(headers()).toContain('Services')
      await user.click(screen.getByRole('button', { name: 'Table settings' }))
      const menu = await screen.findByRole('menu')
      const toggle = within(menu).getByRole('menuitemcheckbox', { name: 'Services' })
      expect(toggle.getAttribute('aria-checked')).toBe('true')
      await user.click(toggle)
      expect(headers()).not.toContain('Services')
      expect(JSON.parse(localStorage.getItem('anix.table.admin.nodes')).hidden).toContain('serviceStatus')
      localStorage.removeItem('anix.table.admin.nodes')
    })

    it('asks for nothing when there are no nodes', async () => {
      await mountNodes()
      expect(telemetryApi.getNodesServicesSummary).not.toHaveBeenCalled()
    })
  })

  describe('nodes that wait for approval', () => {
    const rows = [
      { id: 5, name: 'hk-01', host: 'hk.example', status: 1, approval: 'approved', node_group_id: 3, node_group_name: 'HK' },
      { id: 6, name: 'hk-02', host: 'hk2.example', status: 0, approval: 'pending', node_group_id: 3, node_group_name: 'HK' }
    ]
    const group = (nodes = { live: 2, pending: 1 }) => ({
      id: 3, name: 'HK', kind: 'proxy', nodes, require_approval: true, token_expires_at: null, token_expired: false
    })

    it('shows a banner with the count of every group and a 待确认 badge on the row', async () => {
      kernelApi.getKernelAdminNodes.mockResolvedValue({ list: rows, total: 2 })
      kernelApi.listKernelNodeGroups.mockResolvedValue({ groups: [group(), { ...group({ live: 1, pending: 2 }), id: 4, name: 'edge', kind: 'forward' }] })
      const { wrapper } = await mountNodes()

      expect(wrapper.find('[data-testid="approval-banner"]').text()).toContain('3 machine(s) wait for your approval')
      const badges = wrapper.findAll('[data-testid="node-approval-pending"]')
      expect(badges).toHaveLength(1)
      expect(badges[0].text()).toBe('Awaiting approval')
      expect(badges[0].element.closest('tr').textContent).toContain('hk-02')
    })

    it('has no banner while nothing waits', async () => {
      kernelApi.getKernelAdminNodes.mockResolvedValue({ list: rows.slice(0, 1), total: 1 })
      kernelApi.listKernelNodeGroups.mockResolvedValue({ groups: [group({ live: 1, pending: 0 })] })
      const { wrapper } = await mountNodes()
      expect(wrapper.find('[data-testid="approval-banner"]').exists()).toBe(false)
      expect(wrapper.find('[data-testid="node-approval-pending"]').exists()).toBe(false)
    })

    it('filters with the chip, from the banner too, and keeps the filter in the URL', async () => {
      kernelApi.getKernelAdminNodes.mockResolvedValue({ list: rows, total: 2 })
      kernelApi.listKernelNodeGroups.mockResolvedValue({ groups: [group()] })
      const { wrapper, router } = await mountNodes()

      await wrapper.find('[data-testid="approval-banner-view"]').trigger('click')
      await flushPromises()
      expect(kernelApi.getKernelAdminNodes).toHaveBeenLastCalledWith({ page: 1, page_size: 20, approval: 'pending' })
      expect(router.currentRoute.value.query).toEqual({ status: 'awaiting' })

      const { router: reloaded } = await mountNodes('/admin/nodes?status=awaiting&q=hk')
      expect(kernelApi.getKernelAdminNodes).toHaveBeenLastCalledWith({ page: 1, page_size: 20, search: 'hk', approval: 'pending' })
      expect(reloaded.currentRoute.value.query).toEqual({ q: 'hk', status: 'awaiting' })
    })

    it('sends the banner to the groups when only forward nodes wait', async () => {
      kernelApi.getKernelAdminNodes.mockResolvedValue({ list: rows.slice(0, 1), total: 1 })
      kernelApi.listKernelNodeGroups.mockResolvedValue({
        groups: [group({ live: 2, pending: 0 }), { ...group({ live: 1, pending: 2 }), id: 4, name: 'edge', kind: 'forward' }]
      })
      const { wrapper, router } = await mountNodes()
      expect(wrapper.find('[data-testid="approval-banner"]').text()).toContain('2 machine(s)')
      await wrapper.find('[data-testid="approval-banner-view"]').trigger('click')
      await flushPromises()
      expect(router.currentRoute.value.query).toEqual({ tab: 'groups' })
      expect(wrapper.find('[data-testid="node-groups-panel"]').exists()).toBe(true)
    })

    it('approves a row from its menu and reloads the list and the groups', async () => {
      kernelApi.getKernelAdminNodes.mockResolvedValue({ list: rows, total: 2 })
      kernelApi.listKernelNodeGroups.mockResolvedValue({ groups: [group()] })
      kernelApi.approveKernelNode.mockResolvedValue({ node: 'proxy-6', approval: 'approved' })
      const user = userEvent.setup()
      await mountNodes()

      // A node that is not waiting has no such action.
      await user.click(screen.getByRole('button', { name: 'Actions for hk-01' }))
      expect(screen.queryByRole('menuitem', { name: 'Approve' })).toBeNull()
      await user.keyboard('{Escape}')

      const listCalls = kernelApi.getKernelAdminNodes.mock.calls.length
      const groupCalls = kernelApi.listKernelNodeGroups.mock.calls.length
      await user.click(screen.getByRole('button', { name: 'Actions for hk-02' }))
      await user.click(await screen.findByRole('menuitem', { name: 'Approve' }))
      await flushPromises()
      expect(kernelApi.approveKernelNode).toHaveBeenCalledWith('proxy-6')
      expect(toastMessages('success')).toEqual(['hk-02 approved'])
      expect(kernelApi.getKernelAdminNodes.mock.calls.length).toBe(listCalls + 1)
      expect(kernelApi.listKernelNodeGroups.mock.calls.length).toBe(groupCalls + 1)
    })

    it('says why an approval failed', async () => {
      kernelApi.getKernelAdminNodes.mockResolvedValue({ list: rows, total: 2 })
      kernelApi.approveKernelNode.mockRejectedValue({ response: { status: 409, data: { error: { code: 'node_not_pending', message: 'the node is not pending approval' } } } })
      const user = userEvent.setup()
      await mountNodes()
      await user.click(screen.getByRole('button', { name: 'Actions for hk-02' }))
      await user.click(await screen.findByRole('menuitem', { name: 'Approve' }))
      await flushPromises()
      expect(toastMessages('error')).toEqual(['Couldn’t do that: the node is not pending approval'])
    })
  })

  describe('the node group tab', () => {
    it('is the second tab and keeps its place in the URL', async () => {
      kernelApi.listKernelNodeGroups.mockResolvedValue({
        groups: [{ id: 3, name: 'HK', kind: 'proxy', nodes: { live: 2, pending: 0 }, require_approval: false, token_expires_at: null, token_expired: false }]
      })
      const { wrapper, router } = await mountNodes('/admin/nodes?tab=groups')
      expect(wrapper.find('[data-testid="node-groups-panel"]').text()).toContain('HK')
      expect(wrapper.find('[data-testid="node-groups-panel"]').text()).toContain('2')

      const tabs = wrapper.findAll('[role="tab"]')
      expect(tabs.map(tab => tab.text())).toEqual(expect.arrayContaining([expect.stringContaining('Nodes'), expect.stringContaining('Node groups')]))
      await tabs.find(tab => tab.text().startsWith('Nodes')).trigger('mousedown')
      await flushPromises()
      expect(router.currentRoute.value.query.tab).toBeUndefined()
      expect(wrapper.find('[data-testid="node-groups-panel"]').exists()).toBe(false)
    })
  })

  describe('columns from the Agent inventory', () => {
    const node = { id: 5, name: 'hk-01', host: 'hk.example', status: 1, server_version: 'v4.1.0', last_check_at: 1783526000, node_group_id: 3, node_group_name: 'HK' }
    const inventory = {
      nodes: [{
        node: 'proxy-5', agent_version: 'v5.0.0', last_seen_at: '2026-10-02T07:59:00Z', certificate_state: 'valid',
        last_certificate: { serial: 'ab12', not_after: '2999-02-01T12:00:00Z', renew_after: '2999-01-01T00:00:00Z' },
        connection: { type: 'mtls_stream', transport: 'mtls-stream', last_seen_at: '2026-10-02T07:59:00Z' }
      }]
    }

    it('shows the certificate, the Agent version and when it was last seen by default; the group and connection are one click away', async () => {
      kernelApi.getKernelAdminNodes.mockResolvedValue({ list: [node], total: 1 })
      kernelApi.getKernelAgentTransports.mockResolvedValue(inventory)
      const user = userEvent.setup()
      const { wrapper } = await mountNodes()

      const headers = () => wrapper.findAll('thead th').map(th => th.text())
      expect(headers()).toEqual(expect.arrayContaining(['Certificate', 'Agent version', 'Last seen']))
      expect(headers()).not.toContain('Connection')
      expect(headers()).not.toContain('Node group')
      // The Agent's own version wins over the one the node row remembers.
      expect(wrapper.find('[data-testid="node-agent-version"]').text()).toBe('v5.0.0')
      expect(wrapper.find('[data-testid="node-certificate"]').attributes('data-certificate')).toBe('valid')

      await user.click(screen.getByRole('button', { name: 'Table settings' }))
      const menu = await screen.findByRole('menu')
      await user.click(within(menu).getByRole('menuitemcheckbox', { name: 'Node group' }))
      await user.click(within(menu).getByRole('menuitemcheckbox', { name: 'Connection' }))
      expect(headers()).toEqual(expect.arrayContaining(['Node group', 'Connection']))
      expect(wrapper.find('tbody').text()).toContain('HK')
      expect(wrapper.find('[data-testid="node-connection"]').attributes('data-connection')).toBe('mtls_stream')
    })

    it('falls back to the node’s own version and last check while the inventory has nothing', async () => {
      kernelApi.getKernelAdminNodes.mockResolvedValue({ list: [node], total: 1 })
      kernelApi.getKernelAgentTransports.mockResolvedValue({ nodes: [] })
      const { wrapper } = await mountNodes()
      expect(wrapper.find('[data-testid="node-agent-version"]').text()).toBe('v4.1.0')
      expect(wrapper.find('tbody').text()).not.toContain('Never')
    })
  })
})
