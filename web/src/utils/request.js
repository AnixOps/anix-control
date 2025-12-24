import axios from 'axios'
import { useUserStore } from '@/stores/user'

const service = axios.create({
  baseURL: '/api/v1',
  timeout: 5000
})

// Request interceptor
service.interceptors.request.use(
  config => {
    const userStore = useUserStore()
    if (userStore.token) {
      config.headers['Authorization'] = userStore.token
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
    return Promise.reject(error)
  }
)

export default service
