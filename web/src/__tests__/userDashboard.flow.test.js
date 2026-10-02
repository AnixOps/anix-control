import { describe, it, expect, beforeEach, afterEach, vi } from 'vitest'
import { render, screen, waitFor, within } from '@testing-library/vue'
import userEvent from '@testing-library/user-event'
import { createPinia, setActivePinia } from 'pinia'
import { useUserStore } from '@/stores/user'
import { setEdition } from '@/composables/useEdition'
import Dashboard from '@/views/user/Dashboard.vue'
import { toastMessages } from './helpers/feedback'

const api = vi.hoisted(() => ({
  getProfile: vi.fn(),
  getSubscription: vi.fn(),
  getKnowledgeList: vi.fn(),
  getTickets: vi.fn(),
}))

vi.mock('@/api/user', () => api)

vi.mock('vue-router', async () => {
  const { h } = await vi.importActual('vue')
  const RouterLink = {
    props: ['to'],
    setup: (props, { slots }) => () => h('a', {
      href: typeof props.to === 'string' ? props.to : `${props.to.path}?${new URLSearchParams(props.to.query)}`
    }, slots.default?.())
  }
  return { RouterLink }
})

const GIB = 1024 ** 3
const NOW = Date.UTC(2026, 9, 1) / 1000

function summary(overrides = {}) {
  return {
    code: 0,
    data: {
      plan_id: 3,
      plan_name: 'Standard',
      transfer_enable: 200 * GIB,
      used_traffic: 71.6 * GIB,
      upload_traffic: 1.6 * GIB,
      download_traffic: 70 * GIB,
      expired_at: NOW + 60 * 86400,
      days_remaining: 60,
      is_expired: false,
      subscribe_path: '/s',
      cached_at: new Date().toISOString(),
      ...overrides,
    },
  }
}

function renderPage() {
  const user = userEvent.setup()
  render(Dashboard)
  return { user }
}

