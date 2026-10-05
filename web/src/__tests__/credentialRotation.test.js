// "Rotate Agent credentials" (views/admin/nodes/NodeRotateCredentials.vue and
// useCredentialRotation.js, POST /api/v4/kernel/agents/rotate-credentials):
// the confirmation, what is sent, the one-time credential in the result
// dialog and, above all, that the credential is gone when that dialog closes.
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { effectScope, nextTick } from 'vue'
import { render, screen, waitFor, within } from '@testing-library/vue'
import userEvent from '@testing-library/user-event'
import NodeRotateCredentials from '@/views/admin/nodes/NodeRotateCredentials.vue'
import {
  DEFAULT_ROTATE_TTL, ROTATE_REASON_MAX, ROTATE_TTL_SECONDS, agentNodeKind, useCredentialRotation, utf8Length
} from '@/views/admin/nodes/useCredentialRotation'
import { setLocale } from '@/i18n'
import i18n from '@/i18n'

const kernelApi = vi.hoisted(() => ({ rotateKernelAgentCredentials: vi.fn() }))
vi.mock('@/api/kernel', () => kernelApi)

const NOW = Date.UTC(2026, 9, 2, 8, 0, 0)
const CREDENTIAL = 'anixagt_TestCredentialNeverRealAbCdEf0123456789'

function answer(extra = {}) {
  return {
    node: 'proxy-5',
    revoked: { certificates: 1, enrollments: 2, link_certificates: 0 },
    api_key_rotated: false,
    enrollment: { id: 'e1', method: 'enrollment_credential' },
    expires_at: new Date(NOW + 3600 * 1000).toISOString(),
    credential: CREDENTIAL,
    ...extra
  }
}

function refusal(status, code, message) {
  return { response: { status, data: { error: { code, message } } }, message: `Request failed with status code ${status}` }
}

describe('rotation helpers', () => {
  it('tells a proxy node from a forward node by its inventory name', () => {
    expect(agentNodeKind('proxy-12')).toBe('proxy')
    expect(agentNodeKind('forward-3')).toBe('forward')
    expect(agentNodeKind('')).toBe('proxy')
  })

  it('counts the UTF-8 bytes the server counts (a Chinese character is three)', () => {
    expect(utf8Length('abc')).toBe(3)
    expect(utf8Length('主机')).toBe(6)
    expect(utf8Length(null)).toBe(0)
    expect(ROTATE_REASON_MAX).toBe(200)
    expect(DEFAULT_ROTATE_TTL).toBe(3600)
    expect(ROTATE_TTL_SECONDS).toEqual([3600, 21600, 86400, 604800])
  })
})

