import { describe, expect, it } from 'vitest'
import { parseNodeHosts, standbyForm, standbyPrepareRequest, tierTone } from '@/components/forward/standbyModel'
import zhStandby from '@/locales/modules/zh-CN/standby'
import enStandby from '@/locales/modules/en/standby'

function keys(messages, prefix = '') {
  return Object.entries(messages).flatMap(([key, value]) =>
    value && typeof value === 'object' ? keys(value, `${prefix}${key}.`) : [`${prefix}${key}`])
}

describe('cross-border standby model', () => {
  it('reads node=domain lines', () => {
    expect(parseNodeHosts('50=Edge.Example.net\nproxy-57: de.example.net\n\nbad line\n 12 x.example.org ')).toEqual({
      50: 'edge.example.net', 57: 'de.example.net', 12: 'x.example.org'
    })
  })

  it('fills the form from the overview and builds the request', () => {
    const overview = {
      links: [{
        transport_id: 't-a2',
        channels: [
          { tier: 'reality', exit_node_id: 50, reality_server_name: 'www.example.org', reality_dest: 'www.example.org:443' },
          { tier: 'xhttp', exit_node_id: 50, xhttp_domain: 'edge.example.net' }
        ]
      }]
    }
    const form = standbyForm(overview)
    expect(form).toEqual({ serverName: 'www.example.org', dest: 'www.example.org:443', domains: '50=edge.example.net' })
    expect(standbyPrepareRequest(form)).toEqual({
      reality_server_name: 'www.example.org', reality_dest: 'www.example.org:443', xhttp_domains: { 50: 'edge.example.net' }
    })
    expect(standbyForm(null).serverName).toBe('')
    expect(tierTone('off')).toBe('success')
  })

  it('has the same message keys in both languages', () => {
    expect(keys(enStandby).sort()).toEqual(keys(zhStandby).sort())
  })
})
