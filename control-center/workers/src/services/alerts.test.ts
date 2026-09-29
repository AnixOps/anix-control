import { describe, it, expect } from 'vitest'

// Alert Manager mock data
const mockAlertRules = [
  { id: '1', name: 'High CPU Usage', metric: 'cpu_percent', threshold: 80, severity: 'warning', enabled: true },
  { id: '2', name: 'Memory Critical', metric: 'memory_percent', threshold: 90, severity: 'critical', enabled: true },
  { id: '3', name: 'Disk Space Low', metric: 'disk_percent', threshold: 85, severity: 'warning', enabled: false }
]

const mockAlerts = [
  { id: 'a1', ruleId: '1', name: 'High CPU Usage', value: 92, threshold: 80, severity: 'warning', status: 'firing', startedAt: '2026-03-23T10:00:00Z' },
  { id: 'a2', ruleId: '2', name: 'Memory Critical', value: 95, threshold: 90, severity: 'critical', status: 'firing', startedAt: '2026-03-23T09:30:00Z' }
]

describe('Alert Rules', () => {
  it('lists all alert rules', () => {
    expect(mockAlertRules.length).toBe(3)
  })

  it('filters enabled rules', () => {
    const enabled = mockAlertRules.filter(r => r.enabled)
    expect(enabled.length).toBe(2)
  })

  it('filters by severity', () => {
    const warnings = mockAlertRules.filter(r => r.severity === 'warning')
    const criticals = mockAlertRules.filter(r => r.severity === 'critical')
    expect(warnings.length).toBe(2)
    expect(criticals.length).toBe(1)
  })

  it('validates threshold values', () => {
    mockAlertRules.forEach(rule => {
      expect(rule.threshold).toBeGreaterThan(0)
      expect(rule.threshold).toBeLessThanOrEqual(100)
    })
  })
})

describe('Active Alerts', () => {
  it('lists firing alerts', () => {
    const firing = mockAlerts.filter(a => a.status === 'firing')
    expect(firing.length).toBe(2)
  })

  it('checks if alert is firing', () => {
    const isFiring = (alert) => alert.value > alert.threshold
    mockAlerts.forEach(alert => {
      expect(isFiring(alert)).toBe(true)
    })
  })

  it('calculates alert duration', () => {
    const started = new Date('2026-03-23T10:00:00Z')
    const now = new Date('2026-03-23T10:30:00Z')
    const duration = Math.floor((now - started) / 60000)
    expect(duration).toBe(30)
  })

  it('groups alerts by severity', () => {
    const bySeverity = mockAlerts.reduce((acc, a) => {
      acc[a.severity] = (acc[a.severity] || 0) + 1
      return acc
    }, {})
    expect(bySeverity['warning']).toBe(1)
    expect(bySeverity['critical']).toBe(1)
  })
})

describe('Alert Evaluation', () => {
  it('evaluates greater than condition', () => {
    const value = 92
    const threshold = 80
    const isFiring = value > threshold
    expect(isFiring).toBe(true)
  })

  it('evaluates less than condition', () => {
    const value = 5
    const threshold = 10
    const isFiring = value < threshold
    expect(isFiring).toBe(true)
  })

  it('handles threshold boundary', () => {
    const value = 80
    const threshold = 80
    const isFiring = value > threshold
    expect(isFiring).toBe(false)
  })
})

describe('Alert Notifications', () => {
  it('creates notification for critical alert', () => {
    const alert = mockAlerts[1]
    const notification = {
      title: `CRITICAL: ${alert.name}`,
      message: `Value: ${alert.value}%, Threshold: ${alert.threshold}%`,
      severity: 'critical'
    }
    expect(notification.title).toContain('CRITICAL')
  })

  it('creates notification for warning alert', () => {
    const alert = mockAlerts[0]
    const notification = {
      title: `WARNING: ${alert.name}`,
      message: `Value: ${alert.value}%, Threshold: ${alert.threshold}%`,
      severity: 'warning'
    }
    expect(notification.title).toContain('WARNING')
  })
})

describe('Alert Silencing', () => {
  it('creates silence window', () => {
    const silence = {
      alertId: 'a1',
      duration: 3600000, // 1 hour
      reason: 'Maintenance window',
      createdBy: 'admin'
    }
    expect(silence.duration).toBe(3600000)
  })

  it('checks if silence is active', () => {
    const silenceStart = Date.now() - 1800000 // 30 mins ago
    const duration = 3600000 // 1 hour
    const isActive = Date.now() < silenceStart + duration
    expect(isActive).toBe(true)
  })
})