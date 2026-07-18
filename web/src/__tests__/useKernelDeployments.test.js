import { describe, expect, it } from 'vitest'
import {
  assignmentPayload,
  extractNodes,
  topologyInputFromJSON,
  topologyJSONFromDetail,
} from '@/composables/useKernelDeployments'

describe('useKernelDeployments data helpers', () => {
  it('normalizes the assignment payload without changing its kernel field names', () => {
    expect(assignmentPayload({
      service_scope: 'forward',
      plugin_id: 'gost-mesh',
      role: 'relay',
      desired_version: '1.0.0',
      desired_config_revision: -2,
      enabled: 1,
      rollout_group: 'canary-a',
    })).toEqual({
      service_scope: 'forward',
      plugin_id: 'gost-mesh',
      role: 'relay',
      desired_version: '1.0.0',
      desired_config_revision: 0,
      enabled: true,
      rollout_group: 'canary-a',
    })
  })

  it('round-trips canonical topology JSON with optional node IDs and graph configs', () => {
    const json = topologyJSONFromDetail({
      vertices: [
        {
          key: 'entry',
          kind: 'agent',
          node_id: 11,
          plugin_id: 'gost-mesh',
          role: 'relay',
          config: '{"listen":{"port":443}}',
        },
        {
          key: 'exit',
          kind: 'external',
          config: { address: '198.51.100.10' },
        },
      ],
      edges: [
        {
          source_key: 'entry',
          target_key: 'exit',
          protocol: 'quic',
          secret_id: 'edge-secret',
          config: { keepalive_seconds: 20 },
        },
      ],
    })

    expect(json).toBe(`{
  "vertices": [
    {
      "key": "entry",
      "kind": "agent",
      "node_id": 11,
      "plugin_id": "gost-mesh",
      "role": "relay",
      "config": {
        "listen": {
          "port": 443
        }
      }
    },
    {
      "key": "exit",
      "kind": "external",
      "config": {
        "address": "198.51.100.10"
      }
    }
  ],
  "edges": [
    {
      "source_key": "entry",
      "target_key": "exit",
      "protocol": "quic",
      "secret_id": "edge-secret",
      "config": {
        "keepalive_seconds": 20
      }
    }
  ]
}`)

    expect(topologyInputFromJSON(json, 'Canary rollout', 'Invalid graph')).toEqual({
      message: 'Canary rollout',
      vertices: [
        {
          key: 'entry',
          kind: 'agent',
          node_id: 11,
          plugin_id: 'gost-mesh',
          role: 'relay',
          config: '{"listen":{"port":443}}',
        },
        {
          key: 'exit',
          kind: 'external',
          node_id: null,
          plugin_id: '',
          role: '',
          config: '{"address":"198.51.100.10"}',
        },
      ],
      edges: [
        {
          source_key: 'entry',
          target_key: 'exit',
          protocol: 'quic',
          secret_id: 'edge-secret',
          config: '{"keepalive_seconds":20}',
        },
      ],
    })
  })

  it('rejects invalid topology JSON with the caller-provided message', () => {
    expect(() => topologyInputFromJSON('{invalid', 'ignored', 'Invalid graph')).toThrow('Invalid graph')
  })

  it('extracts valid node rows from the panel envelope without widening the request surface', () => {
    expect(extractNodes({
      code: 0,
      data: { list: [{ id: '11', name: 'Shanghai entry' }, { id: 0, name: 'invalid' }] },
    }, 'Unable to load nodes')).toEqual([{ id: 11, name: 'Shanghai entry' }])

    expect(() => extractNodes({ code: 400, msg: 'node source unavailable' }, 'Unable to load nodes')).toThrow('node source unavailable')
  })
})
