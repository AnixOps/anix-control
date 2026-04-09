import { config } from '@vue/test-utils'
import { beforeAll, beforeEach, vi } from 'vitest'
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

let i18nModule = null

beforeAll(async () => {
  i18nModule = await import('@/i18n')
  config.global.plugins = [i18nModule.default]
  await i18nModule.initI18n()
})

beforeEach(async () => {
  localStorage.clear()
  sessionStorage.clear()
  localStorage.setItem('app.locale', 'en')
  await i18nModule.setLocale('en')
})
