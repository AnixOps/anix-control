import { createI18n } from 'vue-i18n'

export const LOCALE_STORAGE_KEY = 'app.locale'
export const DEFAULT_LOCALE = 'zh-CN'
export const SUPPORTED_LOCALES = ['zh-CN', 'en']

// Messages come in groups, one chunk per locale and group, so a page loads
// only what it shows: `core` (sign-in, user portal, shell, components) at
// startup, `admin` (the admin shell: navigation, ⌘K, the dashboard) once an
// admin route opens or an admin is signed in, and `adminPages` (every other
// admin page and the forward suite) before the first of those pages opens
// (router/index.js). Only the active locale is loaded; en and zh-CN carry the
// same keys (localeParity.test.js), so the fallback locale is not fetched up
// front.
const localeLoaders = {
  'zh-CN': {
    core: () => import('./locales/zh-CN.js'),
    admin: () => import('./locales/zh-CN.admin.js'),
    adminPages: () => import('./locales/zh-CN.adminPages.js')
  },
  en: {
    core: () => import('./locales/en.js'),
    admin: () => import('./locales/en.admin.js'),
    adminPages: () => import('./locales/en.adminPages.js')
  }
}

export const MESSAGE_GROUPS = ['core', 'admin', 'adminPages']

const activeGroups = new Set(['core'])
const loadedGroups = new Set()
const loadingGroups = new Map()

function normalizeLocale(value) {
  const raw = typeof value === 'string' ? value.trim().toLowerCase() : ''
  if (raw.startsWith('zh')) {
    return 'zh-CN'
  }
  if (raw.startsWith('en')) {
    return 'en'
  }
  return DEFAULT_LOCALE
}

function resolveInitialLocale() {
  if (typeof localStorage !== 'undefined') {
    const stored = localStorage.getItem(LOCALE_STORAGE_KEY)
    if (stored) {
      return normalizeLocale(stored)
    }
  }

  if (typeof navigator !== 'undefined') {
    const candidates = [navigator.language, ...(navigator.languages || [])]
    for (const candidate of candidates) {
      const normalized = normalizeLocale(candidate)
      if (SUPPORTED_LOCALES.includes(normalized)) {
        return normalized
      }
    }
  }

  return DEFAULT_LOCALE
}

function applyLocaleState(locale, { persist = true } = {}) {
  i18n.global.locale.value = locale
  if (persist && typeof localStorage !== 'undefined') {
    localStorage.setItem(LOCALE_STORAGE_KEY, locale)
  }
  if (typeof document !== 'undefined') {
    document.documentElement.lang = locale
  }
  return locale
}

function loadLocaleGroup(locale, group) {
  const key = `${locale}:${group}`
  if (loadedGroups.has(key)) {
    return Promise.resolve(locale)
  }

  const pending = loadingGroups.get(key)
  if (pending) {
    return pending
  }

  const loader = localeLoaders[locale]?.[group]
  if (!loader) {
    return Promise.reject(new Error(`Unsupported locale or message group: ${key}`))
  }

  const promise = loader()
    .then((module) => {
      // Groups share no message key, so merging keeps each one whole.
      i18n.global.mergeLocaleMessage(locale, module.default || module)
      loadedGroups.add(key)
      loadingGroups.delete(key)
      return locale
    })
    .catch((error) => {
      loadingGroups.delete(key)
      throw error
    })

  loadingGroups.set(key, promise)
  return promise
}

async function ensureLocale(locale) {
  const nextLocale = normalizeLocale(locale)
  await Promise.all([...activeGroups].map((group) => loadLocaleGroup(nextLocale, group)))
  return nextLocale
}

// Load a message group for the current locale, and for every locale switched
// to afterwards.
export async function loadMessageGroup(group) {
  if (!MESSAGE_GROUPS.includes(group)) {
    throw new Error(`Unknown message group: ${group}`)
  }
  activeGroups.add(group)
  await loadLocaleGroup(normalizeLocale(i18n.global.locale.value), group)
}

const initialLocale = resolveInitialLocale()
const i18n = createI18n({
  legacy: false,
  locale: initialLocale,
  fallbackLocale: DEFAULT_LOCALE,
  messages: {}
})

let initPromise = null

export async function initI18n() {
  if (!initPromise) {
    initPromise = ensureLocale(initialLocale).then(() => applyLocaleState(initialLocale, { persist: false }))
  }
  return initPromise
}

export async function setLocale(locale) {
  const nextLocale = normalizeLocale(locale)
  await ensureLocale(nextLocale)
  return applyLocaleState(nextLocale)
}

if (typeof document !== 'undefined') {
  document.documentElement.lang = initialLocale
}

export default i18n
