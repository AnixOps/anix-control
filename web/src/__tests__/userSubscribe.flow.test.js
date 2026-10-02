import { describe, it, expect, beforeEach, afterEach, vi } from 'vitest'
import { render, screen, waitFor, within } from '@testing-library/vue'
import userEvent from '@testing-library/user-event'
import { createPinia, setActivePinia } from 'pinia'
import { useUserStore } from '@/stores/user'
import Subscribe from '@/views/user/Subscribe.vue'
import UiHost from '@/ui/UiHost.vue'
import { toastMessages } from './helpers/feedback'

const mockGetSubscription = vi.fn()
const mockCreateTicket = vi.fn()
const mockFetch = vi.fn()

vi.mock('@/api/user', () => ({
  getProfile: vi.fn(async () => ({ data: {} })),
  getSubscription: (...args) => mockGetSubscription(...args),
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

  it('confirms in place before asking an administrator to reset the link', async () => {
    mockGetSubscription.mockResolvedValue(ok(SUMMARY))
    mockCreateTicket.mockResolvedValue(ok({ id: 2041, subject: 'Please reset my subscription link' }))
    const { user } = await renderPage()

    const open = await screen.findByRole('button', { name: 'Request a reset…' })
    await user.click(open)
    const group = screen.getByRole('group', { name: 'Ask an administrator to reset your subscription link?' })
    await waitFor(() => expect(document.activeElement).toBe(within(group).getByRole('button', { name: 'Cancel' })))

    // Cancel (and Esc) put it away and return focus.
    await user.keyboard('{Escape}')
    await waitFor(() => expect(screen.queryByRole('group', { name: /reset your subscription link/ })).toBeNull())
    await waitFor(() => expect(document.activeElement).toBe(screen.getByRole('button', { name: 'Request a reset…' })))
    expect(mockCreateTicket).not.toHaveBeenCalled()

    await user.click(screen.getByRole('button', { name: 'Request a reset…' }))
    await user.click(screen.getByRole('button', { name: 'Send request' }))
    expect(mockCreateTicket).toHaveBeenCalledWith({ subject: 'Please reset my subscription link', level: 2, message: 'My subscription link may have leaked. Please reset it.' })
    expect(await screen.findByText('Reset requested. Your administrator will reply in the ticket.')).toBeTruthy()
    expect(screen.getByRole('link', { name: 'View ticket' }).getAttribute('href')).toBe('/user/tickets?ticket=2041')
  })

  it('keeps a failed reset request open with the error', async () => {
    mockGetSubscription.mockResolvedValue(ok(SUMMARY))
    mockCreateTicket.mockResolvedValue({ code: 1, msg: '创建工单失败' })
    const { user } = await renderPage()
    await user.click(await screen.findByRole('button', { name: 'Request a reset…' }))
    await user.click(screen.getByRole('button', { name: 'Send request' }))
    expect((await screen.findByRole('alert')).textContent).toBe('The request was not sent: 创建工单失败')
    expect(screen.getByRole('group', { name: /reset your subscription link/ })).toBeTruthy()
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
