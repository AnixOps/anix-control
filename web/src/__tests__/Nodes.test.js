// Node list (UI U7): the list template on UiDataTable, server search and
// status chips in the URL, rows that open the node page, and the
// registration key and parent-node deployment sheets.
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { enableAutoUnmount, flushPromises, mount } from '@vue/test-utils'
import { createMemoryHistory, createRouter } from 'vue-router'
import { inBody } from './helpers/feedback'
import Nodes from '@/views/admin/Nodes.vue'

const adminApi = vi.hoisted(() => ({
  getNodes: vi.fn(),
  getNode: vi.fn(),
  getNodeStats: vi.fn(),
  getNodeLogs: vi.fn(),
  createNode: vi.fn(),
  updateNode: vi.fn(),
  deleteNode: vi.fn(),
  syncNodeProtocol: vi.fn(),
  getNodeCredentials: vi.fn(),
  getNodeProtocols: vi.fn(),
  createNodeProtocol: vi.fn(),
  updateNodeProtocol: vi.fn(),
  deleteNodeProtocol: vi.fn(),
  getProtocolTemplates: vi.fn(),
  getAuthKeys: vi.fn(),
  generateAuthKey: vi.fn(),
  generateWireGuardKeypair: vi.fn()
}))

vi.mock('@/api/admin', () => adminApi)

// The Agent connection of the nodes in view (GET /api/v4/kernel/agents/transports).
const kernelApi = vi.hoisted(() => ({ getKernelAgentTransports: vi.fn() }))
vi.mock('@/api/kernel', async importOriginal => ({ ...(await importOriginal()), ...kernelApi }))

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

function parseAgentConfigSnippet(snippet) {
  return JSON.parse(String(snippet).slice(String(snippet).indexOf('\n') + 1))
}

enableAutoUnmount(afterEach)

