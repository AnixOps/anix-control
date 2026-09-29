import { describe, it, expect } from 'vitest'

// Config Map mock data
const mockConfigMaps = [
  { name: 'app-config', namespace: 'default', data: { 'APP_ENV': 'production', 'LOG_LEVEL': 'info' } },
  { name: 'database-config', namespace: 'default', data: { 'DB_HOST': 'localhost', 'DB_PORT': '5432' } }
]

// Secret mock data (values are base64 encoded)
const mockSecrets = [
  { name: 'db-credentials', namespace: 'default', type: 'Opaque', data: { 'username': 'YWRtaW4=', 'password': 'c2VjcmV0' } },
  { name: 'api-keys', namespace: 'default', type: 'Opaque', data: { 'api_key': 'YWJjZDEyMzQ=' } }
]

describe('Config Maps', () => {
  it('lists all config maps', () => {
    expect(mockConfigMaps.length).toBe(2)
  })

  it('filters by namespace', () => {
    const defaultNs = mockConfigMaps.filter(cm => cm.namespace === 'default')
    expect(defaultNs.length).toBe(2)
  })

  it('gets config map data', () => {
    const config = mockConfigMaps[0]
    expect(config.data['APP_ENV']).toBe('production')
  })

  it('counts data keys', () => {
    const config = mockConfigMaps[0]
    expect(Object.keys(config.data).length).toBe(2)
  })
})

describe('Secrets', () => {
  it('lists all secrets', () => {
    expect(mockSecrets.length).toBe(2)
  })

  it('filters by type', () => {
    const opaque = mockSecrets.filter(s => s.type === 'Opaque')
    expect(opaque.length).toBe(2)
  })

  it('decodes base64 value', () => {
    const decoded = atob('YWRtaW4=')
    expect(decoded).toBe('admin')
  })

  it('encodes value to base64', () => {
    const encoded = btoa('secret')
    expect(encoded).toBe('c2VjcmV0')
  })

  it('masks secret values', () => {
    const mask = (value) => '*'.repeat(Math.min(value.length, 8))
    expect(mask('mypassword')).toBe('********')
  })
})

describe('Config Validation', () => {
  it('validates config map name', () => {
    const validName = 'app-config'
    const isValid = /^[a-z0-9]([-a-z0-9]*[a-z0-9])?$/.test(validName)
    expect(isValid).toBe(true)
  })

  it('rejects invalid config map name', () => {
    const invalidName = 'App_Config'
    const isValid = /^[a-z0-9]([-a-z0-9]*[a-z0-9])?$/.test(invalidName)
    expect(isValid).toBe(false)
  })

  it('validates key names', () => {
    const validKeys = ['APP_ENV', 'db_host', 'key123']
    const isValidKey = (key) => /^[a-zA-Z_][a-zA-Z0-9_]*$/.test(key)
    validKeys.forEach(key => {
      expect(isValidKey(key)).toBe(true)
    })
  })
})

describe('Config Updates', () => {
  it('adds new key to config map', () => {
    const config = { ...mockConfigMaps[0] }
    config.data['NEW_KEY'] = 'new_value'
    expect(config.data['NEW_KEY']).toBe('new_value')
  })

  it('updates existing key', () => {
    const config = { ...mockConfigMaps[0] }
    config.data['LOG_LEVEL'] = 'debug'
    expect(config.data['LOG_LEVEL']).toBe('debug')
  })

  it('removes key from config map', () => {
    const config = { ...mockConfigMaps[0] }
    delete config.data['LOG_LEVEL']
    expect(config.data['LOG_LEVEL']).toBeUndefined()
  })
})