describe('User overview', () => {
  beforeEach(() => {
    setActivePinia(createPinia())
    useUserStore().login('jwt', { id: 7, email: 'lin.xiao@example.com', token: 'sub-token-1' })
    for (const fn of Object.values(api)) fn.mockReset()
    api.getKnowledgeList.mockResolvedValue({ data: [
      { id: 4, title: 'Maintenance on 8 October', category: 'News', body: '', updated_at: NOW },
      { id: 5, title: 'Shadowrocket import fails', category: 'Guides', body: '', updated_at: NOW },
    ] })
    api.getTickets.mockResolvedValue({ data: [
      { id: 1987, subject: 'Router setup', status: 2, updated_at: NOW - 86400 },
      { id: 2041, subject: 'Slow at night', status: 1, updated_at: NOW },
    ] })
  })

  afterEach(() => {
    vi.useRealTimers()
  })

  it('greets the user and shows the remaining traffic, status and expiry in the hero', async () => {
    api.getSubscription.mockResolvedValue(summary())
    renderPage()
    expect(screen.getByRole('heading', { level: 1, name: 'Hello, lin.xiao' })).toBeTruthy()
    const ring = await screen.findByRole('img', { name: '128.4 GB of 200 GB traffic left' })
    expect(ring.textContent).toContain('128.4')
    expect(ring.textContent).toContain('GB left · 200 GB total')
    expect(screen.getByText('Your subscription is active, with 64% of your traffic left.')).toBeTruthy()
    const hero = ring.closest('section')
    expect(within(hero).getByText('Active')).toBeTruthy()
    expect(within(hero).getByText('In 60 days')).toBeTruthy()
    expect(within(hero).getByText('71.6 GB')).toBeTruthy()
    expect(within(hero).getByText('Subscription template: Standard')).toBeTruthy()
    expect(within(hero).getByRole('link', { name: 'Import to a client' }).getAttribute('href')).toBe('/user/subscribe')
    // The commercial card is not in the community edition.
    expect(screen.queryByRole('link', { name: 'Choose a plan' })).toBeNull()
  })

  it('copies the subscription link with the primary button', async () => {
    api.getSubscription.mockResolvedValue(summary())
    const { user } = renderPage()
    await user.click(await screen.findByRole('button', { name: 'Copy subscription link' }))
    expect(await navigator.clipboard.readText()).toBe(`${window.location.protocol}//${window.location.host}/s/sub-token-1`)
    expect(toastMessages('success')).toEqual(['Subscription link copied'])
  })

  it('refreshes the cached numbers on request', async () => {
    api.getSubscription.mockResolvedValueOnce(summary()).mockResolvedValueOnce(summary({ used_traffic: 100 * GIB }))
    const { user } = renderPage()
    await screen.findByRole('img', { name: /traffic left/ })
    await user.click(screen.getByRole('button', { name: 'Refresh' }))
    expect(api.getSubscription).toHaveBeenLastCalledWith(true)
    expect(await screen.findByRole('img', { name: '100 GB of 200 GB traffic left' })).toBeTruthy()
  })

  it('lists the first help articles and the latest tickets', async () => {
    api.getSubscription.mockResolvedValue(summary())
    renderPage()
    const help = (await screen.findByRole('heading', { level: 2, name: 'Help Center' })).closest('section')
    expect((await within(help).findByRole('link', { name: 'Maintenance on 8 October' })).getAttribute('href')).toBe('/user/knowledge?article=4')
    const tickets = screen.getByRole('heading', { level: 2, name: 'Recent tickets' }).closest('section')
    const links = await within(tickets).findAllByRole('link', { name: /Slow at night|Router setup/ })
    expect(links.map(link => link.textContent)).toEqual(['Slow at night', 'Router setup'])
    expect(within(tickets).getByText('Answered')).toBeTruthy()
    expect(within(tickets).getByRole('link', { name: 'New ticket' }).getAttribute('href')).toBe('/user/tickets?new=1')
  })

  it('says what to do when the subscription expired', async () => {
    api.getSubscription.mockResolvedValue(summary({ is_expired: true, expired_at: NOW - 86400, days_remaining: 0 }))
    renderPage()
    expect(await screen.findByText(/Your subscription expired on \d{4}-\d{2}-\d{2}\. Ask your administrator to set one up or renew it\./)).toBeTruthy()
    expect(screen.getByText('Expired')).toBeTruthy()
    expect(screen.getByText('Ended')).toBeTruthy()
  })

  it('shows traffic used up and no plan', async () => {
    api.getSubscription.mockResolvedValueOnce(summary({ used_traffic: 200 * GIB }))
    renderPage()
    expect(await screen.findByText('Out of traffic')).toBeTruthy()
    expect(screen.getByRole('img', { name: '0 B of 200 GB traffic left' })).toBeTruthy()
  })

  it('offers plans and orders in the commercial edition', async () => {
    setEdition('commercial')
    api.getSubscription.mockResolvedValue(summary({ plan_id: null, plan_name: '无套餐', transfer_enable: 0, used_traffic: 0 }))
    renderPage()
    expect(await screen.findByText('You don’t have a subscription yet. Choose a plan to get started.')).toBeTruthy()
    expect(screen.getByRole('link', { name: 'Choose a plan' }).getAttribute('href')).toBe('/user/plans')
    expect(screen.getByRole('link', { name: 'My orders' })).toBeTruthy()
    expect(screen.queryByText(/无套餐/)).toBeNull()
  })

  it('shows a skeleton only after 300 ms, and a retryable error', async () => {
    vi.useFakeTimers({ shouldAdvanceTime: true })
    let fail
    api.getSubscription.mockImplementationOnce(() => new Promise((_resolve, reject) => { fail = reject })).mockResolvedValueOnce(summary())
    const user = userEvent.setup({ advanceTimers: vi.advanceTimersByTime })
    render(Dashboard)
    await vi.advanceTimersByTimeAsync(250)
    expect(document.querySelector('[data-home-skeleton]')).toBeNull()
    await vi.advanceTimersByTimeAsync(100)
    expect(document.querySelector('[data-home-skeleton]')).not.toBeNull()
    fail({ response: { status: 502, data: { msg: '获取订阅信息失败' } } })
    expect(await screen.findByRole('heading', { name: 'Could not load your subscription.' })).toBeTruthy()
    await user.click(screen.getByRole('button', { name: 'Try again' }))
    await waitFor(() => expect(screen.getByRole('img', { name: /traffic left/ })).toBeTruthy())
  })
})
