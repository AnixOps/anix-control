// 节点组 (5.0 node onboarding): the groups behind the join command, as the
// second tab of the node page: counts, the approval switch, the token's end,
// the row actions 接入节点, 编辑 and 删除, and the edit sheet with the
// protocol template and the API's validation errors.
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { render, screen, waitFor, within } from '@testing-library/vue'
import userEvent from '@testing-library/user-event'
import { createMemoryHistory, createRouter } from 'vue-router'
import Nodes from '@/views/admin/Nodes.vue'
import UiHost from '@/ui/UiHost.vue'
import { toastMessages } from './helpers/feedback'

const admin = vi.hoisted(() => ({
  getNodeStats: vi.fn(),
  getSubscriptionGroups: vi.fn()
}))
vi.mock('@/api/admin', async importOriginal => ({ ...(await importOriginal()), ...admin }))

const kernel = vi.hoisted(() => ({
  getKernelAdminNodes: vi.fn(),
  getKernelAgentTransports: vi.fn(),
  listKernelNodeGroups: vi.fn(),
  createKernelNodeGroup: vi.fn(),
  updateKernelNodeGroup: vi.fn(),
  deleteKernelNodeGroup: vi.fn(),
  getKernelInstallReadiness: vi.fn(),
  getKernelNodeGroupCommand: vi.fn(),
  getKernelNodeGroupNodes: vi.fn()
}))
vi.mock('@/api/kernel', async importOriginal => ({ ...(await importOriginal()), ...kernel }))
vi.mock('@/api/machineTelemetry', async importOriginal => ({
  ...(await importOriginal()),
  getNodesServicesSummary: vi.fn(async () => [])
}))

const Harness = {
  components: { Nodes, UiHost },
  template: '<div><Nodes /><UiHost /></div>'
}

const hk = (extra = {}) => ({
  id: 3, name: 'HK', kind: 'proxy', max_nodes: 5, require_approval: false, rate: 1.5, tags: ['hk', 'iepl'],
  monthly_limit_bytes: 2 * 1024 ** 3, reset_day: 5, sort: 2,
  protocol_template: [{
    name: 'VLESS Reality', type: 'vless', port_range: [20000, 40000], transport: 'tcp', tls: 'reality',
    reality: { server_name: 'www.example.com', dest: 'www.example.com:443' }, settings: { flow: 'xtls-rprx-vision' }
  }],
  subscription_group_ids: [9], token_reset_at: '2026-10-12T00:00:00Z', token_expires_at: null, token_expired: false,
  nodes: { live: 4, pending: 1 }, ...extra
})
const edge = () => ({
  id: 4, name: 'edge', kind: 'forward', max_nodes: 0, require_approval: true, rate: 1, tags: [], monthly_limit_bytes: 0,
  reset_day: 1, sort: 0, protocol_template: [], subscription_group_ids: [], token_expires_at: '2999-01-02T12:00:00Z',
  token_expired: false, nodes: { live: 0, pending: 0 }
})

async function renderGroups() {
  const router = createRouter({
    history: createMemoryHistory(),
    routes: [{ path: '/admin/nodes', component: Harness }, { path: '/admin/nodes/:id', component: { template: '<div />' } }]
  })
  await router.push('/admin/nodes?tab=groups')
  await router.isReady()
  const view = render(Harness, { global: { plugins: [router] } })
  await screen.findByTestId('node-groups-panel')
  return { ...view, router }
}

async function rowAction(user, group, name) {
  await user.click(await screen.findByRole('button', { name: `Actions for ${group}` }))
  await user.click(await screen.findByRole('menuitem', { name }))
}

