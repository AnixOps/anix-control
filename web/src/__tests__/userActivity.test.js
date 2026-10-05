import { beforeEach, describe, expect, it, vi } from 'vitest'
import { mount } from '@vue/test-utils'
import { setLocale } from '@/i18n'
import { useUserActivity } from '@/views/admin/users/useUserActivity'
import UserLastOnline from '@/views/admin/users/UserLastOnline.vue'

const api = vi.hoisted(() => ({ getUsersActivity: vi.fn() }))
vi.mock('@/api/admin', () => api)

describe('useUserActivity', () => {
  beforeEach(() => {
    vi.resetAllMocks()
    vi.spyOn(console, 'error').mockImplementation(() => {})
  })

  it('is undefined until the answer is in, null for a user never seen, else Unix seconds', async () => {
    let release
    api.getUsersActivity.mockReturnValue(new Promise((resolve) => { release = resolve }))
    const activity = useUserActivity()
    const loading = activity.load([3, 5])
    expect(api.getUsersActivity).toHaveBeenCalledWith([3, 5])
    expect(activity.lastOnlineOf(3)).toBeUndefined()
    release([{ user_id: 3, last_online_at: 1760000000 }, { user_id: 5, last_online_at: null }])
    await loading
    expect(activity.lastOnlineOf(3)).toBe(1760000000)
    expect(activity.lastOnlineOf(5)).toBeNull()
    expect(activity.lastOnlineOf(9)).toBeUndefined()
    expect(activity.failed.value).toBe(false)
  })

  it('asks for nothing when the page has no users', async () => {
    const activity = useUserActivity()
    await activity.load([])
    expect(api.getUsersActivity).not.toHaveBeenCalled()
  })

  it('records a failure without throwing, and recovers on the next page', async () => {
    api.getUsersActivity.mockRejectedValueOnce(new Error('Network Error')).mockResolvedValueOnce([{ user_id: 3, last_online_at: 5 }])
    const activity = useUserActivity()
    await activity.load([3])
    expect(activity.failed.value).toBe(true)
    await activity.load([3])
    expect(activity.failed.value).toBe(false)
    expect(activity.lastOnlineOf(3)).toBe(5)
  })

  it('drops the answer to a page the administrator has left', async () => {
    let releaseFirst
    api.getUsersActivity
      .mockReturnValueOnce(new Promise((resolve) => { releaseFirst = resolve }))
      .mockResolvedValueOnce([{ user_id: 8, last_online_at: 8 }])
    const activity = useUserActivity()
    const first = activity.load([3])
    await activity.load([8])
    releaseFirst([{ user_id: 3, last_online_at: 3 }])
    await first
    expect(activity.lastOnlineOf(3)).toBeUndefined()
    expect(activity.lastOnlineOf(8)).toBe(8)

    // A failure of an earlier request does not mark the newer page as failed.
    let rejectOld
    api.getUsersActivity
      .mockReturnValueOnce(new Promise((_, reject) => { rejectOld = reject }))
      .mockResolvedValueOnce([{ user_id: 9, last_online_at: 9 }])
    const old = activity.load([1])
    await activity.load([9])
    rejectOld(new Error('late'))
    await old
    expect(activity.failed.value).toBe(false)
  })
})

describe('UserLastOnline', () => {
  beforeEach(async () => {
    await setLocale('en')
  })

  it('shows a relative time with the exact time on hover', () => {
    const seconds = Math.floor(Date.now() / 1000) - 7200
    const wrapper = mount(UserLastOnline, { props: { value: seconds } })
    const time = wrapper.get('time')
    expect(time.text()).toBe('2 hours ago')
    expect(time.attributes('datetime')).toBe(new Date(seconds * 1000).toISOString())
    expect(time.attributes('title')).toMatch(/^\d{4}-\d{2}-\d{2} \d{2}:\d{2}$/)
  })

  it('says Never for null, a dash while loading and Unavailable for a failure', () => {
    expect(mount(UserLastOnline, { props: { value: null } }).text()).toBe('Never')
    // A dash for the eye, "Loading" for a screen reader.
    const loading = mount(UserLastOnline)
    expect(loading.get('[aria-hidden="true"]').text()).toBe('—')
    expect(loading.get('.visually-hidden').text()).toBe('Loading')
    expect(mount(UserLastOnline, { props: { value: 5, failed: true } }).text()).toBe('Unavailable')
  })
})
