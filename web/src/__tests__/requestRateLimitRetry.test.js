import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

// The request client sends a rate-limited read (429) again after the server's
// Retry-After, at most twice; writes and aborted reads are not repeated.

function rateLimited(config, headers = { 'retry-after': '1' }) {
  return { config, response: { status: 429, headers } }
}

async function loadClient() {
  vi.resetModules()
  const interceptors = {}
  const service = vi.fn()
  service.interceptors = {
    request: { use: vi.fn() },
    response: {
      use: vi.fn((fulfilled, rejected) => {
        interceptors.fulfilled = fulfilled
        interceptors.rejected = rejected
      }),
    },
  }
  vi.doMock('axios', () => ({ default: { create: vi.fn(() => service) } }))
  vi.doMock('@/stores/user', () => ({ useUserStore: () => ({ token: '', logout: vi.fn() }) }))
  const requestModule = await vi.importActual('@/utils/request')
  await requestModule.loadRequestClient()
  return { service, rejected: interceptors.rejected, rateLimitRetryDelay: requestModule.rateLimitRetryDelay }
}

describe('rate-limited reads', () => {
  beforeEach(() => {
    vi.useFakeTimers()
    vi.spyOn(console, 'error').mockImplementation(() => {})
  })

  afterEach(() => {
    vi.useRealTimers()
    vi.restoreAllMocks()
    vi.resetModules()
  })

  it('sends a GET again after Retry-After and answers what the second try answers', async () => {
    const { service, rejected } = await loadClient()
    service.mockResolvedValueOnce({ data: [] })
    const config = { url: '/plugin-installations', method: 'get' }
    const answer = rejected(rateLimited(config))
    await vi.advanceTimersByTimeAsync(999)
    expect(service).not.toHaveBeenCalled()
    await vi.advanceTimersByTimeAsync(1)
    await expect(answer).resolves.toEqual({ data: [] })
    expect(service).toHaveBeenCalledTimes(1)
    expect(service.mock.calls[0][0]).toBe(config)
    expect(config.rateLimitRetries).toBe(1)
  })

  it('stops after two retries and then rejects like any other error', async () => {
    const { rejected } = await loadClient()
    const config = { url: '/x', method: 'get', rateLimitRetries: 2 }
    const error = rateLimited(config)
    await expect(rejected(error)).rejects.toBe(error)
  })

  it('never repeats a write', async () => {
    const { service, rejected } = await loadClient()
    for (const method of ['post', 'put', 'patch', 'delete']) {
      const error = rateLimited({ url: '/x', method })
      await expect(rejected(error)).rejects.toBe(error)
    }
    expect(service).not.toHaveBeenCalled()
  })

  it('does not send again a read whose caller aborted it while waiting', async () => {
    const { service, rejected } = await loadClient()
    const controller = new AbortController()
    const error = rateLimited({ url: '/x', method: 'get', signal: controller.signal })
    // Attach the expectation first: the promise rejects while the timers run.
    const answer = expect(rejected(error)).rejects.toBe(error)
    controller.abort()
    await vi.advanceTimersByTimeAsync(1000)
    await answer
    expect(service).not.toHaveBeenCalled()
  })

  it('waits what the server asks, one second without a usable header, five at most', async () => {
    const { rateLimitRetryDelay } = await loadClient()
    const get = { url: '/x', method: 'get' }
    expect(rateLimitRetryDelay(rateLimited({ ...get }, { 'retry-after': '2' }))).toBe(2000)
    expect(rateLimitRetryDelay(rateLimited({ ...get }, { 'retry-after': 'soon' }))).toBe(1000)
    expect(rateLimitRetryDelay(rateLimited({ ...get }, {}))).toBe(1000)
    expect(rateLimitRetryDelay(rateLimited({ ...get }, { 'retry-after': '60' }))).toBe(5000)
    expect(rateLimitRetryDelay(rateLimited({ ...get }, { get: () => '3' }))).toBe(3000)
    expect(rateLimitRetryDelay(rateLimited({ ...get, noRateLimitRetry: true }))).toBeNull()
    expect(rateLimitRetryDelay({ config: get, response: { status: 503, headers: {} } })).toBeNull()
  })
})
