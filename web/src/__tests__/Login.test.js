import { describe, it, expect, beforeEach, vi } from 'vitest'
import { render, screen, waitFor, within } from '@testing-library/vue'
import userEvent from '@testing-library/user-event'
import { createPinia, setActivePinia } from 'pinia'
import Login from '@/views/Login.vue'
import { useUserStore } from '@/stores/user'
import { setEdition } from '@/composables/useEdition'
import { toastMessages } from './helpers/feedback'

const mockLogin = vi.hoisted(() => vi.fn())
const mockRegister = vi.hoisted(() => vi.fn())
const mockPush = vi.hoisted(() => vi.fn())

vi.mock('vue-router', () => ({
  useRouter: () => ({ push: mockPush }),
}))

vi.mock('@/api/auth', () => ({
  login: (...args) => mockLogin(...args),
  register: (...args) => mockRegister(...args),
}))

function ok(data) {
  return { code: 0, msg: '操作成功', ts: 1783536000000, data }
}

async function signIn(user, email = 'lin@example.com', password = 'password123') {
  await user.type(screen.getByLabelText(/^Email/), email)
  await user.type(screen.getByLabelText(/^Password/), password)
  await user.click(screen.getByRole('button', { name: 'Sign in' }))
}

function codeBoxes() {
  return screen.getAllByLabelText(/^Digit \d of 6$/)
}

