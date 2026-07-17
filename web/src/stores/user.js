import { defineStore } from 'pinia'
import { ref, computed } from 'vue'
import { getProfile } from '@/api/user'

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

function normalizePermissionMode(value) {
  return value === 'legacy' || value === 'mixed' || value === 'authoritative' ? value : ''
}

function isPluginID(value) {
  return /^[a-z0-9](?:[a-z0-9._-]{0,118}[a-z0-9])?$/.test(value) && !value.includes('..')
}

function normalizeRestrictedPlugins(value) {
  if (Array.isArray(value)) {
    return [...new Set(value
      .filter(item => typeof item === 'string')
      .map(item => item.trim())
      .filter(isPluginID))]
  }
  if (value && typeof value === 'object') {
    return Object.entries(value)
      .filter(([, enabled]) => enabled === true || enabled === 1 || enabled === '1')
      .map(([pluginID]) => pluginID.trim())
      .filter(isPluginID)
  }
  return null
}

function normalizeUserInfo(value) {
  const user = value && typeof value === 'object' ? { ...value } : {}
  if ('is_admin' in user) {
    user.is_admin = normalizeAdminFlag(user.is_admin)
  } else if ('isAdmin' in user) {
    user.is_admin = normalizeAdminFlag(user.isAdmin)
  }
  const declaredPermissionMode = normalizePermissionMode(user.permission_mode ?? user.permissionMode)
  if (declaredPermissionMode) {
    user.permission_mode = declaredPermissionMode
  } else {
    delete user.permission_mode
  }
  if (Object.prototype.hasOwnProperty.call(user, 'restricted_plugins') || Object.prototype.hasOwnProperty.call(user, 'restrictedPlugins')) {
    user.restricted_plugins = normalizeRestrictedPlugins(user.restricted_plugins ?? user.restrictedPlugins)
  }
  delete user.permissionMode
  delete user.restrictedPlugins
  return user
}

function normalizePermissionList(value) {
  if (Array.isArray(value)) {
    return [...new Set(value.filter(item => typeof item === 'string').map(item => item.trim()).filter(Boolean))]
  }
  if (value && typeof value === 'object') {
    return Object.entries(value)
      .filter(([, enabled]) => enabled === true || enabled === 1 || enabled === '1')
      .map(([permission]) => permission.trim())
      .filter(Boolean)
  }
  return null
}

function hasOwnPermissions(value) {
  return Object.prototype.hasOwnProperty.call(value, 'permissions')
}

function hasOwnPermissionMode(value) {
  return Object.prototype.hasOwnProperty.call(value, 'permission_mode')
}

function hasOwnRestrictedPlugins(value) {
  return Object.prototype.hasOwnProperty.call(value, 'restricted_plugins')
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
  const permissions = computed(() => normalizePermissionList(userInfo.value.permissions))
  const restrictedPlugins = computed(() => normalizeRestrictedPlugins(userInfo.value.restricted_plugins))
  const permissionMode = computed(() => {
    const declared = normalizePermissionMode(userInfo.value.permission_mode)
    if (declared) {
      return declared
    }
    if (permissions.value !== null) {
      return 'authoritative'
    }
    return 'missing'
  })

  function hasPermission(permission) {
    const required = typeof permission === 'string' ? permission.trim() : ''
    if (!required) {
      return true
    }
    const mode = permissionMode.value
    if (mode === 'legacy' && isAdmin.value) {
      return true
    }
    const restricted = restrictedPlugins.value?.some(pluginID => required.startsWith(`${pluginID}.`)) === true
    if (
      mode === 'mixed' &&
      isAdmin.value &&
      restrictedPlugins.value !== null &&
      !restricted
    ) {
      return true
    }
    const granted = permissions.value
    if (granted === null) {
      return false
    }
    if (granted.includes(required)) {
      return true
    }
    return granted.some(permission => permission.endsWith('.*') && required.startsWith(permission.slice(0, -1)))
  }

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
      const res = await getProfile()
      if (res.data) {
        const profile = normalizeUserInfo(res.data)
        const profileHasPermissionMetadata = hasOwnPermissions(profile) || hasOwnPermissionMode(profile) || hasOwnRestrictedPlugins(profile)
        if (!profileHasPermissionMetadata) {
          if (hasOwnPermissions(userInfo.value)) {
            profile.permissions = userInfo.value.permissions
          }
          if (hasOwnPermissionMode(userInfo.value)) {
            profile.permission_mode = userInfo.value.permission_mode
          }
          if (hasOwnRestrictedPlugins(userInfo.value)) {
            profile.restricted_plugins = userInfo.value.restricted_plugins
          }
        }
        userInfo.value = profile
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
    permissions,
    restrictedPlugins,
    permissionMode,
    hasPermission,
    login,
    logout,
    getUserInfo
  }
})
