import { describe, it, expect, beforeEach, vi } from 'vitest'
import { render, screen, waitFor, within } from '@testing-library/vue'
import userEvent from '@testing-library/user-event'
import Traffic from '@/views/user/Traffic.vue'

const mockGetMine = vi.fn()

vi.mock('@/api/user', () => ({
  getMyTrafficDestinations: (...args) => mockGetMine(...args)
}))

const SUMMARY = {
  upload: 300,
  download: 2700,
  connections: 12,
  categories: [
    { category: 'video', upload: 100, download: 1900, connections: 4 },
    { category: 'messaging', upload: 200, download: 800, connections: 8 }
  ],
  domains: [
    { domain: 'youtube.com', category: 'video', upload: 100, download: 1900, connections: 4 },
    { domain: 'telegram.org', category: 'messaging', upload: 200, download: 800, connections: 8 }
  ],
  domain_count: 2,
  settings: { enabled: true, retention_days: 30 }
}

describe('User 访问统计 page', () => {
  beforeEach(() => {
    mockGetMine.mockReset()
  })

  it('shows the user\'s categories and sites, and reloads for another period', async () => {
    mockGetMine.mockResolvedValue(SUMMARY)
    const user = userEvent.setup()
    render(Traffic)
    expect(await screen.findAllByText('youtube.com')).not.toHaveLength(0)
    expect(mockGetMine).toHaveBeenCalledWith({ hours: 24, limit: 200 })
    const list = document.querySelector('[data-destination-categories]')
    expect(within(list).getByText('Video')).toBeTruthy()
    expect(within(list).getByText('Messaging')).toBeTruthy()
    expect(list.textContent).toContain('66.7%')
    expect(screen.getByText('Records are kept for 30 days, then deleted.')).toBeTruthy()

    await user.click(screen.getByText('7 days'))
    await waitFor(() => expect(mockGetMine).toHaveBeenLastCalledWith({ hours: 168, limit: 200 }))
  })

  it('says when the statistics are off and when there is nothing yet', async () => {
    mockGetMine.mockResolvedValue({ categories: [], domains: [], domain_count: 0, settings: { enabled: false, retention_days: 30 } })
    render(Traffic)
    expect(await screen.findByText('Your administrator turned site statistics off: nothing new is recorded.')).toBeTruthy()
    expect((await screen.findAllByText('No sites visited in this period')).length).toBeGreaterThan(0)
  })
})
