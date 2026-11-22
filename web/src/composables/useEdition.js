import { computed, reactive } from 'vue'
import { getPublicConfig } from '@/api/public'

// The product edition decides which commercial entry points the web app
// shows. Menus, routes and fields that only the commercial edition serves
// carry `edition: 'commercial'` (routes: `meta.edition`); this module is the
// one place that decides whether they are shown.
export const EDITION_COMMUNITY = 'community'
export const EDITION_COMMERCIAL = 'commercial'

const state = reactive({
  edition: EDITION_COMMUNITY,
  registrationEnabled: true,
  requireInvite: false,
  // The source code address (source_url): the AGPL-3.0 offer of the source
  // to everyone who uses the panel, linked from the sign-in page and the
  // account menu.
  sourceUrl: '',
  loaded: false
})

let pending = null

export function normalizeEdition(value) {
  return String(value || '').trim().toLowerCase() === EDITION_COMMERCIAL ? EDITION_COMMERCIAL : EDITION_COMMUNITY
}

export function applyPublicConfig(config = {}) {
  state.edition = normalizeEdition(config.edition)
  state.registrationEnabled = config.registration?.enabled !== false
  state.requireInvite = config.registration?.require_invite === true
  state.sourceUrl = /^https?:\/\//i.test(config.source_url || '') ? config.source_url : ''
  state.loaded = true
  return state
}

// loadEdition reads the public configuration once. A failure keeps the
// community defaults: commercial entry points are never shown by accident.
export function loadEdition({ force = false } = {}) {
  if (state.loaded && !force) {
    return Promise.resolve(state)
  }
  if (!pending) {
    pending = Promise.resolve()
      .then(() => getPublicConfig())
      .then(config => applyPublicConfig(config || {}))
      .catch(() => {
        state.loaded = true
        return state
      })
      .finally(() => {
        pending = null
      })
  }
  return pending
}

// setEdition sets the edition without the backend (tests, previews).
export function setEdition(edition, { requireInvite = false, registrationEnabled = true } = {}) {
  return applyPublicConfig({
    edition: normalizeEdition(edition),
    registration: { enabled: registrationEnabled, require_invite: requireInvite }
  })
}

export function resetEdition() {
  state.edition = EDITION_COMMUNITY
  state.registrationEnabled = true
  state.requireInvite = false
  state.sourceUrl = ''
  state.loaded = false
  pending = null
}

export function isCommercialEdition() {
  return state.edition === EDITION_COMMERCIAL
}

// editionAllows reports whether an item marked with `edition` is shown.
export function editionAllows(required) {
  return !required || normalizeEdition(required) === state.edition
}

// filterByEdition drops the items (menu entries, actions, fields) the
// current edition does not show.
export function filterByEdition(items) {
  return (items || []).filter(item => editionAllows(item?.edition))
}

// routeAllowedByEdition checks a resolved route against `meta.edition`.
export function routeAllowedByEdition(route) {
  return !(route?.matched || []).some(record => !editionAllows(record?.meta?.edition))
}

export function useEdition() {
  return {
    edition: computed(() => state.edition),
    isCommercial: computed(() => state.edition === EDITION_COMMERCIAL),
    isCommunity: computed(() => state.edition !== EDITION_COMMERCIAL),
    requireInvite: computed(() => state.requireInvite),
    registrationEnabled: computed(() => state.registrationEnabled),
    sourceUrl: computed(() => state.sourceUrl),
    loaded: computed(() => state.loaded),
    filterByEdition,
    loadEdition
  }
}
