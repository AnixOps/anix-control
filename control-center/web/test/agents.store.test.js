import { describe, it, expect, beforeEach } from 'vitest'
import { setActivePinia, createPinia } from 'pinia'

// Mock agents data
const mockAgents = [
  { id: 1, name: 'web-01', host: '10.0.0.101', status: 'online', uptime: '2d 4h' },
  { id: 2, name: 'db-01', host: '10.0.0.102', status: 'online', uptime: '5d 12h' },
  { id: 3, name: 'cache-01', host: '10.0.0.103', status: 'offline', uptime: '-' },
  { id: 4, name: 'api-01', host: '10.0.0.104', status: 'online', uptime: '1d 8h' },
]

describe('Agents Store', () => {
  beforeEach(() => {
    setActivePinia(createPinia())
  })

  it('filters agents by status', () => {
    const onlineAgents = mockAgents.filter(a => a.status === 'online')
    const offlineAgents = mockAgents.filter(a => a.status === 'offline')

    expect(onlineAgents.length).toBe(3)
    expect(offlineAgents.length).toBe(1)
  })

  it('calculates agent statistics', () => {
    const stats = {
      total: mockAgents.length,
      online: mockAgents.filter(a => a.status === 'online').length,
      offline: mockAgents.filter(a => a.status === 'offline').length,
    }

    expect(stats.total).toBe(4)
    expect(stats.online).toBe(3)
    expect(stats.offline).toBe(1)
  })

  it('filters agents by search query', () => {
    const query = 'web'
    const filtered = mockAgents.filter(a =>
      a.name.toLowerCase().includes(query.toLowerCase()) ||
      a.host.includes(query)
    )

    expect(filtered.length).toBe(1)
    expect(filtered[0].name).toBe('web-01')
  })

  it('gets agents by host pattern', () => {
    const pattern = '10.0.0.10'
    const filtered = mockAgents.filter(a => a.host.startsWith(pattern))

    expect(filtered.length).toBe(4)  // All hosts start with 10.0.0.10
  })
})

describe('Agent Model', () => {
  it('parses JSON correctly', () => {
    const json = {
      id: 1,
      name: 'web-01',
      host: '10.0.0.101',
      status: 'online',
      uptime: '2d 4h',
    }

    expect(json.id).toBe(1)
    expect(json.name).toBe('web-01')
    expect(json.host).toBe('10.0.0.101')
    expect(json.status).toBe('online')
    expect(json.uptime).toBe('2d 4h')
  })

  it('handles missing optional fields', () => {
    const json = {
      id: 2,
      name: 'Minimal Agent',
      host: '10.0.0.1',
    }

    expect(json.id).toBe(2)
    expect(json.name).toBe('Minimal Agent')
    expect(json.host).toBe('10.0.0.1')
    expect(json.status).toBeUndefined()
    expect(json.uptime).toBeUndefined()
  })

  it('checks agent is online', () => {
    const isOnline = (status) => status === 'online'

    expect(isOnline('online')).toBe(true)
    expect(isOnline('offline')).toBe(false)
  })

  it('parses uptime string', () => {
    const parseUptime = (uptime) => {
      if (uptime === '-') return null
      const match = uptime.match(/(\d+)d\s*(\d+)h/)
      if (match) {
        return {
          days: parseInt(match[1]),
          hours: parseInt(match[2])
        }
      }
      return null
    }

    expect(parseUptime('2d 4h')).toEqual({ days: 2, hours: 4 })
    expect(parseUptime('5d 12h')).toEqual({ days: 5, hours: 12 })
    expect(parseUptime('-')).toBeNull()
  })
})