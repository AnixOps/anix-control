// "Create an API token" (views/admin/security/ApiTokenCreate.vue and
// useApiTokenCreate.js, POST /api/v4/kernel/api-tokens): the form, what is sent, the one-time token in the result dialog and,
// above all, that the token is gone when that dialog closes.
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { effectScope } from 'vue'
import { flushPromises } from '@vue/test-utils'
import { render, screen, waitFor, within } from '@testing-library/vue'
import userEvent from '@testing-library/user-event'
import { createPinia, setActivePinia } from 'pinia'
import ApiTokenCreate from '@/views/admin/security/ApiTokenCreate.vue'
import { useApiTokenCreate } from '@/views/admin/security/useApiTokenCreate'
import { setLocale } from '@/i18n'

const kernelApi = vi.hoisted(() => ({ createKernelApiToken: vi.fn() }))
vi.mock('@/api/kernel', () => kernelApi)

// A made-up token: it opens nothing anywhere.
const TOKEN = 'anixadm_TestTokenNotRealAbCdEfGhIjKlMnOpQrStUvWxYz0'
const NOW = Date.UTC(2026, 10, 5, 12, 0, 0)

function created(extra = {}) {
  return {
    token: TOKEN,
    api_token: {
      id: 'tok-1', user_id: 7, name: 'nightly export', scope: 'read', hint: 'Yz0', expires_at: new Date(NOW + 90 * 86_400_000).toISOString(),
      last_used_at: null, created_at: new Date(NOW).toISOString(), revoked_at: null
    },
    ...extra
  }
}

function refusal(status, code, message) {
  return { response: { status, data: { error: { code, message } } }, message: `Request failed with status code ${status}` }
}

