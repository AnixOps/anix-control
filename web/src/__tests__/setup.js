// Vitest Setup
import { config } from '@vue/test-utils'

// Global test configuration
config.global.mocks = {
  $t: (key) => key, // i18n mock
  $router: {
    push: vi.fn(),
    replace: vi.fn(),
    go: vi.fn(),
  },
  $route: {
    path: '/',
    params: {},
    query: {},
  },
}

// Mock localStorage
const localStorageMock = {
  getItem: vi.fn(),
  setItem: vi.fn(),
  clear: vi.fn(),
  removeItem: vi.fn(),
}
global.localStorage = localStorageMock

// Mock sessionStorage
global.sessionStorage = localStorageMock