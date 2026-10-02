// Pure helpers of the forward rules page (moved out of Forward.vue in UI U7
// without changes): address checks, relay-panel import/export, addresses.
import { describe, expect, it } from 'vitest'
import {
  arrayMove,
  buildExportData,
  findInvalidTargetLine,
  formatInAddress,
  formatRemoteAddress,
  getDirectFilterStatus,
  normalizeForward,
  parseImportEntries,
  qualityKey
} from '@/views/admin/forward/forwardModel'

describe('forwardModel', () => {
  it('finds the first target line that is not ipv4:port, [ipv6]:port or domain:port', () => {
    expect(findInvalidTargetLine('1.1.1.1:443\nexample.com:8443\n[2001:db8::1]:443')).toBe(-1)
    expect(findInvalidTargetLine('1.1.1.1:443\nexample.com\n[2001:db8::1]:443')).toBe(1)
    expect(findInvalidTargetLine('2001:db8::1:443')).toBe(0)
  })

  it('round-trips relay-panel JSON and keeps the legacy pipe lines', () => {
    const json = buildExportData([{ name: 'web', inPort: 1401, remoteAddr: '1.1.1.1:443,2.2.2.2:8443' }])
    expect(JSON.parse(json)).toEqual([{ dest: ['1.1.1.1:443', '2.2.2.2:8443'], listen_port: 1401, name: 'web' }])
    expect(parseImportEntries(json)).toEqual([
      { source: 'web', remoteAddr: '1.1.1.1:443,2.2.2.2:8443', name: 'web', inPortRaw: '1401' }
    ])
    expect(parseImportEntries('a.example.com:443|Legacy|1601\nbad-line')).toEqual([
      { source: 'a.example.com:443|Legacy|1601', remoteAddr: 'a.example.com:443', name: 'Legacy', inPortRaw: '1601', legacyParts: 3 },
      { source: 'bad-line', remoteAddr: 'bad-line', name: '', inPortRaw: '', legacyParts: 1 }
    ])
  })

  it('formats ingress and target addresses with a "+n" for extra ones', () => {
    expect(formatInAddress('203.0.113.10', 80)).toBe('203.0.113.10:80')
    expect(formatInAddress('2001:db8::1,203.0.113.10', 80)).toBe('[2001:db8::1]:80 (+1)')
    expect(formatInAddress('', 80)).toBe('')
    expect(formatRemoteAddress('a:1, b:2 ,c:3')).toBe('a:1 (+2)')
  })

  it('derives the running / paused / error filter status from the runtime state', () => {
    expect(getDirectFilterStatus(normalizeForward({ id: 1, tunnelId: 1, status: 1 }))).toBe('running')
    expect(getDirectFilterStatus(normalizeForward({ id: 1, tunnelId: 1, status: 1, runtimeStatus: 1, runtimeBackend: 'gost' }))).toBe('paused')
    expect(getDirectFilterStatus(normalizeForward({ id: 1, tunnelId: 1, status: 1, runtimeStatus: 3 }))).toBe('error')
    expect(getDirectFilterStatus(normalizeForward({ id: 1, tunnelId: 1, status: -1 }))).toBe('error')
  })

  it('keeps the flux-panel quality thresholds and moves items', () => {
    expect(qualityKey(20, 0)).toBe('excellent')
    expect(qualityKey(90, 0.5)).toBe('good')
    expect(qualityKey(250, 10)).toBe('veryPoor')
    expect(qualityKey(undefined, 0)).toBe('unknown')
    expect(arrayMove([1, 2, 3], 0, 2)).toEqual([2, 3, 1])
  })
})
