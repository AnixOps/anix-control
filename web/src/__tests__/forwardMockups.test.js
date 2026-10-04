import { describe, expect, it } from 'vitest'
import { ROUTES } from '@/mockups/forward/mockData'
import { CODE_TEXT, previewDraft, validateDraft } from '@/mockups/forward/mockPlanner'

// The F5b mockups (dev only) map violations to fields by their stable
// sdk/forward/validate codes; keep the stand-in planner honest.
describe('forward mockup planner', () => {
  it('plans every mocked route without violations', () => {
    for (const { route } of ROUTES) {
      expect(validateDraft(route, { onCreate: false }), route.name).toEqual([])
    }
  })

  it('refuses an nftables entry into a TLS exit with link_unsupported', () => {
    const route = {
      name: 'x',
      listen: { address: '0.0.0.0', port: 443, protocol: 'L4_PROTOCOL_TCP' },
      hops: [
        { role: 'HOP_ROLE_ENTRY', engine: 'ENGINE_NFTABLES', node_refs: ['forward-71'] },
        { role: 'HOP_ROLE_EXIT', engine: 'ENGINE_GOST', node_refs: ['forward-61'], ingress: { security: 'LINK_SECURITY_TLS' } }
      ],
      targets: [{ host: '10.0.3.8', port: 7777 }],
      policy: { target_policy: 'TARGET_POLICY_PUBLIC_ONLY' },
      limits: {}
    }
    const preview = previewDraft(route)
    expect(preview.states).toEqual([])
    expect(preview.violations.map(item => [item.field, item.code])).toEqual([
      ['listen.port', 'port_reserved'],
      ['hops[1].ingress.security', 'link_unsupported'],
      ['targets[0].host', 'target_not_allowed']
    ])
    for (const item of preview.violations) expect(CODE_TEXT[item.code]).toBeTruthy()
  })

  it('previews one state per hop and node', () => {
    const preview = previewDraft(ROUTES[0].route, { onCreate: false })
    expect(preview.states.map(state => `${state.hop_index}:${state.node_ref}`)).toEqual(['0:forward-41', '1:forward-51', '2:forward-61', '2:forward-62'])
  })
})
