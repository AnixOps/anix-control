// Entry HA through DNS (L2, docs/forwarding/v4-api.md "Entry HA through
// DNS"): the provider form built from GET /dns/kinds, the binding draft of
// the route editor and the words and tones of the route's DNS status.
import { forwardErrorMessage } from './messages'
import { num } from './routeModel'

// A stored credential is shown as this placeholder. Sent back unchanged it
// keeps the stored value (PUT /dns/providers/{id}).
export const CREDENTIAL_PLACEHOLDER = '********'

export const DNS_MODES = Object.freeze(['DNS_BINDING_MODE_DDNS', 'DNS_BINDING_MODE_CNAME'])
export const RECORD_TYPES = Object.freeze(['DNS_RECORD_TYPE_A', 'DNS_RECORD_TYPE_AAAA'])
export const DEFAULT_TTL = 60
export const MAX_TTL = 86_400

export const STATE_TONES = Object.freeze({
  ok: 'success',
  pending: 'info',
  degraded: 'warning',
  error: 'danger',
  rate_limited: 'warning',
  paused: 'neutral',
  unbound: 'neutral',
  route_missing: 'danger',
  hostname_mismatch: 'danger'
})

export const REASON_TONES = Object.freeze({
  healthy: 'success',
  converging: 'info',
  not_in_inventory: 'neutral',
  no_address: 'warning',
  never_reported: 'neutral',
  report_stale: 'warning',
  offline: 'danger',
  hop_error: 'danger',
  upstreams_down: 'danger'
})

export const stateTone = state => STATE_TONES[state] || 'neutral'
export const reasonTone = reason => REASON_TONES[reason] || 'neutral'

// recordTypeLabel: DNS_RECORD_TYPE_AAAA → AAAA.
export const recordTypeLabel = type => String(type || '').replace(/^DNS_RECORD_TYPE_/, '') || 'A'

// kindName: the kind's display name from GET /dns/kinds, else the enum
// without its prefix.
export function kindName(kinds, kind) {
  const hit = (kinds || []).find(item => item.kind === kind)
  return hit?.name || String(kind || '').replace(/^DNS_PROVIDER_KIND_/, '').toLowerCase()
}

// providerForm is the form state of a provider sheet. A new provider starts
// from the first kind; an existing one shows its stored credentials as the
// placeholder (the API never answers their values).
export function providerForm(provider, kinds) {
  const kind = provider?.kind || kinds?.[0]?.kind || ''
  const spec = (kinds || []).find(item => item.kind === kind)
  const stored = new Set(provider?.credential_names || [])
  const config = {}
  for (const field of spec?.config || []) config[field] = provider?.config?.[field] || ''
  const credentials = {}
  for (const field of spec?.credentials || []) credentials[field] = stored.has(field) ? CREDENTIAL_PLACEHOLDER : ''
  return { id: provider?.id || '', name: provider?.name || '', kind, config, credentials, stored: [...stored] }
}

// switchKind resets the config and credential fields for another kind (new
// providers only: a provider's kind cannot change).
export function switchKind(form, kind, kinds) {
  const next = providerForm({ kind }, kinds)
  return { ...form, kind, config: next.config, credentials: next.credentials, stored: [] }
}

// providerErrors checks what the kind's schema makes required: the name,
// required_config, and every credential of a new provider (an existing one
// keeps a stored credential left as the placeholder).
export function providerErrors(form, kinds, required) {
  const spec = (kinds || []).find(item => item.kind === form.kind)
  const errors = {}
  if (!String(form.name || '').trim()) errors.name = required
  if (!spec) errors.kind = required
  for (const field of spec?.required_config || []) {
    if (!String(form.config?.[field] || '').trim()) errors[`config.${field}`] = required
  }
  for (const field of spec?.credentials || []) {
    const value = String(form.credentials?.[field] || '')
    const kept = form.id && form.stored?.includes(field) && (value === '' || value === CREDENTIAL_PLACEHOLDER)
    if (!value.trim() && !kept) errors[`credentials.${field}`] = required
  }
  return errors
}

// providerRequest is the provider and credentials of the write. Only the
// kind's config fields go; empty ones are left out (the provider default).
// Credentials still at the placeholder are left out, so they keep their
// stored values and no secret round-trips through the browser.
export function providerRequest(form, kinds) {
  const spec = (kinds || []).find(item => item.kind === form.kind)
  const config = {}
  for (const field of spec?.config || []) {
    const value = String(form.config?.[field] || '').trim()
    if (value) config[field] = value
  }
  const credentials = {}
  for (const field of spec?.credentials || []) {
    const value = String(form.credentials?.[field] || '')
    if (value && value !== CREDENTIAL_PLACEHOLDER) credentials[field] = value
  }
  return { provider: { name: String(form.name || '').trim(), kind: form.kind, config }, credentials }
}