describe('useApiTokenCreate', () => {
  let scope
  const t = (key, params = {}) => `${key}${params.message ? `:${params.message}` : ''}`

  beforeEach(() => {
    vi.resetAllMocks()
    kernelApi.createKernelApiToken.mockResolvedValue(created())
  })

  afterEach(() => {
    scope?.stop()
  })

  async function setup(onCreated) {
    scope = effectScope()
    const create = scope.run(() => useApiTokenCreate({ t, onCreated }))
    create.open()
    await flushPromises()
    return create
  }

  function fill(create, extra = {}) {
    Object.assign(create.form, { name: 'nightly export', ...extra })
  }

  it('sends the name, scope and expiry, and keeps the token in result only', async () => {
    const onCreated = vi.fn()
    const create = await setup(onCreated)
    fill(create, { name: '  nightly export  ', scope: 'admin', expiry: '30' })
    expect(await create.submit()).toEqual({ ok: true, focus: '' })
    expect(kernelApi.createKernelApiToken).toHaveBeenCalledWith({ name: '  nightly export  ', scope: 'admin', expiresInDays: 30 })
    expect(create.stage.value).toBe('result')
    expect(create.result.value.token).toBe(TOKEN)
    expect(create.result.value.record).toMatchObject({ id: 'tok-1', name: 'nightly export', scope: 'read', hint: 'Yz0' })
    // The form is back to its defaults.
    expect(create.form).toEqual({ name: '', scope: 'read', expiry: '90', customDays: '' })
    // The page is told about the record, never the token.
    expect(onCreated).toHaveBeenCalledTimes(1)
    expect(JSON.stringify(onCreated.mock.calls)).not.toContain(TOKEN)
    expect(onCreated.mock.calls[0][0]).toMatchObject({ id: 'tok-1', hint: 'Yz0' })
  })

  it('sends 90 days for the default, the typed days for custom, and no expiry for never', async () => {
    const create = await setup()
    fill(create)
    await create.submit()
    expect(kernelApi.createKernelApiToken.mock.calls[0][0].expiresInDays).toBe(90)
    create.dismiss()

    create.open()
    await flushPromises()
    fill(create, { expiry: 'custom', customDays: '45' })
    await create.submit()
    expect(kernelApi.createKernelApiToken.mock.calls[1][0].expiresInDays).toBe(45)
    create.dismiss()

    create.open()
    await flushPromises()
    fill(create, { expiry: 'never' })
    expect(create.neverExpires.value).toBe(true)
    await create.submit()
    expect(kernelApi.createKernelApiToken.mock.calls[2][0].expiresInDays).toBe(0)
  })

  it('says what is missing without a request, and where to look', async () => {
    const create = await setup()
    expect(await create.submit()).toEqual({ ok: false, focus: 'name' })
    expect(create.nameError.value).toBe('adminApiTokens.create.errors.name.required')
    fill(create, { expiry: 'custom', customDays: '731' })
    expect(await create.submit()).toEqual({ ok: false, focus: 'expiry' })
    expect(create.expiryError.value).toBe('adminApiTokens.create.errors.expiry')
    expect(kernelApi.createKernelApiToken).not.toHaveBeenCalled()
  })

  it('a second send while it is working does nothing', async () => {
    let release
    kernelApi.createKernelApiToken.mockReturnValueOnce(new Promise((resolve) => { release = resolve }))
    const create = await setup()
    fill(create)
    const first = create.submit()
    expect(create.busy.value).toBe(true)
    expect((await create.submit()).ok).toBe(false)
    release(created())
    expect((await first).ok).toBe(true)
    expect(kernelApi.createKernelApiToken).toHaveBeenCalledTimes(1)
  })

  describe('the token is gone when it is closed', () => {
    it('after the result is dismissed', async () => {
      const create = await setup()
      fill(create)
      await create.submit()
      expect(create.result.value.token).toBe(TOKEN)
      create.dismiss()
      expect(create.result.value).toBeNull()
      expect(create.stage.value).toBe('idle')
    })

    it('when the page goes', async () => {
      const create = await setup()
      fill(create)
      await create.submit()
      scope.stop()
      expect(create.result.value).toBeNull()
      expect(create.form.name).toBe('')
    })

    it('when the form is opened again', async () => {
      const create = await setup()
      fill(create)
      await create.submit()
      create.open()
      expect(create.result.value).toBeNull()
    })

    it('an answer that arrives after the page went is dropped, and tells nobody', async () => {
      let release
      kernelApi.createKernelApiToken.mockReturnValueOnce(new Promise((resolve) => { release = resolve }))
      const onCreated = vi.fn()
      const create = await setup(onCreated)
      fill(create)
      const pending = create.submit()
      scope.stop()
      release(created())
      expect((await pending).ok).toBe(false)
      expect(create.result.value).toBeNull()
      expect(create.stage.value).not.toBe('result')
      expect(onCreated).not.toHaveBeenCalled()
    })

    it('cancel does nothing while a request is out, and clears the form otherwise', async () => {
      let release
      kernelApi.createKernelApiToken.mockReturnValueOnce(new Promise((resolve) => { release = resolve }))
      const create = await setup()
      fill(create)
      const pending = create.submit()
      create.cancel()
      expect(create.stage.value).toBe('form')
      release(created())
      await pending
      create.dismiss()

      create.open()
      fill(create)
      create.cancel()
      expect(create.form.name).toBe('')
      expect(create.stage.value).toBe('idle')
    })

    it('an answer without a token is a failure, not an empty result', async () => {
      kernelApi.createKernelApiToken.mockResolvedValue(created({ token: '' }))
      const create = await setup()
      fill(create)
      expect((await create.submit()).ok).toBe(false)
      expect(create.result.value).toBeNull()
      expect(create.stage.value).toBe('form')
      expect(create.error.value).toContain('errors.failed')
    })
  })

  describe('what the route refuses', () => {
    async function refused(error) {
      kernelApi.createKernelApiToken.mockRejectedValueOnce(error)
      const create = await setup()
      fill(create)
      const outcome = await create.submit()
      return { create, outcome }
    }

    it('"sign in again" is a form-level error that offers a new sign-in', async () => {
      const { create, outcome } = await refused(refusal(403, 'step_up_sign_in_stale', 'sign in again and retry within 10 minutes'))
      expect(outcome.focus).toBe('')
      expect(create.error.value).toBe('adminApiTokens.create.errors.signInAgain')
      expect(create.signInAgain.value).toBe(true)
      expect(create.stage.value).toBe('form')
      // Trying again clears it with the next attempt.
      kernelApi.createKernelApiToken.mockResolvedValueOnce(created())
      await create.submit()
      expect(create.signInAgain.value).toBe(false)
    })

    it('25 active tokens, a demoted owner, a bad request and anything else are said once, above the buttons', async () => {
      let { create } = await refused(refusal(409, 'too_many_tokens', 'an administrator may hold at most 25 active API tokens: revoke one first'))
      expect(create.error.value).toBe('adminApiTokens.create.errors.too_many_tokens')
      scope.stop()
      ;({ create } = await refused(refusal(403, 'not_an_administrator', 'the token owner is not an administrator')))
      expect(create.error.value).toBe('adminApiTokens.create.errors.not_an_administrator')
      scope.stop()
      ;({ create } = await refused(refusal(400, 'invalid_request', 'scope must be read or admin')))
      expect(create.error.value).toBe('adminApiTokens.create.errors.invalid_request:scope must be read or admin')
      scope.stop()
      ;({ create } = await refused(new Error('Network Error')))
      expect(create.error.value).toBe('adminApiTokens.create.errors.failed:Network Error')
      expect(create.stage.value).toBe('form')
    })
  })
})

