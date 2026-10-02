import { describe, it, expect, beforeEach, afterEach, vi } from 'vitest'
import { render, screen, waitFor, within } from '@testing-library/vue'
import userEvent from '@testing-library/user-event'
import { createPinia, setActivePinia } from 'pinia'
import { useUserStore } from '@/stores/user'
import Subscribe from '@/views/user/Subscribe.vue'
import UiHost from '@/ui/UiHost.vue'
import { toastMessages } from './helpers/feedback'

const mockGetSubscription = vi.fn()
const mockGetMfaStatus = vi.fn()
const mockResetSubscription = vi.fn()
const mockCreateTicket = vi.fn()
const mockFetch = vi.fn()

vi.mock('@/api/user', () => ({
  getProfile: vi.fn(async () => ({ data: {} })),
  getSubscription: (...args) => mockGetSubscription(...args),
  getMfaStatus: (...args) => mockGetMfaStatus(...args),
  resetSubscription: (...args) => mockResetSubscription(...args),
  createTicket: (...args) => mockCreateTicket(...args),
}))

vi.mock('vue-router', async () => {
  const actual = await vi.importActual('vue-router')
  const { computed, h } = await vi.importActual('vue')
  const RouterLink = {
    props: ['to'],
    setup(props, { slots }) {
      const href = computed(() => (typeof props.to === 'string' ? props.to : `${props.to.path}?${new URLSearchParams(props.to.query)}`))
      return () => h('a', { href: href.value }, slots.default?.())
    }
  }
  return { ...actual, RouterLink }
})

const SUMMARY = {
  plan_id: 3,
  plan_name: 'Standard',
  transfer_enable: 200 * 1024 ** 3,
  used_traffic: 71.6 * 1024 ** 3,
  expired_at: 1795996800,
  subscribe_path: '/s',
}

function ok(data) {
  return { code: 0, msg: 'ok', data }
}

const Harness = {
  components: { Subscribe, UiHost },
  template: '<div><Subscribe /><UiHost /></div>'
}

async function renderPage() {
  const user = userEvent.setup()
  const result = render(Harness)
  await screen.findByRole('heading', { level: 1, name: 'Subscription' })
  return { user, ...result }
}

