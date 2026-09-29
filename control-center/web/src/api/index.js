import axios from 'axios'
import { useAuthStore } from '@/stores/auth'

// Use environment variable or default to Workers API
const baseURL = import.meta.env.VITE_API_URL || 'https://api.anixops.com/api/v1'

const api = axios.create({
  baseURL,
  timeout: 30000,
  headers: {
    'Content-Type': 'application/json'
  }
})

// The Control kernel has its own origin and JWT issuer. Its default path uses
// the Vite /api proxy in development and a same-origin route in production.
function kernelBaseURL(configuredURL = import.meta.env.VITE_KERNEL_API_URL || '/api/v3') {
  const configured = configuredURL.replace(/\/+$/, '')
  if (/\/api\/v\d+$/.test(configured)) return configured.replace(/\/api\/v\d+$/, '/api/v3')
  return `${configured}/api/v3`
}

const kernelAuthClient = axios.create({
  baseURL: kernelBaseURL().replace(/\/api\/v3$/, '/api/v2'),
  timeout: 30000,
  headers: { 'Content-Type': 'application/json' }
})

const kernelApiClient = axios.create({
  baseURL: kernelBaseURL(),
  timeout: 30000,
  headers: { 'Content-Type': 'application/json' }
})

kernelApiClient.interceptors.request.use((config) => {
  const token = sessionStorage.getItem('kernel_token')
  if (token) config.headers.Authorization = `Bearer ${token}`
  return config
})

kernelApiClient.interceptors.response.use(
  (response) => response,
  async (error) => {
    if (error.response?.status === 401) {
      const requestToken = String(error.config?.headers?.Authorization || '').replace(/^Bearer\s+/, '')
      if (!requestToken || requestToken === sessionStorage.getItem('kernel_token')) {
        const authStore = useAuthStore()
        authStore.disconnectKernel('Control session expired. Connect again.')
      }
    }
    return Promise.reject(error)
  }
)

// Request interceptor
api.interceptors.request.use(
  (config) => {
    const token = localStorage.getItem('token')
    if (token) {
      config.headers.Authorization = `Bearer ${token}`
    }
    return config
  },
  (error) => Promise.reject(error)
)

// Response interceptor
api.interceptors.response.use(
  (response) => response,
  async (error) => {
    if (error.response?.status === 401) {
      const authStore = useAuthStore()
      authStore.logout()
      window.location.href = '/login'
    }
    return Promise.reject(error)
  }
)

export default api

export const kernelAuthApi = {
  login: async ({ email, password, mfaCode, mfaMethod }) => {
    const response = await kernelAuthClient.post('/login', {
      email,
      password,
      ...(mfaCode ? { mfa_code: mfaCode } : {}),
      ...(mfaMethod ? { mfa_method: mfaMethod } : {})
    })
    return response.data
  }
}

// API methods
export const nodesApi = {
  list: () => api.get('/nodes'),
  get: (id) => api.get(`/nodes/${id}`),
  create: (data) => api.post('/nodes', data),
  update: (id, data) => api.put(`/nodes/${id}`, data),
  delete: (id) => api.delete(`/nodes/${id}`),
  stats: (id) => api.get(`/nodes/${id}/stats`)
}

export const agentsApi = {
  list: () => api.get('/agents'),
  connect: (data) => api.post('/agents/connect', data),
  disconnect: () => api.post('/agents/disconnect'),
  exec: (data) => api.post('/agents/exec', data),
  info: (id) => api.get(`/agents/${id}/info`)
}

export const usersApi = {
  list: (params) => api.get('/users', { params }),
  get: (id) => api.get(`/users/${id}`),
  create: (data) => api.post('/users', data),
  update: (id, data) => api.put(`/users/${id}`, data),
  delete: (id) => api.delete(`/users/${id}`),
  ban: (id) => api.post(`/users/${id}/ban`),
  unban: (id) => api.post(`/users/${id}/unban`)
}

export const playbooksApi = {
  list: () => api.get('/playbooks'),
  get: (name) => api.get(`/playbooks/${name}`),
  run: (data) => api.post('/playbooks/run', data),
  validate: (data) => api.post('/playbooks/validate', data)
}

export const dashboardApi = {
  get: () => api.get('/dashboard'),
  stats: () => api.get('/dashboard/stats')
}

export const pluginsApi = {
  list: () => api.get('/plugins'),
  get: (name) => api.get(`/plugins/${name}`),
  execute: (name, action, params) => api.post(`/plugins/${name}/execute`, { action, params }),
  status: (name) => api.get(`/plugins/${name}/status`),
  start: (name) => api.post(`/admin/plugins/${name}/start`),
  stop: (name) => api.post(`/admin/plugins/${name}/stop`)
}

function unwrapKernel(response) {
  return response?.data?.data ?? response?.data ?? response
}

function withKernelOperationHeaders(response, result) {
  if (!result || typeof result !== 'object') return result
  return {
    ...result,
    operation_id: response.headers?.['x-anixops-operation-id'] || '',
    operation_chain: response.headers?.['x-anixops-operation-chain'] || ''
  }
}

export const kernelPluginsApi = {
  list: async () => unwrapKernel(await kernelApiClient.get('/plugins')),
  releases: async (pluginId) => unwrapKernel(await kernelApiClient.get('/plugin-releases', {
    params: pluginId ? { plugin_id: pluginId } : undefined
  })),
  installations: async () => unwrapKernel(await kernelApiClient.get('/plugin-installations')),
  upsertInstallation: async (installation) => {
    const response = await kernelApiClient.put('/plugin-installations', installation)
    return withKernelOperationHeaders(response, unwrapKernel(response))
  },
  action: async (installationId, action, options = {}) => {
    const data = { action, idempotency_key: options.idempotencyKey }
    if (options.targetVersion) data.target_version = options.targetVersion
    const response = await kernelApiClient.post(`/plugin-installations/${installationId}/actions`, data)
    return withKernelOperationHeaders(response, unwrapKernel(response))
  },
  operations: async () => unwrapKernel(await kernelApiClient.get('/operations')),
  getConfig: async (installationId) => unwrapKernel(await kernelApiClient.get(`/plugin-installations/${installationId}/config`)),
  updateConfig: async (installationId, config, expectedRevision) => {
    const response = await kernelApiClient.put(
      `/plugin-installations/${installationId}/config`,
      { config, ...(expectedRevision === undefined ? {} : { expected_revision: expectedRevision }) }
    )
    return withKernelOperationHeaders(response, unwrapKernel(response))
  },
  extensions: async () => unwrapKernel(await kernelApiClient.get('/extensions'))
}

export { kernelApiClient, kernelAuthClient, kernelBaseURL }

export const logsApi = {
  list: (params) => api.get('/logs', { params })
}

export const settingsApi = {
  get: () => api.get('/settings'),
  update: (data) => api.put('/settings', data)
}
