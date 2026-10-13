import { describe, it, expect, beforeEach, afterEach, vi } from 'vitest'
import { render, screen, waitFor } from '@testing-library/vue'
import userEvent from '@testing-library/user-event'
import { createPinia, setActivePinia } from 'pinia'
import Claim from '@/views/Claim.vue'
import { useUserStore } from '@/stores/user'
import { scrollBehavior } from '@/router/scroll'
import { toastMessages } from './helpers/feedback'

const mockStatus = vi.hoisted(() => vi.fn())
const mockSubmit = vi.hoisted(() => vi.fn())
const mockLogin = vi.hoisted(() => vi.fn())
const mockPush = vi.hoisted(() => vi.fn())

vi.mock('vue-router', () => ({
  useRouter: () => ({ push: mockPush }),
}))

vi.mock('@/api/claim', () => ({
  getClaimStatus: (...args) => mockStatus(...args),
  submitClaim: (...args) => mockSubmit(...args),
  signInAfterClaim: (...args) => mockLogin(...args),
}))

const TOKEN = 'anixclaim_' + 'Ab9-_'.repeat(8) + 'xyz'

function openAt(hash) {
  window.history.replaceState({ router: 'state' }, '', `/claim${hash}`)
}

function refusal(status, code, message = '') {
  return Object.assign(new Error(code), { response: { status, data: { error: { code, message } } } })
}

async function fill(user, { email = 'owner@example.com', password = 'correct-horse-12', confirm = password } = {}) {
  await user.type(await screen.findByLabelText(/^Email/), email)
  await user.type(screen.getByLabelText(/^Password/), password)
  await user.type(screen.getByLabelText(/^Confirm password/), confirm)
  await user.click(screen.getByRole('button', { name: 'Create administrator' }))
}

