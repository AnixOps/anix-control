import { describe, expect, it } from 'vitest'
import { FIXED, lineDraft, lineForm, parseTargets, protocolsOn, validate } from '../lineForm'

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
