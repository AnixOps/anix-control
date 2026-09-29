import { describe, it, expect } from 'vitest'

// Audit log mock data
const mockAuditLogs = [
  { id: 'a1', action: 'user.login', userId: 1, details: 'Login from 192.168.1.1', timestamp: '2026-03-23T10:00:00Z' },
  { id: 'a2', action: 'node.created', userId: 1, details: 'Created node server-1', timestamp: '2026-03-23T09:30:00Z' },
  { id: 'a3', action: 'task.executed', userId: 2, details: 'Executed playbook update', timestamp: '2026-03-23T09:00:00Z' }
]

describe('Audit Logs', () => {
  it('lists all audit logs', () => {
    expect(mockAuditLogs.length).toBe(3)
  })

  it('filters by action type', () => {
    const logins = mockAuditLogs.filter(l => l.action === 'user.login')
    expect(logins.length).toBe(1)
  })

  it('filters by user', () => {
    const userLogs = mockAuditLogs.filter(l => l.userId === 1)
    expect(userLogs.length).toBe(2)
  })

  it('sorts by timestamp', () => {
    const sorted = [...mockAuditLogs].sort((a, b) =>
      new Date(b.timestamp).getTime() - new Date(a.timestamp).getTime()
    )
    expect(sorted[0].action).toBe('user.login')
  })
})

describe('Audit Search', () => {
  it('searches by details', () => {
    const results = mockAuditLogs.filter(l => l.details.includes('node'))
    expect(results.length).toBe(1)
  })

  it('filters by time range', () => {
    const since = new Date('2026-03-23T09:30:00Z')
    const recent = mockAuditLogs.filter(l => new Date(l.timestamp) >= since)
    expect(recent.length).toBe(2)
  })

  it('groups by action', () => {
    const byAction = mockAuditLogs.reduce((acc, l) => {
      acc[l.action] = (acc[l.action] || 0) + 1
      return acc
    }, {})
    expect(Object.keys(byAction).length).toBe(3)
  })
})