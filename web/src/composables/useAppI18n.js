import { computed } from 'vue'
import i18n, { setLocale, SUPPORTED_LOCALES } from '@/i18n'

function normalizeDateInput(value) {
  if (value instanceof Date) {
    return Number.isNaN(value.getTime()) ? null : value
  }

  if (typeof value === 'number' && Number.isFinite(value)) {
    const timestamp = value > 1e12 ? value : value * 1000
    const date = new Date(timestamp)
    return Number.isNaN(date.getTime()) ? null : date
  }

  if (typeof value === 'string') {
    const trimmed = value.trim()
    if (!trimmed) {
      return null
    }
    if (/^\d+$/.test(trimmed)) {
      return normalizeDateInput(Number(trimmed))
    }
    const date = new Date(trimmed)
    return Number.isNaN(date.getTime()) ? null : date
  }

  return null
}

export function useAppI18n() {
  const localeRef = i18n.global.locale

  const localeBinding = computed({
    get: () => localeRef.value,
    set: (value) => {
      void setLocale(value)
    }
  })

  const localeOptions = computed(() => ([
    { value: 'zh-CN', label: t('common.locale.zhCN'), shortLabel: t('common.locale.zhShort') },
    { value: 'en', label: t('common.locale.en'), shortLabel: t('common.locale.enShort') }
  ]))

  const currentLocale = computed(() => localeBinding.value)

  function switchLocale(nextLocale) {
    return setLocale(nextLocale)
  }

  function toggleLocale() {
    switchLocale(localeBinding.value === 'zh-CN' ? 'en' : 'zh-CN')
  }

  function t(...args) {
    return i18n.global.t(...args)
  }

  function formatDate(value, options = {}) {
    const date = normalizeDateInput(value)
    if (!date) {
      return ''
    }
    return new Intl.DateTimeFormat(localeRef.value, options).format(date)
  }

  function formatDateTime(value, options = {}) {
    const date = normalizeDateInput(value)
    if (!date) {
      return ''
    }
    return new Intl.DateTimeFormat(localeRef.value, {
      year: 'numeric',
      month: 'short',
      day: 'numeric',
      hour: '2-digit',
      minute: '2-digit',
      ...options
    }).format(date)
  }

  function translateLiteral(value) {
    if (typeof value !== 'string') {
      return value
    }
    const messages = i18n.global.getLocaleMessage(localeRef.value) || {}
    const map = messages.legacy || {}
    return map[value] || value
  }

  return {
    currentLocale,
    locale: localeBinding,
    localeOptions,
    supportedLocales: SUPPORTED_LOCALES,
    t,
    switchLocale,
    toggleLocale,
    formatDate,
    formatDateTime,
    translateLiteral
  }
}
