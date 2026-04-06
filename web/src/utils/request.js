import axios from 'axios'
import { useUserStore } from '@/stores/user'

const service = axios.create({
  baseURL: '/api/v2',
  timeout: 5000
})

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
    return response.data
  },
  error => {
    console.error('Request error:', error)
    if (error?.response?.status === 401 && !isAuthEndpoint(error.config)) {
      clearAuthState()
      redirectToLogin()
    }
    return Promise.reject(error)
  }
)

export default service