describe('User Subscribe flow', () => {
  beforeEach(() => {
    localStorage.clear()
    setActivePinia(createPinia())
    const store = useUserStore()
    store.login('jwt', { id: 7, email: 'lin@example.com', token: 'sub-token-1' })
    mockGetSubscription.mockReset()
    mockCreateTicket.mockReset()
    mockGetMfaStatus.mockReset()
    mockGetMfaStatus.mockResolvedValue(ok({ enabled: false }))
    mockResetSubscription.mockReset()
    mockFetch.mockReset()
    vi.stubGlobal('fetch', mockFetch)
  })

  afterEach(() => {
    vi.unstubAllGlobals()
  })

  it('shows the link, its QR code and the traffic facts', async () => {
    mockGetSubscription.mockResolvedValue(ok(SUMMARY))
    await renderPage()
    const link = await screen.findByLabelText('Subscription link', { selector: '#subscribe-link' })
    expect(link.value).toBe(`${window.location.protocol}//${window.location.host}/s/sub-token-1`)
    expect(mockGetSubscription).toHaveBeenCalledWith(false)
    // The QR code encodes the real link (not a picture of one).
    expect(await screen.findByRole('img', { name: 'QR code of your subscription link' })).toBeTruthy()
    expect(screen.getByText('128.4 GB')).toBeTruthy()
    expect(screen.getByText('71.6 GB')).toBeTruthy()
  })

  it('copies the link and says so', async () => {
    mockGetSubscription.mockResolvedValue(ok(SUMMARY))
    const { user } = await renderPage()
    const card = (await screen.findByLabelText('Subscription link', { selector: '#subscribe-link' })).closest('[data-subscribe-link]')
    await user.click(within(card).getByRole('button', { name: /^Copy/ }))
    expect(await navigator.clipboard.readText()).toContain('/s/sub-token-1')
    expect(within(card).getByRole('button', { name: /Copied/ })).toBeTruthy()
  })

  it('reads a legacy { data } answer as well as the panel envelope', async () => {
    mockGetSubscription.mockResolvedValue({ data: { ...SUMMARY, subscribe_path: '/sub' } })
    await renderPage()
    expect((await screen.findByLabelText('Subscription link', { selector: '#subscribe-link' })).value).toContain('/sub/sub-token-1')
  })

  it('switches the subscription domain when several are configured', async () => {
    mockGetSubscription.mockResolvedValue(ok({ ...SUMMARY, subscribe_domains: ['sub-a.example.com', 'sub-b.example.com'] }))
    const { user } = await renderPage()
    const trigger = await screen.findByRole('combobox', { name: 'Subscription domain' })
    const link = () => screen.getByLabelText('Subscription link', { selector: '#subscribe-link' }).value
    expect(link()).toContain('//sub-a.example.com/s/sub-token-1')
    trigger.focus()
    await user.keyboard('{Enter}')
    await screen.findByRole('listbox')
    await user.keyboard('{ArrowDown}{Enter}')
    await waitFor(() => expect(link()).toContain('//sub-b.example.com/s/sub-token-1'))
  })

  it('imports into clients with a URL scheme and copies the format link for the others', async () => {
    mockGetSubscription.mockResolvedValue(ok(SUMMARY))
    const { user } = await renderPage()
    const clash = await screen.findByRole('link', { name: 'Import to Clash Verge' })
    const href = clash.getAttribute('href')
    expect(href.startsWith('clash://install-config?url=')).toBe(true)
    expect(decodeURIComponent(href.split('url=')[1].split('&')[0])).toMatch(/\/s\/sub-token-1\?type=clash$/)
    const rocket = screen.getByRole('link', { name: 'Import to Shadowrocket' }).getAttribute('href')
    expect(atob(rocket.replace('shadowrocket://add/sub://', '').split('?')[0])).toMatch(/\?type=shadowrocket$/)
    expect(screen.getByRole('link', { name: 'Import to sing-box' }).getAttribute('href')).toMatch(/^sing-box:\/\/import-remote-profile\?url=/)

    await user.click(screen.getByRole('button', { name: 'Copy the v2rayN subscription link' }))
    expect(await navigator.clipboard.readText()).toMatch(/\/s\/sub-token-1\?type=v2ray$/)
    expect(toastMessages('success')).toEqual(['Copied the v2rayN link'])
  })

  it('previews another format from this origin and downloads it with the right extension', async () => {
    mockGetSubscription.mockResolvedValue(ok({ ...SUMMARY, subscribe_domains: ['sub-a.example.com'] }))
    mockFetch.mockResolvedValue({ ok: true, status: 200, text: async () => '[Interface]\nPrivateKey = test' })
    const createObjectURL = vi.fn(() => 'blob:wireguard-preview')
    const revokeObjectURL = vi.fn()
    const click = vi.spyOn(HTMLAnchorElement.prototype, 'click').mockImplementation(() => {})
    vi.stubGlobal('URL', { createObjectURL, revokeObjectURL })
    const { user } = await renderPage()

    const trigger = await screen.findByRole('combobox', { name: 'Format' })
    trigger.focus()
    await user.keyboard('{Enter}')
    const options = within(await screen.findByRole('listbox')).getAllByRole('option')
    const index = options.findIndex(option => option.textContent.includes('WireGuard'))
    await user.keyboard(`${'{ArrowDown}'.repeat(index)}{Enter}`)
    await waitFor(() => expect(document.querySelector('[data-subscribe-format-link] input').value).toMatch(/sub-a\.example\.com\/s\/sub-token-1\?type=wireguard$/))

    await user.click(screen.getByRole('button', { name: 'Preview' }))
    const sheet = await screen.findByRole('dialog', { name: 'WireGuard (.conf) subscription content' })
    // The preview reads from this page's own host, not the subscription domain.
    expect(mockFetch).toHaveBeenCalledWith(`${window.location.protocol}//${window.location.host}/s/sub-token-1?type=wireguard`)
    expect(await within(sheet).findByText(/PrivateKey = test/)).toBeTruthy()
    await user.click(within(sheet).getByRole('button', { name: 'Download' }))
    expect(click.mock.instances[0].download).toBe('subscription-wireguard.conf')
    click.mockRestore()
  })

  it('resets the link after the password and shows the new link and its QR code', async () => {
    mockGetSubscription.mockResolvedValue(ok(SUMMARY))
    mockResetSubscription
      .mockResolvedValueOnce({ code: -1, msg: 'invalid password', data: null })
      .mockResolvedValueOnce(ok({ token: 'sub-token-2' }))
    const { user } = await renderPage()

    await user.click(await screen.findByRole('button', { name: 'Reset link…' }))
    const dialog = await screen.findByRole('dialog', { name: 'Reset your subscription link?' })
    const password = await within(dialog).findByLabelText(/^Current password/)
    await waitFor(() => expect(document.activeElement).toBe(password))

    // Nothing is sent without a password.
    await user.click(within(dialog).getByRole('button', { name: 'Reset link' }))
    expect(await within(dialog).findByText('Enter your current password.')).toBeTruthy()
    expect(mockResetSubscription).not.toHaveBeenCalled()

    await user.type(password, 'wrong')
    await user.click(within(dialog).getByRole('button', { name: 'Reset link' }))
    expect(await within(dialog).findByText('That password is not correct.')).toBeTruthy()
    expect(mockResetSubscription).toHaveBeenLastCalledWith({ password: 'wrong' })

    await user.clear(password)
    await user.type(password, 'correct-horse{Enter}')
    await waitFor(() => expect(mockResetSubscription).toHaveBeenLastCalledWith({ password: 'correct-horse' }))
    const done = await screen.findByRole('dialog', { name: 'Subscription link reset' })
    const newLink = `${window.location.protocol}//${window.location.host}/s/sub-token-2`
    expect(within(done).getByLabelText('New subscription link').value).toBe(newLink)
    expect(within(done).getByRole('img', { name: 'QR code of your subscription link' })).toBeTruthy()
    expect(toastMessages('success')).toEqual(['Subscription link reset. Import it again on all your devices.'])
    // The page shows the new link and reloads its data; there is no ticket.
    expect(screen.getByLabelText('Subscription link', { selector: '#subscribe-link' }).value).toBe(newLink)
    expect(useUserStore().userInfo.token).toBe('sub-token-2')
    expect(JSON.parse(localStorage.getItem('userInfo')).token).toBe('sub-token-2')
    expect(mockGetSubscription).toHaveBeenLastCalledWith(true)
    expect(mockCreateTicket).not.toHaveBeenCalled()

    await user.click(within(done).getByRole('button', { name: 'Done' }))
    await waitFor(() => expect(screen.queryByRole('dialog')).toBeNull())
  })

  it('asks for the 6-digit code or a recovery code when two-factor authentication is on', async () => {
    mockGetSubscription.mockResolvedValue(ok(SUMMARY))
    mockGetMfaStatus.mockResolvedValue(ok({ enabled: true }))
    mockResetSubscription
      .mockResolvedValueOnce({ code: -1, msg: 'invalid mfa code', data: null })
      .mockResolvedValueOnce(ok({ token: 'sub-token-3' }))
    const { user } = await renderPage()

    await user.click(await screen.findByRole('button', { name: 'Reset link…' }))
    const dialog = await screen.findByRole('dialog', { name: 'Reset your subscription link?' })
    const boxes = await within(dialog).findAllByLabelText(/^Digit \d of 6$/)
    expect(within(dialog).queryByLabelText(/^Current password/)).toBeNull()
    await waitFor(() => expect(document.activeElement).toBe(boxes[0]))

    // The sixth digit submits; a wrong code clears the boxes.
    await user.keyboard('123456')
    await waitFor(() => expect(mockResetSubscription).toHaveBeenCalledWith({ code: '123456', method: 'totp' }))
    expect(await within(dialog).findByText('That code is not correct or has expired.')).toBeTruthy()
    await waitFor(() => expect(within(dialog).getAllByLabelText(/^Digit \d of 6$/).map(box => box.value).join('')).toBe(''))

    await user.click(within(dialog).getByRole('button', { name: 'Use a recovery code' }))
    const field = await within(dialog).findByLabelText(/^Recovery code/)
    await user.type(field, 'abc')
    await user.click(within(dialog).getByRole('button', { name: 'Reset link' }))
    expect(await within(dialog).findByText(/A recovery code has 8 letters or digits/)).toBeTruthy()
    await user.clear(field)
    await user.type(field, 'abcd 1234')
    await user.click(within(dialog).getByRole('button', { name: 'Reset link' }))
    await waitFor(() => expect(mockResetSubscription).toHaveBeenLastCalledWith({ code: 'ABCD-1234', method: 'backup' }))
    expect(await screen.findByRole('dialog', { name: 'Subscription link reset' })).toBeTruthy()
    expect(useUserStore().userInfo.token).toBe('sub-token-3')
  })

  it('switches to the code when the server asks for one, and explains the limit', async () => {
    mockGetSubscription.mockResolvedValue(ok(SUMMARY))
    mockGetMfaStatus.mockRejectedValue(new Error('offline'))
    mockResetSubscription
      .mockResolvedValueOnce({ code: -1, msg: 'mfa code required', data: null })
      .mockResolvedValueOnce({ code: -1, msg: 'too many subscription reset attempts, please try again later', data: null })
    const { user } = await renderPage()

    await user.click(await screen.findByRole('button', { name: 'Reset link…' }))
    const dialog = await screen.findByRole('dialog', { name: 'Reset your subscription link?' })
    await user.type(await within(dialog).findByLabelText(/^Current password/), 'correct-horse{Enter}')
    const boxes = await within(dialog).findAllByLabelText(/^Digit \d of 6$/)
    expect(boxes).toHaveLength(6)
    await user.keyboard('654321')
    expect(await within(dialog).findByText('Too many attempts. Try again in an hour.')).toBeTruthy()
    expect(useUserStore().userInfo.token).toBe('sub-token-1')

    // Esc closes it; nothing changed.
    await user.keyboard('{Escape}')
    await waitFor(() => expect(screen.queryByRole('dialog')).toBeNull())
    expect(screen.getByLabelText('Subscription link', { selector: '#subscribe-link' }).value).toContain('/s/sub-token-1')
  })

  it('shows a failed reset with the server message', async () => {
    mockGetSubscription.mockResolvedValue(ok(SUMMARY))
    mockResetSubscription.mockResolvedValue({ code: -1, msg: '重置订阅失败: database is locked', data: null })
    const { user } = await renderPage()
    await user.click(await screen.findByRole('button', { name: 'Reset link…' }))
    const dialog = await screen.findByRole('dialog', { name: 'Reset your subscription link?' })
    await user.type(await within(dialog).findByLabelText(/^Current password/), 'correct-horse{Enter}')
    expect(await within(dialog).findByText('The link was not reset: 重置订阅失败: database is locked')).toBeTruthy()
    expect(mockCreateTicket).not.toHaveBeenCalled()
  })

  it('shows a retryable error when the subscription cannot be loaded', async () => {
    mockGetSubscription.mockResolvedValueOnce({ code: 1, msg: '获取订阅信息失败' }).mockResolvedValueOnce(ok(SUMMARY))
    const { user } = await renderPage()
    expect(await screen.findByRole('heading', { name: 'Could not load your subscription.' })).toBeTruthy()
    expect(screen.getByText('获取订阅信息失败')).toBeTruthy()
    await user.click(screen.getByRole('button', { name: 'Try again' }))
    expect(await screen.findByLabelText('Subscription link', { selector: '#subscribe-link' })).toBeTruthy()
  })

  it('says so when the account has no subscription link yet', async () => {
    useUserStore().login('jwt', { id: 7, email: 'lin@example.com' })
    mockGetSubscription.mockResolvedValue(ok({ plan_id: null }))
    await renderPage()
    expect(await screen.findByRole('heading', { name: 'You don’t have a subscription link yet.' })).toBeTruthy()
    expect(screen.queryByRole('img', { name: /QR code/ })).toBeNull()
  })
})
