import { describe, it, expect, beforeEach, vi } from 'vitest'
import { createPinia, setActivePinia } from 'pinia'
import { useUserStore } from '@/stores/user'

const mockGetProfile = vi.hoisted(() => vi.fn())

vi.mock('@/api/user', () => ({
  getProfile: (...args) => mockGetProfile(...args),
}))

describe('user store', () => {
  beforeEach(() => {
    localStorage.clear()
    setActivePinia(createPinia())
    mockGetProfile.mockReset()
  })

  it('loads user info from a panel envelope profile payload', async () => {
    mockGetProfile.mockResolvedValue({
      code: 0,
      msg: '操作成功',
      ts: 1783536000000,
      data: {
        id: 12,
        email: 'profile@example.com',
        uuid: 'profile-uuid',
        token: 'profile-sub-token',
        is_admin: 1,
      },
    })

    const userStore = useUserStore()
    await userStore.getUserInfo()

    expect(mockGetProfile).toHaveBeenCalledTimes(1)
    expect(userStore.userInfo).toMatchObject({
      id: 12,
      email: 'profile@example.com',
      uuid: 'profile-uuid',
      token: 'profile-sub-token',
      is_admin: true,
    })
    expect(JSON.parse(localStorage.getItem('userInfo'))).toMatchObject({
      email: 'profile@example.com',
      is_admin: true,
    })
  })
})
