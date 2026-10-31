// The reverse proxy form and its request body (ReverseProxySheet), and the
// messages of the reverse proxy API's errors.

// parseLines splits a textarea into its non-empty lines, trimmed, without
// repeats.
export function parseLines(text) {
  const seen = new Set()
  return String(text || '').split(/[\n,]+/).map(item => item.trim()).filter(item => {
    if (!item || seen.has(item)) return false
    seen.add(item)
    return true
  })
}

// parseUpstreams reads "host:port" or "host:port weight" lines; IPv6 hosts
// in brackets. Answers { upstreams, bad } with the lines it could not read.
export function parseUpstreams(text) {
  const upstreams = []
  const bad = []
  for (const line of parseLines(String(text || '').replace(/,/g, '\n'))) {
    const match = /^(\[[0-9a-fA-F:.]+\]|[^\s:[\]]+):(\d{1,5})(?:\s+(\d{1,4}))?$/.exec(line)
    const port = match ? Number(match[2]) : 0
    if (!match || port < 1 || port > 65535) {
      bad.push(line)
      continue
    }
    const upstream = { host: match[1].replace(/^\[|\]$/g, ''), port }
    if (match[3]) upstream.weight = Number(match[3])
    upstreams.push(upstream)
  }
  return { upstreams, bad }
}

export function formatUpstreams(upstreams) {
  return (upstreams || []).map(upstream => {
    const host = upstream.host.includes(':') ? `[${upstream.host}]` : upstream.host
    return `${host}:${upstream.port}${upstream.weight ? ` ${upstream.weight}` : ''}`
  }).join('\n')
}

export function emptyRoute() {
  return { path: '/', stripPrefix: false, hostHeader: 'client', host: '', upstreams: '', balance: '' }
}

export function emptySite() {
  return { hosts: '', routes: [emptyRoute()] }
}

const nodeID = ref => Number(String(ref || '').replace(/^proxy-/, '')) || undefined

export function reverseProxyForm(proxy) {
  return {
    name: proxy?.name || '',
    enabled: proxy ? proxy.enabled !== false : true,
    entryNodeId: nodeID(proxy?.entries?.[0]),
    extraEntries: (proxy?.entries || []).slice(1),
    viaNodeId: nodeID(proxy?.via?.[proxy.via.length - 1]) || 0,
    extraVia: (proxy?.via || []).slice(0, -1),
    listenPort: proxy?.listen_port || 443,
    tlsMode: proxy?.tls_mode || 'manual',
    httpPort: proxy ? proxy.http_port || null : 80,
    certificatePem: '',
    privateKeyPem: '',
    hasCertificate: Boolean(proxy?.has_certificate),
    dnsProviderId: Number(proxy?.dns_provider_id) || undefined,
    dnsZone: proxy?.dns_zone || '',
    sites: (proxy?.sites?.length ? proxy.sites : [null]).map(site => site
      ? {
          hosts: (site.hosts || []).join('\n'),
          routes: (site.routes || []).map(route => ({
            path: route.path || '/',
            stripPrefix: Boolean(route.strip_prefix),
            hostHeader: route.host ? 'custom' : route.host_header || 'client',
            host: route.host || '',
            upstreams: formatUpstreams(route.upstreams),
            balance: route.balance || '',
            headers: route.headers || undefined,
            health: route.health || undefined
          }))
        }
      : emptySite())
  }
}

