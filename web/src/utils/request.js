import { useUserStore } from '@/stores/user'

// axios (about 18 KB gzip) loads with the first request instead of with the
// app, which keeps it off the sign-in page's critical path (performance
// budget, docs/reference/frontend-design.md "Bundle").

let unauthorizedRedirectPending = false

function getUserStoreSafely() {
  try {
    return useUserStore()
  } catch {
    return null
  }
}

function readStoredToken() {
  try {
    return (localStorage.getItem('token') || '').trim()
  } catch {
    return ''
  }
}

function formatAuthorizationHeader(token) {
  const normalized = typeof token === 'string' ? token.trim() : ''
  if (!normalized) {
    return ''
  }
  return /^Bearer\s+/i.test(normalized) ? normalized : `Bearer ${normalized}`
}

function clearStoredAuth() {
  try {
    localStorage.removeItem('token')
    localStorage.removeItem('userInfo')
  } catch {
    // Ignore storage cleanup failures and keep request rejection behavior unchanged.
  }
}

function clearAuthState() {
  const userStore = getUserStoreSafely()
  if (userStore?.logout) {
    try {
      userStore.logout()
      return
    } catch {
      // Fall back to direct storage cleanup when the store is unavailable.
    }
  }
  clearStoredAuth()
}

function isAuthEndpoint(config) {
  const url = typeof config?.url === 'string' ? config.url : ''
  return url.endsWith('/login') || url.endsWith('/register')
}

function redirectToLogin() {
  if (typeof window === 'undefined' || unauthorizedRedirectPending) {
    return
  }

  const pathname = window.location?.pathname || ''
  if (pathname === '/login') {
    return
  }

  unauthorizedRedirectPending = true
  if (typeof window.location?.replace === 'function') {
    window.location.replace('/login')
    return
  }
  window.location.href = '/login'
}

// The server's rate limiter (internal/middleware/rate_limiter.go) answers 429
// with Retry-After once a client has spent its burst. All administrator APIs
// share one bucket per client address, so opening a page right after another
// one loaded can spend it: the built-in packages page then showed "didn't
// load" (CI run 43652174366). A read is safe to send again, so wait as long
// as the server asks (at most 5 s) and send it again, at most twice. Writes
// are never repeated; their callers show the refusal.
const RATE_LIMIT_RETRIES = 2
const RATE_LIMIT_MAX_WAIT_MS = 5000

function retryAfterHeader(headers) {
  if (!headers) return undefined
  if (typeof headers.get === 'function') return headers.get('retry-after')
  return headers['retry-after'] ?? headers['Retry-After']
}

// Milliseconds to wait before sending a rate-limited read again, or null when
// it must not be sent again.
export function rateLimitRetryDelay(error) {
  const config = error?.config
  if (!config || error?.response?.status !== 429) return null
  const method = String(config.method || 'get').toLowerCase()
  if (method !== 'get' && method !== 'head') return null
  if (config.noRateLimitRetry || config.signal?.aborted) return null
  if ((config.rateLimitRetries || 0) >= RATE_LIMIT_RETRIES) return null
  const seconds = Number.parseInt(retryAfterHeader(error.response.headers), 10)
  const wait = Number.isFinite(seconds) && seconds > 0 ? seconds * 1000 : 1000
  return Math.min(wait, RATE_LIMIT_MAX_WAIT_MS)
}

let clientPromise = null

function createClient(axios) {
  const service = axios.create({
    baseURL: '/api/v2',
    timeout: 5000
  })

  // Request interceptor
  service.interceptors.request.use(
    config => {
      const userStore = getUserStoreSafely()
      const token = userStore?.token || readStoredToken()
      const authorization = formatAuthorizationHeader(token)

      config.headers = config.headers || {}
      if (authorization && !config.headers.Authorization) {
        config.headers.Authorization = authorization
      }
      return config
    },
    error => {
      return Promise.reject(error)
    }
  )

  // Response interceptor
  service.interceptors.response.use(
    response => {
      // Kernel lifecycle writes need response headers for operation identity and
      // dependency-chain display. Keep the historical body-only contract for
      // every other caller and opt in per request.
      return response.config?.rawResponse ? response : response.data
    },
    error => {
      const retryDelay = rateLimitRetryDelay(error)
      if (retryDelay !== null) {
        const config = error.config
        config.rateLimitRetries = (config.rateLimitRetries || 0) + 1
        return new Promise(resolve => setTimeout(resolve, retryDelay)).then(() => (
          config.signal?.aborted ? Promise.reject(error) : service(config)
        ))
      }
      if (error?.config?.sensitive) {
        // A request that carries a credential (a password or a one-time code
        // for a step-up): log what failed, never the body the error holds.
        if (error.config) error.config.data = undefined
        console.error('Request error:', error.config?.method, error.config?.url, error.response?.status ?? error.code ?? '')
      } else {
        console.error('Request error:', error)
      }
      if (error?.response?.status === 401 && !isAuthEndpoint(error.config)) {
        clearAuthState()
        redirectToLogin()
      }
      return Promise.reject(error)
    }
  )

  return service
}

// The configured axios instance, created on first use.
export function loadRequestClient() {
  if (!clientPromise) {
    clientPromise = import('axios')
      .then(({ default: axios }) => createClient(axios))
      .catch((error) => {
        clientPromise = null
        throw error
      })
  }
  return clientPromise
}

export default async function request(config) {
  const service = await loadRequestClient()
  return service(config)
}