describe('node groups', () => {
  beforeEach(() => {
    localStorage.clear()
    for (const fn of [...Object.values(admin), ...Object.values(kernel)]) fn.mockReset()
    vi.spyOn(console, 'error').mockImplementation(() => {})
    kernel.getKernelAdminNodes.mockResolvedValue({ list: [], total: 0 })
    kernel.getKernelAgentTransports.mockResolvedValue({ nodes: [] })
    kernel.listKernelNodeGroups.mockResolvedValue({ groups: [hk(), edge()] })
    admin.getNodeStats.mockResolvedValue({ data: { total: 0, online: 0, offline: 0, pending: 0 } })
    admin.getSubscriptionGroups.mockResolvedValue({ data: [{ id: 9, name: 'default' }, { id: 10, name: 'vip' }] })
  })

  afterEach(() => {
    vi.restoreAllMocks()
  })

  it('lists the groups with their nodes, approval and token', async () => {
    await renderGroups()
    const panel = screen.getByTestId('node-groups-panel')
    const rows = within(panel).getAllByRole('row').slice(1)
    expect(rows).toHaveLength(2)
    expect(rows[0].textContent).toContain('HK')
    expect(rows[0].textContent).toContain('Regular')
    expect(rows[0].textContent).toContain('4')
    expect(rows[0].textContent).toContain('1 awaiting')
    expect(rows[0].textContent).toContain('Does not expire')
    expect(rows[1].textContent).toContain('edge')
    expect(rows[1].textContent).toContain('Relay')
    expect(rows[1].textContent).toContain('Ends 2999-01-02')
    expect(within(rows[0]).getByRole('switch').getAttribute('aria-checked')).toBe('false')
    expect(within(rows[1]).getByRole('switch').getAttribute('aria-checked')).toBe('true')
  })

  it('shows an empty state that points at Join a node', async () => {
    kernel.listKernelNodeGroups.mockResolvedValue({ groups: [] })
    await renderGroups()
    expect(await screen.findByText('No node groups yet')).toBeTruthy()
    expect(screen.getByTestId('groups-empty-join')).toBeTruthy()
  })

  it('opens the join dialog for the group of a row', async () => {
    const user = userEvent.setup()
    kernel.getKernelInstallReadiness.mockResolvedValue({ ready: true, checks: [] })
    kernel.getKernelNodeGroupCommand.mockResolvedValue({
      group: edge(), token: 'anixjoin_EDGE', token_expires_at: null, commands: [{ mirror: 'control', available: true, command: 'curl … -t anixjoin_EDGE' }],
      uninstall_command: 'sudo anix-agent uninstall'
    })
    kernel.getKernelNodeGroupNodes.mockResolvedValue({ nodes: [] })
    await renderGroups()
    await rowAction(user, 'edge', 'Join a node')
    const dialog = await screen.findByRole('dialog', { name: 'Join a node' })
    await waitFor(() => expect(within(dialog).getByTestId('join-command-text').textContent).toContain('anixjoin_EDGE'))
    expect(kernel.getKernelNodeGroupCommand).toHaveBeenCalledWith(4)
  })

  it('turns approval on with the whole group sent back, and reloads', async () => {
    const user = userEvent.setup()
    kernel.updateKernelNodeGroup.mockResolvedValue(hk({ require_approval: true }))
    await renderGroups()
    const callsBefore = kernel.listKernelNodeGroups.mock.calls.length
    await user.click(screen.getByTestId('group-approval-3'))
    await waitFor(() => expect(kernel.updateKernelNodeGroup).toHaveBeenCalledTimes(1))
    const [id, body] = kernel.updateKernelNodeGroup.mock.calls[0]
    expect(id).toBe(3)
    expect(body).toEqual({
      name: 'HK', kind: 'proxy', max_nodes: 5, require_approval: true, rate: 1.5, tags: ['hk', 'iepl'],
      monthly_limit_bytes: 2 * 1024 ** 3, reset_day: 5, sort: 2, token_expires_at: null,
      protocol_template: hk().protocol_template, subscription_group_ids: [9]
    })
    await waitFor(() => expect(toastMessages('success')).toEqual(['New nodes of “HK” now need your approval.']))
    expect(kernel.listKernelNodeGroups.mock.calls.length).toBeGreaterThan(callsBefore)
  })

  it('does not send a group whose token has ended: the sheet sets a new validity', async () => {
    const user = userEvent.setup()
    kernel.listKernelNodeGroups.mockResolvedValue({ groups: [hk({ token_expired: true, token_expires_at: '2020-01-01T00:00:00Z' })] })
    await renderGroups()
    expect(within(screen.getByTestId('node-groups-panel')).getByText('Expired')).toBeTruthy()
    await user.click(screen.getByTestId('group-approval-3'))
    await waitFor(() => expect(toastMessages('error')).toEqual(['This group’s token has expired. Set a new validity under Edit first.']))
    expect(kernel.updateKernelNodeGroup).not.toHaveBeenCalled()

    // Saving the sheet needs a choice; a new validity goes with the body.
    kernel.updateKernelNodeGroup.mockResolvedValue(hk())
    await rowAction(user, 'HK', 'Edit')
    const sheet = await screen.findByRole('dialog', { name: 'Edit node group' })
    await user.click(within(sheet).getByTestId('save-group'))
    expect(within(sheet).getByText('The token has expired: choose a new validity.')).toBeTruthy()
    expect(kernel.updateKernelNodeGroup).not.toHaveBeenCalled()
    await user.click(within(sheet).getByRole('combobox', { name: 'Token validity' }))
    await user.click(await screen.findByRole('option', { name: '30 days' }))
    await user.click(within(sheet).getByTestId('save-group'))
    await waitFor(() => expect(kernel.updateKernelNodeGroup).toHaveBeenCalledTimes(1))
    const expiry = Date.parse(kernel.updateKernelNodeGroup.mock.calls[0][1].token_expires_at)
    expect(expiry - Date.now()).toBeGreaterThan(29 * 86_400_000)
    expect(expiry - Date.now()).toBeLessThan(31 * 86_400_000)
  })

  it('creates a group with the preset template, in the default subscription group', async () => {
    const user = userEvent.setup()
    kernel.createKernelNodeGroup.mockResolvedValue(hk({ id: 5, name: 'JP' }))
    await renderGroups()
    await user.click(screen.getByTestId('add-node-group'))
    const sheet = await screen.findByRole('dialog', { name: 'New node group' })

    // A Reality protocol on a random port is there to start from.
    expect(within(sheet).getByTestId('template-protocol-0')).toBeTruthy()
    await user.click(within(sheet).getByTestId('save-group'))
    expect(within(sheet).getByText('Required')).toBeTruthy()
    expect(kernel.createKernelNodeGroup).not.toHaveBeenCalled()

    await user.type(within(sheet).getByTestId('group-name'), 'JP')
    await waitFor(() => expect(within(sheet).getByRole('checkbox', { name: 'default' }).getAttribute('aria-checked')).toBe('true'))
    await user.click(within(sheet).getByTestId('save-group'))
    await waitFor(() => expect(kernel.createKernelNodeGroup).toHaveBeenCalledTimes(1))
    const body = kernel.createKernelNodeGroup.mock.calls[0][0]
    expect(body).toMatchObject({
      name: 'JP', kind: 'proxy', max_nodes: 0, require_approval: false, rate: 1, tags: [], monthly_limit_bytes: 0, reset_day: 1,
      sort: 0, token_expires_at: null, subscription_group_ids: [9]
    })
    expect(body.protocol_template).toEqual([{
      name: 'VLESS Reality', type: 'vless', transport: 'tcp', tls: 'reality', port_range: [20000, 40000],
      reality: { server_name: 'www.microsoft.com' }, settings: { flow: 'xtls-rprx-vision' }
    }])
    await waitFor(() => expect(toastMessages('success')).toEqual(['Node group JP created']))
  })

  it('edits the defaults and the template of a group', async () => {
    const user = userEvent.setup()
    kernel.updateKernelNodeGroup.mockResolvedValue(hk())
    await renderGroups()
    await rowAction(user, 'HK', 'Edit')
    const sheet = await screen.findByRole('dialog', { name: 'Edit node group' })

    // The sheet shows what the group holds.
    expect(within(sheet).getByTestId('group-name').value).toBe('HK')
    expect(within(sheet).getByRole('textbox', { name: /Tags/ }).value).toBe('hk, iepl')
    expect(within(sheet).getByTestId('group-max-nodes').value).toBe('5')
    expect(within(sheet).getByRole('textbox', { name: /Reality site/ }).value).toBe('www.example.com')
    expect(within(sheet).getByRole('checkbox', { name: 'default' }).getAttribute('aria-checked')).toBe('true')
    expect(within(sheet).getByRole('checkbox', { name: 'vip' }).getAttribute('aria-checked')).toBe('false')

    const name = within(sheet).getByTestId('group-name')
    await user.clear(name)
    await user.type(name, 'HK 2')
    await user.click(within(sheet).getByTestId('group-require-approval'))
    await user.click(within(sheet).getByRole('checkbox', { name: 'vip' }))
    const site = within(sheet).getByRole('textbox', { name: /Reality site/ })
    await user.clear(site)
    await user.type(site, 'www.example.net')
    await user.click(within(sheet).getByTestId('template-add'))
    await user.click(within(sheet).getByTestId('save-group'))

    await waitFor(() => expect(kernel.updateKernelNodeGroup).toHaveBeenCalledTimes(1))
    const [id, body] = kernel.updateKernelNodeGroup.mock.calls[0]
    expect(id).toBe(3)
    expect(body).toMatchObject({
      name: 'HK 2', kind: 'proxy', require_approval: true, subscription_group_ids: [9, 10], max_nodes: 5, tags: ['hk', 'iepl'],
      monthly_limit_bytes: 2 * 1024 ** 3, reset_day: 5, token_expires_at: null
    })
    expect(body.protocol_template).toHaveLength(2)
    expect(body.protocol_template[0].reality).toEqual({ server_name: 'www.example.net', dest: 'www.example.com:443' })
    // The second protocol is the one added: Shadowsocks 2022 on a random port.
    expect(body.protocol_template[1]).toMatchObject({ type: 'shadowsocks', transport: 'tcp', tls: 'none', port_range: [20000, 40000], settings: { method: '2022-blake3-aes-128-gcm' } })
    await waitFor(() => expect(toastMessages('success')).toEqual(['Node group HK 2 saved']))
  })

  it('shows the API’s validation error in the sheet and keeps it open', async () => {
    const user = userEvent.setup()
    kernel.updateKernelNodeGroup.mockRejectedValue({
      response: { status: 400, data: { error: { code: 'invalid_request', message: 'invalid node group: protocol_template[0]: reality.server_name must be a host name' } } }
    })
    await renderGroups()
    await rowAction(user, 'HK', 'Edit')
    const sheet = await screen.findByRole('dialog', { name: 'Edit node group' })
    await user.click(within(sheet).getByTestId('save-group'))
    const error = await within(sheet).findByTestId('group-form-error')
    expect(error.textContent).toContain('protocol_template[0]: reality.server_name must be a host name')
    expect(screen.getByRole('dialog', { name: 'Edit node group' })).toBeTruthy()
    expect(toastMessages('success')).toEqual([])
  })

  it('shows a name that is taken at the name', async () => {
    const user = userEvent.setup()
    kernel.createKernelNodeGroup.mockRejectedValue({ response: { status: 409, data: { error: { code: 'name_taken', message: 'a node group with this name exists' } } } })
    await renderGroups()
    await user.click(screen.getByTestId('add-node-group'))
    const sheet = await screen.findByRole('dialog', { name: 'New node group' })
    await user.type(within(sheet).getByTestId('group-name'), 'HK')
    await user.click(within(sheet).getByTestId('save-group'))
    expect(await within(sheet).findByText('A node group with this name exists.')).toBeTruthy()
  })

  it('stops a template that is not valid JSON or a port before it is sent', async () => {
    const user = userEvent.setup()
    await renderGroups()
    await rowAction(user, 'HK', 'Edit')
    const sheet = await screen.findByRole('dialog', { name: 'Edit node group' })
    await user.click(within(sheet).getByText('Advanced'))
    const settings = within(sheet).getByRole('textbox', { name: 'Protocol settings (JSON)' })
    await user.clear(settings)
    await user.type(settings, 'not json')
    await user.click(within(sheet).getByTestId('save-group'))
    expect(await within(sheet).findByText('Not valid JSON.')).toBeTruthy()
    expect(within(sheet).getByTestId('group-form-error').textContent).toContain('Fix the protocol settings')
    expect(kernel.updateKernelNodeGroup).not.toHaveBeenCalled()
  })

  it('does not offer Reality for a protocol that cannot use it', async () => {
    const user = userEvent.setup()
    await renderGroups()
    await user.click(screen.getByTestId('add-node-group'))
    const sheet = await screen.findByRole('dialog', { name: 'New node group' })
    const protocol = within(sheet).getByTestId('template-protocol-0')
    await user.click(within(protocol).getByRole('combobox', { name: 'Protocol' }))
    await user.click(await screen.findByRole('option', { name: 'VMess' }))
    await waitFor(() => expect(within(protocol).queryByRole('textbox', { name: /Reality site/ })).toBeNull())
    await user.click(within(protocol).getByRole('combobox', { name: 'TLS' }))
    expect(await screen.findByRole('option', { name: 'No TLS' })).toBeTruthy()
    expect(screen.queryByRole('option', { name: /Reality/ })).toBeNull()
  })

  it('deletes a group after a confirmation, and says why a group with nodes stays', async () => {
    const user = userEvent.setup()
    kernel.deleteKernelNodeGroup
      .mockRejectedValueOnce({ response: { status: 409, data: { error: { code: 'node_group_in_use', message: 'the node group has live nodes: delete or move them first' } } } })
      .mockResolvedValueOnce({ id: 3, deleted: true })
    await renderGroups()

    await rowAction(user, 'HK', 'Delete')
    let dialog = await screen.findByRole('alertdialog', { name: 'Delete node group HK?' })
    await user.click(within(dialog).getByRole('button', { name: 'Cancel' }))
    await waitFor(() => expect(screen.queryByRole('alertdialog')).toBeNull())
    expect(kernel.deleteKernelNodeGroup).not.toHaveBeenCalled()

    await rowAction(user, 'HK', 'Delete')
    dialog = await screen.findByRole('alertdialog')
    await user.click(within(dialog).getByRole('button', { name: 'Delete node group' }))
    expect((await within(dialog).findByRole('alert')).textContent).toContain('The group still has nodes')
    expect(kernel.deleteKernelNodeGroup).toHaveBeenCalledWith(3)

    kernel.listKernelNodeGroups.mockResolvedValue({ groups: [edge()] })
    await user.click(within(dialog).getByRole('button', { name: 'Delete node group' }))
    await waitFor(() => expect(screen.queryByRole('alertdialog')).toBeNull())
    await waitFor(() => expect(toastMessages('success')).toEqual(['Node group HK deleted']))
    await waitFor(() => expect(within(screen.getByTestId('node-groups-panel')).queryByText('HK')).toBeNull())
  })
})
