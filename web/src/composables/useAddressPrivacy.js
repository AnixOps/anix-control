import { ref } from 'vue'

// Server IP addresses are masked on the admin pages by default, so a screen
// share or a screenshot does not show them. The administrator can reveal them
// from the account menu or the command palette. The choice lasts for this
// browser tab (sessionStorage): a new tab or window starts masked again.
// Only what the pages display is masked: forms, copy buttons and the API keep
// the real values.

const STORAGE_KEY = 'anixops-reveal-addresses'

function readStored() {
  try {
    return sessionStorage.getItem(STORAGE_KEY) === '1'
  } catch {
    return false
  }
}

const revealed = ref(readStored())

function setRevealed(value) {
  revealed.value = Boolean(value)
  try {
    if (revealed.value) sessionStorage.setItem(STORAGE_KEY, '1')
    else sessionStorage.removeItem(STORAGE_KEY)
  } catch {
    // Storage blocked: the choice lasts until the page reloads.
  }
}

// An IPv4 address not glued to other digits or dots of a longer token.
const IPV4 = /(?<![\d.])(\d{1,3})\.(\d{1,3})\.(\d{1,3})\.(\d{1,3})(?![\d]|\.\d)/g
// A run of hex digits and colons that may be an IPv6 address; ipv6Like
// decides. Times such as 12:30:45 and host:port pairs do not qualify.
const IPV6_CANDIDATE = /(?<![\w.:])[0-9A-Fa-f:]*:[0-9A-Fa-f:]*(?![\w:])/g

function ipv6Like(token) {
  if (!/[0-9A-Fa-f]/.test(token)) return false
  const groups = token.split(':')
  if (groups.some(group => group.length > 4)) return false
  const compressed = token.includes('::')
  if (compressed) return !token.includes(':::') && token.indexOf('::') === token.lastIndexOf('::') && groups.length <= 8
  return groups.length === 8 && groups.every(group => group.length > 0)
}

function validOctets(parts) {
  return parts.every(part => Number(part) <= 255)
}

// maskAddresses replaces the middle of every IP address in text:
// 203.0.113.7 → 203.*.*.7, 2001:db8::1 → 2001:****. Everything else in the
// text (ports, names, separators) stays as it is.
export function maskAddresses(text) {
  if (text === null || text === undefined) return text
  const value = String(text)
  return value
    .replace(IPV4, (match, a, b, c, d) => (validOctets([a, b, c, d]) ? `${a}.*.*.${d}` : match))
    .replace(IPV6_CANDIDATE, (match) => {
      if (!ipv6Like(match)) return match
      if (match.startsWith('::')) return '::****'
      const first = match.split(':').find(group => group !== '') || ''
      return `${first}:****`
    })
}

// useAddressPrivacy: `display(text)` is what a page shows for text that may
// contain server addresses. It reads the shared state, so a template that
// calls it re-renders when the administrator reveals or hides the addresses.
export function useAddressPrivacy() {
  return {
    revealed,
    setRevealed,
    toggle: () => setRevealed(!revealed.value),
    display: text => (revealed.value ? text : maskAddresses(text))
  }
}
