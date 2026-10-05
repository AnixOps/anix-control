// The node page (UI U7): header, sections in the URL, protocols with the
// editor sheet and the template picker, logs, credentials read on demand,
// deployment, and the danger zone.
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { render, screen, waitFor, within } from '@testing-library/vue'
import userEvent from '@testing-library/user-event'
import { createMemoryHistory, createRouter } from 'vue-router'
import NodeDetail from '@/views/admin/NodeDetail.vue'
import UiHost from '@/ui/UiHost.vue'
import { toastMessages } from './helpers/feedback'

const api = vi.hoisted(() => ({
  getNode: vi.fn(),
  getNodes: vi.fn(),
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
  generateWireGuardKeypair: vi.fn(),
  getAuthKeys: vi.fn(),
  generateAuthKey: vi.fn()
}))

vi.mock('@/api/admin', () => api)

const telemetry = vi.hoisted(() => ({
  nodeHasServicesTable: vi.fn(),
  getNodeServices: vi.fn(),
  saveNodeServicesSettings: vi.fn()
}))

vi.mock('@/api/machineTelemetry', async importOriginal => ({ ...(await importOriginal()), ...telemetry }))

// The node's Agent connection and certificate (GET /api/v4/kernel/agents/transports?node=proxy-5).
const kernelApi = vi.hoisted(() => ({ getKernelAgentTransports: vi.fn() }))
vi.mock('@/api/kernel', async importOriginal => ({ ...(await importOriginal()), ...kernelApi }))

const Harness = {
  components: { NodeDetail, UiHost },
  template: '<div><NodeDetail /><UiHost /></div>'
}

const now = Math.floor(Date.now() / 1000)
const node = {
  id: 5,
  name: 'hk-01',
  host: 'hk.example.test',
  status: 1,
  rate: 1,
  sort: 0,
  tags: 'HK,IEPL',
  monthly_reset_day: 1,
  last_check_at: now - 30,
  server_version: 'v1.4.2',
  cpu_usage: 23,
  memory_usage: 41,
  disk_usage: 12,
  protocols: [{ id: 61 }]
}

async function renderPage(path = '/admin/nodes/5') {
  const router = createRouter({
    history: createMemoryHistory(),
    routes: [
      { path: '/admin/nodes', component: { template: '<div>list</div>' } },
      { path: '/admin/nodes/:id', component: Harness }
    ]
  })
  await router.push(path)
  await router.isReady()
  const result = render({ template: '<router-view />' }, { global: { plugins: [router] } })
  return { ...result, router }
}

