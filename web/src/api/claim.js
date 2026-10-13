// The first administrator's claim link (docs/design/single-binary, N10):
// GET /api/v5/claim/status and POST /api/v5/claim, both public. They use
// their own client, like api/public.js, so a refusal never triggers the v2
// sign-out handling; axios loads on first use (see utils/request.js).
//
// The claim token travels only in the POST body: the link carries it in
// the URL fragment, which the browser never sends.
let client = null

async function claimClient() {
  if (!client) {
    const { default: axios } = await import('axios')
    client = axios.create({
      baseURL: '/api/v5',
      timeout: 15000
    })
  }
  return client
}

// getClaimStatus is true while the panel has no administrator, false
// otherwise and on any answer that does not say so.
export async function getClaimStatus() {
  const response = await (await claimClient()).get('/claim/status')
  return response?.data?.data?.claimable === true
}

// submitClaim creates the first administrator: { user_id, email, next }.
// A refusal rejects with the axios error (status and error.code).
export async function submitClaim({ token, email, password }) {
  const response = await (await claimClient()).post('/claim', { token, email, password })
  return response?.data?.data ?? {}
}