describe('Login.vue', () => {
  beforeEach(() => {
    localStorage.clear()
    setActivePinia(createPinia())
    mockLogin.mockReset()
    mockRegister.mockReset()
    mockPush.mockReset()
    setEdition('community', { requireInvite: false })
  })

  it('shows one centred card: one H1, email and password, no dead links or decorative stats', async () => {
    render(Login)
    expect(screen.getAllByRole('heading', { level: 1 }).map(h => h.textContent)).toEqual(['Sign in to AnixOps Control'])
    expect(screen.getByLabelText(/^Email/).getAttribute('autocomplete')).toBe('username')
    expect(screen.getByLabelText(/^Password/).getAttribute('type')).toBe('password')
    // The password can be revealed.
    await userEvent.setup().click(screen.getByRole('button', { name: 'Show password' }))
    expect(screen.getByLabelText(/^Password/).getAttribute('type')).toBe('text')
    // No password reset endpoint, so no "forgot password" link; no stats.
    expect(screen.queryByText(/forgot/i)).toBeNull()
    expect(screen.queryByRole('link')).toBeNull()
    expect(document.body.textContent).not.toMatch(/Users\s*Users|用户管理/)
  })

  it('checks the fields before calling the server', async () => {
    const user = userEvent.setup()
    render(Login)
    await user.click(screen.getByRole('button', { name: 'Sign in' }))
    expect(await screen.findByText('Enter your email.')).toBeTruthy()
    expect(screen.getByText('Enter your password.')).toBeTruthy()
    await user.type(screen.getByLabelText(/^Email/), 'not-an-email')
    await user.click(screen.getByRole('button', { name: 'Sign in' }))
    expect(await screen.findByText('Enter an email like name@example.com.')).toBeTruthy()
    expect(mockLogin).not.toHaveBeenCalled()
  })

  it('signs in from a panel envelope and keeps the permission fields', async () => {
    mockLogin.mockResolvedValue(ok({
      token: 'login-token', is_admin: true, user_id: 9, email: 'admin@example.com',
      permission_mode: 'mixed', permissions: ['forward.view'], restricted_plugins: ['forward'],
    }))
    render(Login)
    await signIn(userEvent.setup(), 'admin@example.com')
    expect(mockLogin).toHaveBeenCalledWith({ email: 'admin@example.com', password: 'password123' })
    const store = useUserStore()
    expect(store.token).toBe('login-token')
    expect(store.userInfo).toMatchObject({ id: 9, is_admin: true, permission_mode: 'mixed', permissions: ['forward.view'], restricted_plugins: ['forward'] })
    expect(mockPush).toHaveBeenCalledWith('/admin/dashboard')
  })

  it('shows a server error in an alert and keeps the form', async () => {
    mockLogin.mockResolvedValue({ code: -1, msg: '用户不存在或密码错误', data: null })
    render(Login)
    await signIn(userEvent.setup())
    expect((await screen.findByRole('alert')).textContent).toContain('Could not sign in: 用户不存在或密码错误')
    expect(useUserStore().token).toBe('')
  })

  it('says when there were too many attempts', async () => {
    mockLogin.mockRejectedValue({ response: { status: 429, data: { msg: 'too many login attempts, please try again later' } } })
    render(Login)
    await signIn(userEvent.setup())
    expect((await screen.findByRole('alert')).textContent).toContain('Too many attempts')
  })

  it('asks for the 6-digit code as a second step and submits it when complete', async () => {
    const user = userEvent.setup()
    mockLogin
      .mockResolvedValueOnce(ok({ mfa_required: true, methods: ['totp', 'backup'], user_id: 10, email: 'mfa@example.com' }))
      .mockResolvedValueOnce(ok({ token: 'mfa-token', is_admin: false, user_id: 10, email: 'mfa@example.com' }))
    render(Login)
    await signIn(user, 'mfa@example.com')

    expect(await screen.findByRole('heading', { level: 1, name: 'Two-factor authentication' })).toBeTruthy()
    expect(screen.getByText('mfa@example.com')).toBeTruthy()
    expect(useUserStore().token).toBe('')
    await waitFor(() => expect(document.activeElement).toBe(codeBoxes()[0]))

    // Typing moves from box to box; the sixth digit submits.
    await user.keyboard('123456')
    await waitFor(() => expect(mockLogin).toHaveBeenCalledTimes(2))
    expect(mockLogin).toHaveBeenNthCalledWith(2, { email: 'mfa@example.com', password: 'password123', mfa_code: '123456', mfa_method: 'totp' })
    expect(useUserStore().token).toBe('mfa-token')
    expect(mockPush).toHaveBeenCalledWith('/user/dashboard')
  })

  it('takes a pasted code, and clears the boxes after a wrong one', async () => {
    const user = userEvent.setup()
    mockLogin
      .mockResolvedValueOnce(ok({ mfa_required: true, methods: ['totp'], email: 'mfa@example.com' }))
      .mockResolvedValueOnce({ code: -1, msg: 'invalid mfa code', data: null })
    render(Login)
    await signIn(user, 'mfa@example.com')
    await screen.findByRole('heading', { level: 1, name: 'Two-factor authentication' })

    await user.click(codeBoxes()[2])
    await user.paste('654 321')
    await waitFor(() => expect(mockLogin).toHaveBeenCalledTimes(2))
    expect(mockLogin.mock.calls[1][0]).toMatchObject({ mfa_code: '654321' })
    expect(await screen.findByText(/That code is not correct/)).toBeTruthy()
    await waitFor(() => expect(codeBoxes().map(box => box.value).join('')).toBe(''))
  })

  it('requires all six digits before verifying', async () => {
    const user = userEvent.setup()
    mockLogin.mockResolvedValueOnce(ok({ mfa_required: true, methods: ['totp'], email: 'mfa@example.com' }))
    render(Login)
    await signIn(user, 'mfa@example.com')
    await screen.findByRole('heading', { level: 1, name: 'Two-factor authentication' })
    await user.keyboard('12')
    await user.click(screen.getByRole('button', { name: 'Verify' }))
    expect(await screen.findByText('Enter all 6 digits.')).toBeTruthy()
    expect(mockLogin).toHaveBeenCalledTimes(1)
  })

  it('offers a recovery code only when the account has one', async () => {
    const user = userEvent.setup()
    mockLogin.mockResolvedValueOnce(ok({ mfa_required: true, methods: ['totp'], email: 'mfa@example.com' }))
    render(Login)
    await signIn(user, 'mfa@example.com')
    await screen.findByRole('heading', { level: 1, name: 'Two-factor authentication' })
    expect(screen.queryByRole('button', { name: 'Use a recovery code' })).toBeNull()
  })

  it('signs in with a recovery code, normalised to XXXX-XXXX', async () => {
    const user = userEvent.setup()
    mockLogin
      .mockResolvedValueOnce(ok({ mfa_required: true, methods: ['totp', 'backup'], email: 'mfa@example.com' }))
      .mockResolvedValueOnce(ok({ token: 'backup-token', is_admin: false, user_id: 10, email: 'mfa@example.com' }))
    render(Login)
    await signIn(user, 'mfa@example.com')
    await user.click(await screen.findByRole('button', { name: 'Use a recovery code' }))
    expect(await screen.findByRole('heading', { level: 1, name: 'Use a recovery code' })).toBeTruthy()

    const field = screen.getByLabelText(/^Recovery code/)
    await user.type(field, 'abc')
    await user.click(screen.getByRole('button', { name: 'Verify' }))
    expect(await screen.findByText(/A recovery code has 8 letters or digits/)).toBeTruthy()

    await user.clear(field)
    await user.type(field, 'abcd 1234')
    await user.click(screen.getByRole('button', { name: 'Verify' }))
    await waitFor(() => expect(mockLogin).toHaveBeenCalledTimes(2))
    expect(mockLogin).toHaveBeenNthCalledWith(2, { email: 'mfa@example.com', password: 'password123', mfa_code: 'ABCD-1234', mfa_method: 'backup' })
    expect(useUserStore().token).toBe('backup-token')

  })

  it('goes back from the code step to email and password', async () => {
    const user = userEvent.setup()
    mockLogin.mockResolvedValueOnce(ok({ mfa_required: true, methods: ['totp', 'backup'], email: 'mfa@example.com' }))
    render(Login)
    await signIn(user, 'mfa@example.com')
    await user.click(await screen.findByRole('button', { name: 'Back' }))
    expect(await screen.findByRole('heading', { level: 1, name: 'Sign in to AnixOps Control' })).toBeTruthy()
    expect(screen.getByLabelText(/^Password/).value).toBe('')
    expect(screen.getByLabelText(/^Email/).value).toBe('mfa@example.com')
  })

  it('explains what to do when the administrator requires two-factor authentication first', async () => {
    const user = userEvent.setup()
    mockLogin.mockResolvedValueOnce(ok({ mfa_enrollment_required: true, mfa_setup_required: true, methods: ['totp'], user_id: 12, email: 'enroll@example.com' }))
    render(Login)
    await signIn(user, 'enroll@example.com')

    const heading = await screen.findByRole('heading', { level: 1, name: 'Turn on two-factor authentication first' })
    await waitFor(() => expect(document.activeElement).toBe(heading))
    expect(screen.getByText('enroll@example.com')).toBeTruthy()
    expect(screen.getAllByRole('listitem')).toHaveLength(3)
    expect(screen.queryAllByLabelText(/^Digit/)).toHaveLength(0)
    expect(useUserStore().token).toBe('')

    await user.click(screen.getByRole('button', { name: 'Back to sign in' }))
    expect(await screen.findByRole('heading', { level: 1, name: 'Sign in to AnixOps Control' })).toBeTruthy()
  })

  it('registers without an invite code when none is required', async () => {
    const user = userEvent.setup()
    mockRegister.mockResolvedValue({ data: { token: 'registered-token', is_admin: false, user_id: 8, email: 'open@example.com' } })
    render(Login)
    await user.click(screen.getByRole('button', { name: 'Create an account' }))
    expect(await screen.findByRole('heading', { level: 1, name: 'Create your account' })).toBeTruthy()
    expect(screen.queryByLabelText(/Invite code/)).toBeNull()

    await user.type(screen.getByLabelText(/^Email/), 'open@example.com')
    await user.type(screen.getByLabelText(/^Password/), 'password123')
    await user.type(screen.getByLabelText(/^Confirm password/), 'password12')
    await user.click(screen.getByRole('button', { name: 'Create account' }))
    expect(await screen.findByText('The passwords do not match.')).toBeTruthy()
    expect(mockRegister).not.toHaveBeenCalled()

    await user.type(screen.getByLabelText(/^Confirm password/), '3')
    await user.click(screen.getByRole('button', { name: 'Create account' }))
    await waitFor(() => expect(mockRegister).toHaveBeenCalledWith({ email: 'open@example.com', password: 'password123' }))
    expect(useUserStore().token).toBe('registered-token')
    expect(toastMessages('success')).toEqual(['Account created'])
  })

  it('asks for the invite code when registration requires one', async () => {
    const user = userEvent.setup()
    setEdition('community', { requireInvite: true })
    mockRegister.mockResolvedValue({
      data: { token: 'registered-token', is_admin: false, user_id: 7, email: 'invite-user@example.com', permission_mode: 'authoritative', permissions: ['subscription.view'], restricted_plugins: ['forward'] },
    })
    render(Login)
    await user.click(screen.getByRole('button', { name: 'Create an account' }))
    await user.type(await screen.findByLabelText(/^Email/), 'invite-user@example.com')
    await user.type(screen.getByLabelText(/^Password/), 'password123')
    await user.type(screen.getByLabelText(/^Confirm password/), 'password123')
    await user.click(screen.getByRole('button', { name: 'Create account' }))
    expect(await screen.findByText('Enter your invite code.')).toBeTruthy()

    await user.type(screen.getByLabelText(/^Invite code/), 'INVITE123')
    await user.click(screen.getByRole('button', { name: 'Create account' }))
    await waitFor(() => expect(mockRegister).toHaveBeenCalledWith({ email: 'invite-user@example.com', password: 'password123', invite_code: 'INVITE123' }))
    expect(useUserStore().userInfo).toMatchObject({ permission_mode: 'authoritative', permissions: ['subscription.view'], restricted_plugins: ['forward'] })
  })

  it('hides registration when the server closed it', () => {
    setEdition('community', { registrationEnabled: false })
    render(Login)
    expect(screen.queryByRole('button', { name: 'Create an account' })).toBeNull()
  })

  it('keeps a failed registration in the form', async () => {
    const user = userEvent.setup()
    mockRegister.mockResolvedValue({ code: -1, msg: '邮箱已被注册', data: null })
    render(Login)
    await user.click(screen.getByRole('button', { name: 'Create an account' }))
    await user.type(await screen.findByLabelText(/^Email/), 'taken@example.com')
    await user.type(screen.getByLabelText(/^Password/), 'password123')
    await user.type(screen.getByLabelText(/^Confirm password/), 'password123')
    await user.click(screen.getByRole('button', { name: 'Create account' }))
    const alert = await screen.findByRole('alert')
    expect(within(alert).getByText('Could not create the account: 邮箱已被注册')).toBeTruthy()
    expect(useUserStore().token).toBe('')
  })
})
