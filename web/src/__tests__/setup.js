import { config } from '@vue/test-utils'
import { afterEach, beforeAll, beforeEach, vi } from 'vitest'
import { resetEdition } from '@/composables/useEdition'

// No test reaches the backend for the public configuration: the edition is
// community unless a test calls setEdition (or mocks getPublicConfig).
vi.mock('@/api/public', () => ({
  getPublicConfig: vi.fn(async () => ({
    edition: 'community',
    hidden_packages: ['affiliate', 'order', 'payment'],
    registration: { enabled: true, require_invite: false }
  }))
}))
config.global.plugins = []
config.global.mocks = {
  $router: {
    push: vi.fn(),
    replace: vi.fn(),
    go: vi.fn()
  },
  $route: {
    path: '/',
    params: {},
    query: {}
  }
}

function createStorageMock() {
  let store = {}
  return {
    getItem: vi.fn((key) => (key in store ? store[key] : null)),
    setItem: vi.fn((key, value) => {
      store[key] = String(value)
    }),
    clear: vi.fn(() => {
      store = {}
    }),
    removeItem: vi.fn((key) => {
      delete store[key]
    })
  }
}

const localStorageMock = createStorageMock()
const sessionStorageMock = createStorageMock()

global.localStorage = localStorageMock
global.sessionStorage = sessionStorageMock

if (typeof window.alert !== 'function') {
  window.alert = () => {}
}

if (typeof window.confirm !== 'function') {
  window.confirm = () => true
}

let i18nModule = null

beforeAll(async () => {
  i18nModule = await import('@/i18n')
  config.global.plugins = [i18nModule.default]
  await i18nModule.initI18n()
})

beforeEach(async () => {
  resetEdition()
  localStorage.clear()
  sessionStorage.clear()
  localStorage.setItem('app.locale', 'en')
  await i18nModule.setLocale('en')
})

afterEach(() => {
  vi.unstubAllGlobals()
  vi.useRealTimers()
})
