import { defineStore } from 'pinia'
import { ref, computed } from 'vue'

function isMockLoginEnabled() {
  return import.meta.env.DEV && import.meta.env.VITE_ENABLE_MOCK_LOGIN === 'true'
}

function readStoredUserInfo() {
  try {
    return JSON.parse(localStorage.getItem('userInfo') || '{}')
  } catch {
    localStorage.removeItem('userInfo')
    return {}
  }
}

function normalizeToken(value) {
  const token = typeof value === 'string' ? value.trim() : ''
  if (!token) {
    return ''
  }
  const normalized = token.replace(/^Bearer\s+/i, '').trim()
  if (normalized.startsWith('mock-token-') && !isMockLoginEnabled()) {
    localStorage.removeItem('token')
    localStorage.removeItem('userInfo')
    return ''
  }
  return normalized
}

function normalizeAdminFlag(value) {
  return value === true || value === 1 || value === '1'
}

function normalizeUserInfo(value) {
  const user = value && typeof value === 'object' ? { ...value } : {}
  if ('is_admin' in user) {
    user.is_admin = normalizeAdminFlag(user.is_admin)
  } else if ('isAdmin' in user) {
    user.is_admin = normalizeAdminFlag(user.isAdmin)
  }
  return user
}

export const useUserStore = defineStore('user', () => {
  const token = ref(normalizeToken(localStorage.getItem('token') || ''))
  const userInfo = ref(normalizeUserInfo(readStoredUserInfo()))

  if (!token.value && Object.keys(userInfo.value).length > 0) {
    userInfo.value = {}
    localStorage.removeItem('userInfo')
  }

  const isLoggedIn = computed(() => !!token.value)
  const isAdmin = computed(() => normalizeAdminFlag(userInfo.value.is_admin))

  function login(newToken, user) {
    token.value = normalizeToken(newToken)
    userInfo.value = normalizeUserInfo(user)

    if (token.value) {
      localStorage.setItem('token', token.value)
    } else {
      localStorage.removeItem('token')
    }
    localStorage.setItem('userInfo', JSON.stringify(userInfo.value))
  }

  function logout() {
    token.value = ''
    userInfo.value = {}
    localStorage.removeItem('token')
    localStorage.removeItem('userInfo')
  }

  async function getUserInfo() {
    try {
      const { getProfile } = await import('@/api/user')
      const res = await getProfile()
      if (res.data) {
        userInfo.value = normalizeUserInfo(res.data)
        localStorage.setItem('userInfo', JSON.stringify(userInfo.value))
      }
    } catch (e) {
      console.error('Failed to fetch user info:', e)
    }
  }

  return {
    token,
    userInfo,
    isLoggedIn,
    isAdmin,
    login,
    logout,
    getUserInfo
  }
})
