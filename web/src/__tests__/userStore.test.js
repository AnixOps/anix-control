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

  it('normalizes grants, fails closed on missing metadata, and honors explicit legacy mode', () => {
    const userStore = useUserStore()

    userStore.login('admin-token', {
      id: 1,
      is_admin: true,
      permissions: ['machine-telemetry.view', 'machine-telemetry.view', ' '],
    })
    expect(userStore.permissions).toEqual(['machine-telemetry.view'])
    expect(userStore.hasPermission('machine-telemetry.view')).toBe(true)
    expect(userStore.hasPermission('wireguard.view')).toBe(false)

    userStore.login('admin-token', { id: 1, is_admin: true })
    expect(userStore.permissions).toBe(null)
    expect(userStore.permissionMode).toBe('missing')
    expect(userStore.hasPermission('legacy.view')).toBe(false)

    userStore.login('admin-token', { id: 1, is_admin: true, permission_mode: 'legacy' })
    expect(userStore.permissions).toBe(null)
    expect(userStore.permissionMode).toBe('legacy')
    expect(userStore.hasPermission('legacy.view')).toBe(true)

    userStore.login('admin-token', {
      id: 1,
      is_admin: true,
      permissions: { 'forward.view': true, 'wireguard.view': false, 'gost.view': '1' },
    })
    expect(userStore.permissions).toEqual(['forward.view', 'gost.view'])
    expect(userStore.permissionMode).toBe('authoritative')

    userStore.login('admin-token', {
      id: 1,
      is_admin: true,
      permissions: ['machine-telemetry.*', '*'],
    })
    expect(userStore.hasPermission('machine-telemetry.view')).toBe(true)
    expect(userStore.hasPermission('machine-telemetry.manage')).toBe(true)
    expect(userStore.hasPermission('wireguard.view')).toBe(false)
  })

  it('keeps legacy admin access outside restricted plugins in mixed mode', () => {
    const userStore = useUserStore()
    userStore.login('admin-token', {
      id: 1,
      is_admin: true,
      permission_mode: 'mixed',
      restricted_plugins: ['forward', 'forward', ' ', 'wireguard'],
      permissions: ['forward.view', 'wireguard.*', '*'],
    })

    expect(userStore.permissionMode).toBe('mixed')
    expect(userStore.restrictedPlugins).toEqual(['forward', 'wireguard'])
    expect(userStore.hasPermission('forward.view')).toBe(true)
    expect(userStore.hasPermission('forward.manage')).toBe(false)
    expect(userStore.hasPermission('wireguard.view')).toBe(true)
    expect(userStore.hasPermission('other-plugin.view')).toBe(true)
    expect(userStore.hasPermission('other-plugin.manage')).toBe(true)
  })

  it('fails closed for mixed admins when restricted plugin metadata is absent', () => {
    const userStore = useUserStore()
    userStore.login('admin-token', {
      id: 1,
      is_admin: true,
      permission_mode: 'mixed',
      permissions: ['*'],
    })

    expect(userStore.permissionMode).toBe('mixed')
    expect(userStore.restrictedPlugins).toBe(null)
    expect(userStore.hasPermission('forward.view')).toBe(false)
  })

  it('matches dotted plugin IDs without collapsing their permission namespace', () => {
    const userStore = useUserStore()
    userStore.login('admin-token', {
      id: 1,
      is_admin: true,
      permission_mode: 'mixed',
      restricted_plugins: ['machine.telemetry'],
      permissions: ['machine.telemetry.*'],
    })

    expect(userStore.restrictedPlugins).toEqual(['machine.telemetry'])
    expect(userStore.hasPermission('machine.telemetry.view')).toBe(true)
    expect(userStore.hasPermission('machine.telemetry.manage')).toBe(true)
    expect(userStore.hasPermission('machine.other.view')).toBe(true)
  })

  it('keeps server-authoritative grants when a profile refresh omits the permissions field', async () => {
    const userStore = useUserStore()
    userStore.login('admin-token', {
      id: 1,
      email: 'before-refresh@example.com',
      is_admin: true,
      permission_mode: 'mixed',
      restricted_plugins: ['machine-telemetry'],
      permissions: ['machine-telemetry.view'],
    })
    mockGetProfile.mockResolvedValue({
      data: {
        id: 1,
        email: 'after-refresh@example.com',
        is_admin: true,
      },
    })

    await userStore.getUserInfo()

    expect(userStore.userInfo).toMatchObject({
      email: 'after-refresh@example.com',
      permissions: ['machine-telemetry.view'],
    })
    expect(userStore.permissions).toEqual(['machine-telemetry.view'])
    expect(userStore.permissionMode).toBe('mixed')
    expect(userStore.restrictedPlugins).toEqual(['machine-telemetry'])
    expect(userStore.hasPermission('machine-telemetry.view')).toBe(true)
    expect(userStore.hasPermission('wireguard.view')).toBe(true)
    expect(JSON.parse(localStorage.getItem('userInfo')).permissions).toEqual(['machine-telemetry.view'])
  })

  it('fails closed when an authoritative profile omits its permission list', async () => {
    const userStore = useUserStore()
    userStore.login('admin-token', {
      id: 1,
      is_admin: true,
      permissions: ['machine-telemetry.view'],
    })
    mockGetProfile.mockResolvedValue({
      data: {
        id: 1,
        is_admin: true,
        permission_mode: 'authoritative',
      },
    })

    await userStore.getUserInfo()

    expect(userStore.permissions).toBe(null)
    expect(userStore.permissionMode).toBe('authoritative')
    expect(userStore.hasPermission('machine-telemetry.view')).toBe(false)
    expect(userStore.userInfo).not.toHaveProperty('permissions')
  })
})
