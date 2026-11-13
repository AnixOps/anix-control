import { describe, expect, it } from 'vitest'
import { FIXED, hasPolicy, lineDraft, lineForm, parseTargets, protocolsOn, validate } from '../lineForm'

const stored = {
  id: 'r1', name: 'sha-hk-tyo', enabled: true, show: true, port: 443, l4: ['tcp'], revision: 4,
  chain: [{ devices: ['proxy-1', 'proxy-2'] }, { devices: ['proxy-3'], transport: 'jacket' }, { devices: ['proxy-4'] }],
  protocol: { id: 9, type: 'vless', device: 'proxy-4', port: 8443 }
}

describe('lineForm', () => {
  it('reads a stored line and writes it back as the same draft', () => {
    const form = lineForm(stored)
    expect(form).toMatchObject({ entries: ['proxy-1', 'proxy-2'], via: ['proxy-3'], exit: 'proxy-4', protocol: 9, tcp: true, udp: false })
    expect(lineDraft(form, 4)).toEqual({
      name: 'sha-hk-tyo', enabled: true, show: true, port: 443, l4: ['tcp'], protocol_id: 9, revision: 4,
      chain: [{ devices: ['proxy-1', 'proxy-2'] }, { devices: ['proxy-3'] }, { devices: ['proxy-4'] }]
    })
  })

  it('writes fixed targets, never offered in subscriptions, and a line on one device', () => {
    const form = { ...lineForm(null), name: 'game', entries: ['proxy-1'], exit: 'proxy-1', protocol: FIXED, targets: 'game.example.jp:7777\n[2001:db8::1]:80', port: 30443 }
    expect(validate(form)).toEqual({})
    expect(lineDraft(form)).toMatchObject({
      show: false, chain: [{ devices: ['proxy-1'] }],
      targets: [{ host: 'game.example.jp', port: 7777, weight: 1 }, { host: '2001:db8::1', port: 80, weight: 1 }]
    })
    expect(lineDraft(form)).not.toHaveProperty('protocol_id')
  })

  it('reads back the policy and limits it writes, in the units the sheet shows', () => {
    const line = {
      ...stored, hop_balance: 'least_conn', quota_bytes: 2 * 1024 ** 3, expires_at_unix_ms: new Date('2027-01-02T03:04').getTime(),
      policy: { proxy_protocol: 'v2', bandwidth_bps: 50_000_000, max_conns: 100, health: { interval_ms: 10_000, timeout_ms: 3000 }, circuit_breaker: { failure_threshold: 5, open_ms: 60_000 } }
    }
    const form = lineForm(line)
    expect(form).toMatchObject({ proxyProtocol: 'v2', balance: 'default', bandwidth: 50, quota: 2, maxConns: 100, interval: 10, timeout: 3, threshold: 5, openFor: 60, expires: '2027-01-02T03:04' })
    expect(hasPolicy(form)).toBe(true)
    const draft = lineDraft(form, 4)
    expect(draft).toMatchObject({ policy: line.policy, hop_balance: 'least_conn', quota_bytes: line.quota_bytes, expires_at_unix_ms: line.expires_at_unix_ms })
    expect(draft.policy).toEqual(line.policy)
  })

  it('sends no policy for a line without one, and balance only over fixed targets', () => {
    const form = lineForm(stored)
    expect(hasPolicy(form)).toBe(false)
    expect(Object.keys(lineDraft(form))).not.toContain('policy')
    expect(lineDraft({ ...form, balance: 'failover' })).not.toHaveProperty('policy')
    const fixed = { ...form, protocol: FIXED, targets: 'a:1\nb:2', balance: 'failover', healthOff: true }
    expect(lineDraft(fixed).policy).toEqual({ balance: 'failover', health: { disabled: true } })
    expect(validate({ ...form, interval: 2, timeout: 2 })).toEqual({ timeout: 'timeout' })
  })

  it('names what is missing or wrong', () => {
    expect(validate(lineForm(null))).toEqual({ name: 'required', entries: 'required', exit: 'required', protocol: 'required', port: 'required' })
    const form = { ...lineForm(stored), via: ['proxy-4'], tcp: false, protocol: FIXED, targets: 'nohost' }
    expect(validate(form)).toEqual({ exit: 'reused', l4: 'required', targets: 'target' })
    expect(parseTargets(' a:1 \n\n')).toEqual([{ host: 'a', port: 1 }])
  })

  it('lists the protocols served on a device', () => {
    const network = { lines: [stored, { id: 'protocol-9', protocol: stored.protocol }, { id: 'protocol-2', protocol: { id: 2, type: 'trojan', device: 'proxy-4', port: 443 } }, { id: 'x', protocol: { id: 5, device: 'proxy-1' } }] }
    expect(protocolsOn(network, 'proxy-4').map(item => item.id)).toEqual([2, 9])
  })
})
