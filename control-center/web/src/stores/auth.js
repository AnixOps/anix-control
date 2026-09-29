import { defineStore } from 'pinia'
import { ref, computed } from 'vue'
import api, { kernelAuthApi } from '@/api'

export const useAuthStore = defineStore('auth', () => {
  const token = ref(localStorage.getItem('token') || null)
  const user = ref(JSON.parse(localStorage.getItem('user') || 'null'))
  const kernelToken = ref(sessionStorage.getItem('kernel_token') || null)
  const kernelError = ref('')

  const isAuthenticated = computed(() => !!token.value)
  const isAdmin = computed(() => user.value?.role === 'admin')
  const isKernelAuthenticated = computed(() => !!kernelToken.value)

  function disconnectKernel(reason = '') {
    kernelToken.value = null
    kernelError.value = reason
    sessionStorage.removeItem('kernel_token')
  }

  async function connectKernel({ email, password, mfaCode = '', mfaMethod = '' }) {
    disconnectKernel()
    try {
      const response = await kernelAuthApi.login({ email, password, mfaCode, mfaMethod })
      if (response?.code !== 0) throw new Error(response?.msg || 'Control sign in failed')
      const data = response.data || {}
      if (data.mfa_enrollment_required || data.mfa_setup_required) {
        return { enrollmentRequired: true, methods: data.methods || data.mfa_methods || [] }
      }
      if (data.mfa_required) {
        return { mfaRequired: true, methods: data.methods || data.mfa_methods || [] }
      }
      if (!data.token) throw new Error('Control did not return a session token')
      if (data.is_admin !== true) throw new Error('Control administrator access is required')
      kernelToken.value = data.token
      sessionStorage.setItem('kernel_token', data.token)
      return { connected: true }
    } catch (error) {
      kernelError.value = error?.response?.data?.msg || error?.response?.data?.message || error?.message || 'Control sign in failed'
      return { connected: false }
    }
  }

  async function login(email, password) {
    try {
      const response = await api.post('/auth/login', { email, password })

      // Handle Workers API response format: { success: true, data: { access_token, user } }
      const data = response.data.data || response.data
      disconnectKernel()
      token.value = data.access_token
      user.value = {
        id: data.user?.id || '1',
        email: data.user?.email || email,
        role: data.user?.role || 'admin'
      }

      localStorage.setItem('token', token.value)
      localStorage.setItem('user', JSON.stringify(user.value))

      return { success: true }
    } catch (error) {
      return {
        success: false,
        error: error.response?.data?.error || 'Login failed'
      }
    }
  }

  function logout() {
    disconnectKernel()
    token.value = null
    user.value = null
    localStorage.removeItem('token')
    localStorage.removeItem('user')
  }

  async function refreshToken() {
    try {
      const response = await api.post('/auth/refresh')
      const data = response.data.data || response.data
      token.value = data.access_token
      localStorage.setItem('token', token.value)
      return true
    } catch {
      logout()
      return false
    }
  }

  return {
    token,
    user,
    kernelToken,
    kernelError,
    isAuthenticated,
    isAdmin,
    isKernelAuthenticated,
    login,
    logout,
    refreshToken,
    connectKernel,
    disconnectKernel
  }
})