describe('NodeDetail', () => {
  beforeEach(() => {
    for (const fn of Object.values(api)) fn.mockReset()
    vi.spyOn(console, 'error').mockImplementation(() => {})
    api.getNode.mockResolvedValue({ code: 0, data: node })
    api.getNodes.mockResolvedValue({ data: { list: [node, { id: 7, name: 'jp-01', host: 'jp.example.test' }], total: 2 } })
    api.getNodeProtocols.mockResolvedValue({ data: [{ id: 61, type: 'vless', port: 443, enable: 1, show: 1, tls: 2 }] })
    api.getProtocolTemplates.mockResolvedValue({
      data: [
        { name: 'VLESS Reality', type: 'vless', default_port: 8443, tls: 2, transport: 'tcp' },
        { name: 'Trojan', type: 'trojan', default_port: 9443, tls: 1, transport: 'tcp' }
      ]
    })
    api.getNodeLogs.mockResolvedValue({ data: { list: [{ id: 1, level: 'error', source: 'xray', message: 'listen failed', created_at: 1790000000 }], total: 1 } })
    api.getAuthKeys.mockResolvedValue({ data: [] })
    for (const fn of Object.values(telemetry)) fn.mockReset()
    telemetry.nodeHasServicesTable.mockResolvedValue(false)
    kernelApi.getKernelAgentTransports.mockReset()
    kernelApi.getKernelAgentTransports.mockResolvedValue({ nodes: [] })
  })

  afterEach(() => {
    vi.restoreAllMocks()
  })

  it('shows the node with its status, health and traffic', async () => {
    await renderPage()
    expect(await screen.findByRole('heading', { level: 1, name: 'hk-01' })).toBeTruthy()
    expect(api.getNode).toHaveBeenCalledWith('5')
    expect(screen.getByTestId('node-status').textContent).toContain('Online')
    expect(screen.getByText('v1.4.2')).toBeTruthy()
    expect(screen.getByRole('link', { name: 'Nodes' }).getAttribute('href')).toBe('/admin/nodes')
    expect(screen.getByRole('tab', { name: 'Overview', selected: true })).toBeTruthy()
  })

  describe('Agent connection and certificate', () => {
    const certificate = (extra = {}) => ({
      serial: 'ab12', issued_at: '2026-10-01T12:00:00Z', not_after: '2999-02-01T12:00:00Z', renew_after: '2999-01-20T12:00:00Z', revoked_at: null, ...extra
    })
    const entry = extra => ({ nodes: [{ node: 'proxy-5', certificate_state: 'valid', last_certificate: certificate(), connection: { type: 'mtls_stream', transport: 'mtls-stream', last_seen_at: '2026-10-02T07:59:00Z' }, ...extra }] })

    it('shows the connection, the transport, when it was last seen and the certificate dates for this node', async () => {
      kernelApi.getKernelAgentTransports.mockResolvedValue(entry())
      await renderPage()
      const group = await screen.findByTestId('node-agent')
      await waitFor(() => expect(within(group).getByTestId('node-connection').textContent).toBe('mTLS stream'))
      expect(kernelApi.getKernelAgentTransports).toHaveBeenCalledTimes(1)
      expect(kernelApi.getKernelAgentTransports).toHaveBeenCalledWith({ nodes: ['proxy-5'] })
      expect(within(group).getByRole('heading', { level: 2, name: 'Agent connection' })).toBeTruthy()
      expect(group.textContent).toContain('Last transport')
      expect(group.textContent).toContain('mTLS stream')
      expect(within(group).getByTestId('node-certificate-state').textContent).toBe('Valid')
      expect(within(group).getByTestId('node-certificate-not-after').textContent).toContain('2999-02-01')
      expect(within(group).getByTestId('node-certificate-renew-after').textContent).toContain('2999-01-20')
      expect(within(group).queryByTestId('node-certificate-revoked-at')).toBeNull()
      expect(within(group).queryByTestId('node-certificate-overdue')).toBeNull()
    })

    it('says the Agent did not renew in time when a valid certificate is past renew_after', async () => {
      kernelApi.getKernelAgentTransports.mockResolvedValue(entry({ last_certificate: certificate({ renew_after: '2000-01-01T00:00:00Z' }) }))
      await renderPage()
      const notice = await screen.findByTestId('node-certificate-overdue')
      expect(notice.textContent).toContain('has not renewed it in time')
      expect(screen.getByTestId('node-certificate-state').textContent).toBe('Renewal overdue')
    })

    it('shows when and why a revoked certificate ended', async () => {
      kernelApi.getKernelAgentTransports.mockResolvedValue(entry({
        certificate_state: 'revoked',
        last_certificate: certificate({ not_after: '2026-10-08T12:00:00Z', revoked_at: '2026-10-03T12:00:00Z', revoke_reason: 'node disabled' }),
        connection: { type: 'legacy', transport: 'http-legacy', last_seen_at: '2026-10-02T07:59:00Z' }
      }))
      await renderPage()
      expect((await screen.findByTestId('node-certificate-state')).textContent).toBe('Revoked')
      expect(screen.getByTestId('node-certificate-revoked').textContent).toContain('has to enroll again')
      expect(screen.getByTestId('node-certificate-revoked-at').textContent).toContain('2026-10-03')
      expect(screen.getByTestId('node-certificate-revoke-reason').textContent).toContain('node disabled')
      expect(screen.getByTestId('node-connection').textContent).toBe('Legacy')
    })

    it('says so when Control has no Agent record of the node', async () => {
      await renderPage()
      expect((await screen.findByTestId('node-agent-empty')).textContent).toContain('No Agent record yet')
    })

    it('does not hold the node page up, and a failed call is a row with Retry', async () => {
      const user = userEvent.setup()
      kernelApi.getKernelAgentTransports.mockRejectedValueOnce(Object.assign(new Error('forbidden'), { response: { status: 403 } }))
      await renderPage()
      expect(await screen.findByRole('heading', { level: 1, name: 'hk-01' })).toBeTruthy()
      const failed = await screen.findByTestId('node-agent-error')
      expect(failed.textContent).toContain('Couldn’t load the Agent connection')
      expect(screen.getByText('v1.4.2')).toBeTruthy()
      kernelApi.getKernelAgentTransports.mockResolvedValueOnce(entry())
      await user.click(within(failed).getByRole('button', { name: 'Retry' }))
      await waitFor(() => expect(screen.getByTestId('node-connection').textContent).toBe('mTLS stream'))
    })
  })

  it('keeps the section in the URL', async () => {
    const user = userEvent.setup()
    const { router } = await renderPage('/admin/nodes/5?section=logs')
    expect(await screen.findByRole('tab', { name: 'Logs', selected: true })).toBeTruthy()
    expect(await screen.findByText('listen failed')).toBeTruthy()
    expect(api.getNodeLogs).toHaveBeenCalledWith(5, { page: 1, page_size: 20, level: undefined, source: undefined, search: undefined })

    await user.click(screen.getByRole('tab', { name: 'Protocols' }))
    await waitFor(() => expect(router.currentRoute.value.query).toEqual({ section: 'protocols' }))
    await user.click(screen.getByRole('tab', { name: 'Overview' }))
    await waitFor(() => expect(router.currentRoute.value.query).toEqual({}))
  })

  it('shows 服务 only when the node’s machine-telemetry release reports services', async () => {
    await renderPage()
    await screen.findByRole('heading', { level: 1, name: 'hk-01' })
    await waitFor(() => expect(telemetry.nodeHasServicesTable).toHaveBeenCalledWith('5'))
    expect(screen.queryByRole('tab', { name: 'Services' })).toBeNull()
  })

  it('opens 服务 from the URL once the capability is known', async () => {
    telemetry.nodeHasServicesTable.mockResolvedValue(true)
    telemetry.getNodeServices.mockResolvedValue({
      node_id: 5, enabled: true, include: [], exclude: [], reported: true, supported: true, unsupported_reason: '', stale: false,
      observed_at: new Date().toISOString(), version: '4.1.0', window_seconds: 600,
      summary: { total: 1, failed: 0, active: 1, inactive: 0 },
      units: [{ name: 'nginx.service', active_state: 'active', sub_state: 'running', cpu_avg_percent: 1.5, cpu_peak_percent: 3, memory_bytes: 1048576, memory_peak_bytes: 2097152 }]
    })
    await renderPage('/admin/nodes/5?section=services')
    expect(await screen.findByRole('tab', { name: 'Services', selected: true })).toBeTruthy()
    expect(await screen.findByText('nginx.service')).toBeTruthy()
    expect(telemetry.getNodeServices).toHaveBeenCalledWith(5)
  })

  it('syncs from the header and reports with a toast', async () => {
    const user = userEvent.setup()
    api.syncNodeProtocol.mockResolvedValue({ data: {} })
    await renderPage()
    await user.click(await screen.findByRole('button', { name: 'Sync and reload' }))
    await waitFor(() => expect(toastMessages('success')).toEqual(['Node “hk-01” accepted the sync']))
    expect(api.syncNodeProtocol).toHaveBeenCalledWith(5)
  })

  it('manages protocols: delete after a confirmation, invalid JSON inline, templates through a picker', async () => {
    const user = userEvent.setup()
    api.deleteNodeProtocol.mockResolvedValue({ data: {} })
    api.createNodeProtocol.mockResolvedValue({ data: { id: 62 } })
    await renderPage('/admin/nodes/5?section=protocols')

    await screen.findByText('VLESS')
    await user.click(screen.getByRole('button', { name: 'Actions for VLESS :443' }))
    await user.click(await screen.findByRole('menuitem', { name: 'Delete' }))
    let confirm = await screen.findByRole('alertdialog', { name: 'Delete protocol VLESS :443?' })
    await user.click(within(confirm).getByRole('button', { name: 'Cancel' }))
    await waitFor(() => expect(screen.queryByRole('alertdialog')).toBeNull())
    expect(api.deleteNodeProtocol).not.toHaveBeenCalled()

    await user.click(screen.getByRole('button', { name: 'Actions for VLESS :443' }))
    await user.click(await screen.findByRole('menuitem', { name: 'Delete' }))
    confirm = await screen.findByRole('alertdialog')
    await user.click(within(confirm).getByRole('button', { name: 'Delete protocol' }))
    await waitFor(() => expect(api.deleteNodeProtocol).toHaveBeenCalledWith(5, 61))
    await waitFor(() => expect(toastMessages('success')).toEqual(['Protocol VLESS :443 deleted']))

    await user.click(screen.getAllByRole('button', { name: 'Add protocol' })[0])
    const sheet = await screen.findByRole('dialog', { name: 'Add protocol' })
    const editor = within(sheet).getByRole('textbox', { name: /Protocol JSON/ })
    await user.clear(editor)
    await user.type(editor, 'not json')
    await user.click(within(sheet).getByRole('button', { name: 'Add protocol' }))
    expect(within(sheet).getByTestId('protocol-form-error').textContent).toMatch(/^Invalid JSON: /)
    expect(api.createNodeProtocol).not.toHaveBeenCalled()

    await user.click(within(sheet).getByRole('button', { name: 'Load from template' }))
    const picker = await screen.findByRole('dialog', { name: 'Load from template' })
    await user.click(within(picker).getByRole('button', { name: 'Load template' }))
    expect(within(picker).getByText('Choose a template to load.')).toBeTruthy()
    await user.click(within(picker).getByRole('combobox'))
    await user.click(await screen.findByRole('option', { name: 'Trojan' }))
    await user.click(within(picker).getByRole('button', { name: 'Load template' }))
    await waitFor(() => expect(screen.queryByRole('dialog', { name: 'Load from template' })).toBeNull())
    expect(JSON.parse(editor.value)).toMatchObject({ type: 'trojan', port: 9443, tls: 1 })

    await user.click(within(sheet).getByRole('button', { name: 'Add protocol' }))
    await waitFor(() => expect(api.createNodeProtocol).toHaveBeenCalledWith(5, expect.objectContaining({ type: 'trojan', port: 9443, tls: 1, transport: 'tcp' })))
    await waitFor(() => expect(screen.queryByRole('dialog', { name: 'Add protocol' })).toBeNull())
  })

  it('reads the API key only when asked, and shows it masked', async () => {
    const user = userEvent.setup()
    api.getNodeCredentials.mockResolvedValue({ code: 0, data: { api_key: 'node-api-key', secret: 'never-shown' } })
    await renderPage('/admin/nodes/5?section=credentials')
    const reveal = await screen.findByRole('button', { name: 'Read API key' })
    expect(api.getNodeCredentials).not.toHaveBeenCalled()
    await user.click(reveal)
    const field = await screen.findByLabelText('API key', { selector: 'input' })
    expect(field.type).toBe('password')
    expect(field.value).toBe('node-api-key')
    expect(document.body.textContent).not.toContain('never-shown')
    expect(api.getNodeCredentials).toHaveBeenCalledTimes(1)
  })

  it('shows the registration key and the Agent config in Deployment', async () => {
    await renderPage('/admin/nodes/5?section=deploy')
    expect(await screen.findByTestId('agent-config')).toBeTruthy()
    expect(api.getAuthKeys).toHaveBeenCalledTimes(1)
    expect(screen.getByRole('button', { name: 'Open deployment helper' })).toBeTruthy()
  })

  it('disables a node with the edit body and status 3 after a confirmation', async () => {
    const user = userEvent.setup()
    api.updateNode.mockResolvedValue({ data: {} })
    await renderPage('/admin/nodes/5?section=danger')
    await user.click(await screen.findByRole('button', { name: 'Disable…' }))
    const confirm = await screen.findByRole('alertdialog', { name: 'Disable node hk-01?' })
    await user.click(within(confirm).getByRole('button', { name: 'Disable node' }))
    await waitFor(() => expect(api.updateNode).toHaveBeenCalledWith(5, {
      name: 'hk-01', host: 'hk.example.test', tags: 'HK,IEPL', rate: 1, sort: 0, parent_id: null, monthly_limit: null, monthly_reset_day: 1, status: 3
    }))
    await waitFor(() => expect(toastMessages('success')).toEqual(['Node hk-01 disabled']))
    expect(api.getNode).toHaveBeenCalledTimes(2)
  })

  it('deletes a node after its name is typed and returns to the list', async () => {
    const user = userEvent.setup()
    api.deleteNode.mockResolvedValue({ data: {} })
    const { router } = await renderPage('/admin/nodes/5?section=danger')
    await user.click(await screen.findByRole('button', { name: 'Delete…' }))
    const dialog = await screen.findByRole('alertdialog', { name: 'Delete node hk-01?' })
    await user.type(within(dialog).getByRole('textbox'), 'hk-01')
    await user.click(within(dialog).getByRole('button', { name: 'Delete node' }))
    await waitFor(() => expect(api.deleteNode).toHaveBeenCalledWith(5))
    await waitFor(() => expect(router.currentRoute.value.path).toBe('/admin/nodes'))
  })

  it('edits the node in a sheet with parents from the node list', async () => {
    const user = userEvent.setup()
    api.updateNode.mockResolvedValue({ data: {} })
    await renderPage()
    await user.click(await screen.findByRole('button', { name: 'Edit' }))
    const sheet = await screen.findByRole('dialog', { name: 'Edit node' })
    await waitFor(() => expect(api.getNodes).toHaveBeenCalledWith({ page: 1, page_size: 100 }))
    await user.click(within(sheet).getByRole('button', { name: 'Save changes' }))
    await waitFor(() => expect(api.updateNode).toHaveBeenCalledWith(5, expect.objectContaining({ name: 'hk-01', status: 1 })))
  })

  it('says when the node does not exist', async () => {
    api.getNode.mockRejectedValue({ response: { status: 404, data: { message: '节点不存在' } } })
    await renderPage('/admin/nodes/99')
    expect(await screen.findByText('Node #99 doesn’t exist')).toBeTruthy()
  })

  it('offers a retry when the node does not load', async () => {
    const user = userEvent.setup()
    api.getNode.mockRejectedValueOnce({ response: { status: 502, data: { message: 'bad gateway' } } })
    await renderPage()
    expect(await screen.findByText('Couldn’t load this node')).toBeTruthy()
    await user.click(screen.getByRole('button', { name: 'Try again' }))
    expect(await screen.findByRole('heading', { level: 1, name: 'hk-01' })).toBeTruthy()
  })
})
