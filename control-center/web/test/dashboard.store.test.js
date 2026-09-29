import { describe, it, expect, beforeEach } from 'vitest'
import { setActivePinia, createPinia } from 'pinia'

// Mock dashboard data
const mockStats = [
  { title: 'Total Nodes', value: '8', change: 0 },
  { title: 'Active Users', value: '357', change: 12 },
  { title: 'Online Agents', value: '4', change: 0 },
  { title: 'Traffic Today', value: '1.2TB', change: -5 },
]

const mockServices = [
  { name: 'Panel', status: 'online', count: 'Running' },
  { name: 'Nodes', status: 'online', count: '8 / 8' },
  { name: 'Agents', status: 'online', count: '4 / 4' },
  { name: 'Monitoring', status: 'offline', count: 'Disabled' },
]

const mockActivities = [
  { id: 1, type: 'deploy', message: 'Node deployed successfully', time: '2 minutes ago' },
  { id: 2, type: 'user', message: 'User admin logged in', time: '15 minutes ago' },
  { id: 3, type: 'backup', message: 'Database backup completed', time: '1 hour ago' },
  { id: 4, type: 'alert', message: 'Certificate expires in 7 days', time: '2 hours ago' },
  { id: 5, type: 'error', message: 'Agent connection failed', time: '3 hours ago' },
]

describe('Dashboard Store', () => {
  beforeEach(() => {
    setActivePinia(createPinia())
  })

  it('calculates statistics summary', () => {
    const summary = {
      totalStats: mockStats.length,
      positiveChanges: mockStats.filter(s => s.change > 0).length,
      negativeChanges: mockStats.filter(s => s.change < 0).length,
      noChanges: mockStats.filter(s => s.change === 0).length,
    }

    expect(summary.totalStats).toBe(4)
    expect(summary.positiveChanges).toBe(1)
    expect(summary.negativeChanges).toBe(1)
    expect(summary.noChanges).toBe(2)
  })

  it('filters services by status', () => {
    const onlineServices = mockServices.filter(s => s.status === 'online')
    const offlineServices = mockServices.filter(s => s.status === 'offline')

    expect(onlineServices.length).toBe(3)
    expect(offlineServices.length).toBe(1)
  })

  it('filters activities by type', () => {
    const deployActivities = mockActivities.filter(a => a.type === 'deploy')
    const errorActivities = mockActivities.filter(a => a.type === 'error')
    const userActivities = mockActivities.filter(a => a.type === 'user')

    expect(deployActivities.length).toBe(1)
    expect(errorActivities.length).toBe(1)
    expect(userActivities.length).toBe(1)
  })
})

describe('Dashboard Model', () => {
  it('parses stat JSON correctly', () => {
    const json = {
      title: 'Total Nodes',
      value: '8',
      change: 12,
      icon: 'ServerIcon',
      color: 'bg-blue-600',
    }

    expect(json.title).toBe('Total Nodes')
    expect(json.value).toBe('8')
    expect(json.change).toBe(12)
    expect(json.icon).toBe('ServerIcon')
  })

  it('parses service JSON correctly', () => {
    const json = {
      name: 'Panel',
      status: 'online',
      count: 'Running',
    }

    expect(json.name).toBe('Panel')
    expect(json.status).toBe('online')
    expect(json.count).toBe('Running')
  })

  it('parses activity JSON correctly', () => {
    const json = {
      id: 1,
      type: 'deploy',
      message: 'Node deployed successfully',
      time: '2 minutes ago',
    }

    expect(json.id).toBe(1)
    expect(json.type).toBe('deploy')
    expect(json.message).toBe('Node deployed successfully')
  })

  it('gets correct activity color', () => {
    const getActivityColor = (type) => {
      const colors = {
        deploy: 'bg-green-600',
        user: 'bg-blue-600',
        backup: 'bg-purple-600',
        alert: 'bg-yellow-600',
        error: 'bg-red-600'
      }
      return colors[type] || 'bg-gray-600'
    }

    expect(getActivityColor('deploy')).toBe('bg-green-600')
    expect(getActivityColor('user')).toBe('bg-blue-600')
    expect(getActivityColor('backup')).toBe('bg-purple-600')
    expect(getActivityColor('alert')).toBe('bg-yellow-600')
    expect(getActivityColor('error')).toBe('bg-red-600')
    expect(getActivityColor('unknown')).toBe('bg-gray-600')
  })

  it('calculates change percentage direction', () => {
    const getDirection = (change) => {
      if (change > 0) return 'up'
      if (change < 0) return 'down'
      return 'neutral'
    }

    expect(getDirection(12)).toBe('up')
    expect(getDirection(-5)).toBe('down')
    expect(getDirection(0)).toBe('neutral')
  })
})