describe('Nodes.vue', () => {
  beforeEach(() => {
    vi.resetAllMocks()
    vi.spyOn(console, 'error').mockImplementation(() => {})
    adminApi.getNodes.mockResolvedValue({ data: { list: [], total: 0 } })
    adminApi.getNodeStats.mockResolvedValue({ data: { total: 0, online: 0, offline: 0, pending: 0 } })
    adminApi.getNodeCredentials.mockResolvedValue({ data: { api_key: '' } })
    adminApi.getAuthKeys.mockResolvedValue({ data: [] })
    kernelApi.getKernelAgentTransports.mockResolvedValue({ nodes: [] })
  })

  it('stops follow-up requests and shows the error state when the nodes do not load', async () => {
    adminApi.getNodes.mockRejectedValueOnce(new Error('401'))

    const { wrapper } = await mountNodes()

    expect(adminApi.getNodes).toHaveBeenCalledTimes(1)
    expect(adminApi.getNodeStats).not.toHaveBeenCalled()
    expect(adminApi.getAuthKeys).not.toHaveBeenCalled()
    expect(wrapper.text()).toContain('Couldn’t load nodes')

    adminApi.getNodes.mockResolvedValueOnce({ data: { list: [{ id: 1, name: 'hk-01', host: 'hk.example' }], total: 1 } })
    await wrapper.findAll('button').find(button => button.text() === 'Try again').trigger('click')
    await flushPromises()
    expect(wrapper.text()).toContain('hk-01')
    expect(adminApi.getNodeStats).toHaveBeenCalledTimes(1)
  })

  it('reads the list and the stats from legacy and panel envelopes', async () => {
    adminApi.getNodes.mockResolvedValueOnce({
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

    expect(adminApi.getNodes).toHaveBeenLastCalledWith({ page: 2, page_size: 20, search: 'hk', status: 2 })

    await wrapper.findAll('[aria-pressed]').find(chip => chip.text() === 'Disabled').trigger('click')
    await flushPromises()
    expect(adminApi.getNodes).toHaveBeenLastCalledWith({ page: 1, page_size: 20, search: 'hk', status: 3 })
    expect(router.currentRoute.value.query).toEqual({ q: 'hk', status: 'disabled' })
  })

  it('opens the node page from a row', async () => {
    adminApi.getNodes.mockResolvedValue({ data: { list: [{ id: 5, name: 'hk-01', host: 'hk.example', status: 1 }], total: 1 } })
    const { wrapper, router } = await mountNodes()

    await wrapper.find('tbody tr').trigger('click')
    await flushPromises()
    expect(router.currentRoute.value.fullPath).toBe('/admin/nodes/5')
  })

  it('shows a registration key once: the list masks keys, a generated key stays shown', async () => {
    adminApi.getAuthKeys.mockResolvedValue({ data: [{ id: 3, key: '********', used: 1 }] })
    adminApi.generateAuthKey.mockResolvedValueOnce({
      code: 0,
      msg: '操作成功',
      data: { id: 4, name: 'Panel key', key: 'fresh-registration-key' },
      ts: 1783526400000
    })

    const { wrapper } = await mountNodes()
    await wrapper.find('[data-testid="open-auth-key"]').trigger('click')
    await flushPromises()

    const deploy = wrapper.vm.deploy
    expect(deploy.authKey).toBe('')
    expect(deploy.authKeyMasked).toBe(true)
    expect(inBody('[data-testid="auth-key-empty"]').text()).toContain('Hidden (********)')
    expect(parseAgentConfigSnippet(deploy.configSnippet).Nodes[0].AuthKey).toBe('<your-auth-key>')

    await inBody('[data-testid="generate-auth-key"]').trigger('click')
    await flushPromises()

    expect(adminApi.generateAuthKey).toHaveBeenCalledWith(expect.objectContaining({ expire_days: 0 }))
    expect(adminApi.generateAuthKey.mock.calls[0][0].name).toBeTruthy()
    expect(deploy.authKey).toBe('fresh-registration-key')
    // Masked in a password field with a reveal toggle.
    const field = inBody('[data-testid="auth-key-value"] input')
    expect(field.attributes('type')).toBe('password')
    expect(field.element.value).toBe('fresh-registration-key')
    expect(parseAgentConfigSnippet(deploy.configSnippet).Nodes[0].AuthKey).toBe('fresh-registration-key')

    // Reading the masked list again keeps the key this page issued.
    adminApi.getAuthKeys.mockResolvedValue({ data: [{ id: 4, key: '********', used: 0 }, { id: 3, key: '********', used: 1 }] })
    await deploy.loadAuthKeysPreview()
    expect(deploy.authKey).toBe('fresh-registration-key')
  })

  it('reads the parent nodes’ API keys into the Ansible inventory', async () => {
    adminApi.getNodes.mockResolvedValue({
      data: { list: [{ id: 9, name: 'Root Node', host: 'root.example' }, { id: 10, name: 'Child', host: 'child.example', parent_id: 9 }], total: 2 }
    })
    adminApi.getNodeCredentials.mockResolvedValueOnce({ code: 0, msg: '操作成功', data: { api_key: 'node-api-key' }, ts: 1 })

    const { wrapper } = await mountNodes()
    await wrapper.find('[data-testid="open-deploy"]').trigger('click')
    await flushPromises()

    expect(adminApi.getNodeCredentials).toHaveBeenCalledTimes(1)
    expect(adminApi.getNodeCredentials).toHaveBeenCalledWith(9)
    expect(wrapper.vm.deploy.deployRows).toHaveLength(1)
    expect(wrapper.vm.deploy.deployInventoryPreview).toContain('root-node ansible_host=root.example')
    expect(wrapper.vm.deploy.deployInventoryPreview).toContain('node_id=9 api_key=node-api-key')
    expect(inBody('[data-testid="node-deploy-sheet"]').text()).toContain('inventory.ini')
  })

  describe('sorting on the server (GET /admin/nodes sort and order)', () => {
    const rows = [{ id: 5, name: 'hk-01', host: 'hk.example', status: 1, protocols: [{ id: 1 }] }]

    it('sorts only the columns the API supports and goes back to the first page', async () => {
      adminApi.getNodes.mockResolvedValue({ data: { list: rows, total: 40 } })
      const { wrapper, router } = await mountNodes('/admin/nodes?page=2')
      const header = label => wrapper.findAll('th').find(cell => cell.text().startsWith(label))
      // Name, address and last heartbeat sort (so do the ID and the load, which
      // start hidden); the derived status, the protocol count and the two
      // Agent columns do not. Version and load are off until the table
      // settings turn them on, to make room for the two Agent columns.
      for (const label of ['Name', 'Address', 'Last heartbeat']) expect(header(label).find('button').exists()).toBe(true)
      expect(header('Load')).toBeUndefined()
      expect(header('Agent version')).toBeUndefined()
      for (const label of ['Protocols', 'Status', 'Connection', 'Certificate']) expect(header(label).find('button').exists()).toBe(false)

      await header('Last heartbeat').find('button').trigger('click')
      await flushPromises()
      // A date column starts newest first.
      expect(adminApi.getNodes).toHaveBeenLastCalledWith({ page: 1, page_size: 20, sort: 'last_check_at', order: 'desc' })
      expect(router.currentRoute.value.query).toEqual({ sort: 'last_check_at', order: 'desc' })

      wrapper.vm.changeSort({ key: 'load', direction: 'desc' })
      await flushPromises()
      expect(adminApi.getNodes).toHaveBeenLastCalledWith({ page: 1, page_size: 20, sort: 'cpu_usage', order: 'desc' })
      expect(router.currentRoute.value.query).toEqual({ sort: 'cpu_usage', order: 'desc' })

      await header('Address').find('button').trigger('click')
      await flushPromises()
      expect(adminApi.getNodes).toHaveBeenLastCalledWith({ page: 1, page_size: 20, sort: 'host', order: 'asc' })
    })

    it('restores the sort from the URL, sends it with the filters, and drops one the API does not know', async () => {
      adminApi.getNodes.mockResolvedValue({ data: { list: rows, total: 1 } })
      const { router } = await mountNodes('/admin/nodes?q=hk&sort=cpu_usage&order=asc&page=2')
      expect(adminApi.getNodes).toHaveBeenLastCalledWith({ page: 2, page_size: 20, search: 'hk', sort: 'cpu_usage', order: 'asc' })
      expect(router.currentRoute.value.query).toEqual({ q: 'hk', sort: 'cpu_usage', order: 'asc', page: '2' })

      adminApi.getNodes.mockClear()
      await mountNodes('/admin/nodes?sort=traffic&order=desc')
      expect(adminApi.getNodes).toHaveBeenLastCalledWith({ page: 1, page_size: 20 })
    })
  })

  describe('Agent connection and certificate of the nodes in view', () => {
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
      adminApi.getNodes.mockResolvedValue({ data: { list: nodes, total: 3 } })
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
      adminApi.getNodes.mockResolvedValue({ data: { list: [nodes[0]], total: 1 } })
      kernelApi.getKernelAgentTransports.mockResolvedValue({ nodes: [{ node: 'proxy-5', certificate_state: 'valid', last_certificate: cert({ renew_after: '2000-01-01T00:00:00Z', not_after: '2999-02-01T12:00:00Z' }), connection: { type: 'apikey_stream' } }] })
      const { wrapper } = await mountNodes()
      expect(wrapper.find('[data-testid="node-certificate"]').text()).toContain('Renewal overdue')
    })

    it('does not hold the list up: rows show while the call is pending, and a failed call only says so', async () => {
      adminApi.getNodes.mockResolvedValue({ data: { list: nodes, total: 3 } })
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
      adminApi.getNodes.mockResolvedValue({ data: { list: nodes.slice(0, 1), total: 40 } })
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
})
