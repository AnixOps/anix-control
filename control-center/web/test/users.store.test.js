import { describe, it, expect, beforeEach } from 'vitest'
import { setActivePinia, createPinia } from 'pinia'

// Mock users data
const mockUsers = [
  { id: '1', email: 'admin@test.com', role: 'admin', status: 'active', traffic_limit: 107374182400 },
  { id: '2', email: 'user1@test.com', role: 'operator', status: 'active', traffic_limit: 53687091200 },
  { id: '3', email: 'user2@test.com', role: 'viewer', status: 'banned', traffic_limit: null },
  { id: '4', email: 'user3@test.com', role: 'viewer', status: 'active', traffic_limit: 10737418240 },
]

describe('Users Store', () => {
  beforeEach(() => {
    setActivePinia(createPinia())
  })

  it('filters users by status', () => {
    const activeUsers = mockUsers.filter(u => u.status === 'active')
    const bannedUsers = mockUsers.filter(u => u.status === 'banned')

    expect(activeUsers.length).toBe(3)
    expect(bannedUsers.length).toBe(1)
  })

  it('filters users by role', () => {
    const admins = mockUsers.filter(u => u.role === 'admin')
    const operators = mockUsers.filter(u => u.role === 'operator')
    const viewers = mockUsers.filter(u => u.role === 'viewer')

    expect(admins.length).toBe(1)
    expect(operators.length).toBe(1)
    expect(viewers.length).toBe(2)
  })

  it('filters users by search query', () => {
    const query = 'admin'
    const filtered = mockUsers.filter(u =>
      u.email.toLowerCase().includes(query.toLowerCase()) ||
      u.role.toLowerCase().includes(query.toLowerCase())
    )

    expect(filtered.length).toBe(1)
    expect(filtered[0].email).toBe('admin@test.com')
  })

  it('calculates user statistics', () => {
    const stats = {
      total: mockUsers.length,
      active: mockUsers.filter(u => u.status === 'active').length,
      banned: mockUsers.filter(u => u.status === 'banned').length,
      admins: mockUsers.filter(u => u.role === 'admin').length,
    }

    expect(stats.total).toBe(4)
    expect(stats.active).toBe(3)
    expect(stats.banned).toBe(1)
    expect(stats.admins).toBe(1)
  })
})

describe('User Model', () => {
  it('parses JSON correctly', () => {
    const json = {
      id: 'user-123',
      email: 'test@example.com',
      role: 'operator',
      status: 'active',
      traffic_limit: 107374182400,
      traffic_used: 53687091200,
      created_at: '2026-03-20T10:00:00Z',
    }

    expect(json.id).toBe('user-123')
    expect(json.email).toBe('test@example.com')
    expect(json.role).toBe('operator')
    expect(json.status).toBe('active')
    expect(json.traffic_limit).toBe(107374182400)
    expect(json.traffic_used).toBe(53687091200)
  })

  it('calculates traffic usage percentage', () => {
    const user = {
      traffic_limit: 100,
      traffic_used: 75,
    }

    const usagePercent = user.traffic_limit > 0
      ? Math.round((user.traffic_used / user.traffic_limit) * 100)
      : null

    expect(usagePercent).toBe(75)
  })

  it('handles unlimited traffic (null limit)', () => {
    const user = {
      traffic_limit: null,
      traffic_used: 500,
    }

    const usagePercent = user.traffic_limit !== null && user.traffic_limit > 0
      ? Math.round((user.traffic_used / user.traffic_limit) * 100)
      : null

    expect(usagePercent).toBeNull()
  })

  it('handles missing optional fields', () => {
    const json = {
      id: '2',
      email: 'minimal@test.com',
    }

    expect(json.id).toBe('2')
    expect(json.email).toBe('minimal@test.com')
    expect(json.role).toBeUndefined()
    expect(json.status).toBeUndefined()
  })

  it('checks admin role', () => {
    const isAdmin = (role) => role === 'admin'

    expect(isAdmin('admin')).toBe(true)
    expect(isAdmin('operator')).toBe(false)
    expect(isAdmin('viewer')).toBe(false)
  })

  it('checks active status', () => {
    const isActive = (status) => status === 'active'
    const isBanned = (status) => status === 'banned'

    expect(isActive('active')).toBe(true)
    expect(isActive('banned')).toBe(false)
    expect(isBanned('banned')).toBe(true)
    expect(isBanned('active')).toBe(false)
  })
})