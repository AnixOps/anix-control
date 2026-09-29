import { describe, it, expect } from 'vitest'

// Event data
const mockEvents = [
  { id: 'e1', type: 'node.created', message: 'Node server-1 created', timestamp: '2026-03-23T10:00:00Z', severity: 'info' },
  { id: 'e2', type: 'task.failed', message: 'Task #123 failed on node-2', timestamp: '2026-03-23T09:30:00Z', severity: 'error' },
  { id: 'e3', type: 'user.login', message: 'User admin logged in', timestamp: '2026-03-23T09:00:00Z', severity: 'info' }
]

describe('Events', () => {
  it('lists all events', () => {
    expect(mockEvents.length).toBe(3)
  })

  it('filters by severity', () => {
    const errors = mockEvents.filter(e => e.severity === 'error')
    const info = mockEvents.filter(e => e.severity === 'info')
    expect(errors.length).toBe(1)
    expect(info.length).toBe(2)
  })

  it('filters by type', () => {
    const nodeEvents = mockEvents.filter(e => e.type.startsWith('node.'))
    expect(nodeEvents.length).toBe(1)
  })

  it('sorts by timestamp descending', () => {
    const sorted = [...mockEvents].sort((a, b) =>
      new Date(b.timestamp).getTime() - new Date(a.timestamp).getTime()
    )
    expect(sorted[0].id).toBe('e1')
  })

  it('formats timestamp', () => {
    const formatTime = (ts) => new Date(ts).toLocaleString()
    expect(formatTime('2026-03-23T10:00:00Z')).toContain('2026')
  })
})

describe('Event Severity', () => {
  it('maps severity to colors', () => {
    const colors = { info: 'blue', warning: 'orange', error: 'red' }
    expect(colors['info']).toBe('blue')
    expect(colors['error']).toBe('red')
  })

  it('counts events by severity', () => {
    const bySeverity = mockEvents.reduce((acc, e) => {
      acc[e.severity] = (acc[e.severity] || 0) + 1
      return acc
    }, {})
    expect(bySeverity['error']).toBe(1)
    expect(bySeverity['info']).toBe(2)
  })
})

describe('Event Filtering', () => {
  it('searches by message', () => {
    const results = mockEvents.filter(e =>
      e.message.toLowerCase().includes('failed')
    )
    expect(results.length).toBe(1)
    expect(results[0].type).toBe('task.failed')
  })

  it('filters by time range', () => {
    const since = new Date('2026-03-23T09:30:00Z')
    const recent = mockEvents.filter(e =>
      new Date(e.timestamp) >= since
    )
    expect(recent.length).toBe(2)
  })
})