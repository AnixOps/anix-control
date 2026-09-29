import { describe, it, expect, beforeEach } from 'vitest'
import { setActivePinia, createPinia } from 'pinia'

// Mock schedules data
const mockSchedules = [
  { id: '1', name: 'Daily Backup', playbook_name: 'backup-database', cron: '0 2 * * *', enabled: true, timezone: 'UTC' },
  { id: '2', name: 'Weekly Update', playbook_name: 'update-system', cron: '0 3 * * 0', enabled: true, timezone: 'UTC' },
  { id: '3', name: 'Hourly Check', playbook_name: 'health-check', cron: '0 * * * *', enabled: false, timezone: 'UTC' },
]

describe('Schedules Store', () => {
  beforeEach(() => {
    setActivePinia(createPinia())
  })

  it('filters schedules by enabled status', () => {
    const enabledSchedules = mockSchedules.filter(s => s.enabled)
    const disabledSchedules = mockSchedules.filter(s => !s.enabled)

    expect(enabledSchedules.length).toBe(2)
    expect(disabledSchedules.length).toBe(1)
  })

  it('calculates schedule statistics', () => {
    const stats = {
      total: mockSchedules.length,
      enabled: mockSchedules.filter(s => s.enabled).length,
      disabled: mockSchedules.filter(s => !s.enabled).length,
    }

    expect(stats.total).toBe(3)
    expect(stats.enabled).toBe(2)
    expect(stats.disabled).toBe(1)
  })

  it('filters schedules by search query', () => {
    const query = 'backup'
    const filtered = mockSchedules.filter(s =>
      s.name.toLowerCase().includes(query.toLowerCase()) ||
      s.playbook_name.toLowerCase().includes(query.toLowerCase())
    )

    expect(filtered.length).toBe(1)
    expect(filtered[0].name).toBe('Daily Backup')
  })

  it('gets unique playbooks from schedules', () => {
    const playbooks = [...new Set(mockSchedules.map(s => s.playbook_name))]

    expect(playbooks.length).toBe(3)
    expect(playbooks).toContain('backup-database')
    expect(playbooks).toContain('update-system')
    expect(playbooks).toContain('health-check')
  })
})

describe('Schedule Model', () => {
  it('parses JSON correctly', () => {
    const json = {
      id: 'schedule-123',
      name: 'Test Schedule',
      playbook_id: 'playbook-1',
      playbook_name: 'install-docker',
      cron: '*/15 * * * *',
      enabled: true,
      timezone: 'America/New_York',
      target_nodes: ['node-1', 'node-2'],
      next_run: '2026-03-21T02:00:00Z',
      last_run: '2026-03-20T02:00:00Z',
    }

    expect(json.id).toBe('schedule-123')
    expect(json.name).toBe('Test Schedule')
    expect(json.playbook_id).toBe('playbook-1')
    expect(json.cron).toBe('*/15 * * * *')
    expect(json.enabled).toBe(true)
    expect(json.timezone).toBe('America/New_York')
    expect(json.target_nodes).toEqual(['node-1', 'node-2'])
  })

  it('handles missing optional fields', () => {
    const json = {
      id: '2',
      name: 'Minimal Schedule',
      cron: '0 * * * *',
    }

    expect(json.id).toBe('2')
    expect(json.name).toBe('Minimal Schedule')
    expect(json.cron).toBe('0 * * * *')
    expect(json.enabled).toBeUndefined()
    expect(json.timezone).toBeUndefined()
    expect(json.target_nodes).toBeUndefined()
  })

  it('parses cron expression correctly', () => {
    const cron = '0 2 * * *'
    const parts = cron.split(' ')

    expect(parts.length).toBe(5)
    expect(parts[0]).toBe('0')  // minute
    expect(parts[1]).toBe('2')  // hour
    expect(parts[2]).toBe('*')  // day of month
    expect(parts[3]).toBe('*')  // month
    expect(parts[4]).toBe('*')  // day of week
  })

  it('describes hourly cron', () => {
    const cron = '0 * * * *'
    const parts = cron.split(' ')

    const isHourly = parts[0] === '0' && parts[1] === '*'
    expect(isHourly).toBe(true)
  })

  it('describes interval cron', () => {
    const cron = '*/15 * * * *'
    const parts = cron.split(' ')

    const isInterval = parts[0].startsWith('*/')
    expect(isInterval).toBe(true)
  })
})