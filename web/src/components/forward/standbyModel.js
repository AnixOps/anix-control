// The cross-border standby page's helpers (CrossBorderStandby): the prepare
// form and its request body, and labels of the tiers.

export const STANDBY_TIERS = Object.freeze(['reality', 'xhttp'])
export const STANDBY_OFF = 'off'

// parseNodeHosts reads "node=host" lines (node: an id or proxy-<id>; "="
// or ":" or a space separates) into { "<id>": host }; blank lines and
// lines without a host are dropped.
export function parseNodeHosts(text) {
  const out = {}
  for (const line of String(text || '').split('\n')) {
    const match = /^\s*(?:proxy-)?(\d+)\s*[=:\s]\s*(\S+)\s*$/.exec(line)
    if (match) out[match[1]] = match[2].toLowerCase()
  }
  return out
}

export function formatNodeHosts(map) {
  return Object.entries(map || {}).map(([id, host]) => `${id}=${host}`).join('\n')
}

// standbyDomains is the xhttp domain of each exit node the overview knows.
export function standbyDomains(overview) {
  const out = {}
  for (const link of overview?.links || []) {
    for (const channel of link.channels || []) {
      if (channel.tier === 'xhttp' && channel.xhttp_domain) out[channel.exit_node_id] = channel.xhttp_domain
    }
  }
  return out
}

export function standbyForm(overview) {
  let serverName = ''
  let dest = ''
  for (const link of overview?.links || []) {
    for (const channel of link.channels || []) {
      if (channel.tier === 'reality' && channel.reality_server_name && !serverName) {
        serverName = channel.reality_server_name
        dest = channel.reality_dest || ''
      }
    }
  }
  return { serverName, dest, domains: formatNodeHosts(standbyDomains(overview)) }
}

export function standbyPrepareRequest(form) {
  return {
    reality_server_name: form.serverName.trim(),
    reality_dest: form.dest.trim(),
    xhttp_domains: parseNodeHosts(form.domains)
  }
}

export function tierTone(tier) {
  if (tier === 'reality') return 'warning'
  if (tier === 'xhttp') return 'danger'
  return 'success'
}
