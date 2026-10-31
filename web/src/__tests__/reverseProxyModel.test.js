import { describe, expect, it } from 'vitest'
import { parseUpstreams, reverseProxyForm, reverseProxyInput, reverseProxyRequest, validateReverseProxy } from '@/components/forward/reverseProxyModel'

const messages = {
  required: 'required', sameNode: 'same', port: 'port', httpPort: 'http', path: 'path', zone: 'zone', outsideZone: host => `outside ${host}`, upstream: line => `bad ${line}`
}

describe('reverseProxyModel', () => {
  it('reads upstream lines with ports, weights and IPv6 brackets', () => {
    expect(parseUpstreams('10.0.0.5:8080\n[fd00::7]:80 3\napp.internal:443')).toEqual({
      upstreams: [{ host: '10.0.0.5', port: 8080 }, { host: 'fd00::7', port: 80, weight: 3 }, { host: 'app.internal', port: 443 }],
      bad: []
    })
    expect(parseUpstreams('nope\n1.2.3.4:70000').bad).toEqual(['nope', '1.2.3.4:70000'])
  })

  it('turns a listed proxy into a form and back into the same request', () => {
    const proxy = {
      id: 3, name: 'blog', enabled: true, entries: ['proxy-1'], via: ['proxy-2'], listen_port: 443, http_port: 80, tls_mode: 'manual',
      has_certificate: true,
      sites: [{ hosts: ['blog.example.com'], routes: [
        { path: '/api', strip_prefix: true, host_header: 'upstream', upstreams: [{ host: '10.0.0.5', port: 8080 }], balance: 'least_conn' },
        { path: '/', host: 'origin.example.com', upstreams: [{ host: '10.0.0.6', port: 80, weight: 2 }] }
      ] }]
    }
    const form = reverseProxyForm(proxy)
    expect(form.entryNodeId).toBe(1)
    expect(form.viaNodeId).toBe(2)
    expect(validateReverseProxy(form, messages)).toEqual({})
    expect(reverseProxyRequest(form)).toEqual({
      name: 'blog', enabled: true, entries: ['proxy-1'], via: ['proxy-2'], listen_port: 443, http_port: 80, tls_mode: 'manual',
      certificate_pem: '', private_key_pem: '', dns_provider_id: 0, dns_zone: '',
      sites: [{ hosts: ['blog.example.com'], routes: [
        { path: '/api', strip_prefix: true, host_header: 'upstream', upstreams: [{ host: '10.0.0.5', port: 8080 }], balance: 'least_conn' },
        { path: '/', strip_prefix: false, host: 'origin.example.com', upstreams: [{ host: '10.0.0.6', port: 80, weight: 2 }] }
      ] }]
    })
    expect(reverseProxyInput(proxy, { enabled: false }).enabled).toBe(false)
  })

  it('marks what the API would refuse', () => {
    const form = reverseProxyForm(null)
    form.sites[0].routes[0].path = '/api/'
    form.sites[0].routes[0].upstreams = 'x'
    const errors = validateReverseProxy(form, messages)
    expect(errors).toMatchObject({
      name: 'required', entry: 'required', certificatePem: 'required', privateKeyPem: 'required',
      'sites.0.hosts': 'required', 'sites.0.routes.0.path': 'path', 'sites.0.routes.0.upstreams': 'bad x'
    })
  })

  it('sends an acme proxy with its DNS provider and zone, and checks the hosts are in it', () => {
    const form = reverseProxyForm(null)
    Object.assign(form, { name: 'shop', entryNodeId: 1, tlsMode: 'acme', dnsProviderId: 7, dnsZone: 'Example.com' })
    form.sites[0].hosts = 'shop.example.com\n*.shop.example.com'
    form.sites[0].routes[0].upstreams = '10.0.0.9:80'
    expect(validateReverseProxy(form, messages)).toEqual({})
    const body = reverseProxyRequest(form)
    expect(body).toMatchObject({ tls_mode: 'acme', dns_provider_id: 7, dns_zone: 'Example.com', http_port: 80, certificate_pem: '', private_key_pem: '' })
    form.sites[0].hosts = 'shop.example.net\n*'
    form.dnsProviderId = undefined
    expect(validateReverseProxy(form, messages)).toMatchObject({ dnsProvider: 'required', 'sites.0.hosts': 'outside shop.example.net' })
    form.dnsZone = 'nodot'
    expect(validateReverseProxy(form, messages).dnsZone).toBe('zone')
    expect(reverseProxyInput({ name: 'x', tls_mode: 'acme', dns_provider_id: '7', dns_zone: 'example.com' })).toMatchObject({ dns_provider_id: 7, dns_zone: 'example.com' })
  })
})
