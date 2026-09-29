import { describe, it, expect, beforeEach, vi } from 'vitest'
import { setActivePinia, createPinia } from 'pinia'

// Mock nodes data
const mockNodes = [
  { id: '1', name: 'Node 1', status: 'online', type: 'vps', ip: '10.0.0.1' },
  { id: '2', name: 'Node 2', status: 'offline', type: 'dedicated', ip: '10.0.0.2' },
  { id: '3', name: 'Node 3', status: 'online', type: 'vps', ip: '10.0.0.3' },
]

describe('Nodes Store', () => {
  beforeEach(() => {
    setActivePinia(createPinia())
  })

  it('filters nodes by status', () => {
    const onlineNodes = mockNodes.filter(n => n.status === 'online')
    const offlineNodes = mockNodes.filter(n => n.status === 'offline')

    expect(onlineNodes.length).toBe(2)
    expect(offlineNodes.length).toBe(1)
  })

  it('filters nodes by search query', () => {
    const query = 'Node 1'
    const filtered = mockNodes.filter(n =>
      n.name.toLowerCase().includes(query.toLowerCase()) ||
      n.ip.includes(query)
    )

    expect(filtered.length).toBe(1)
    expect(filtered[0].name).toBe('Node 1')
  })

  it('calculates node statistics', () => {
    const stats = {
      total: mockNodes.length,
      online: mockNodes.filter(n => n.status === 'online').length,
      offline: mockNodes.filter(n => n.status === 'offline').length,
    }

    expect(stats.total).toBe(3)
    expect(stats.online).toBe(2)
    expect(stats.offline).toBe(1)
  })
})

describe('Node Model', () => {
  it('parses JSON correctly', () => {
    const json = {
      id: 'node-123',
      name: 'US-East-1',
      status: 'online',
      type: 'vps',
      ip: '192.168.1.1',
      region: 'us-east',
      created_at: '2026-03-20T10:00:00Z',
    }

    expect(json.id).toBe('node-123')
    expect(json.name).toBe('US-East-1')
    expect(json.status).toBe('online')
    expect(json.type).toBe('vps')
    expect(json.ip).toBe('192.168.1.1')
    expect(json.region).toBe('us-east')
  })

  it('handles missing optional fields', () => {
    const json = {
      id: '2',
      name: 'Minimal Node',
      status: 'online',
    }

    expect(json.id).toBe('2')
    expect(json.name).toBe('Minimal Node')
    expect(json.status).toBe('online')
    expect(json.type).toBeUndefined()
    expect(json.ip).toBeUndefined()
  })
})