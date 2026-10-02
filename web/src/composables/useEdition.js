import { computed, reactive } from 'vue'
import { getPublicConfig } from '@/api/public'

// The product edition decides which commercial entry points the web app
// shows. Menus, routes and fields that only the commercial edition serves
// carry `edition: 'commercial'` (routes: `meta.edition`); this module is the
// one place that decides whether they are shown.
export const EDITION_COMMUNITY = 'community'
export const EDITION_COMMERCIAL = 'commercial'

// Packages the community edition hides when the backend does not say
// (config/editions.json). Their extension menus are not shown.
const DEFAULT_HIDDEN_PACKAGES = Object.freeze(['affiliate', 'order', 'payment'])

const state = reactive({
  edition: EDITION_COMMUNITY,
  hiddenPackages: [...DEFAULT_HIDDEN_PACKAGES],
  registrationEnabled: true,
  requireInvite: false,
  loaded: false
})

let pending = null

export function normalizeEdition(value) {
  return String(value || '').trim().toLowerCase() === EDITION_COMMERCIAL ? EDITION_COMMERCIAL : EDITION_COMMUNITY
}

export function applyPublicConfig(config = {}) {
  const edition = normalizeEdition(config.edition)
  state.edition = edition
  state.hiddenPackages = Array.isArray(config.hidden_packages)
    ? config.hidden_packages.map(String)
    : (edition === EDITION_COMMERCIAL ? [] : [...DEFAULT_HIDDEN_PACKAGES])
  state.registrationEnabled = config.registration?.enabled !== false
  state.requireInvite = config.registration?.require_invite === true
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
export function setEdition(edition, { requireInvite = false, registrationEnabled = true, hiddenPackages } = {}) {
  const normalized = normalizeEdition(edition)
  return applyPublicConfig({
    edition: normalized,
    hidden_packages: hiddenPackages ?? (normalized === EDITION_COMMERCIAL ? [] : [...DEFAULT_HIDDEN_PACKAGES]),
    registration: { enabled: registrationEnabled, require_invite: requireInvite }
  })
}

export function resetEdition() {
  state.edition = EDITION_COMMUNITY
  state.hiddenPackages = [...DEFAULT_HIDDEN_PACKAGES]
  state.registrationEnabled = true
  state.requireInvite = false
  state.loaded = false
  pending = null
}

export function currentEdition() {
  return state.edition
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

// routeAllowedByEdition checks a resolved route against `meta.edition` and
// the extension pages of hidden packages (`meta.extensionPluginID`).
export function routeAllowedByEdition(route) {
  return !(route?.matched || []).some(record => (
    !editionAllows(record?.meta?.edition) ||
    (record?.meta?.extensionPluginID && state.hiddenPackages.includes(record.meta.extensionPluginID))
  ))
}

// extensionMenuAllowed hides the extension menus of hidden packages.
export function extensionMenuAllowed(menu) {
  const pluginID = String(menu?.pluginID || menu?.plugin_id || String(menu?.id || '').split('.')[0] || '')
  return !state.hiddenPackages.includes(pluginID)
}

export function useEdition() {
  return {
    edition: computed(() => state.edition),
    isCommercial: computed(() => state.edition === EDITION_COMMERCIAL),
    isCommunity: computed(() => state.edition !== EDITION_COMMERCIAL),
    requireInvite: computed(() => state.requireInvite),
    registrationEnabled: computed(() => state.registrationEnabled),
    loaded: computed(() => state.loaded),
    filterByEdition,
    loadEdition
  }
}
