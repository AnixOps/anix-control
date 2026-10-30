// How a node group and its protocol template are edited and sent
// (views/admin/nodes/nodeGroupData.js): the bodies, the presets, the
// validation that runs before a request and the reading of the API's errors.
import { describe, expect, it } from 'vitest'
import {
  DEFAULT_PORT_RANGE, DEFAULT_REALITY_SERVER_NAME, PRESET_KEYS, PROTOCOL_PRESETS, adaptProtocol, blankProtocol,
  buildGroupInput, buildQuickGroupInput, entryFromProtocol, groupFormFrom, hasProblems, inputFromGroup,
  jsonObjectError, pickDefaultSubscriptionGroup, protocolFromEntry, protocolProblems, readKernelError
} from '@/views/admin/nodes/nodeGroupData'

const NOW = Date.parse('2026-10-12T00:00:00Z')

describe('protocol templates', () => {
  it('round-trips an entry of the API through the editor, keeping what the editor does not show', () => {
    const entry = {
      name: 'VLESS Reality', type: 'vless', port_range: [20000, 40000], transport: 'grpc', tls: 'reality',
      reality: { server_name: 'www.example.com', dest: 'www.example.com:8443', fingerprint: 'firefox' },
      settings: { flow: 'xtls-rprx-vision' }, transport_settings: { service_name: 'svc' }
    }
    expect(entryFromProtocol(protocolFromEntry(entry))).toEqual(entry)
  })

  it('sends a fixed port, and leaves an empty destination and settings out', () => {
    const protocol = { ...blankProtocol('vmess'), name: ' vm ', portMode: 'fixed', port: '8443', settingsText: '' }
    expect(entryFromProtocol(protocol)).toEqual({ name: 'vm', type: 'vmess', transport: 'tcp', tls: 'none', port: 8443 })
    const reality = { ...blankProtocol('vless'), portMode: 'fixed', port: 443, dest: ' ', settingsText: '' }
    expect(entryFromProtocol(reality).reality).toEqual({ server_name: DEFAULT_REALITY_SERVER_NAME })
  })

  it('starts from the defaults of each type', () => {
    expect(blankProtocol('vless')).toMatchObject({ tls: 'reality', transport: 'tcp', portMode: 'range', portFrom: DEFAULT_PORT_RANGE[0], portTo: DEFAULT_PORT_RANGE[1] })
    expect(JSON.parse(blankProtocol('vless').settingsText)).toEqual({ flow: 'xtls-rprx-vision' })
    expect(JSON.parse(blankProtocol('shadowsocks').settingsText)).toEqual({ method: '2022-blake3-aes-128-gcm' })
    expect(blankProtocol('wireguard')).toMatchObject({ transport: 'udp', tls: 'none' })
  })

  it('keeps types, transports and TLS in step', () => {
    const vmess = adaptProtocol({ ...blankProtocol('vless'), type: 'vmess' })
    expect(vmess.tls).toBe('none')
    const realityOnWs = adaptProtocol({ ...blankProtocol('vless'), transport: 'ws', tls: 'reality' })
    expect(realityOnWs.transport).toBe('tcp')
    const wireguardTransport = adaptProtocol({ ...blankProtocol('vless'), type: 'wireguard', transport: 'tcp' })
    expect(wireguardTransport.transport).toBe('udp')
  })

  it('finds what is wrong before a request', () => {
    expect(hasProblems(protocolProblems(blankProtocol('vless')))).toBe(false)
    expect(protocolProblems({ ...blankProtocol('vless'), portFrom: 30000, portTo: 20000 }).port).toBe('range')
    expect(protocolProblems({ ...blankProtocol('vless'), portFrom: 0 }).port).toBe('range')
    expect(protocolProblems({ ...blankProtocol('vless'), portMode: 'fixed', port: 70000 }).port).toBe('port')
    expect(protocolProblems({ ...blankProtocol('vless'), serverName: ' ' }).serverName).toBe('required')
    expect(protocolProblems({ ...blankProtocol('vmess'), serverName: '' }).serverName).toBe('')
    expect(protocolProblems({ ...blankProtocol('vless'), settingsText: '{' }).settings).toBe('json')
    expect(protocolProblems({ ...blankProtocol('vless'), transportSettingsText: '[1]' }).transportSettings).toBe('object')
    expect(jsonObjectError('')).toBe('')
    expect(jsonObjectError(' {"a":1} ')).toBe('')
    expect(jsonObjectError('null')).toBe('object')
  })

  it('has presets that a server accepts: one Reality protocol on a random port to start from', () => {
    expect(PRESET_KEYS).toEqual(['vless-reality', 'shadowsocks-2022', 'vless-reality-shadowsocks', 'none'])
    const [vless] = PROTOCOL_PRESETS['vless-reality']()
    expect(entryFromProtocol(vless)).toEqual({
      name: 'VLESS Reality', type: 'vless', transport: 'tcp', tls: 'reality', port_range: [20000, 40000],
      reality: { server_name: 'www.microsoft.com' }, settings: { flow: 'xtls-rprx-vision' }
    })
    expect(PROTOCOL_PRESETS['vless-reality-shadowsocks']()).toHaveLength(2)
  })
})

