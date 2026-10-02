// Clients the subscription page offers, each tied to a format the kernel
// serves (internal/model/subscription.go SubscriptionFormat; the kernel also
// detects these clients by User-Agent, internal/handler/subscribe.go).
// `importUrl` builds the client's own import link where the client has a
// documented URL scheme; the others copy their format's link instead.
// Product names are not translated.

function encode(value) {
  return encodeURIComponent(value)
}

function base64(value) {
  // Subscription links are ASCII; btoa is enough.
  return typeof btoa === 'function' ? btoa(value) : ''
}

export const SUBSCRIPTION_CLIENTS = Object.freeze([
  {
    id: 'clash',
    name: 'Clash Verge',
    glyph: 'C',
    platforms: 'Windows · macOS · Linux',
    format: 'clash',
    importUrl: (url, name) => `clash://install-config?url=${encode(url)}&name=${encode(name)}`
  },
  {
    id: 'shadowrocket',
    name: 'Shadowrocket',
    glyph: 'S',
    platforms: 'iOS · iPadOS',
    format: 'shadowrocket',
    importUrl: (url, name) => `shadowrocket://add/sub://${base64(url)}?remark=${encode(name)}`
  },
  {
    id: 'sing-box',
    name: 'sing-box',
    glyph: 'sb',
    platformsKey: 'portal.subscribe.clients.allPlatforms',
    format: 'sing-box',
    importUrl: (url, name) => `sing-box://import-remote-profile?url=${encode(url)}#${encode(name)}`
  },
  {
    id: 'v2rayn',
    name: 'v2rayN',
    glyph: 'v2',
    platforms: 'Windows',
    format: 'v2ray',
    importUrl: null
  },
  {
    id: 'stash',
    name: 'Stash',
    glyph: 'St',
    platforms: 'iOS · macOS',
    format: 'stash',
    importUrl: (url, name) => `stash://install-config?url=${encode(url)}&name=${encode(name)}`
  },
  {
    id: 'surge',
    name: 'Surge',
    glyph: 'Su',
    platforms: 'iOS · macOS',
    format: 'surge',
    importUrl: url => `surge:///install-config?url=${encode(url)}`
  },
  {
    id: 'quantumultx',
    name: 'Quantumult X',
    glyph: 'QX',
    platforms: 'iOS · iPadOS',
    format: 'quantumultx',
    importUrl: (url, name) => `quantumult-x:///add-resource?remote-resource=${encode(JSON.stringify({ server_remote: [`${url}, tag=${name}`] }))}`
  },
  {
    id: 'loon',
    name: 'Loon',
    glyph: 'L',
    platforms: 'iOS · iPadOS',
    format: 'loon',
    importUrl: url => `loon://import?sub=${encode(url)}`
  }
])

// Every format the kernel serves, for 「其他格式」; `ext` names the file a
// download gets. The keys of portal.subscribe.formats.names.
export const SUBSCRIPTION_FORMATS = Object.freeze([
  { value: 'auto', nameKey: 'auto', ext: 'txt' },
  { value: 'v2ray', nameKey: 'v2ray', ext: 'txt' },
  { value: 'clash', nameKey: 'clash', ext: 'yaml' },
  { value: 'stash', nameKey: 'stash', ext: 'yaml' },
  { value: 'egern', nameKey: 'egern', ext: 'yaml' },
  { value: 'surge', nameKey: 'surge', ext: 'conf' },
  { value: 'loon', nameKey: 'loon', ext: 'conf' },
  { value: 'shadowrocket', nameKey: 'shadowrocket', ext: 'txt' },
  { value: 'quantumultx', nameKey: 'quantumultx', ext: 'conf' },
  { value: 'sing-box', nameKey: 'singBox', ext: 'json' },
  { value: 'wireguard', nameKey: 'wireguard', ext: 'conf' },
  { value: 'json', nameKey: 'json', ext: 'json' },
  { value: 'base64json', nameKey: 'base64json', ext: 'txt' }
])