// validateReverseProxy answers the form's errors by field ("name",
// "sites.0.hosts", "sites.0.routes.1.upstreams", ...); messages are the
// caller's.
export function validateReverseProxy(form, messages) {
  const errors = {}
  if (!form.name.trim()) errors.name = messages.required
  if (!form.entryNodeId) errors.entry = messages.required
  if (form.viaNodeId && form.viaNodeId === form.entryNodeId) errors.via = messages.sameNode
  const port = Number(form.listenPort)
  if (!Number.isInteger(port) || port < 1 || port > 65535) errors.listenPort = messages.port
  if (form.tlsMode !== 'none' && form.httpPort) {
    const http = Number(form.httpPort)
    if (!Number.isInteger(http) || http < 1 || http > 65535 || http === port) errors.httpPort = messages.httpPort
  }
  if (form.tlsMode === 'acme') {
    if (!form.dnsProviderId) errors.dnsProvider = messages.required
    const zone = form.dnsZone.trim().replace(/\.$/, '').toLowerCase()
    if (!zone) errors.dnsZone = messages.required
    else if (!/^[a-z0-9_-]+(\.[a-z0-9_-]+)+$/.test(zone)) errors.dnsZone = messages.zone
    else {
      form.sites.forEach((site, i) => {
        const outside = parseLines(site.hosts).find(host => {
          const name = host.toLowerCase().replace(/^\*\./, '')
          return host === '*' || (name !== zone && !name.endsWith(`.${zone}`))
        })
        if (outside) errors[`sites.${i}.hosts`] = messages.outsideZone(outside)
      })
    }
  }
  if (form.tlsMode === 'manual') {
    const cert = form.certificatePem.trim()
    const key = form.privateKeyPem.trim()
    if (!form.hasCertificate || cert || key) {
      if (!cert) errors.certificatePem = messages.required
      if (!key) errors.privateKeyPem = messages.required
    }
  }
  form.sites.forEach((site, i) => {
    if (!parseLines(site.hosts).length) errors[`sites.${i}.hosts`] = messages.required
    if (!site.routes.length) errors[`sites.${i}.routes`] = messages.required
    site.routes.forEach((route, j) => {
      const path = route.path.trim() || '/'
      if (!path.startsWith('/') || (path.length > 1 && path.endsWith('/'))) errors[`sites.${i}.routes.${j}.path`] = messages.path
      const { upstreams, bad } = parseUpstreams(route.upstreams)
      if (bad.length) errors[`sites.${i}.routes.${j}.upstreams`] = messages.upstream(bad[0])
      else if (!upstreams.length) errors[`sites.${i}.routes.${j}.upstreams`] = messages.required
      if (route.hostHeader === 'custom' && !route.host.trim()) errors[`sites.${i}.routes.${j}.host`] = messages.required
    })
  })
  if (!form.sites.length) errors.sites = messages.required
  return errors
}

export function reverseProxyRequest(form) {
  const mode = ['manual', 'acme'].includes(form.tlsMode) ? form.tlsMode : 'none'
  const manual = mode === 'manual'
  const acme = mode === 'acme'
  return {
    name: form.name.trim(),
    enabled: Boolean(form.enabled),
    entries: [`proxy-${form.entryNodeId}`, ...form.extraEntries],
    via: form.viaNodeId ? [...form.extraVia, `proxy-${form.viaNodeId}`] : [],
    listen_port: Number(form.listenPort),
    http_port: mode !== 'none' && form.httpPort ? Number(form.httpPort) : 0,
    tls_mode: mode,
    certificate_pem: manual ? form.certificatePem.trim() : '',
    private_key_pem: manual ? form.privateKeyPem.trim() : '',
    dns_provider_id: acme ? Number(form.dnsProviderId) : 0,
    dns_zone: acme ? form.dnsZone.trim() : '',
    sites: form.sites.map(site => ({
      hosts: parseLines(site.hosts),
      routes: site.routes.map(route => {
        const out = {
          path: route.path.trim() || '/',
          strip_prefix: Boolean(route.stripPrefix),
          upstreams: parseUpstreams(route.upstreams).upstreams
        }
        if (route.hostHeader === 'custom') out.host = route.host.trim()
        else if (route.hostHeader === 'upstream') out.host_header = 'upstream'
        if (route.balance) out.balance = route.balance
        if (route.headers) out.headers = route.headers
        if (route.health) out.health = route.health
        return out
      })
    }))
  }
}

// reverseProxyInput is a listed proxy as PUT takes it, with changes; the
// stored certificate is kept.
export function reverseProxyInput(proxy, changes = {}) {
  return {
    name: proxy.name,
    enabled: proxy.enabled,
    entries: proxy.entries || [],
    via: proxy.via || [],
    listen_port: proxy.listen_port,
    http_port: proxy.http_port || 0,
    tls_mode: proxy.tls_mode,
    certificate_pem: '',
    private_key_pem: '',
    dns_provider_id: Number(proxy.dns_provider_id) || 0,
    dns_zone: proxy.dns_zone || '',
    sites: proxy.sites || [],
    ...changes
  }
}

export function reverseProxyHosts(proxy) {
  return (proxy?.sites || []).flatMap(site => site.hosts || [])
}

export function reverseProxyErrorMessage(error) {
  const answer = error?.response?.data
  return answer?.error?.message || answer?.message || error?.message || String(error)
}