describe('group bodies', () => {
  const group = {
    id: 3, name: 'HK', max_nodes: 5, require_approval: true, rate: 1.5, tags: ['hk', 'iepl'],
    monthly_limit_bytes: 2 * 1024 ** 3, reset_day: 5, sort: 2, subscription_group_ids: [9, 10],
    protocol_template: [{ name: 'SS', type: 'shadowsocks', port: 8388, transport: 'tcp', tls: 'none', settings: { method: '2022-blake3-aes-128-gcm' } }],
    token_expires_at: '2999-01-01T00:00:00Z', token_expired: false
  }

  it('turns a group into the form and back into the body it was', () => {
    const form = groupFormFrom(group, { now: NOW })
    expect(form).toMatchObject({ name: 'HK', tagsText: 'hk, iepl', monthlyLimitGb: 2, resetDay: 5, maxNodes: 5, requireApproval: true, expiry: 'keep' })
    expect(buildGroupInput(form, { now: NOW })).toEqual({
      name: 'HK', max_nodes: 5, require_approval: true, rate: 1.5, tags: ['hk', 'iepl'],
      monthly_limit_bytes: 2 * 1024 ** 3, reset_day: 5, sort: 2, token_expires_at: '2999-01-01T00:00:00Z',
      protocol_template: group.protocol_template, subscription_group_ids: [9, 10]
    })
  })

  it('sends an unchanged list body with one field changed', () => {
    expect(inputFromGroup(group, { require_approval: false })).toMatchObject({ name: 'HK', require_approval: false, token_expires_at: '2999-01-01T00:00:00Z', subscription_group_ids: [9, 10] })
    expect(inputFromGroup({ name: 'x' }).protocol_template).toEqual([])
  })

  it('has no group kind: a group without protocols is the same kind of group', () => {
    const plain = { name: 'CN', protocol_template: [] }
    expect(groupFormFrom(plain, { now: NOW })).not.toHaveProperty('kind')
    expect(buildGroupInput(groupFormFrom(plain, { now: NOW }), { now: NOW })).not.toHaveProperty('kind')
  })

  it('maps the token validity choices', () => {
    const form = groupFormFrom(null)
    expect(form.expiry).toBe('never')
    expect(buildGroupInput(form, { now: NOW }).token_expires_at).toBeNull()
    expect(buildGroupInput({ ...form, expiry: '30d' }, { now: NOW }).token_expires_at).toBe('2026-11-11T00:00:00.000Z')
    expect(groupFormFrom({ ...group, token_expired: true, token_expires_at: '2020-01-01T00:00:00Z' }, { now: NOW }).expiry).toBe('expired')
    expect(groupFormFrom({ ...group, token_expires_at: null }, { now: NOW }).expiry).toBe('never')
  })

  it('splits tags on commas of both widths and drops repeats', () => {
    const body = buildGroupInput({ ...groupFormFrom(null), name: ' HK ', tagsText: 'hk， iepl,hk, ,fast' }, { now: NOW })
    expect(body.name).toBe('HK')
    expect(body.tags).toEqual(['hk', 'iepl', 'fast'])
  })

  it('sends a group without protocols an empty template', () => {
    const body = buildGroupInput({ ...groupFormFrom(null), name: 'edge', protocols: [], subscriptionGroupIds: [9] }, { now: NOW })
    expect(body.protocol_template).toEqual([])
    expect(body.subscription_group_ids).toEqual([9])
  })

  it('builds the minimum group of the join dialog', () => {
    expect(buildQuickGroupInput({ name: ' HK ', preset: 'shadowsocks-2022', subscriptionGroupIds: [9] })).toEqual({
      name: 'HK', subscription_group_ids: [9],
      protocol_template: [{ name: 'Shadowsocks 2022', type: 'shadowsocks', transport: 'tcp', tls: 'none', port_range: [20000, 40000], settings: { method: '2022-blake3-aes-128-gcm' } }]
    })
    expect(buildQuickGroupInput({ name: 'edge', preset: 'none', subscriptionGroupIds: [9] })).toEqual({
      name: 'edge', protocol_template: [], subscription_group_ids: []
    })
  })
})

describe('reading answers', () => {
  it('picks the default subscription group by the names the server uses', () => {
    expect(pickDefaultSubscriptionGroup([{ id: 1, name: 'vip' }, { id: 2, name: 'default' }, { id: 3, name: '默认' }]).id).toBe(3)
    expect(pickDefaultSubscriptionGroup([{ id: 1, name: 'vip' }, { id: 2, name: 'default' }]).id).toBe(2)
    expect(pickDefaultSubscriptionGroup([{ id: 1, name: 'vip' }])).toBeNull()
    expect(pickDefaultSubscriptionGroup(null)).toBeNull()
  })

  it('reads the kernel error and the legacy ones', () => {
    expect(readKernelError({ response: { status: 409, data: { error: { code: 'name_taken', message: 'a node group with this name exists' } } } }))
      .toEqual({ status: 409, code: 'name_taken', message: 'a node group with this name exists' })
    expect(readKernelError({ response: { status: 500, data: { message: '服务器错误' } } })).toEqual({ status: 500, code: '', message: '服务器错误' })
    expect(readKernelError(new Error('boom'))).toEqual({ status: 0, code: '', message: 'boom' })
  })
})
