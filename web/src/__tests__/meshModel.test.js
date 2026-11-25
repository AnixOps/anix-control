import { describe, expect, it } from 'vitest'
import { boundaryCount, deviceStatus, enrollmentState, isFeatureDisabled, memberRows, STATUS_STALE_MS, statusSummary } from '@/components/forward/meshModel'
import zhMesh from '@/locales/modules/zh-CN/mesh'
import enMesh from '@/locales/modules/en/mesh'

function keys(messages, prefix = '') {
  return Object.entries(messages).flatMap(([key, value]) =>
    value && typeof value === 'object' ? keys(value, `${prefix}${key}.`) : [`${prefix}${key}`])
}

const cn = { id: 1, name: '中国大陆', members: [
  { id: 11, node_ref: 'proxy-1', address: '10.150.0.1', home: true, boundary: true },
  { id: 12, node_ref: 'proxy-2', address: '10.150.0.2', home: false },
  { id: 13, node_ref: 'proxy-3', address: '10.150.0.3', home: true, boundary: false }
] }
const eu = { id: 2, name: '欧洲', members: [
  { id: 21, node_ref: 'proxy-1', address: '10.152.0.1', home: false },
  { id: 22, node_ref: 'proxy-2', address: '10.152.0.2', home: true, boundary: true }
] }

describe('mesh model', () => {
  it('answers one row per node with its other addresses', () => {
    const rows = memberRows([cn, eu])
    expect(rows.map(row => [row.id, row.region.id, row.others.map(other => other.address)])).toEqual([
      [11, 1, ['10.152.0.1']], [22, 2, ['10.150.0.2']], [13, 1, []]
    ])
    expect(boundaryCount(cn)).toBe(2)
    expect(boundaryCount(eu)).toBe(2)
  })

  it('gathers the key warnings of all of a node\'s memberships', () => {
    const rows = memberRows([
      { id: 1, members: [{ id: 1, node_ref: 'proxy-1', home: true, boundary: true, warnings: [] }] },
      { id: 2, members: [{ id: 2, node_ref: 'proxy-1', home: false, warnings: ['static_key_taken'] }] }
    ])
    expect(rows[0].keyWarnings).toEqual(['static_key_taken'])
    expect(enMesh.mesh.page.alerts.static_key_taken).toBeTruthy()
    expect(zhMesh.mesh.page.alerts.static_key_taken).toBeTruthy()
  })

  it('summarizes the last status', () => {
    const now = Date.parse('2026-11-25T08:00:00Z')
    const fresh = new Date(now - 30_000).toISOString()
    expect(statusSummary({}, now)).toEqual({ state: 'none', alerts: [] })
    expect(statusSummary({ status: { networks: [{ running: true }] }, status_received_at: fresh }, now).state).toBe('running')
    expect(statusSummary({
      status: { networks: [{ running: true }, { running: false, alerts: [{ code: 'address_conflict' }, { code: 'address_conflict' }] }] },
      status_received_at: fresh
    }, now)).toEqual({ state: 'stopped', alerts: ['address_conflict'] })
    // A network's failure shows with its alerts, such as a refused peer.
    expect(statusSummary({
      status: { networks: [{ running: true, failure: { code: 'peer_not_native', detail: 'n-1' }, alerts: [{ code: 'peer_not_native' }] }] },
      status_received_at: fresh
    }, now)).toEqual({ state: 'running', alerts: ['peer_not_native'] })
    // A network held until the admission list names its key is pending,
    // not stopped.
    expect(statusSummary({
      status: { networks: [{ running: true }, { running: false, failure: { code: 'native_keys_pending', detail: 'empty' } }] },
      status_received_at: fresh
    }, now)).toEqual({ state: 'pending', alerts: ['native_keys_pending'] })
    expect(statusSummary({
      status: { networks: [{ running: false, failure: { code: 'native_keys_pending' } }, { running: false }] },
      status_received_at: fresh
    }, now).state).toBe('stopped')
    expect(enMesh.mesh.page.status.pending).toBeTruthy()
    expect(enMesh.mesh.page.alerts.native_keys_pending).toBeTruthy()
    for (const code of ['native_keys_unsupported', 'invalid_kind']) {
      expect(enMesh.mesh.page.alerts[code]).toBeTruthy()
      expect(zhMesh.mesh.page.alerts[code]).toBeTruthy()
    }
    expect(enMesh.mesh.page.alerts.peer_not_native).toBeTruthy()
    expect(zhMesh.mesh.page.alerts.peer_not_native).toBeTruthy()
    expect(statusSummary({ status: { networks: [] }, status_received_at: new Date(now - STATUS_STALE_MS - 1).toISOString() }, now).state).toBe('lost')
  })

  it('marks an enrolled device that has gone silent as lost', () => {
    const now = Date.parse('2026-11-25T08:00:00Z')
    expect(deviceStatus({ enrolled: true, last_seen_at: '2026-11-25T07:59:00Z' }, now).state).toBe('none')
    expect(deviceStatus({ enrolled: true, last_seen_at: '2026-11-25T07:50:00Z' }, now).state).toBe('lost')
    expect(deviceStatus({ enrolled: true }, now).state).toBe('lost')
    expect(deviceStatus({ status: { networks: [{ running: true }] }, status_received_at: '2026-11-25T07:59:30Z' }, now).state).toBe('running')
  })

  it('recognizes the disabled feature', () => {
    expect(isFeatureDisabled({ response: { status: 404, data: { error: { code: 'feature_disabled' } } } })).toBe(true)
    expect(isFeatureDisabled({ response: { status: 404, data: { error: { code: 'not_found' } } } })).toBe(false)
  })

  it('has the same message keys in both languages', () => {
    expect(keys(enMesh).sort()).toEqual(keys(zhMesh).sort())
  })

  it('answers the state of an enrollment credential', () => {
    const now = Date.parse('2026-11-25T08:00:00Z')
    expect(enrollmentState({ used_at: '2026-11-25T07:00:00Z', expires_at: '2026-11-25T07:30:00Z' }, now)).toBe('used')
    expect(enrollmentState({ expires_at: '2026-11-25T07:30:00Z' }, now)).toBe('expired')
    expect(enrollmentState({ expires_at: '2026-11-26T07:30:00Z' }, now)).toBe('valid')
  })
})
