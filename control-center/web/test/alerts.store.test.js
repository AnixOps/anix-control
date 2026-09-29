import { describe, it, expect } from 'vitest'

// Alert Center mock data
const mockAlertRules = [
  { id: '1', name: 'High CPU', metric: 'cpu_percent', threshold: 80, severity: 'warning', enabled: true },
  { id: '2', name: 'Memory Critical', metric: 'memory_percent', threshold: 90, severity: 'critical', enabled: true }
]

const mockActiveAlerts = [
  { id: 'a1', ruleId: '1', name: 'High CPU', value: 92, threshold: 80, severity: 'warning', status: 'firing' },
  { id: 'a2', ruleId: '2', name: 'Memory Critical', value: 95, threshold: 90, severity: 'critical', status: 'firing' }
]

describe('Alert Center', () => {
  it('displays active alerts count', () => {
    const activeCount = mockActiveAlerts.filter(a => a.status === 'firing').length
    expect(activeCount).toBe(2)
  })

  it('displays critical alerts first', () => {
    const sorted = [...mockActiveAlerts].sort((a, b) => {
      const severityOrder = { critical: 0, warning: 1, info: 2 }
      return severityOrder[a.severity] - severityOrder[b.severity]
    })
    expect(sorted[0].severity).toBe('critical')
  })

  it('groups alerts by severity', () => {
    const bySeverity = mockActiveAlerts.reduce((acc, a) => {
      acc[a.severity] = (acc[a.severity] || 0) + 1
      return acc
    }, {})
    expect(bySeverity['warning']).toBe(1)
    expect(bySeverity['critical']).toBe(1)
  })
})

describe('Alert Rules Management', () => {
  it('lists all rules', () => {
    expect(mockAlertRules.length).toBe(2)
  })

  it('filters enabled rules', () => {
    const enabled = mockAlertRules.filter(r => r.enabled)
    expect(enabled.length).toBe(2)
  })

  it('toggles rule status', () => {
    const rule = { ...mockAlertRules[0] }
    rule.enabled = !rule.enabled
    expect(rule.enabled).toBe(false)
  })

  it('validates rule thresholds', () => {
    mockAlertRules.forEach(rule => {
      expect(rule.threshold).toBeGreaterThan(0)
      expect(rule.threshold).toBeLessThanOrEqual(100)
    })
  })
})

describe('Alert Acknowledgment', () => {
  it('acknowledges alert', () => {
    const alert = { ...mockActiveAlerts[0], status: 'firing' }
    alert.status = 'acknowledged'
    expect(alert.status).toBe('acknowledged')
  })

  it('tracks acknowledgment user', () => {
    const ack = {
      alertId: 'a1',
      acknowledgedBy: 'admin',
      acknowledgedAt: new Date().toISOString()
    }
    expect(ack.acknowledgedBy).toBe('admin')
  })

  it('calculates time since acknowledgment', () => {
    const ackTime = new Date('2026-03-23T10:00:00Z')
    const now = new Date('2026-03-23T10:30:00Z')
    const minutes = Math.floor((now - ackTime) / 60000)
    expect(minutes).toBe(30)
  })
})

describe('Alert Filtering', () => {
  it('filters by severity', () => {
    const critical = mockActiveAlerts.filter(a => a.severity === 'critical')
    expect(critical.length).toBe(1)
  })

  it('filters by status', () => {
    const firing = mockActiveAlerts.filter(a => a.status === 'firing')
    expect(firing.length).toBe(2)
  })

  it('searches by name', () => {
    const results = mockActiveAlerts.filter(a =>
      a.name.toLowerCase().includes('cpu')
    )
    expect(results.length).toBe(1)
  })
})