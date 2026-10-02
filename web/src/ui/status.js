// One status vocabulary for nodes, forward nodes, agents, users and jobs.
// Each status has a tone (colour) and an i18n label: the word is always
// shown, colour is never the only signal (guidelines/accessibility.md).
export const STATUS_TONES = Object.freeze({
  online: 'success',
  offline: 'danger',
  disabled: 'neutral',
  pending: 'warning',
  error: 'danger'
})

export const TONES = Object.freeze(['success', 'warning', 'danger', 'info', 'neutral'])

export function statusTone(status) {
  return STATUS_TONES[status] || 'neutral'
}

export function statusLabelKey(status) {
  return STATUS_TONES[status] ? `ui.status.${status}` : ''
}
