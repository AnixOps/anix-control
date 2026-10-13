// 接入节点 (5.0 node onboarding): the one entry point for a new machine. An
// empty panel gets its default node group without input; a Control that
// cannot hand out a working command shows each failing check and its fix; the
// command comes with mirror tabs, copy, the token's expiry, a confirmed reset
// and the uninstall command; the machines that join are listed live.
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { defineComponent, h, ref } from 'vue'
import { flushPromises } from '@vue/test-utils'
import { render, screen, waitFor, within } from '@testing-library/vue'
import userEvent from '@testing-library/user-event'
import NodeJoinSheet from '@/views/admin/nodes/NodeJoinSheet.vue'
import UiHost from '@/ui/UiHost.vue'
import { toastMessages } from './helpers/feedback'

const kernel = vi.hoisted(() => ({
  getKernelInstallReadiness: vi.fn(),
  listKernelNodeGroups: vi.fn(),
  ensureKernelDefaultNodeGroup: vi.fn(),
  createKernelNodeGroup: vi.fn(),
  getKernelNodeGroupCommand: vi.fn(),
  getKernelNodeGroupNodes: vi.fn(),
  resetKernelNodeGroupToken: vi.fn(),
  approveKernelNode: vi.fn(),
  rejectKernelNode: vi.fn()
}))
vi.mock('@/api/kernel', async importOriginal => ({ ...(await importOriginal()), ...kernel }))

const admin = vi.hoisted(() => ({ getSubscriptionGroups: vi.fn() }))
vi.mock('@/api/admin', async importOriginal => ({ ...(await importOriginal()), ...admin }))

const READY = {
  ready: true,
  checks: [
    { id: 'public_url', ok: true, level: 'error', message: 'Nodes reach Control at https://ctl.example.com.', fix: '' },
    { id: 'grpc', ok: true, level: 'error', message: 'Nodes dial ctl.example.com:50051 over TLS.', fix: '' },
    { id: 'ca', ok: true, level: 'error', message: 'The built-in agent CA is on.', fix: '' }
  ]
}

const group = (extra = {}) => ({
  id: 1, name: '默认', kind: 'proxy', nodes: { live: 0, pending: 0 }, require_approval: false,
  token_expires_at: null, token_expired: false, token_reset_at: '2026-10-12T00:00:00Z', ...extra
})

const command = (token = 'anixjoin_AAAA', extra = {}) => ({
  group: group(),
  token,
  token_expires_at: null,
  agent_version: 'v4.2.0',
  commands: [
    { mirror: 'control', available: true, command: `curl -fsSL https://ctl.example.com/install.sh | sudo bash -s -- -u https://ctl.example.com -t ${token}` },
    { mirror: 'cn', available: false, note: 'agent_install.cn_mirror_url is not set', command: `curl -fsSL https://ctl.example.com/install.sh | sudo bash -s -- -u https://ctl.example.com -t ${token} --mirror cn` },
    { mirror: 'github', available: true, command: `curl -fsSL https://github.example/agent-install.sh | sudo bash -s -- -u https://ctl.example.com -t ${token} --mirror github` }
  ],
  uninstall_command: 'sudo anix-agent uninstall',
  script: { url: 'https://ctl.example.com/install.sh', signed: false },
  ...extra
})

const joined = (extra = {}) => ({
  id: 12, node: 'proxy-12', kind: 'proxy', name: 'hk-01', host: '203.0.113.5', approval: 'approved',
  online: false, last_seen: null, agent_version: 'v5.0.0', created_at: '2026-10-12T00:00:00Z', ...extra
})

// The dialog next to the host of confirmations and toasts; `onChanged` hears
// what the page would reload on.
function mountSheet({ groupId = null, onChanged = vi.fn() } = {}) {
  const open = ref(true)
  const Harness = defineComponent({
    setup: () => () => h('div', [
      h(NodeJoinSheet, { open: open.value, groupId, 'onUpdate:open': value => { open.value = value }, onChanged }),
      h(UiHost)
    ])
  })
  const view = render(Harness)
  return { ...view, open, onChanged }
}

async function sheet() {
  return screen.findByRole('dialog', { name: 'Join a node' })
}

