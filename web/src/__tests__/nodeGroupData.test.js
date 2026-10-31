// How a node group is edited and sent (views/admin/nodes/nodeGroupData.js):
// the bodies and the answers. A group has no protocols: its nodes join clean.
import { describe, expect, it } from 'vitest'
import {
  buildGroupInput, buildQuickGroupInput, groupFormFrom, inputFromGroup, readKernelError
} from '@/views/admin/nodes/nodeGroupData'

const NOW = Date.parse('2026-10-12T00:00:00Z')

describe('group bodies', () => {
  const group = {
    id: 3, name: 'HK', max_nodes: 5, require_approval: true, rate: 1.5, tags: ['hk', 'iepl'],
    monthly_limit_bytes: 2 * 1024 ** 3, reset_day: 5, sort: 2,
    token_expires_at: '2999-01-01T00:00:00Z', token_expired: false
  }

  it('turns a group into the form and back into the body it was', () => {
    const form = groupFormFrom(group, { now: NOW })
    expect(form).toMatchObject({ name: 'HK', tagsText: 'hk, iepl', monthlyLimitGb: 2, resetDay: 5, maxNodes: 5, requireApproval: true, expiry: 'keep' })
    expect(buildGroupInput(form, { now: NOW })).toEqual({
      name: 'HK', max_nodes: 5, require_approval: true, rate: 1.5, tags: ['hk', 'iepl'],
      monthly_limit_bytes: 2 * 1024 ** 3, reset_day: 5, sort: 2, token_expires_at: '2999-01-01T00:00:00Z'
    })
  })

  it('sends an unchanged list body with one field changed', () => {
    expect(inputFromGroup(group, { require_approval: false })).toMatchObject({ name: 'HK', require_approval: false, token_expires_at: '2999-01-01T00:00:00Z' })
    expect(inputFromGroup({ name: 'x' })).not.toHaveProperty('protocol_template')
  })

  it('has no group kind and no protocols', () => {
    const body = buildGroupInput(groupFormFrom({ name: 'CN' }, { now: NOW }), { now: NOW })
    expect(body).not.toHaveProperty('kind')
    expect(body).not.toHaveProperty('protocol_template')
    expect(body).not.toHaveProperty('subscription_group_ids')
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

  it('builds the minimum group of the join dialog', () => {
    expect(buildQuickGroupInput({ name: ' HK ' })).toEqual({ name: 'HK' })
  })
})

describe('reading answers', () => {
  it('reads the kernel error and the legacy ones', () => {
    expect(readKernelError({ response: { status: 409, data: { error: { code: 'name_taken', message: 'a node group with this name exists' } } } }))
      .toEqual({ status: 409, code: 'name_taken', message: 'a node group with this name exists' })
    expect(readKernelError({ response: { status: 500, data: { message: '服务器错误' } } })).toEqual({ status: 500, code: '', message: '服务器错误' })
    expect(readKernelError(new Error('boom'))).toEqual({ status: 0, code: '', message: 'boom' })
  })
})