// ---------------------------------------------------------------------------
// Bindings
// ---------------------------------------------------------------------------

// zoneOf guesses the zone of a hostname: its last two labels.
export function zoneOf(hostname) {
  const labels = String(hostname || '').trim().replace(/\.$/, '').split('.').filter(Boolean)
  return labels.length > 2 ? labels.slice(-2).join('.') : labels.join('.')
}

// bindingDraft is the editor's state of a route's binding: the stored one,
// or a new one off.
export function bindingDraft(binding, hostname = '') {
  if (binding) {
    return {
      enabled: true,
      id: String(binding.id || ''),
      provider_id: String(binding.provider_id || ''),
      zone: binding.zone || '',
      record_name: binding.record_name || '',
      mode: binding.mode || 'DNS_BINDING_MODE_DDNS',
      record_types: binding.record_types?.length ? [...binding.record_types] : ['DNS_RECORD_TYPE_A'],
      ttl: num(binding.ttl) || DEFAULT_TTL,
      paused: Boolean(binding.paused)
    }
  }
  return {
    enabled: false,
    id: '',
    provider_id: '',
    zone: zoneOf(hostname),
    record_name: '',
    mode: 'DNS_BINDING_MODE_DDNS',
    record_types: ['DNS_RECORD_TYPE_A'],
    ttl: DEFAULT_TTL,
    paused: false
  }
}

// bindingErrors checks a binding draft that is on.
export function bindingErrors(draft, hostname, t) {
  if (!draft?.enabled) return {}
  const errors = {}
  const required = t('forwardDns.binding.required')
  if (!String(hostname || '').trim()) errors.hostname = t('forwardDns.binding.needsHostname')
  if (!draft.provider_id) errors.provider_id = required
  const zone = String(draft.zone || '').trim().replace(/\.$/, '').toLowerCase()
  if (!zone) errors.zone = required
  const name = draft.mode === 'DNS_BINDING_MODE_CNAME'
    ? String(draft.record_name || '').trim().replace(/\.$/, '').toLowerCase()
    : String(hostname || '').trim().replace(/\.$/, '').toLowerCase()
  if (draft.mode === 'DNS_BINDING_MODE_CNAME' && !name) errors.record_name = required
  else if (zone && name && name !== zone && !name.endsWith(`.${zone}`)) errors[draft.mode === 'DNS_BINDING_MODE_CNAME' ? 'record_name' : 'zone'] = t('forwardDns.binding.outsideZone')
  if (!draft.record_types?.length) errors.record_types = t('forwardDns.binding.typeRequired')
  const ttl = Number(draft.ttl)
  if (draft.ttl !== null && draft.ttl !== '' && (!Number.isInteger(ttl) || ttl < 1 || ttl > MAX_TTL)) errors.ttl = t('forwardDns.binding.ttlRange', { max: MAX_TTL })
  return errors
}

// bindingRequest is the DnsBinding to write for a route. A new binding in
// DDNS mode names the entry hostname; an update sends the stored binding
// whole with record_types, ttl and paused changed (PUT is a full object).
export function bindingRequest(draft, routeId, hostname, stored = null) {
  const changeable = {
    record_types: RECORD_TYPES.filter(type => draft.record_types.includes(type)),
    ttl: Number(draft.ttl) || DEFAULT_TTL,
    paused: Boolean(draft.paused)
  }
  if (stored) return { ...stored, ...changeable }
  return {
    route_id: routeId,
    provider_id: draft.provider_id,
    zone: String(draft.zone || '').trim(),
    record_name: draft.mode === 'DNS_BINDING_MODE_CNAME' ? String(draft.record_name || '').trim() : String(hostname || '').trim(),
    mode: draft.mode,
    ...changeable
  }
}

// bindingChanged: whether the draft differs from the stored binding in
// what a PUT changes.
export function bindingChanged(draft, stored) {
  if (!stored) return Boolean(draft?.enabled)
  const before = bindingDraft(stored)
  return before.paused !== Boolean(draft.paused) ||
    (Number(draft.ttl) || DEFAULT_TTL) !== before.ttl ||
    RECORD_TYPES.filter(type => draft.record_types.includes(type)).join() !== RECORD_TYPES.filter(type => before.record_types.includes(type)).join()
}

// ---------------------------------------------------------------------------
// Errors
// ---------------------------------------------------------------------------

// dnsErrorMessage is one sentence for a failed DNS call: the words for the
// first violation code the UI knows, else forwardErrorMessage.
export function dnsErrorMessage(t, te, error) {
  if (!error) return ''
  if (error.code === 'dns_purge_failed') return t('forwardDns.errors.dns_purge_failed')
  for (const violation of error.violations || []) {
    const key = `forwardDns.codes.${violation.code}`
    if (te(key)) return t(key)
  }
  return forwardErrorMessage(t, error)
}