describe('ApiTokenCreate', () => {
  let writeText
  let pinia
  const sinks = {}

  beforeEach(async () => {
    vi.resetAllMocks()
    pinia = createPinia()
    setActivePinia(pinia)
    await setLocale('en')
    kernelApi.createKernelApiToken.mockResolvedValue(created())
    writeText = vi.fn().mockResolvedValue(undefined)
    // Everything the secret must never reach.
    localStorage.setItem.mockClear()
    sessionStorage.setItem.mockClear()
    for (const name of ['log', 'info', 'warn', 'error', 'debug']) sinks[name] = vi.spyOn(console, name).mockImplementation(() => {})
  })

  afterEach(() => {
    vi.restoreAllMocks()
  })

  // user-event puts its own clipboard on `navigator` when it is set up, so the
  // stub goes in after it.
  function startUser() {
    const user = userEvent.setup()
    Object.defineProperty(navigator, 'clipboard', { value: { writeText }, configurable: true })
    return user
  }

  const leaked = () => {
    const seen = []
    for (const spy of [localStorage.setItem, sessionStorage.setItem, ...Object.values(sinks)]) {
      for (const call of spy.mock.calls) seen.push(JSON.stringify(call))
    }
    return seen.some(text => text.includes(TOKEN)) || window.location.href.includes(TOKEN)
  }

  // Every place on the page a token could be: markup, text and field values.
  const onPage = () => document.body.innerHTML.includes(TOKEN) ||
    document.body.textContent.includes(TOKEN) ||
    [...document.querySelectorAll('input, textarea')].some(field => field.value.includes(TOKEN))

  async function openForm(user) {
    await user.click(screen.getByRole('button', { name: 'Create token' }))
    return screen.findByRole('dialog', { name: 'Create an API token' })
  }

  async function createToken(user, { name = 'nightly export' } = {}) {
    const form = await openForm(user)
    await user.type(within(form).getByRole('textbox', { name: /^Name/ }), name)
    await user.click(within(form).getByRole('button', { name: 'Create token' }))
    return screen.findByRole('dialog', { name: 'Your new API token' })
  }

  it('explains each scope: read is GET and HEAD only and not the two clear-secret reads', async () => {
    const user = startUser()
    render(ApiTokenCreate)
    const form = await openForm(user)
    const read = within(form).getByRole('radio', { name: 'Read' })
    expect(read.getAttribute('aria-checked')).toBe('true')
    expect(form.textContent).toMatch(/GET and HEAD requests/)
    expect(form.textContent).toMatch(/can’t read the Telegram bot token/)
    expect(form.textContent).toMatch(/Everything you may do on the administrator APIs/)
    expect(form.textContent).toMatch(/can’t manage API tokens/)
    // 90 days is the choice, and the recommendation.
    expect(within(form).getByRole('combobox', { name: 'Expires' }).textContent).toContain('90 days (recommended)')
    expect(kernelApi.createKernelApiToken).not.toHaveBeenCalled()
  })

  it('sends nothing, and says what is missing, until the name is there', async () => {
    const user = startUser()
    render(ApiTokenCreate)
    const form = await openForm(user)
    await user.click(within(form).getByRole('button', { name: 'Create token' }))
    expect(within(form).getByText('Enter a name.')).toBeTruthy()
    expect(document.activeElement).toBe(within(form).getByRole('textbox', { name: /^Name/ }))
    expect(within(form).queryByLabelText(/password/i)).toBeNull()
    expect(kernelApi.createKernelApiToken).not.toHaveBeenCalled()
  })

  it('sends the choices and shows the token once, masked, with a warning and a usage hint that has no token in it', async () => {
    const user = startUser()
    const { emitted } = render(ApiTokenCreate)
    const form = await openForm(user)
    await user.type(within(form).getByRole('textbox', { name: /^Name/ }), 'nightly export')
    await user.click(within(form).getByRole('radio', { name: 'Admin' }))
    await user.click(within(form).getByRole('combobox', { name: 'Expires' }))
    await user.click(await screen.findByRole('option', { name: '30 days' }))
    await user.click(within(form).getByRole('button', { name: 'Create token' }))

    const result = await screen.findByRole('dialog', { name: 'Your new API token' })
    expect(kernelApi.createKernelApiToken).toHaveBeenCalledWith({ name: 'nightly export', scope: 'admin', expiresInDays: 30 })
    const field = within(result).getByLabelText('API token')
    expect(field.value).toBe(TOKEN)
    expect(field.type).toBe('password')
    expect(within(result).getByTestId('api-token-warning').textContent).toMatch(/Copy it now.*can’t show it again/)
    expect(within(result).getByTestId('api-token-facts').textContent).toContain('nightly export')
    // The hint shows how to call the API: a header, a placeholder, never a URL.
    const usage = within(result).getByTestId('api-token-usage').textContent
    expect(usage).toContain('Authorization: Bearer $ANIXOPS_TOKEN')
    expect(usage).not.toContain(TOKEN)
    expect(within(result).getByTestId('api-token-no-url').textContent).toMatch(/Never put it in a URL/)
    // Revealing shows it, in that one field; copying writes the clipboard.
    await user.click(within(result).getByRole('button', { name: 'Show value' }))
    expect(field.type).toBe('text')
    await user.click(within(result).getByRole('button', { name: 'Copy token' }))
    expect(writeText).toHaveBeenCalledWith(TOKEN)
    // The page learns of the record, not the token.
    expect(emitted().created).toHaveLength(1)
    expect(JSON.stringify(emitted().created)).not.toContain(TOKEN)
    expect(leaked()).toBe(false)
  })

  it('falls back to selecting the text when the browser refuses to copy', async () => {
    const user = startUser()
    writeText.mockRejectedValue(new Error('denied'))
    document.execCommand = vi.fn(() => false)
    render(ApiTokenCreate)
    const result = await createToken(user)
    await user.click(within(result).getByRole('button', { name: 'Copy token' }))
    await waitFor(() => expect(within(result).getByLabelText('API token').type).toBe('text'))
    expect(within(result).getByText('Could not copy automatically. The text is selected; press Ctrl+C or ⌘C to copy it.')).toBeTruthy()
    expect(leaked()).toBe(false)
    delete document.execCommand
  })

  describe('the token is gone when the dialog closes', () => {
    it('after Done: nowhere on the page, in storage, in the URL or in a log, and the next form is empty', async () => {
      const user = startUser()
      const { container } = render(ApiTokenCreate)
      const result = await createToken(user)
      expect(onPage()).toBe(true)
      await user.click(within(result).getByRole('button', { name: 'Done' }))
      await waitFor(() => expect(screen.queryByRole('dialog')).toBeNull())
      expect(onPage()).toBe(false)
      expect(container.innerHTML).not.toContain(TOKEN)
      // Focus is back on the button that opened it.
      await waitFor(() => expect(document.activeElement).toBe(screen.getByRole('button', { name: 'Create token' })))
      // The store and the logs never had it.
      expect(JSON.stringify(pinia.state.value)).not.toContain(TOKEN)
      expect(leaked()).toBe(false)
      // A new form starts blank: no name, scope read, 90 days.
      const form = await openForm(user)
      expect(within(form).getByRole('textbox', { name: /^Name/ }).value).toBe('')
      expect(within(form).getByRole('radio', { name: 'Read' }).getAttribute('aria-checked')).toBe('true')
      expect(onPage()).toBe(false)
    })

    it('after Esc', async () => {
      const user = startUser()
      render(ApiTokenCreate)
      await createToken(user)
      await user.keyboard('{Escape}')
      await waitFor(() => expect(screen.queryByRole('dialog')).toBeNull())
      expect(onPage()).toBe(false)
      expect(leaked()).toBe(false)
    })

    it('after the close button; a click on the scrim does not close it', async () => {
      const user = startUser()
      render(ApiTokenCreate)
      const result = await createToken(user)
      const overlay = result.parentElement
      await user.click(overlay)
      expect(screen.getByRole('dialog', { name: 'Your new API token' })).toBeTruthy()
      expect(onPage()).toBe(true)
      await user.click(within(result).getByRole('button', { name: 'Close' }))
      await waitFor(() => expect(screen.queryByRole('dialog')).toBeNull())
      expect(onPage()).toBe(false)
    })

    it('when the component is unmounted with the dialog open', async () => {
      const user = startUser()
      const { unmount } = render(ApiTokenCreate)
      await createToken(user)
      expect(onPage()).toBe(true)
      unmount()
      await waitFor(() => expect(onPage()).toBe(false))
      expect(leaked()).toBe(false)
    })

    it('an answer that comes after the component went shows nothing', async () => {
      const user = startUser()
      let release
      kernelApi.createKernelApiToken.mockReturnValueOnce(new Promise((resolve) => { release = resolve }))
      const { unmount } = render(ApiTokenCreate)
      const form = await openForm(user)
      await user.type(within(form).getByRole('textbox', { name: /^Name/ }), 'probe')
        await user.click(within(form).getByRole('button', { name: 'Create token' }))
      unmount()
      release(created())
      await flushPromises()
      expect(onPage()).toBe(false)
      expect(leaked()).toBe(false)
    })
  })

  it('says to sign in again, with a button, when the sign-in is old', async () => {
    const user = startUser()
    kernelApi.createKernelApiToken.mockRejectedValueOnce(refusal(403, 'step_up_sign_in_stale', 'sign in again and retry within 10 minutes'))
    render(ApiTokenCreate)
    const form = await openForm(user)
    await user.type(within(form).getByRole('textbox', { name: /^Name/ }), 'probe')
    await user.click(within(form).getByRole('button', { name: 'Create token' }))
    const alert = await within(form).findByRole('alert')
    expect(alert.textContent).toContain('Your sign-in is too old for this.')
    expect(within(alert).getByRole('button', { name: 'Sign in again' })).toBeTruthy()
  })

  it('names 25 active tokens in the form', async () => {
    const user = startUser()
    kernelApi.createKernelApiToken.mockRejectedValueOnce(refusal(409, 'too_many_tokens', 'at most 25'))
    render(ApiTokenCreate)
    const form = await openForm(user)
    await user.type(within(form).getByRole('textbox', { name: /^Name/ }), 'probe')
    await user.click(within(form).getByRole('button', { name: 'Create token' }))
    expect((await within(form).findByRole('alert')).textContent).toContain('You already hold 25 active API tokens. Revoke one first.')
  })

  it('is off, with the button disabled, while the limit is reached', async () => {
    render(ApiTokenCreate, { props: { disabled: true } })
    expect(screen.getByRole('button', { name: 'Create token' }).disabled).toBe(true)
  })
})