describe('Claim.vue', () => {
  beforeEach(() => {
    localStorage.clear()
    setActivePinia(createPinia())
    mockStatus.mockReset().mockResolvedValue(true)
    mockSubmit.mockReset().mockResolvedValue({ user_id: 1, email: 'owner@example.com', next: '/login' })
    mockLogin.mockReset().mockResolvedValue({ code: 0, data: { token: 'admin-token', is_admin: true, user_id: 1, email: 'owner@example.com' } })
    mockPush.mockReset()
  })

  afterEach(() => {
    window.history.replaceState(null, '', '/')
  })

  it('takes the token from the fragment, removes it from the address and never puts it in a URL', async () => {
    openAt(`#${TOKEN}`)
    render(Claim)
    expect(await screen.findByRole('heading', { level: 1, name: 'Create the first administrator' })).toBeTruthy()
    // Out of the address bar and the history entry, with the router's state kept.
    expect(window.location.href).not.toContain('anixclaim_')
    expect(window.location.hash).toBe('')
    expect(window.history.state).toEqual({ router: 'state' })
    // The status call carries nothing; the token is never rendered.
    expect(mockStatus).toHaveBeenCalledWith()
    expect(document.body.innerHTML).not.toContain(TOKEN)

    await fill(userEvent.setup())
    await waitFor(() => expect(mockPush).toHaveBeenCalledWith('/admin/dashboard'))
    // The token travels only in the claim's body.
    expect(mockSubmit).toHaveBeenCalledTimes(1)
    expect(mockSubmit).toHaveBeenCalledWith({ token: TOKEN, email: 'owner@example.com', password: 'correct-horse-12' })
    // Then the ordinary sign-in with what was just chosen, without the token.
    expect(mockLogin).toHaveBeenCalledWith({ email: 'owner@example.com', password: 'correct-horse-12' })
    expect(JSON.stringify(mockLogin.mock.calls)).not.toContain('anixclaim_')
    expect(useUserStore().token).toBe('admin-token')
    expect(document.body.innerHTML).not.toContain(TOKEN)
  })

  it('goes to sign-in when signing in does not finish', async () => {
    openAt(`#${TOKEN}`)
    mockLogin.mockRejectedValue(new Error('offline'))
    render(Claim)
    await fill(userEvent.setup())
    await waitFor(() => expect(mockPush).toHaveBeenCalledWith('/login'))
    expect(toastMessages('success')).toContain('Administrator created. Sign in with your new password.')
    expect(useUserStore().token).toBe('')
  })

  it('checks the email, the password rule and the confirmation before calling the server', async () => {
    openAt(`#${TOKEN}`)
    render(Claim)
    const user = userEvent.setup()
    await user.click(await screen.findByRole('button', { name: 'Create administrator' }))
    expect(await screen.findByText('Enter your email.')).toBeTruthy()
    expect(screen.getByText('Enter your password.')).toBeTruthy()

    // Twelve characters: eleven are refused before the server is asked.
    expect(screen.getByText('At least 12 characters.')).toBeTruthy()
    await fill(user, { email: 'owner@example.com', password: '12345678901' })
    expect(await screen.findByText('Use at least 12 characters.')).toBeTruthy()
    await user.clear(screen.getByLabelText(/^Password/))
    await user.clear(screen.getByLabelText(/^Confirm password/))
    await user.type(screen.getByLabelText(/^Password/), 'correct-horse-12')
    await user.type(screen.getByLabelText(/^Confirm password/), 'correct-horse-13')
    await user.click(screen.getByRole('button', { name: 'Create administrator' }))
    expect(await screen.findByText('The passwords do not match.')).toBeTruthy()
    expect(mockSubmit).not.toHaveBeenCalled()
  })

  it('shows what the server refused', async () => {
    openAt(`#${TOKEN}`)
    render(Claim)
    const user = userEvent.setup()
    mockSubmit.mockRejectedValueOnce(refusal(422, 'weak_password', 'the password must have at least 12 characters'))
    await fill(user)
    expect(await screen.findByText('Use at least 12 characters.')).toBeTruthy()

    mockSubmit.mockRejectedValueOnce(refusal(409, 'email_taken'))
    await user.click(screen.getByRole('button', { name: 'Create administrator' }))
    expect(await screen.findByText('An account with this email already exists. Use another email.')).toBeTruthy()

    mockSubmit.mockRejectedValueOnce(refusal(429, 'rate_limited'))
    await user.click(screen.getByRole('button', { name: 'Create administrator' }))
    expect((await screen.findByRole('alert')).textContent).toContain('Too many attempts')

    mockSubmit.mockRejectedValueOnce(refusal(503, 'identity_unavailable'))
    await user.click(screen.getByRole('button', { name: 'Create administrator' }))
    expect((await screen.findByRole('alert')).textContent).toContain('Nothing was changed')
    expect(mockPush).not.toHaveBeenCalled()
  })

  it('says when the link expired or is not valid any more, with the command for a new one', async () => {
    for (const [status, code, title] of [[410, 'claim_expired', 'This link has expired'], [403, 'claim_invalid', 'This link is not valid']]) {
      openAt(`#${TOKEN}`)
      mockSubmit.mockRejectedValueOnce(refusal(status, code))
      const view = render(Claim)
      await fill(userEvent.setup())
      expect(await screen.findByRole('heading', { level: 1, name: title })).toBeTruthy()
      expect(screen.getByText('sudo anix-control admin claim-link')).toBeTruthy()
      expect(screen.queryByLabelText(/^Password/)).toBeNull()
      view.unmount()
    }
  })

  it('says a used link has nothing to claim', async () => {
    openAt(`#${TOKEN}`)
    mockSubmit.mockRejectedValueOnce(refusal(404, 'not_found'))
    render(Claim)
    await fill(userEvent.setup())
    expect(await screen.findByRole('heading', { level: 1, name: 'No claim in progress' })).toBeTruthy()
    await userEvent.setup().click(screen.getByRole('button', { name: 'Go to sign in' }))
    expect(mockPush).toHaveBeenCalledWith('/login')
  })

  it('shows a neutral message when nothing can be claimed', async () => {
    openAt(`#${TOKEN}`)
    mockStatus.mockResolvedValue(false)
    render(Claim)
    expect(await screen.findByRole('heading', { level: 1, name: 'No claim in progress' })).toBeTruthy()
    expect(screen.getByText('There is nothing to set up here. If you manage this panel, sign in.')).toBeTruthy()
    expect(screen.queryByLabelText(/^Email/)).toBeNull()
    expect(screen.queryByText(/claim-link/)).toBeNull()
  })

  it('asks for the link when the address has none', async () => {
    for (const hash of ['', '#', '#anixclaim_short', `#${TOKEN}x`, '#%E0%A4%A']) {
      openAt(hash)
      const view = render(Claim)
      expect(await screen.findByRole('heading', { level: 1, name: 'Open the claim link' })).toBeTruthy()
      expect(screen.getByText('sudo anix-control admin claim-link')).toBeTruthy()
      expect(window.location.hash).toBe('')
      view.unmount()
    }
    expect(mockSubmit).not.toHaveBeenCalled()
  })

  it('shows the form when the status cannot be read', async () => {
    openAt(`#${TOKEN}`)
    mockStatus.mockRejectedValue(new Error('offline'))
    render(Claim)
    expect(await screen.findByRole('heading', { level: 1, name: 'Create the first administrator' })).toBeTruthy()
  })

  it('does not scroll to the token as if it were an anchor', async () => {
    const claim = await scrollBehavior({ path: '/claim', hash: `#${TOKEN}`, meta: { hashIsData: true } }, null, null)
    expect(claim).toEqual({ left: 0, top: 0, behavior: 'instant' })
    const anchor = await scrollBehavior({ path: '/admin/system', hash: '#backup', meta: {} }, null, null)
    expect(anchor).toEqual({ el: '#backup', top: 72 })
  })
})