describe('NodeJoinSheet', () => {
  beforeEach(() => {
    vi.useFakeTimers({ toFake: ['setInterval', 'clearInterval'] })
    for (const fn of [...Object.values(kernel), ...Object.values(admin)]) fn.mockReset()
    vi.spyOn(console, 'error').mockImplementation(() => {})
    kernel.getKernelInstallReadiness.mockResolvedValue(READY)
    kernel.listKernelNodeGroups.mockResolvedValue({ groups: [group()] })
    kernel.getKernelNodeGroupCommand.mockResolvedValue(command())
    kernel.getKernelNodeGroupNodes.mockResolvedValue({ nodes: [] })
    admin.getSubscriptionGroups.mockResolvedValue({ data: [{ id: 9, name: 'default' }, { id: 10, name: 'vip' }] })
  })

  afterEach(() => {
    vi.useRealTimers()
    vi.restoreAllMocks()
  })

  it('creates the default node group on a panel without one and shows its command', async () => {
    kernel.listKernelNodeGroups.mockResolvedValueOnce({ groups: [] }).mockResolvedValue({ groups: [group()] })
    kernel.ensureKernelDefaultNodeGroup.mockResolvedValue({ group: group(), created: true })
    const { onChanged } = mountSheet()

    const dialog = await sheet()
    await within(dialog).findByTestId('join-command-text')
    expect(kernel.getKernelInstallReadiness).toHaveBeenCalledTimes(1)
    expect(kernel.ensureKernelDefaultNodeGroup).toHaveBeenCalledTimes(1)
    // Nothing was asked of the administrator: the group's command is there.
    expect(kernel.getKernelNodeGroupCommand).toHaveBeenCalledWith(1)
    expect(within(dialog).getByTestId('join-command-text').textContent).toContain('-u https://ctl.example.com -t anixjoin_AAAA')
    expect(within(dialog).getByTestId('join-default-created').textContent).toContain('“默认”')
    expect(within(dialog).getByTestId('join-token-expiry').textContent).toContain('does not expire')
    expect(within(dialog).getByTestId('join-uninstall-command').textContent).toContain('sudo anix-agent uninstall')
    expect(within(dialog).getByTestId('join-waiting').textContent).toContain('Waiting for a machine to join…')
    // The page behind learns that a group exists now.
    expect(onChanged).toHaveBeenCalledTimes(1)
  })

  it('does not create a group when the panel already has one, and opens the group asked for', async () => {
    kernel.listKernelNodeGroups.mockResolvedValue({ groups: [group(), group({ id: 2, name: 'edge', kind: 'forward' })] })
    kernel.getKernelNodeGroupCommand.mockImplementation(async id => command(`anixjoin_${id}`))
    mountSheet({ groupId: 2 })

    const dialog = await sheet()
    await waitFor(() => expect(within(dialog).getByTestId('join-command-text').textContent).toContain('anixjoin_2'))
    expect(kernel.ensureKernelDefaultNodeGroup).not.toHaveBeenCalled()
    expect(kernel.getKernelNodeGroupCommand).toHaveBeenCalledWith(2)
    expect(kernel.getKernelNodeGroupCommand).not.toHaveBeenCalledWith(1)
  })

  it('shows each failing check with its fix instead of a command, and checks again on request', async () => {
    kernel.getKernelInstallReadiness.mockResolvedValueOnce({
      ready: false,
      checks: [
        { id: 'public_url', ok: false, level: 'error', message: 'Nodes would use http://10.0.0.5:8080, which is not https.', fix: 'Set agent_install.public_url' },
        { id: 'grpc', ok: false, level: 'error', message: 'The gRPC listener is off.', fix: 'Set grpc.enabled' },
        { id: 'ca', ok: true, level: 'error', message: 'The built-in agent CA is on.', fix: '' },
        { id: 'script_signature', ok: false, level: 'warning', message: 'does not verify', fix: 'x' }
      ]
    })
    const user = userEvent.setup({ advanceTimers: vi.advanceTimersByTime })
    mountSheet()

    const dialog = await sheet()
    const blocked = await within(dialog).findByTestId('join-blocked')
    const checks = within(blocked).getAllByRole('listitem')
    expect(checks).toHaveLength(2)
    expect(checks[0].textContent).toContain('Control’s address')
    expect(checks[0].textContent).toContain('agent_install.public_url to the https address')
    expect(checks[0].textContent).toContain('which is not https')
    expect(checks[1].textContent).toContain('gRPC address and certificate')
    // No command, and nothing was created or read for one.
    expect(within(dialog).queryByTestId('join-command-text')).toBeNull()
    expect(kernel.listKernelNodeGroups).not.toHaveBeenCalled()
    expect(kernel.ensureKernelDefaultNodeGroup).not.toHaveBeenCalled()
    expect(kernel.getKernelNodeGroupCommand).not.toHaveBeenCalled()

    await user.click(within(dialog).getByRole('button', { name: 'Check again' }))
    await within(dialog).findByTestId('join-command-text')
    expect(kernel.getKernelInstallReadiness).toHaveBeenCalledTimes(2)
    expect(within(dialog).queryByTestId('join-blocked')).toBeNull()
  })

  it('shows a warning above the command when only a non-blocking check fails', async () => {
    kernel.getKernelInstallReadiness.mockResolvedValue({
      ready: true,
      checks: [{ id: 'script_signature', ok: false, level: 'warning', message: 'The configured install script signature does not verify', fix: 'x' }]
    })
    mountSheet()
    const dialog = await sheet()
    await within(dialog).findByTestId('join-command-text')
    expect(within(dialog).getByTestId('join-warning-script_signature').textContent).toContain('Install script signature')
  })

  it('says so when the readiness check itself fails, and tries again', async () => {
    kernel.getKernelInstallReadiness.mockRejectedValueOnce({ response: { status: 502, data: { error: { code: 'upstream', message: 'bad gateway' } } } })
    const user = userEvent.setup({ advanceTimers: vi.advanceTimersByTime })
    mountSheet()
    const dialog = await sheet()
    expect((await within(dialog).findByTestId('join-error')).textContent).toContain('bad gateway')
    await user.click(within(dialog).getByRole('button', { name: 'Try again' }))
    await within(dialog).findByTestId('join-command-text')
  })

  it('switches mirrors, notes one that is not set up and shows when the token ends', async () => {
    kernel.getKernelNodeGroupCommand.mockResolvedValue(command('anixjoin_AAAA', { token_expires_at: '2999-01-02T03:04:05Z' }))
    const user = userEvent.setup({ advanceTimers: vi.advanceTimersByTime })
    mountSheet()
    const dialog = await sheet()
    const text = await within(dialog).findByTestId('join-command-text')
    expect(text.textContent).not.toContain('--mirror')
    expect(within(dialog).getByTestId('join-token-expiry').textContent).toContain('2999')

    await user.click(within(dialog).getByRole('button', { name: 'GitHub' }))
    expect(within(dialog).getByTestId('join-command-text').textContent).toContain('--mirror github')
    await user.click(within(dialog).getByRole('button', { name: 'Mainland mirror' }))
    expect(within(dialog).getByTestId('join-command-text').textContent).toContain('--mirror cn')
    expect(within(dialog).getByTestId('join-command-fallback').textContent).toContain('agent_install.cn_mirror_url is not set')
  })

  it('tells a staff administrator that only a super administrator can see the command', async () => {
    kernel.getKernelNodeGroupCommand.mockRejectedValue({ response: { status: 403, data: { error: { code: 'super_admin_required', message: 'only a super administrator may show a node group\'s join token' } } } })
    mountSheet()
    const dialog = await sheet()
    expect((await within(dialog).findByTestId('join-command-forbidden')).textContent).toContain('super administrator')
    expect(within(dialog).queryByTestId('join-command-text')).toBeNull()
    expect(within(dialog).queryByTestId('join-reset-token')).toBeNull()
    // The list of machines does not need the token.
    expect(within(dialog).getByTestId('join-waiting')).toBeTruthy()
  })

  it('lists machines as they join, polling every three seconds while it is open', async () => {
    const { onChanged, open } = mountSheet()
    const dialog = await sheet()
    await within(dialog).findByTestId('join-waiting')
    expect(kernel.getKernelNodeGroupNodes).toHaveBeenCalledTimes(1)

    kernel.getKernelNodeGroupNodes.mockResolvedValue({ nodes: [joined()] })
    vi.advanceTimersByTime(3000)
    await flushPromises()
    expect(kernel.getKernelNodeGroupNodes).toHaveBeenCalledTimes(2)
    const row = await within(dialog).findByText('hk-01 joined')
    const item = row.closest('li')
    expect(item.textContent).toContain('proxy-12')
    expect(item.textContent).toContain('203.0.113.5')
    expect(item.textContent).toContain('Approved')
    expect(item.textContent).toContain('Connecting')
    expect(within(dialog).queryByTestId('join-waiting')).toBeNull()
    expect(onChanged).toHaveBeenCalledTimes(1)

    // It comes online on the next poll.
    kernel.getKernelNodeGroupNodes.mockResolvedValue({ nodes: [joined({ online: true })] })
    vi.advanceTimersByTime(3000)
    await flushPromises()
    await waitFor(() => expect(within(dialog).getByText('hk-01 joined').closest('li').textContent).toContain('Online'))

    // Closing stops the polling.
    open.value = false
    await flushPromises()
    const calls = kernel.getKernelNodeGroupNodes.mock.calls.length
    vi.advanceTimersByTime(9000)
    await flushPromises()
    expect(kernel.getKernelNodeGroupNodes.mock.calls.length).toBe(calls)
  })

  it('approves a machine that waits for approval from the list', async () => {
    kernel.listKernelNodeGroups.mockResolvedValue({ groups: [group({ require_approval: true, nodes: { live: 1, pending: 1 } })] })
    kernel.getKernelNodeGroupNodes.mockResolvedValue({ nodes: [joined({ approval: 'pending' })] })
    kernel.approveKernelNode.mockResolvedValue({ node: 'proxy-12', approval: 'approved' })
    const user = userEvent.setup({ advanceTimers: vi.advanceTimersByTime })
    mountSheet()
    const dialog = await sheet()
    const item = (await within(dialog).findByText('hk-01 joined')).closest('li')
    expect(item.textContent).toContain('Awaiting approval')

    kernel.getKernelNodeGroupNodes.mockResolvedValue({ nodes: [joined({ approval: 'approved' })] })
    await user.click(within(item).getByRole('button', { name: 'Approve' }))
    await waitFor(() => expect(kernel.approveKernelNode).toHaveBeenCalledWith('proxy-12'))
    await waitFor(() => expect(toastMessages('success')).toEqual(['hk-01 approved']))
    await waitFor(() => expect(within(dialog).getByText('hk-01 joined').closest('li').textContent).toContain('Approved'))
  })

  it('resets the token only after a confirmation, and shows the new command', async () => {
    kernel.getKernelNodeGroupCommand.mockResolvedValueOnce(command('anixjoin_OLD')).mockResolvedValue(command('anixjoin_NEW'))
    kernel.resetKernelNodeGroupToken.mockResolvedValue(group())
    const user = userEvent.setup({ advanceTimers: vi.advanceTimersByTime })
    mountSheet()
    const dialog = await sheet()
    await waitFor(() => expect(within(dialog).getByTestId('join-command-text').textContent).toContain('anixjoin_OLD'))

    await user.click(within(dialog).getByTestId('join-reset-token'))
    let confirm = await screen.findByRole('alertdialog', { name: 'Reset the token of “默认”?' })
    expect(confirm.textContent).toContain('Every command copied before stops working')
    await user.click(within(confirm).getByRole('button', { name: 'Cancel' }))
    await waitFor(() => expect(screen.queryByRole('alertdialog')).toBeNull())
    expect(kernel.resetKernelNodeGroupToken).not.toHaveBeenCalled()

    await user.click(within(dialog).getByTestId('join-reset-token'))
    confirm = await screen.findByRole('alertdialog')
    await user.click(within(confirm).getByRole('button', { name: 'Reset token' }))
    await waitFor(() => expect(kernel.resetKernelNodeGroupToken).toHaveBeenCalledWith(1))
    await waitFor(() => expect(within(dialog).getByTestId('join-command-text').textContent).toContain('anixjoin_NEW'))
    expect(within(dialog).getByTestId('join-command-text').textContent).not.toContain('anixjoin_OLD')
    await waitFor(() => expect(toastMessages('success')).toEqual(['Token reset. Use the new command.']))
  })

  it('keeps a failed reset inside the confirmation', async () => {
    kernel.resetKernelNodeGroupToken.mockRejectedValue({ response: { status: 403, data: { error: { code: 'super_admin_required', message: 'only a super administrator may reset' } } } })
    const user = userEvent.setup({ advanceTimers: vi.advanceTimersByTime })
    mountSheet()
    const dialog = await sheet()
    await within(dialog).findByTestId('join-command-text')
    await user.click(within(dialog).getByTestId('join-reset-token'))
    const confirm = await screen.findByRole('alertdialog')
    await user.click(within(confirm).getByRole('button', { name: 'Reset token' }))
    expect((await within(confirm).findByRole('alert')).textContent).toContain('only a super administrator may reset')
  })

  it('makes a new node group inline with a name, a type and a preset, in the default subscription group', async () => {
    kernel.createKernelNodeGroup.mockResolvedValue(group({ id: 2, name: 'HK' }))
    kernel.listKernelNodeGroups.mockResolvedValueOnce({ groups: [group()] }).mockResolvedValue({ groups: [group(), group({ id: 2, name: 'HK' })] })
    kernel.getKernelNodeGroupCommand.mockImplementation(async id => command(`anixjoin_${id}`))
    const user = userEvent.setup({ advanceTimers: vi.advanceTimersByTime })
    const { onChanged } = mountSheet()
    const dialog = await sheet()
    await within(dialog).findByTestId('join-command-text')

    await user.click(within(dialog).getByTestId('join-new-group'))
    const form = await within(dialog).findByTestId('quick-group-form')
    // The name is required.
    await user.click(within(form).getByTestId('quick-group-save'))
    expect(within(form).getByText('Required')).toBeTruthy()
    expect(kernel.createKernelNodeGroup).not.toHaveBeenCalled()

    await user.type(within(form).getByRole('textbox', { name: /Node group name/ }), 'HK')
    // The seeded default subscription group starts checked.
    await waitFor(() => expect(within(form).getByRole('checkbox', { name: 'default' }).getAttribute('aria-checked')).toBe('true'))
    expect(within(form).getByRole('checkbox', { name: 'vip' }).getAttribute('aria-checked')).toBe('false')
    await user.click(within(form).getByTestId('quick-group-save'))

    await waitFor(() => expect(kernel.createKernelNodeGroup).toHaveBeenCalledTimes(1))
    const body = kernel.createKernelNodeGroup.mock.calls[0][0]
    expect(body).toMatchObject({ name: 'HK', kind: 'proxy', subscription_group_ids: [9] })
    expect(body.protocol_template).toHaveLength(1)
    expect(body.protocol_template[0]).toMatchObject({ type: 'vless', tls: 'reality', transport: 'tcp', port_range: [20000, 40000] })
    expect(body.protocol_template[0].reality.server_name).toBeTruthy()
    // The new group is the one shown, with its own command.
    await waitFor(() => expect(within(dialog).getByTestId('join-command-text').textContent).toContain('anixjoin_2'))
    expect(within(dialog).queryByTestId('quick-group-form')).toBeNull()
    expect(onChanged).toHaveBeenCalled()
  })

  it('shows the API’s error when the inline group cannot be created', async () => {
    kernel.createKernelNodeGroup.mockRejectedValue({ response: { status: 409, data: { error: { code: 'name_taken', message: 'a node group with this name exists' } } } })
    const user = userEvent.setup({ advanceTimers: vi.advanceTimersByTime })
    mountSheet()
    const dialog = await sheet()
    await within(dialog).findByTestId('join-command-text')
    await user.click(within(dialog).getByTestId('join-new-group'))
    const form = await within(dialog).findByTestId('quick-group-form')
    await user.type(within(form).getByRole('textbox', { name: /Node group name/ }), '默认')
    await user.click(within(form).getByTestId('quick-group-save'))
    expect((await within(form).findByTestId('quick-group-error')).textContent).toContain('A node group with this name exists.')
  })

  it('tells the administrator when the group’s token has expired', async () => {
    kernel.listKernelNodeGroups.mockResolvedValue({ groups: [group({ token_expired: true, token_expires_at: '2020-01-01T00:00:00Z' })] })
    mountSheet()
    const dialog = await sheet()
    expect((await within(dialog).findByTestId('join-token-expired')).textContent).toContain('expired')
  })
})
