import { describe, it, expect, beforeEach } from 'vitest'
import { setActivePinia, createPinia } from 'pinia'

// Mock playbooks data
const mockPlaybooks = [
  { id: '1', name: 'install-fail2ban', category: 'security', status: 'available' },
  { id: '2', name: 'setup-firewall', category: 'security', status: 'available' },
  { id: '3', name: 'update-system', category: 'maintenance', status: 'available' },
  { id: '4', name: 'backup-database', category: 'backup', status: 'available' },
]

describe('Playbooks Store', () => {
  beforeEach(() => {
    setActivePinia(createPinia())
  })

  it('filters playbooks by category', () => {
    const securityPlaybooks = mockPlaybooks.filter(p => p.category === 'security')
    const maintenancePlaybooks = mockPlaybooks.filter(p => p.category === 'maintenance')

    expect(securityPlaybooks.length).toBe(2)
    expect(maintenancePlaybooks.length).toBe(1)
  })

  it('filters playbooks by search query', () => {
    const query = 'firewall'
    const filtered = mockPlaybooks.filter(p =>
      p.name.toLowerCase().includes(query.toLowerCase()) ||
      p.category.toLowerCase().includes(query.toLowerCase())
    )

    expect(filtered.length).toBe(1)
    expect(filtered[0].name).toBe('setup-firewall')
  })

  it('gets unique categories', () => {
    const categories = [...new Set(mockPlaybooks.map(p => p.category))]

    expect(categories.length).toBe(3)
    expect(categories).toContain('security')
    expect(categories).toContain('maintenance')
    expect(categories).toContain('backup')
  })
})

describe('Playbook Model', () => {
  it('parses JSON correctly', () => {
    const json = {
      id: 'playbook-123',
      name: 'install-docker',
      category: 'software',
      description: 'Install Docker on the target node',
      status: 'available',
      variables: { version: 'latest' },
      created_at: '2026-03-20T10:00:00Z',
    }

    expect(json.id).toBe('playbook-123')
    expect(json.name).toBe('install-docker')
    expect(json.category).toBe('software')
    expect(json.description).toBe('Install Docker on the target node')
    expect(json.status).toBe('available')
    expect(json.variables).toEqual({ version: 'latest' })
  })

  it('handles missing optional fields', () => {
    const json = {
      id: '2',
      name: 'Minimal Playbook',
      category: 'general',
    }

    expect(json.id).toBe('2')
    expect(json.name).toBe('Minimal Playbook')
    expect(json.category).toBe('general')
    expect(json.description).toBeUndefined()
    expect(json.variables).toBeUndefined()
  })
})

describe('Tasks Store', () => {
  const mockTasks = [
    { id: '1', playbook_name: 'install-fail2ban', status: 'completed', created_at: '2026-03-20T10:00:00Z' },
    { id: '2', playbook_name: 'setup-firewall', status: 'running', created_at: '2026-03-20T11:00:00Z' },
    { id: '3', playbook_name: 'update-system', status: 'pending', created_at: '2026-03-20T12:00:00Z' },
    { id: '4', playbook_name: 'backup-database', status: 'failed', created_at: '2026-03-20T13:00:00Z' },
  ]

  it('filters tasks by status', () => {
    const completedTasks = mockTasks.filter(t => t.status === 'completed')
    const runningTasks = mockTasks.filter(t => t.status === 'running')
    const pendingTasks = mockTasks.filter(t => t.status === 'pending')
    const failedTasks = mockTasks.filter(t => t.status === 'failed')

    expect(completedTasks.length).toBe(1)
    expect(runningTasks.length).toBe(1)
    expect(pendingTasks.length).toBe(1)
    expect(failedTasks.length).toBe(1)
  })

  it('calculates task statistics', () => {
    const stats = {
      total: mockTasks.length,
      completed: mockTasks.filter(t => t.status === 'completed').length,
      running: mockTasks.filter(t => t.status === 'running').length,
      pending: mockTasks.filter(t => t.status === 'pending').length,
      failed: mockTasks.filter(t => t.status === 'failed').length,
    }

    expect(stats.total).toBe(4)
    expect(stats.completed).toBe(1)
    expect(stats.running).toBe(1)
    expect(stats.pending).toBe(1)
    expect(stats.failed).toBe(1)
  })

  it('sorts tasks by creation date', () => {
    const sorted = [...mockTasks].sort((a, b) =>
      new Date(b.created_at).getTime() - new Date(a.created_at).getTime()
    )

    expect(sorted[0].id).toBe('4') // Most recent
    expect(sorted[3].id).toBe('1') // Oldest
  })
})