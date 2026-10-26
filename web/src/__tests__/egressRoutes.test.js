import { describe, expect, it } from 'vitest'
import { AI_DOMAINS, egressForm, egressRequest, egressRouteInput, parseDomains } from '@/components/forward/egressModel'
import zhEgress from '@/locales/modules/zh-CN/egressRoutes'
import enEgress from '@/locales/modules/en/egressRoutes'

function keys(messages, prefix = '') {
  return Object.entries(messages).flatMap(([key, value]) =>
    value && typeof value === 'object' ? keys(value, `${prefix}${key}.`) : [`${prefix}${key}`])
}

describe('egress route model', () => {
  it('splits the domains on lines, commas and spaces and drops repeats', () => {
    expect(parseDomains(' openai.com\nclaude.ai, openai.com  full:api.x.ai\n\n')).toEqual(['openai.com', 'claude.ai', 'full:api.x.ai'])
    expect(new Set(AI_DOMAINS).size).toBe(AI_DOMAINS.length)
  })

  it('round-trips a listed route through the form', () => {
    const route = {
      id: 3, name: 'AI', source_node: { id: 29 }, exit_node: { id: 50 }, exit_host: '', relay_port: 24680,
      domains: ['openai.com', 'claude.ai'], enabled: true
    }
    const form = egressForm(route)
    expect(form.domains).toBe('openai.com\nclaude.ai')
    expect(egressRequest(form)).toEqual({
      name: 'AI', source_node_id: 29, exit_node_id: 50, exit_host: '', relay_port: 24680,
      domains: ['openai.com', 'claude.ai'], enabled: true, rotate_key: false
    })
    expect(egressRouteInput(route, { enabled: false })).toMatchObject({ source_node_id: 29, exit_node_id: 50, enabled: false })
    expect(egressForm(null).enabled).toBe(true)
  })

  it('has the same message keys in both languages', () => {
    expect(keys(enEgress).sort()).toEqual(keys(zhEgress).sort())
  })
})