describe('useCredentialRotation', () => {
  let scope
  const t = (key, params = {}) => `${key}${params.message ? `:${params.message}` : ''}`
  const KNOWN = ['super_admin_required', 'node_not_found', 'node_disabled', 'agent_pki_disabled']
  const te = key => KNOWN.some(code => key === `admin.nodes.rotate.errors.${code}`)

  beforeEach(() => {
    vi.resetAllMocks()
    vi.useFakeTimers()
    vi.setSystemTime(NOW)
    kernelApi.rotateKernelAgentCredentials.mockResolvedValue(answer())
  })

  afterEach(() => {
    scope?.stop()
    vi.useRealTimers()
  })

  function setup(node = 'proxy-5', onRotated) {
    scope = effectScope()
    return scope.run(() => useCredentialRotation(node, { t, te, onRotated }))
  }

  it('sends what the form says, and only asks to replace the API key for a proxy node', async () => {
    const rotation = setup()
    rotation.open()
    expect(rotation.stage.value).toBe('confirm')
    rotation.form.reason = '  disk stolen '
    rotation.form.ttlSeconds = 86400
    rotation.form.rotateApiKey = true
    await rotation.submit()
    expect(kernelApi.rotateKernelAgentCredentials).toHaveBeenCalledWith({ node: 'proxy-5', rotateApiKey: true, ttlSeconds: 86400, reason: '  disk stolen ' })
    expect(rotation.stage.value).toBe('result')
    expect(rotation.result.value).toMatchObject({
      node: 'proxy-5', credential: CREDENTIAL, expiresAt: NOW + 3600 * 1000, apiKeyRotated: false, revoked: { certificates: 1, enrollments: 2, linkCertificates: 0 }
    })
    // The form is back to its defaults, the secret is not in it.
    expect(rotation.form).toEqual({ reason: '', ttlSeconds: 3600, rotateApiKey: false })
  })

  it('never asks a forward node to replace its API key, even if the flag is set', async () => {
    const rotation = setup('forward-3')
    rotation.open()
    rotation.form.rotateApiKey = true
    await rotation.submit()
    expect(kernelApi.rotateKernelAgentCredentials.mock.calls[0][0].rotateApiKey).toBe(false)
  })

  it('refuses a reason of more than 200 bytes without a request, and takes exactly 200', async () => {
    const rotation = setup()
    rotation.open()
    rotation.form.reason = 'x'.repeat(201)
    expect(rotation.reasonTooLong.value).toBe(true)
    expect(await rotation.submit()).toBe(false)
    expect(kernelApi.rotateKernelAgentCredentials).not.toHaveBeenCalled()
    // 67 Chinese characters are 201 bytes.
    rotation.form.reason = '主'.repeat(67)
    expect(rotation.reasonBytes.value).toBe(201)
    expect(rotation.reasonTooLong.value).toBe(true)
    rotation.form.reason = `  ${'x'.repeat(200)}  `
    expect(rotation.reasonTooLong.value).toBe(false)
    expect(await rotation.submit()).toBe(true)
  })

  it('clears the credential when the result is dismissed, the confirmation cancelled, or the page goes', async () => {
    const rotation = setup()
    rotation.open()
    await rotation.submit()
    expect(rotation.result.value.credential).toBe(CREDENTIAL)
    rotation.dismiss()
    expect(rotation.result.value).toBeNull()
    expect(rotation.stage.value).toBe('idle')

    rotation.open()
    await rotation.submit()
    scope.stop()
    expect(rotation.result.value).toBeNull()
  })

  it('drops an answer that arrives after the dialog was dismissed or the page is gone', async () => {
    let release
    kernelApi.rotateKernelAgentCredentials.mockReturnValueOnce(new Promise((resolve) => { release = resolve }))
    const onRotated = vi.fn()
    const rotation = setup('proxy-5', onRotated)
    rotation.open()
    const pending = rotation.submit()
    scope.stop()
    release(answer())
    await pending
    expect(rotation.result.value).toBeNull()
    expect(onRotated).not.toHaveBeenCalled()
  })

  it('tells the page that a rotation happened, never the secret', async () => {
    const onRotated = vi.fn()
    kernelApi.rotateKernelAgentCredentials.mockResolvedValue(answer({ api_key_rotated: true }))
    const rotation = setup('proxy-5', onRotated)
    rotation.open()
    await rotation.submit()
    expect(onRotated).toHaveBeenCalledWith({ node: 'proxy-5', apiKeyRotated: true })
    expect(JSON.stringify(onRotated.mock.calls)).not.toContain(CREDENTIAL)
  })

  it('does not take an answer without a credential for success', async () => {
    kernelApi.rotateKernelAgentCredentials.mockResolvedValue(answer({ credential: '' }))
    const rotation = setup()
    rotation.open()
    expect(await rotation.submit()).toBe(false)
    expect(rotation.result.value).toBeNull()
    expect(rotation.stage.value).toBe('confirm')
    expect(rotation.error.value).toContain('failed')
  })

  it('names a known refusal, keeps the dialog open, and remembers a 403', async () => {
    const rotation = setup()
    rotation.open()
    kernelApi.rotateKernelAgentCredentials.mockRejectedValueOnce(refusal(409, 'node_disabled', 'the node is disabled'))
    expect(await rotation.submit()).toBe(false)
    expect(rotation.error.value).toBe('admin.nodes.rotate.errors.node_disabled')
    expect(rotation.stage.value).toBe('confirm')
    expect(rotation.superAdmin.value).toBeNull()
    expect(rotation.busy.value).toBe(false)

    kernelApi.rotateKernelAgentCredentials.mockRejectedValueOnce(refusal(403, 'super_admin_required', 'no'))
    await rotation.submit()
    expect(rotation.superAdmin.value).toBe(false)
    // Asking again does not call the route.
    kernelApi.rotateKernelAgentCredentials.mockClear()
    expect(await rotation.submit()).toBe(false)
    expect(kernelApi.rotateKernelAgentCredentials).not.toHaveBeenCalled()
    expect(rotation.error.value).toBe('admin.nodes.rotate.errors.super_admin_required')
  })

  it('shows the route\'s message for a refusal it does not know', async () => {
    const rotation = setup()
    rotation.open()
    kernelApi.rotateKernelAgentCredentials.mockRejectedValueOnce(refusal(400, 'invalid_request', 'ttl_seconds must be between 60 and 604800 (7 days)'))
    await rotation.submit()
    expect(rotation.error.value).toBe('admin.nodes.rotate.errors.failed:ttl_seconds must be between 60 and 604800 (7 days)')
    // A 403 without the code is still a 403.
    kernelApi.rotateKernelAgentCredentials.mockRejectedValueOnce({ response: { status: 403, data: {} }, message: 'Forbidden' })
    await rotation.submit()
    expect(rotation.superAdmin.value).toBe(false)
  })

  it('a second click while it is working does nothing', async () => {
    let release
    kernelApi.rotateKernelAgentCredentials.mockReturnValueOnce(new Promise((resolve) => { release = resolve }))
    const rotation = setup()
    rotation.open()
    const first = rotation.submit()
    expect(rotation.busy.value).toBe(true)
    expect(await rotation.submit()).toBe(false)
    release(answer())
    await first
    expect(kernelApi.rotateKernelAgentCredentials).toHaveBeenCalledTimes(1)
  })
})

