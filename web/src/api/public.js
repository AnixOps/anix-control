// The public kernel configuration (GET /api/v4/public/config): the product
// edition and the registration settings, readable before sign-in. It uses
// its own client so a failure never triggers the v2 sign-out handling.
// axios loads on first use (see utils/request.js).
let client = null

export async function getPublicConfig() {
  if (!client) {
    const { default: axios } = await import('axios')
    client = axios.create({
      baseURL: '/api/v4',
      timeout: 5000
    })
  }
  const response = await client.get('/public/config')
  return response?.data?.data ?? response?.data ?? {}
}
