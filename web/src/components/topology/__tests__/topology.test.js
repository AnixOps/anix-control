import { describe, expect, it } from 'vitest'
import { locate, project } from '../geo'
import { buildTopology, focusOf, layoutGraph } from '../topologyModel'

const NOW = 1_800_000_000_000
const node = (id, name, extra = {}) => ({ id, name, host: `203.0.113.${id}`, status: 1, last_check_at: NOW / 1000 - 10, online_users: id, ...extra })
const route = (id, name, hops, status = 'healthy') => ({ route: { id, name, hops: hops.map(refs => ({ node_refs: refs })) }, status })

describe('locate', () => {
  it('reads the place from the name and the tags, the most specific first', () => {
    expect(locate({ name: 'ZouterHK' }).key).toBe('hk')
    expect(locate({ name: 'us-lax-01' }).key).toBe('losangeles')
    expect(locate({ name: 'osaka-01', tags: 'JP' }).key).toBe('osaka')
    expect(locate({ name: 'LIQUN-SH-9929+CN2' }).key).toBe('shanghai')
    expect(locate({ name: '东京 01' }).key).toBe('jp')
    expect(locate({ name: 'edge-864' })).toBeNull()
  })

  it('puts Shanghai left of Los Angeles on the map', () => {
    expect(project(31.2, 121.5).x).toBeLessThan(project(34.1, -118.2).x)
  })
})

describe('buildTopology', () => {
  const topology = buildTopology({
    now: NOW,
    nodes: [node(1, 'sha-01'), node(2, 'hk-01'), node(3, 'tyo-01'), node(4, 'osaka-01', { last_check_at: NOW / 1000 - 9000 }), node(5, 'sg-01')],
    routes: [route('a', 'sha-hk-tyo', [['proxy-1'], ['proxy-2'], ['proxy-3']]), route('b', 'sha-osaka', [['proxy-1'], ['proxy-4']]), route('c', 'gone', [['proxy-99']])],
    traffic: new Map([['a', { up: 1, down: 2 }]]),
    egress: [{ id: 7, name: 'ai', enabled: true, source_node: { id: 2 }, exit_node: { id: 3 } }]
  })
  const device = name => topology.devices.find(item => item.name === name)

  it('gives each device its roles and lines', () => {
    expect(device('sha-01').roles).toEqual(['entry'])
    expect(device('hk-01').roles).toEqual(['relay', 'exit'])
    expect(device('tyo-01').roles).toEqual(['exit'])
    expect(device('sg-01').roles).toEqual(['direct'])
    expect(device('sha-01').lines).toEqual(['a', 'b'])
  })

  it('breaks a line whose hop is offline and skips routes of unknown devices', () => {
    expect(topology.lines.map(line => [line.id, line.tone, line.traffic])).toEqual([['a', 'ok', 3], ['b', 'bad', 0], ['egress-7', 'ok', 0]])
    expect(topology.links.find(link => link.key === 'proxy-1>proxy-4').tone).toBe('bad')
  })

  it('lights the lines through a selected device', () => {
    const focus = focusOf(topology, { type: 'device', id: 'proxy-4' })
    expect([...focus.devices].sort()).toEqual(['proxy-1', 'proxy-4'])
    expect([...focus.links]).toEqual(['proxy-1>proxy-4'])
    expect(focusOf(topology, null)).toBeNull()
  })

  it('lays the devices of the lines out from entry to exit', () => {
    const wired = topology.devices.filter(item => item.lines.length)
    const layout = layoutGraph({ devices: wired, links: topology.links })
    expect(layout.columns.map(column => [column.role, column.count])).toEqual([['entry', 1], ['relay', 1], ['exit', 2]])
    expect(layout.at.get('proxy-1').x).toBeLessThan(layout.at.get('proxy-2').x)
    expect(layout.at.get('proxy-3').x).toBe(layout.at.get('proxy-4').x)
  })
})
