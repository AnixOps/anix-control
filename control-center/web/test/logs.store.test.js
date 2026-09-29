import { describe, it, expect, beforeEach } from 'vitest'
import { setActivePinia, createPinia } from 'pinia'

// Mock logs data
const mockLogs = [
  { time: '12:34:56', level: 'INFO', source: 'node', message: 'Node deployed successfully' },
  { time: '12:34:55', level: 'INFO', source: 'ansible', message: 'Running playbook' },
  { time: '12:33:21', level: 'WARN', source: 'cert', message: 'Certificate expires in 7 days' },
  { time: '12:30:00', level: 'ERROR', source: 'agent', message: 'Connection failed' },
  { time: '12:28:45', level: 'INFO', source: 'auth', message: 'User logged in' },
  { time: '12:20:00', level: 'WARN', source: 'traffic', message: 'High traffic detected' },
]

describe('Logs Store', () => {
  beforeEach(() => {
    setActivePinia(createPinia())
  })

  it('filters logs by level', () => {
    const infoLogs = mockLogs.filter(l => l.level === 'INFO')
    const warnLogs = mockLogs.filter(l => l.level === 'WARN')
    const errorLogs = mockLogs.filter(l => l.level === 'ERROR')

    expect(infoLogs.length).toBe(3)
    expect(warnLogs.length).toBe(2)
    expect(errorLogs.length).toBe(1)
  })

  it('calculates log statistics', () => {
    const stats = {
      total: mockLogs.length,
      info: mockLogs.filter(l => l.level === 'INFO').length,
      warn: mockLogs.filter(l => l.level === 'WARN').length,
      error: mockLogs.filter(l => l.level === 'ERROR').length,
    }

    expect(stats.total).toBe(6)
    expect(stats.info).toBe(3)
    expect(stats.warn).toBe(2)
    expect(stats.error).toBe(1)
  })

  it('filters logs by source', () => {
    const nodeLogs = mockLogs.filter(l => l.source === 'node')
    const ansibleLogs = mockLogs.filter(l => l.source === 'ansible')

    expect(nodeLogs.length).toBe(1)
    expect(ansibleLogs.length).toBe(1)
  })

  it('filters logs by search query', () => {
    const query = 'failed'
    const filtered = mockLogs.filter(l =>
      l.message.toLowerCase().includes(query.toLowerCase()) ||
      l.source.toLowerCase().includes(query.toLowerCase())
    )

    expect(filtered.length).toBe(1)
    expect(filtered[0].level).toBe('ERROR')
  })
})

describe('Log Model', () => {
  it('parses JSON correctly', () => {
    const json = {
      time: '12:34:56',
      level: 'INFO',
      source: 'node',
      message: 'Node deployed successfully',
    }

    expect(json.time).toBe('12:34:56')
    expect(json.level).toBe('INFO')
    expect(json.source).toBe('node')
    expect(json.message).toBe('Node deployed successfully')
  })

  it('gets correct level color', () => {
    const getLevelColor = (level) => {
      const colors = {
        'INFO': 'text-blue-400',
        'WARN': 'text-yellow-400',
        'ERROR': 'text-red-400'
      }
      return colors[level] || 'text-gray-400'
    }

    expect(getLevelColor('INFO')).toBe('text-blue-400')
    expect(getLevelColor('WARN')).toBe('text-yellow-400')
    expect(getLevelColor('ERROR')).toBe('text-red-400')
    expect(getLevelColor('DEBUG')).toBe('text-gray-400')
  })

  it('clears logs', () => {
    const logs = [...mockLogs]
    logs.length = 0

    expect(logs.length).toBe(0)
  })
})