import axios from 'axios'

// The public kernel configuration (GET /api/v4/public/config): the product
// edition and the registration settings, readable before sign-in. It uses
// its own client so a failure never triggers the v2 sign-out handling.
const client = axios.create({
  baseURL: '/api/v4',
  timeout: 5000
})

export async function getPublicConfig() {
  const response = await client.get('/public/config')
  return response?.data?.data ?? response?.data ?? {}
}
