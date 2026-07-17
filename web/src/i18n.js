import { createI18n } from 'vue-i18n'

export const LOCALE_STORAGE_KEY = 'app.locale'
export const DEFAULT_LOCALE = 'zh-CN'
export const SUPPORTED_LOCALES = ['zh-CN', 'en']

const localeLoaders = {
  'zh-CN': () => import('./locales/zh-CN.js'),
  en: () => import('./locales/en.js')
}

const loadedLocales = new Set()
const loadingLocales = new Map()

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

async function loadLocaleMessages(locale) {
  const nextLocale = normalizeLocale(locale)
  if (loadedLocales.has(nextLocale)) {
    return nextLocale
  }

  const pending = loadingLocales.get(nextLocale)
  if (pending) {
    return pending
  }

  const loader = localeLoaders[nextLocale]
  if (!loader) {
    throw new Error(`Unsupported locale: ${nextLocale}`)
  }

  const promise = loader()
    .then((module) => {
      const messages = module.default || module
      i18n.global.setLocaleMessage(nextLocale, messages)
      loadedLocales.add(nextLocale)
      loadingLocales.delete(nextLocale)
      return nextLocale
    })
    .catch((error) => {
      loadingLocales.delete(nextLocale)
      throw error
    })

  loadingLocales.set(nextLocale, promise)
  return promise
}

async function ensureLocaleChain(locale) {
  const nextLocale = normalizeLocale(locale)
  const requiredLocales = nextLocale === DEFAULT_LOCALE
    ? [nextLocale]
    : [nextLocale, DEFAULT_LOCALE]

  await Promise.all(requiredLocales.map(loadLocaleMessages))
  return nextLocale
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
    initPromise = ensureLocaleChain(initialLocale).then(() => applyLocaleState(initialLocale, { persist: false }))
  }
  return initPromise
}

export async function setLocale(locale) {
  const nextLocale = normalizeLocale(locale)
  await ensureLocaleChain(nextLocale)
  return applyLocaleState(nextLocale)
}

if (typeof document !== 'undefined') {
  document.documentElement.lang = initialLocale
}

export default i18n
