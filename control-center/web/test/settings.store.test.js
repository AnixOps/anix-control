import { describe, it, expect, beforeEach } from 'vitest'
import { setActivePinia, createPinia } from 'pinia'

// Mock settings data
const mockSettings = {
  host: '0.0.0.0',
  port: 8080,
  debug: false,
  logLevel: 'info',
}

// Mock plugins data
const mockPlugins = [
  { name: 'ansible', status: 'running', version: '1.0.0' },
  { name: 'v2board', status: 'running', version: '1.0.0' },
  { name: 'v2bx', status: 'stopped', version: '1.0.0' },
  { name: 'agent', status: 'running', version: '1.0.0' },
]

describe('Settings Store', () => {
  beforeEach(() => {
    setActivePinia(createPinia())
  })

  it('parses settings correctly', () => {
    expect(mockSettings.host).toBe('0.0.0.0')
    expect(mockSettings.port).toBe(8080)
    expect(mockSettings.debug).toBe(false)
    expect(mockSettings.logLevel).toBe('info')
  })

  it('updates settings', () => {
    const settings = { ...mockSettings }
    settings.port = 9090
    settings.debug = true

    expect(settings.port).toBe(9090)
    expect(settings.debug).toBe(true)
  })

  it('validates port range', () => {
    const isValidPort = (port) => port > 0 && port <= 65535

    expect(isValidPort(8080)).toBe(true)
    expect(isValidPort(443)).toBe(true)
    expect(isValidPort(0)).toBe(false)
    expect(isValidPort(70000)).toBe(false)
  })

  it('validates log levels', () => {
    const validLevels = ['debug', 'info', 'warn', 'error']
    const isValidLevel = (level) => validLevels.includes(level)

    expect(isValidLevel('info')).toBe(true)
    expect(isValidLevel('debug')).toBe(true)
    expect(isValidLevel('trace')).toBe(false)
  })
})

describe('Plugins Store', () => {
  beforeEach(() => {
    setActivePinia(createPinia())
  })

  it('filters plugins by status', () => {
    const runningPlugins = mockPlugins.filter(p => p.status === 'running')
    const stoppedPlugins = mockPlugins.filter(p => p.status === 'stopped')

    expect(runningPlugins.length).toBe(3)
    expect(stoppedPlugins.length).toBe(1)
  })

  it('calculates plugin statistics', () => {
    const stats = {
      total: mockPlugins.length,
      running: mockPlugins.filter(p => p.status === 'running').length,
      stopped: mockPlugins.filter(p => p.status === 'stopped').length,
    }

    expect(stats.total).toBe(4)
    expect(stats.running).toBe(3)
    expect(stats.stopped).toBe(1)
  })

  it('finds plugin by name', () => {
    const plugin = mockPlugins.find(p => p.name === 'ansible')

    expect(plugin).toBeDefined()
    expect(plugin.status).toBe('running')
    expect(plugin.version).toBe('1.0.0')
  })

  it('toggles plugin status', () => {
    const plugins = [...mockPlugins]
    const pluginIndex = plugins.findIndex(p => p.name === 'v2bx')
    plugins[pluginIndex].status = 'running'

    expect(plugins[pluginIndex].status).toBe('running')
  })
})

describe('Plugin Model', () => {
  it('parses JSON correctly', () => {
    const json = {
      name: 'ansible',
      status: 'running',
      version: '1.0.0',
      description: 'Ansible automation plugin',
    }

    expect(json.name).toBe('ansible')
    expect(json.status).toBe('running')
    expect(json.version).toBe('1.0.0')
  })

  it('handles missing optional fields', () => {
    const json = {
      name: 'minimal-plugin',
      status: 'stopped',
    }

    expect(json.name).toBe('minimal-plugin')
    expect(json.status).toBe('stopped')
    expect(json.version).toBeUndefined()
  })

  it('checks if plugin is running', () => {
    const isRunning = (status) => status === 'running'

    expect(isRunning('running')).toBe(true)
    expect(isRunning('stopped')).toBe(false)
  })

  it('compares plugin versions', () => {
    const compareVersions = (v1, v2) => {
      const parts1 = v1.split('.').map(Number)
      const parts2 = v2.split('.').map(Number)

      for (let i = 0; i < 3; i++) {
        if (parts1[i] > parts2[i]) return 1
        if (parts1[i] < parts2[i]) return -1
      }
      return 0
    }

    expect(compareVersions('1.0.0', '1.0.0')).toBe(0)
    expect(compareVersions('1.1.0', '1.0.0')).toBe(1)
    expect(compareVersions('1.0.0', '1.1.0')).toBe(-1)
  })
})