describe('NodeRotateCredentials', () => {
  let writeText
  const sinks = {}

  beforeEach(async () => {
    vi.resetAllMocks()
    await setLocale('en')
    kernelApi.rotateKernelAgentCredentials.mockResolvedValue(answer())
    writeText = vi.fn().mockResolvedValue(undefined)
    // Everything the secret must never reach.
    localStorage.setItem.mockClear()
    sessionStorage.setItem.mockClear()
    for (const name of ['log', 'info', 'warn', 'error', 'debug']) sinks[name] = vi.spyOn(console, name).mockImplementation(() => {})
  })

  afterEach(() => {
    vi.useRealTimers()
    vi.restoreAllMocks()
  })

  // user-event puts its own clipboard on `navigator` when it is set up, so the
  // stub goes in after it.
  function startUser(options) {
    const user = userEvent.setup(options)
    Object.defineProperty(navigator, 'clipboard', { value: { writeText }, configurable: true })
    return user
  }

  function renderRotate(props = {}) {
    return render(NodeRotateCredentials, { props: { node: 'proxy-5', nodeLabel: 'hk-01', ...props } })
  }

  async function openConfirm(user) {
    await user.click(screen.getByRole('button', { name: 'Rotate credentials…' }))
    return screen.findByRole('alertdialog', { name: 'Rotate the credentials of hk-01?' })
  }

  const leaked = () => {
    const seen = []
    for (const spy of [localStorage.setItem, sessionStorage.setItem, ...Object.values(sinks)]) {
      for (const call of spy.mock.calls) seen.push(JSON.stringify(call))
    }
    return seen.some(text => text.includes(CREDENTIAL)) || window.location.href.includes(CREDENTIAL)
  }

  it('asks first, says what is revoked, and starts on a one-hour credential with no reason', async () => {
    const user = startUser()
    renderRotate()
    const dialog = await openConfirm(user)
    expect(kernelApi.rotateKernelAgentCredentials).not.toHaveBeenCalled()
    expect(within(dialog).getByText(/Every Agent certificate, enrollment and forward link certificate of this node is revoked/)).toBeTruthy()
    expect(within(dialog).getByText(/shown once/)).toBeTruthy()
    expect(within(dialog).getByRole('combobox', { name: 'Credential lifetime' }).textContent).toContain('1 hour')
    expect(within(dialog).getByRole('textbox', { name: 'Reason' }).value).toBe('')
    expect(within(dialog).getByRole('checkbox', { name: 'Also replace the node’s API key' }).getAttribute('aria-checked')).toBe('false')
    // The danger button says what it does; Cancel has the focus.
    expect(within(dialog).getByRole('button', { name: 'Rotate credentials' })).toBeTruthy()
    expect(document.activeElement).toBe(within(dialog).getByRole('button', { name: 'Cancel' }))
    await user.click(within(dialog).getByRole('button', { name: 'Cancel' }))
    await waitFor(() => expect(screen.queryByRole('alertdialog')).toBeNull())
    expect(kernelApi.rotateKernelAgentCredentials).not.toHaveBeenCalled()
  })

  it('rotates with the defaults and shows the credential once, masked, with a copy button', async () => {
    const user = startUser()
    const { emitted } = renderRotate()
    const confirm = await openConfirm(user)
    await user.click(within(confirm).getByRole('button', { name: 'Rotate credentials' }))

    const result = await screen.findByRole('dialog', { name: 'New credential for hk-01' })
    expect(kernelApi.rotateKernelAgentCredentials).toHaveBeenCalledWith({ node: 'proxy-5', rotateApiKey: false, ttlSeconds: 3600, reason: '' })
    const field = within(result).getByLabelText('One-time credential')
    expect(field.value).toBe(CREDENTIAL)
    expect(field.type).toBe('password')
    expect(within(result).getByTestId('rotate-warning').textContent).toContain('Treat it like a password')
    expect(within(result).getByTestId('rotate-api-key-result').textContent).toContain('old key still works')
    expect(within(result).getByTestId('rotate-revoked').textContent).toMatch(/Agent certificates\s*1.*Enrollments\s*2.*Forward link certificates\s*0/)
    const guide = within(result).getByRole('link', { name: /Rotating a node’s credentials/ })
    expect(guide.getAttribute('href')).toContain('docs/guide/agent-onboarding.md#rotating-a-nodes-credentials')
    expect(guide.getAttribute('rel')).toContain('noopener')
    // There is no --reset command to show: the credential and the guide are it.
    expect(result.textContent).not.toContain('--reset')
    expect(result.textContent).not.toContain('install.sh')

    await user.click(within(result).getByRole('button', { name: 'Copy credential' }))
    expect(writeText).toHaveBeenCalledWith(CREDENTIAL)
    expect(emitted().rotated).toEqual([[{ node: 'proxy-5', apiKeyRotated: false }]])
    expect(leaked()).toBe(false)
  })

  it('falls back to showing the text, selected, when the browser refuses to copy', async () => {
    const user = startUser()
    writeText.mockRejectedValue(new Error('denied'))
    document.execCommand = vi.fn(() => false)
    renderRotate()
    await openConfirm(user)
    await user.click(screen.getByRole('button', { name: 'Rotate credentials' }))
    const result = await screen.findByRole('dialog', { name: 'New credential for hk-01' })
    await user.click(within(result).getByRole('button', { name: 'Copy credential' }))
    const field = within(result).getByLabelText('One-time credential')
    await waitFor(() => expect(field.type).toBe('text'))
    expect(within(result).getByText('Could not copy automatically. The text is selected; press Ctrl+C or ⌘C to copy it.')).toBeTruthy()
    // The credential is still only in the dialog.
    expect(leaked()).toBe(false)
    delete document.execCommand
  })

  it('sends the reason, the lifetime and the API key choice, and warns that the old key stops', async () => {
    const user = startUser()
    kernelApi.rotateKernelAgentCredentials.mockResolvedValue(answer({ api_key_rotated: true }))
    renderRotate()
    const confirm = await openConfirm(user)
    await user.type(within(confirm).getByRole('textbox', { name: 'Reason' }), 'disk of the host was stolen')
    await user.click(within(confirm).getByRole('combobox', { name: 'Credential lifetime' }))
    await user.click(await screen.findByRole('option', { name: '24 hours' }))
    const box = within(confirm).getByRole('checkbox', { name: 'Also replace the node’s API key' })
    expect(within(confirm).queryByTestId('rotate-api-key-notice')).toBeNull()
    await user.click(box)
    expect(within(confirm).getByTestId('rotate-api-key-notice').textContent).toContain('old API key stops working as soon as you confirm')

    await user.click(within(confirm).getByRole('button', { name: 'Rotate credentials' }))
    const result = await screen.findByRole('dialog', { name: 'New credential for hk-01' })
    expect(kernelApi.rotateKernelAgentCredentials).toHaveBeenCalledWith({ node: 'proxy-5', rotateApiKey: true, ttlSeconds: 86400, reason: 'disk of the host was stolen' })
    expect(within(result).getByTestId('rotate-api-key-result').textContent).toContain('API key was replaced')
  })

  it('counts the reason in bytes and does not send one that is too long', async () => {
    const user = startUser()
    renderRotate()
    const confirm = await openConfirm(user)
    const reason = within(confirm).getByRole('textbox', { name: 'Reason' })
    expect(within(confirm).getByText(/0 \/ 200 bytes/)).toBeTruthy()
    await user.click(reason)
    await user.paste('主'.repeat(67))
    expect(within(confirm).getByText('The reason is too long: 201 of 200 bytes.')).toBeTruthy()
    await user.click(within(confirm).getByRole('button', { name: 'Rotate credentials' }))
    expect(kernelApi.rotateKernelAgentCredentials).not.toHaveBeenCalled()
    expect(screen.getByRole('alertdialog')).toBeTruthy()
  })

  it('has no API key option for a forward node', async () => {
    const user = startUser()
    renderRotate({ node: 'forward-3', nodeLabel: 'relay-hk' })
    await user.click(screen.getByRole('button', { name: 'Rotate credentials…' }))
    const confirm = await screen.findByRole('alertdialog', { name: 'Rotate the credentials of relay-hk?' })
    expect(within(confirm).queryByRole('checkbox')).toBeNull()
    await user.click(within(confirm).getByRole('button', { name: 'Rotate credentials' }))
    await screen.findByRole('dialog', { name: 'New credential for relay-hk' })
    expect(kernelApi.rotateKernelAgentCredentials).toHaveBeenCalledWith({ node: 'forward-3', rotateApiKey: false, ttlSeconds: 3600, reason: '' })
  })

  it('counts down to the expiry once a second and says so when it has passed', async () => {
    const user = startUser({ advanceTimers: vi.advanceTimersByTime })
    vi.useFakeTimers({ toFake: ['Date', 'setInterval', 'clearInterval'] })
    vi.setSystemTime(NOW)
    renderRotate()
    await openConfirm(user)
    await user.click(screen.getByRole('button', { name: 'Rotate credentials' }))
    const result = await screen.findByRole('dialog', { name: 'New credential for hk-01' })
    expect(within(result).getByTestId('rotate-expiry').textContent).toMatch(/Valid until .* \(in 1h\)\. It works once\./)

    vi.setSystemTime(NOW + 60_000)
    await vi.advanceTimersByTimeAsync(1000)
    await waitFor(() => expect(within(result).getByTestId('rotate-expiry').textContent).toContain('in 58m 59s'))

    vi.setSystemTime(NOW + 3600 * 1000)
    await vi.advanceTimersByTimeAsync(1000)
    await waitFor(() => expect(within(result).getByTestId('rotate-expired').textContent).toContain('has expired'))
    expect(within(result).queryByTestId('rotate-expiry')).toBeNull()
  })

  describe('the credential is gone when the dialog closes', () => {
    async function openResult(user) {
      await openConfirm(user)
      await user.click(screen.getByRole('button', { name: 'Rotate credentials' }))
      return screen.findByRole('dialog', { name: 'New credential for hk-01' })
    }

    it('after Done: not on the page, not in the component, and the next rotation starts from nothing', async () => {
      const user = startUser()
      const { container } = renderRotate()
      const result = await openResult(user)
      expect(document.body.innerHTML).toContain(CREDENTIAL)
      await user.click(within(result).getByRole('button', { name: 'Done' }))
      await waitFor(() => expect(screen.queryByRole('dialog')).toBeNull())
      expect(document.body.innerHTML).not.toContain(CREDENTIAL)
      expect(container.innerHTML).not.toContain(CREDENTIAL)
      // The button is back where it was, and has the focus.
      await waitFor(() => expect(document.activeElement).toBe(screen.getByRole('button', { name: 'Rotate credentials…' })))
      // A new confirmation is a clean form.
      const confirm = await openConfirm(user)
      expect(within(confirm).getByRole('textbox', { name: 'Reason' }).value).toBe('')
      expect(document.body.innerHTML).not.toContain(CREDENTIAL)
      expect(leaked()).toBe(false)
    })

    it('after Esc', async () => {
      const user = startUser()
      renderRotate()
      await openResult(user)
      await user.keyboard('{Escape}')
      await waitFor(() => expect(screen.queryByRole('dialog')).toBeNull())
      expect(document.body.innerHTML).not.toContain(CREDENTIAL)
    })

    it('after the close button; a click on the scrim does not close it', async () => {
      const user = startUser()
      renderRotate()
      const result = await openResult(user)
      const overlay = result.parentElement
      await user.click(overlay)
      expect(screen.queryByRole('dialog', { name: 'New credential for hk-01' })).toBeTruthy()
      expect(document.body.innerHTML).toContain(CREDENTIAL)
      await user.click(within(result).getByRole('button', { name: 'Close' }))
      await waitFor(() => expect(screen.queryByRole('dialog')).toBeNull())
      expect(document.body.innerHTML).not.toContain(CREDENTIAL)
    })

    it('when the page goes away while it is shown', async () => {
      const user = startUser()
      const { unmount } = renderRotate()
      await openResult(user)
      expect(document.body.innerHTML).toContain(CREDENTIAL)
      unmount()
      await waitFor(() => expect(document.body.innerHTML).not.toContain(CREDENTIAL))
      expect(leaked()).toBe(false)
    })
  })

  it('shows a refusal inside the confirmation, which stays open, and turns the button into a sentence after a 403', async () => {
    const user = startUser()
    kernelApi.rotateKernelAgentCredentials.mockRejectedValue(refusal(403, 'super_admin_required', 'only a super administrator may rotate'))
    renderRotate()
    const confirm = await openConfirm(user)
    await user.click(within(confirm).getByRole('button', { name: 'Rotate credentials' }))
    expect((await within(confirm).findByRole('alert')).textContent).toBe('Only super administrators can rotate credentials.')
    expect(screen.queryByRole('dialog', { name: 'New credential for hk-01' })).toBeNull()
    await user.click(within(confirm).getByRole('button', { name: 'Cancel' }))
    await waitFor(() => expect(screen.queryByRole('alertdialog')).toBeNull())
    expect(screen.queryByRole('button', { name: 'Rotate credentials…' })).toBeNull()
    expect(screen.getByTestId('rotate-super-only').textContent).toBe('Only super administrators can rotate credentials.')
  })

  it('names a disabled node, and a node that is gone', async () => {
    const user = startUser()
    kernelApi.rotateKernelAgentCredentials.mockRejectedValueOnce(refusal(409, 'node_disabled', 'the node is disabled'))
    renderRotate()
    const confirm = await openConfirm(user)
    await user.click(within(confirm).getByRole('button', { name: 'Rotate credentials' }))
    expect((await within(confirm).findByRole('alert')).textContent).toBe('This node is disabled. Enable it first.')
    kernelApi.rotateKernelAgentCredentials.mockRejectedValueOnce(refusal(404, 'node_not_found', 'the node does not exist'))
    await user.click(within(confirm).getByRole('button', { name: 'Rotate credentials' }))
    await waitFor(() => expect(within(confirm).getByRole('alert').textContent).toBe('This node no longer exists.'))
    // Still a button: neither is a 403.
    await user.click(within(confirm).getByRole('button', { name: 'Cancel' }))
    expect(await screen.findByRole('button', { name: 'Rotate credentials…' })).toBeTruthy()
  })

  it('is a disabled button for a disabled node', () => {
    renderRotate({ disabled: true })
    expect(screen.getByRole('button', { name: 'Rotate credentials…' }).disabled).toBe(true)
  })

  it('reads in Chinese too', async () => {
    await setLocale('zh-CN')
    const user = startUser()
    renderRotate()
    await user.click(screen.getByRole('button', { name: '轮换凭据…' }))
    const confirm = await screen.findByRole('alertdialog', { name: '轮换 hk-01 的凭据？' })
    expect(within(confirm).getByRole('button', { name: '轮换凭据' })).toBeTruthy()
    await user.click(within(confirm).getByRole('button', { name: '轮换凭据' }))
    const result = await screen.findByRole('dialog', { name: 'hk-01 的新凭据' })
    expect(within(result).getByText('只显示一次。关闭此对话框后无法再次查看。')).toBeTruthy()
    await setLocale('en')
  })
})
