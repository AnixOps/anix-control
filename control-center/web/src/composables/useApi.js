import api from '@/api'

export function useApi() {
  const get = (url, params) => api.get(url, { params })
  const post = (url, data) => api.post(url, data)
  const put = (url, data) => api.put(url, data)
  const del = (url) => api.delete(url)

  return { get, post, put, del }
}