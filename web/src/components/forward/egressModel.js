// The egress route form and its request body (EgressRouteSheet), and the
// messages of the egress route API's errors.

// Common AI service domains, the sheet's preset. A bare domain matches its
// subdomains too.
export const AI_DOMAINS = Object.freeze([
  'openai.com', 'chatgpt.com', 'oaistatic.com', 'oaiusercontent.com', 'sora.com',
  'anthropic.com', 'claude.ai', 'claude.com', 'claudeusercontent.com',
  'gemini.google.com', 'aistudio.google.com', 'generativelanguage.googleapis.com', 'deepmind.google', 'notebooklm.google',
  'x.ai', 'grok.com', 'perplexity.ai', 'copilot.microsoft.com', 'mistral.ai', 'poe.com', 'cursor.com', 'cursor.sh'
])

// parseDomains splits the textarea: one domain per line (commas and spaces
// also separate), empty entries and repeats dropped.
export function parseDomains(text) {
  const seen = new Set()
  return String(text || '').split(/[\s,]+/).map(item => item.trim()).filter(item => {
    if (!item || seen.has(item)) return false
    seen.add(item)
    return true
  })
}

export function egressForm(route) {
  return {
    name: route?.name || '',
    sourceNodeId: route?.source_node?.id || undefined,
    exitNodeId: route?.exit_node?.id || undefined,
    exitHost: route?.exit_host || '',
    relayPort: route?.relay_port || null,
    domains: (route?.domains || []).join('\n'),
    enabled: route ? route.enabled !== false : true,
    rotateKey: false
  }
}

export function egressRequest(form) {
  return {
    name: form.name.trim(),
    source_node_id: Number(form.sourceNodeId),
    exit_node_id: Number(form.exitNodeId),
    exit_host: form.exitHost.trim(),
    relay_port: Number(form.relayPort),
    domains: parseDomains(form.domains),
    enabled: Boolean(form.enabled),
    rotate_key: Boolean(form.rotateKey)
  }
}

// egressRouteInput is a listed route as PUT takes it, with changes.
export function egressRouteInput(route, changes = {}) {
  return {
    name: route.name,
    source_node_id: route.source_node.id,
    exit_node_id: route.exit_node.id,
    exit_host: route.exit_host || '',
    relay_port: route.relay_port,
    domains: route.domains || [],
    enabled: route.enabled,
    ...changes
  }
}

export function egressErrorMessage(error) {
  const answer = error?.response?.data
  return answer?.error?.message || answer?.message || error?.message || String(error)
}
