import { describe, it, expect, beforeEach, vi } from 'vitest'
import { setActivePinia, createPinia } from 'pinia'
import { useAuthStore } from '@/stores/auth'

// Mock localStorage
const localStorageMock = {
  getItem: vi.fn(),
  setItem: vi.fn(),
  removeItem: vi.fn(),
  clear: vi.fn(),
}

Object.defineProperty(global, 'localStorage', {
  value: localStorageMock,
})

describe('Auth Store', () => {
  beforeEach(() => {
    setActivePinia(createPinia())
    localStorageMock.getItem.mockReset()
    localStorageMock.setItem.mockClear()
    localStorageMock.removeItem.mockClear()
  })

  it('initializes with no token', () => {
    localStorageMock.getItem.mockReturnValue(null)

    const store = useAuthStore()

    expect(store.token).toBeNull()
    expect(store.user).toBeNull()
    expect(store.isAuthenticated).toBe(false)
  })

  it('isAuthenticated returns true when token exists', () => {
    // Mock getItem to return token for 'token' key and null for others
    localStorageMock.getItem.mockImplementation((key) => {
      if (key === 'token') return 'test-token'
      return null
    })

    const store = useAuthStore()
    store.token = 'test-token'

    expect(store.isAuthenticated).toBe(true)
  })

  it('isAdmin returns true for admin role', () => {
    localStorageMock.getItem.mockReturnValue(null)

    const store = useAuthStore()
    store.user = { id: '1', email: 'admin@test.com', role: 'admin' }

    expect(store.isAdmin).toBe(true)
  })

  it('isAdmin returns false for non-admin role', () => {
    localStorageMock.getItem.mockReturnValue(null)

    const store = useAuthStore()
    store.user = { id: '1', email: 'user@test.com', role: 'viewer' }

    expect(store.isAdmin).toBe(false)
  })

  it('logout clears token and user', () => {
    localStorageMock.getItem.mockReturnValue(null)

    const store = useAuthStore()
    store.token = 'test-token'
    store.user = { id: '1', email: 'test@test.com', role: 'admin' }

    store.logout()

    expect(store.token).toBeNull()
    expect(store.user).toBeNull()
    expect(localStorageMock.removeItem).toHaveBeenCalledWith('token')
    expect(localStorageMock.removeItem).toHaveBeenCalledWith('user')
  })
})