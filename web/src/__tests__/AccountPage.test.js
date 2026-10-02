import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { render, screen, waitFor, within } from '@testing-library/vue'
import userEvent from '@testing-library/user-event'
import { createPinia, setActivePinia } from 'pinia'
import { useUserStore } from '@/stores/user'
import { useTheme } from '@/composables/useTheme'
import i18n from '@/i18n'
import Account from '@/views/Account.vue'
import UiHost from '@/ui/UiHost.vue'
import { resetConfirms } from '@/ui/composables/useConfirm'

const api = vi.hoisted(() => ({
  getMfaStatus: vi.fn(),
  setupTotp: vi.fn(),
  enableTotp: vi.fn(),
  disableMfa: vi.fn(),
  regenerateBackupCodes: vi.fn()
}))

vi.mock('@/api/user', () => ({
  getProfile: vi.fn(async () => ({ data: {} })),
  ...api
}))

const Harness = {
  components: { Account, UiHost },
  template: '<div><Account /><UiHost /></div>'
}

function ok(data) {
  return { code: 0, msg: 'ok', data }
}

async function renderPage() {
  const result = render(Harness)
  await waitFor(() => expect(api.getMfaStatus).toHaveBeenCalled())
  return result
}

describe('Account.vue', () => {
  beforeEach(() => {
    setActivePinia(createPinia())
    useUserStore().login('token', { id: 7, email: 'lin@example.test', is_admin: false })
    for (const fn of Object.values(api)) fn.mockReset()
    api.getMfaStatus.mockResolvedValue(ok({ enabled: false, has_backup_codes: false, remaining_codes: 0, last_used: null }))
  })

  afterEach(() => {
    resetConfirms()
    document.documentElement.removeAttribute('data-theme')
  })

  it('has one H1 and shows the profile read-only', async () => {
    await renderPage()
    expect(screen.getAllByRole('heading', { level: 1 }).map(heading => heading.textContent)).toEqual(['Account'])
    const profile = screen.getByRole('list', { name: 'Profile' })
    expect(profile.textContent).toContain('lin@example.test')
    expect(profile.textContent).toContain('7')
    expect(profile.textContent).toContain('User')
    // No password or device sections: there is no endpoint for them.
    expect(screen.queryByRole('heading', { name: /password|device|session/i })).toBeNull()
  })

  it('turns on two-factor authentication with a setup key and the first code, then shows the backup codes', async () => {
    const user = userEvent.setup()
    api.setupTotp.mockResolvedValue(ok({ secret: 'JBSWY3DPEHPK3PXP', url: 'otpauth://totp/AnixOps:lin?secret=JBSWY3DPEHPK3PXP', qr_code: 'otpauth://totp/AnixOps:lin?secret=JBSWY3DPEHPK3PXP', backup_codes: ['1111-2222', '3333-4444'] }))
    api.enableTotp.mockResolvedValue(ok({ message: 'MFA enabled successfully' }))
    await renderPage()
    expect(await screen.findByText('Off')).toBeTruthy()

    await user.click(screen.getByRole('button', { name: 'Turn on two-factor authentication' }))
    const dialog = await screen.findByRole('dialog', { name: 'Turn on two-factor authentication' })
    await waitFor(() => expect(within(dialog).getByLabelText('Setup key').value).toBe('JBSWY3DPEHPK3PXP'))
    expect(within(dialog).getByRole('link', { name: 'Open in an authenticator on this device' }).getAttribute('href')).toMatch(/^otpauth:\/\/totp\//)

    const code = within(dialog).getByLabelText(/6-digit code/)
    expect(code.getAttribute('autocomplete')).toBe('one-time-code')
    await user.type(code, '12ab')
    await user.click(within(dialog).getByRole('button', { name: 'Verify and turn on' }))
    expect(await within(dialog).findByText('Enter the 6-digit code')).toBeTruthy()
    expect(api.enableTotp).not.toHaveBeenCalled()

    await user.clear(code)
    await user.type(code, '123456')
    api.getMfaStatus.mockResolvedValue(ok({ enabled: true, has_backup_codes: true, remaining_codes: 2, last_used: null }))
    await user.click(within(dialog).getByRole('button', { name: 'Verify and turn on' }))
    expect(api.enableTotp).toHaveBeenCalledWith('123456')

    const codes = await screen.findByRole('dialog', { name: 'Save your backup codes' })
    expect(within(codes).getAllByRole('listitem').map(item => item.textContent)).toEqual(['1111-2222', '3333-4444'])
    expect(await screen.findByText('On')).toBeTruthy()
    expect(screen.getByText('2 left')).toBeTruthy()
  })

  it('never turns a non-otpauth URL from the API into a link', async () => {
    const user = userEvent.setup()
    api.setupTotp.mockResolvedValue(ok({ secret: 'ABC', url: 'javascript:alert(1)', backup_codes: [] }))
    await renderPage()
    await user.click(await screen.findByRole('button', { name: 'Turn on two-factor authentication' }))
    const dialog = await screen.findByRole('dialog', { name: 'Turn on two-factor authentication' })
    await waitFor(() => expect(within(dialog).getByLabelText('Setup key').value).toBe('ABC'))
    expect(within(dialog).queryByRole('link')).toBeNull()
  })

  it('turns it off with the current password and shows a server error inline', async () => {
    const user = userEvent.setup()
    api.getMfaStatus.mockResolvedValue(ok({ enabled: true, has_backup_codes: true, remaining_codes: 8, last_used: '2026-09-30T08:12:00Z' }))
    api.disableMfa.mockResolvedValueOnce({ code: 1, msg: 'invalid password' })
    api.disableMfa.mockResolvedValueOnce(ok({ message: 'MFA disabled successfully' }))
    await renderPage()

    await user.click(await screen.findByRole('button', { name: 'Turn off…' }))
    const dialog = await screen.findByRole('dialog', { name: 'Turn off two-factor authentication?' })
    await user.click(within(dialog).getByRole('button', { name: 'Turn off' }))
    expect(await within(dialog).findByText('Enter your current password')).toBeTruthy()

    await user.type(within(dialog).getByLabelText(/Current password/), 'wrong')
    await user.click(within(dialog).getByRole('button', { name: 'Turn off' }))
    expect(await within(dialog).findByText('That didn’t work: invalid password')).toBeTruthy()

    await user.clear(within(dialog).getByLabelText(/Current password/))
    await user.type(within(dialog).getByLabelText(/Current password/), 'secret')
    api.getMfaStatus.mockResolvedValue(ok({ enabled: false, remaining_codes: 0 }))
    await user.click(within(dialog).getByRole('button', { name: 'Turn off' }))
    expect(api.disableMfa).toHaveBeenLastCalledWith('secret')
    await waitFor(() => expect(screen.queryByRole('dialog')).toBeNull())
    expect(await screen.findByText('Off')).toBeTruthy()
  })

  it('creates new backup codes after a confirmation', async () => {
    const user = userEvent.setup()
    api.getMfaStatus.mockResolvedValue(ok({ enabled: true, has_backup_codes: true, remaining_codes: 1 }))
    api.regenerateBackupCodes.mockResolvedValue(ok({ backup_codes: ['9999-0000'] }))
    await renderPage()
    await user.click(await screen.findByRole('button', { name: 'New backup codes…' }))
    const confirm = await screen.findByRole('alertdialog', { name: 'Create new backup codes?' })
    await user.click(within(confirm).getByRole('button', { name: 'Create new codes' }))
    const codes = await screen.findByRole('dialog', { name: 'Save your backup codes' })
    expect(within(codes).getByText('9999-0000')).toBeTruthy()
    expect(api.regenerateBackupCodes).toHaveBeenCalledTimes(1)
  })

  it('switches language and appearance', async () => {
    const user = userEvent.setup()
    await renderPage()
    const appearance = screen.getByRole('group', { name: 'Theme' })
    await user.click(within(appearance).getByRole('button', { name: /Dark/ }))
    expect(useTheme().themePreference.value).toBe('dark')
    await user.click(within(appearance).getByRole('button', { name: /System/ }))
    expect(useTheme().themePreference.value).toBe('system')
    const language = screen.getByRole('group', { name: 'Display language' })
    await user.click(within(language).getByRole('button', { name: '简体中文' }))
    await waitFor(() => expect(i18n.global.locale.value).toBe('zh-CN'))
  })

  it('offers a retry when the status cannot be read', async () => {
    const user = userEvent.setup()
    api.getMfaStatus.mockRejectedValueOnce(new Error('offline'))
    await renderPage()
    await user.click(await screen.findByRole('button', { name: 'Try again' }))
    expect(api.getMfaStatus).toHaveBeenCalledTimes(2)
  })
})
