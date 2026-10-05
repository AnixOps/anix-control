// The kernel's alerts on the dashboard (GET /api/v4/kernel/alerts): which tone,
// icon and link each alert gets, and the text composed from its `kind` and
// `detail` (the server's own `message` is English, for notifications; it is
// only the fallback for a kind this build does not know). Pure functions.
import { CalendarClock, FileKey, Hourglass, ShieldAlert, ServerCog, ServerOff } from '@lucide/vue'

// critical is danger, everything else warning (the dashboard has no third tone).
export function alertTone(alert) {
  return alert?.severity === 'critical' ? 'danger' : 'warning'
}

// "proxy-12" is node 12 of the proxy list; "forward-12" a forward node, whose
// page is keyed by that whole reference (the forwarding area, v4.2).
export function alertRoute(alert) {
  if (alert?.subject_kind !== 'node') return null
  const match = /^(proxy|forward)-(\d+)$/.exec(String(alert.subject || ''))
  if (!match) return null
  return match[1] === 'proxy' ? `/admin/nodes/${match[2]}` : `/admin/forward/inventory/${match[1]}-${match[2]}`
}

const KIND_ICONS = {
  agent_certificate_expiring: FileKey,
  link_certificate_expiring: FileKey,
  module_certificate_expiring: FileKey,
  ca_expiring: ShieldAlert,
  node_secrets_split_stalled: Hourglass,
  node_secrets_finalize_interrupted: Hourglass,
  identity_import_stalled: Hourglass,
  identity_cutover_not_finalized: Hourglass
}

export const KERNEL_ALERT_KINDS = Object.freeze(Object.keys(KIND_ICONS))

export function alertIcon(alert) {
  if (KIND_ICONS[alert?.kind]) return KIND_ICONS[alert.kind]
  if (alert?.subject_kind === 'node') return ServerOff
  if (alert?.subject_kind === 'node_secrets' || alert?.subject_kind === 'identity') return ServerCog
  return CalendarClock
}

const text = value => (typeof value === 'string' || typeof value === 'number' ? String(value) : '')

// A node alert's holder name: the node's name, else its reference (proxy-12).
function holderName(alert, detail) {
  return text(detail.node_name) || text(detail.node) || text(alert.subject)
}

// { title, hint } of an alert in the current language. `t` is vue-i18n's, and
// `format` is useFormat(). Resolved alerts say when they ended instead.
export function alertText(alert, { t, format }) {
  const detail = alert?.detail && typeof alert.detail === 'object' ? alert.detail : {}
  const base = 'adminDashboard.alerts.kinds'
  const date = value => (value ? format.dateTime(value) : '')
  const ended = detail.expired === true
  let title = ''
  let hint = ''
  switch (alert?.kind) {
    case 'agent_certificate_expiring':
    case 'link_certificate_expiring': {
      const kind = alert.kind === 'agent_certificate_expiring' ? 'agentCertificate' : 'linkCertificate'
      title = t(`${base}.${kind}.${ended ? 'ended' : 'ending'}`, { name: holderName(alert, detail) })
      hint = t(`${base}.${kind}.${ended ? 'endedHint' : 'endingHint'}`, { date: date(detail.not_after) })
      break
    }
    case 'module_certificate_expiring':
      title = t(`${base}.moduleCertificate.${ended ? 'ended' : 'ending'}`, { name: text(detail.package_id) || text(alert.subject) })
      hint = t(`${base}.moduleCertificate.${ended ? 'endedHint' : 'endingHint'}`, { date: date(detail.not_after) })
      break
    case 'ca_expiring': {
      const ca = detail.ca === 'forward_link' ? t(`${base}.ca.forwardLink`) : t(`${base}.ca.module`)
      title = t(`${base}.ca.${ended ? 'ended' : 'ending'}`, { ca })
      hint = t(`${base}.ca.${detail.next_staged ? 'staged' : 'notStaged'}`, { date: date(detail.not_after) })
      break
    }
    case 'node_secrets_split_stalled':
      title = t(`${base}.nodeSecretsStalled.title`, { table: text(detail.table) || text(alert.subject), phase: text(detail.phase) })
      hint = t(`${base}.nodeSecretsStalled.hint`, { date: date(detail.since) })
      break
    case 'node_secrets_finalize_interrupted':
      title = t(`${base}.nodeSecretsInterrupted.title`, { table: text(detail.table) || text(alert.subject) })
      hint = t(`${base}.nodeSecretsInterrupted.hint`, { date: date(detail.since) })
      break
    case 'identity_import_stalled':
      title = t(`${base}.identityImport.title`)
      hint = t(`${base}.identityImport.hint`, { date: date(detail.since) })
      break
    case 'identity_cutover_not_finalized':
      title = t(`${base}.identityCutover.title`)
      hint = t(`${base}.identityCutover.hint`, { date: date(detail.since) })
      break
    default:
      title = text(alert?.message) || text(alert?.kind) || text(alert?.key)
  }
  if (alert?.status === 'resolved' && alert.resolved_at) {
    hint = t('adminDashboard.alerts.resolvedAt', { date: format.dateTime(alert.resolved_at) })
  }
  return { title, hint }
}
