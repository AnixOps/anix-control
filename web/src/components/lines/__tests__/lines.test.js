import { describe, expect, it } from 'vitest'
import { buildLines, matches } from '../linesModel'

const NOW = 1_800_000_000_000
const node = (id, name, online = true) => ({ id, name, host: `203.0.113.${id}`, status: 1, last_check_at: NOW / 1000 - (online ? 10 : 9000) })
const device = (id, name, extra = {}) => ({ ref: `proxy-${id}`, name, host: `203.0.113.${id}`, kernel: false, ...extra })

describe('buildLines', () => {
  const lines = buildLines({
    now: NOW,
    nodes: [node(1, 'sha-01'), node(2, 'tyo-01'), node(3, 'osaka-01', false)],
    routes: [{ route: { id: 'a' }, status: 'degraded' }],
    traffic: new Map([['a', { up: 1, down: 2 }]]),
    network: {
      devices: [device(1, 'sha-01', { kernel: true, region: 'beijing' }), device(2, 'tyo-01'), device(3, 'osaka-01')],
      lines: [
        { id: 'a', name: 'sha-tyo', enabled: true, show: true, port: 443, rate: 1.5, chain: [{ devices: ['proxy-1'] }, { devices: ['proxy-2'] }], exit: 'proxy-2', protocol: { type: 'vless', device: 'proxy-2' }, rules: [{ name: 'ai', domains: ['openai.com'], exit: 'proxy-3', enabled: true }] },
        { id: 'protocol-7', name: 'osaka', enabled: true, show: true, port: 8443, rate: 1, chain: [{ devices: ['proxy-3'] }], exit: 'proxy-3', protocol: { type: 'trojan', device: 'proxy-3' } },
        { id: 'b', name: 'game', enabled: false, show: false, port: 30443, rate: 1, chain: [{ devices: ['proxy-1'] }, { devices: ['proxy-2'] }], exit: 'proxy-2', targets: [{ host: 'game.example.jp', port: 7777 }] }
      ]
    }
  })
  const line = id => lines.find(item => item.id === id)

  it('reads a line with its hops, what it serves and where', () => {
    expect(line('a')).toMatchObject({ kind: 'relay', protocol: 'vless', port: 443, rate: 1.5, traffic: 3, served: 'proxy-2', route: 'a' })
    expect(line('a').hops.map(hop => [hop.role, hop.devices.map(item => item.name)])).toEqual([['entry', ['sha-01']], ['exit', ['tyo-01']]])
    expect(line('a').hops[0].devices[0]).toMatchObject({ kernel: true, online: true, id: 1 })
    expect(line('a').hops[0].devices[0].place.key).toBe('beijing')
    expect(line('a').rules[0].exit.name).toBe('osaka-01')
  })

  it('tells relayed, direct and fixed-target lines apart', () => {
    expect(lines.map(item => item.kind)).toEqual(['relay', 'direct', 'fixed'])
    expect(line('b').targets).toEqual(['game.example.jp:7777'])
  })

  it('gives each line a tone: its route, a hop that is down, or paused', () => {
    expect(lines.map(item => item.tone)).toEqual(['warn', 'bad', 'off'])
    expect(lines.filter(item => matches(item, 'attention')).map(item => item.id)).toEqual(['a', 'protocol-7'])
    expect(lines.filter(item => matches(item, 'fixed')).map(item => item.id)).toEqual(['b'])
    expect(lines.filter(item => matches(item, ''))).toHaveLength(3)
  })
})
