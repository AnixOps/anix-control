// The list state of the API tokens page (views/admin/security/useApiTokens.js):
// what it asks for, the switch that only a super administrator can use, a stale
// answer, the limit of 25, and revoking.
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { effectScope, nextTick } from 'vue'
import { flushPromises } from '@vue/test-utils'
import { useApiTokens } from '@/views/admin/security/useApiTokens'

const kernelApi = vi.hoisted(() => ({ listKernelApiTokens: vi.fn(), revokeKernelApiToken: vi.fn() }))
vi.mock('@/api/kernel', () => kernelApi)

const NOW = Date.UTC(2026, 9, 5, 12, 0, 0)
const DAY = 86_400_000

function row(id, extra = {}) {
  return { id: `tok-${id}`, user_id: 7, name: `token ${id}`, scope: 'read', hint: 'abcd', expires_at: new Date(NOW + 60 * DAY).toISOString(), last_used_at: null, created_at: '2026-10-01T00:00:00Z', revoked_at: null, ...extra }
}

function refusal(status, code, message = '') {
  return { response: { status, data: { error: { code, message } } }, message: `Request failed with status code ${status}` }
}

describe('useApiTokens', () => {
  let scope

  beforeEach(() => {
    vi.resetAllMocks()
    vi.spyOn(console, 'error').mockImplementation(() => {})
    kernelApi.listKernelApiTokens.mockResolvedValue([row(1), row(2)])
  })

  afterEach(() => {
    scope?.stop()
    vi.restoreAllMocks()
  })

  function setup(options = {}) {
    scope = effectScope()
    return scope.run(() => useApiTokens({ me: 7, now: () => NOW, ...options }))
  }

  it('lists the caller\'s active tokens by default and reads them into the page\'s shape', async () => {
    const list = setup()
    expect(await list.load()).toBe(true)
    expect(kernelApi.listKernelApiTokens).toHaveBeenLastCalledWith({ all: false, includeInactive: false })
    expect(list.tokens.value.map(token => token.id)).toEqual(['tok-1', 'tok-2'])
    expect(list.tokens.value[0]).toMatchObject({ userId: 7, scope: 'read', hint: 'abcd' })
    expect(list.loaded.value).toBe(true)
    expect(list.error.value).toBeNull()
    expect(list.loading.value).toBe(false)
  })

  it('asks again with include_inactive when the switch for ended tokens is turned on', async () => {
    const list = setup()
    await list.load()
    list.showEnded.value = true
    await flushPromises()
    expect(kernelApi.listKernelApiTokens).toHaveBeenLastCalledWith({ all: false, includeInactive: true })
    list.showAll.value = true
    await flushPromises()
    expect(kernelApi.listKernelApiTokens).toHaveBeenLastCalledWith({ all: true, includeInactive: true })
    expect(kernelApi.listKernelApiTokens).toHaveBeenCalledTimes(3)
  })

  it('knows whose a row is: every row of its own list, and by id in the list of everyone\'s', async () => {
    kernelApi.listKernelApiTokens.mockResolvedValue([row(1), row(2, { user_id: 9 })])
    const list = setup()
    await list.load()
    expect(list.tokens.value.every(token => list.isMine(token))).toBe(true)
    list.showAll.value = true
    await flushPromises()
    expect(list.loadedAll.value).toBe(true)
    expect(list.tokens.value.map(token => list.isMine(token))).toEqual([true, false])
  })

  it('does not take rows for the caller\'s when the caller\'s id is not known', async () => {
    kernelApi.listKernelApiTokens.mockResolvedValue([row(1)])
    const list = setup({ me: null })
    list.showAll.value = true
    await flushPromises()
    expect(list.isMine(list.tokens.value[0])).toBe(false)
  })

  it('goes back to the caller\'s own tokens once, and stops offering the switch, after super_admin_required', async () => {
    kernelApi.listKernelApiTokens
      .mockRejectedValueOnce(refusal(403, 'super_admin_required', 'only a super administrator may list other administrators\' API tokens'))
      .mockResolvedValueOnce([row(1)])
    const list = setup()
    list.showAll.value = true
    await flushPromises()
    expect(kernelApi.listKernelApiTokens.mock.calls.map(call => call[0].all)).toEqual([true, false])
    expect(list.superAdmin.value).toBe(false)
    expect(list.showAll.value).toBe(false)
    expect(list.error.value).toBeNull()
    expect(list.tokens.value).toHaveLength(1)
    expect(list.loadedAll.value).toBe(false)
    // Turning it on again asks for nothing more than the caller's own.
    list.showAll.value = true
    await flushPromises()
    expect(kernelApi.listKernelApiTokens.mock.calls.at(-1)[0].all).toBe(false)
  })

  it('keeps the error of a failed load, empties the rows, and recovers on the next load', async () => {
    const failure = refusal(503, 'database_unavailable', 'the database is unavailable')
    kernelApi.listKernelApiTokens.mockRejectedValueOnce(failure)
    const list = setup()
    expect(await list.load()).toBe(false)
    expect(list.error.value).toBe(failure)
    expect(list.tokens.value).toEqual([])
    expect(list.ownActive.value).toBeNull()
    expect(await list.load()).toBe(true)
    expect(list.error.value).toBeNull()
  })

  it('drops an answer that arrives after a newer request was made', async () => {
    let releaseFirst
    kernelApi.listKernelApiTokens
      .mockReturnValueOnce(new Promise((resolve) => { releaseFirst = resolve }))
      .mockResolvedValueOnce([row(2)])
    const list = setup()
    const first = list.load()
    const second = list.load()
    await second
    releaseFirst([row(1)])
    expect(await first).toBe(false)
    expect(list.tokens.value.map(token => token.id)).toEqual(['tok-2'])
    expect(list.loading.value).toBe(false)
  })

  it('counts the caller\'s active tokens against the limit, not other administrators\' or ended ones', async () => {
    const rows = [
      ...Array.from({ length: 23 }, (_, index) => row(index)),
      row(90, { revoked_at: '2026-10-02T00:00:00Z' }),
      row(91, { expires_at: new Date(NOW - DAY).toISOString() }),
      row(92, { user_id: 9 }),
      row(93, { user_id: 9 })
    ]
    kernelApi.listKernelApiTokens.mockResolvedValue(rows)
    const list = setup()
    expect(list.ownActive.value).toBeNull()
    list.showAll.value = true
    list.showEnded.value = true
    await flushPromises()
    expect(list.ownActive.value).toBe(23)
    expect(list.limitReached.value).toBe(false)
    kernelApi.listKernelApiTokens.mockResolvedValue([...rows, row(94), row(95)])
    await list.load()
    expect(list.ownActive.value).toBe(25)
    expect(list.limitReached.value).toBe(true)
  })

  it('revokes by id and says whether anything changed', async () => {
    const list = setup()
    kernelApi.revokeKernelApiToken.mockResolvedValueOnce({ api_token: row(1), changed: true })
    expect(await list.revoke({ id: 'tok-1' })).toEqual({ changed: true })
    expect(kernelApi.revokeKernelApiToken).toHaveBeenLastCalledWith('tok-1')
    kernelApi.revokeKernelApiToken.mockResolvedValueOnce({ api_token: row(1), changed: false })
    expect(await list.revoke({ id: 'tok-1' })).toEqual({ changed: false })
  })

  it('throws the classified refusal, and reloads the list when the token is gone', async () => {
    const list = setup()
    await list.load()
    kernelApi.revokeKernelApiToken.mockRejectedValueOnce(refusal(404, 'not_found', 'the API token does not exist or is not yours'))
    await expect(list.revoke({ id: 'tok-9' })).rejects.toMatchObject({ message: 'the API token does not exist or is not yours', refusal: { kind: 'not_found' } })
    await flushPromises()
    expect(kernelApi.listKernelApiTokens).toHaveBeenCalledTimes(2)

    kernelApi.revokeKernelApiToken.mockRejectedValueOnce(refusal(500, 'database_error', 'database operation failed'))
    await expect(list.revoke({ id: 'tok-1' })).rejects.toMatchObject({ refusal: { kind: 'other' } })
    await nextTick()
    expect(kernelApi.listKernelApiTokens).toHaveBeenCalledTimes(2)
  })

  it('answers nothing into a page that is gone', async () => {
    let release
    kernelApi.listKernelApiTokens.mockReturnValueOnce(new Promise((resolve) => { release = resolve }))
    const list = setup()
    const pending = list.load()
    scope.stop()
    release([row(1)])
    expect(await pending).toBe(false)
    expect(list.tokens.value).toEqual([])
  })
})
