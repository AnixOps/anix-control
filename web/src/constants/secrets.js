// MASKED_SECRET is how the administrator API shows a secret it does not
// return in clear: node protocol secrets, raw configuration secrets, forward
// node API tokens, registration keys and sensitive system configuration
// values. Sending it back in a write keeps the stored secret.
export const MASKED_SECRET = '********'

export function isMaskedSecret(value) {
  return value === MASKED_SECRET
}
