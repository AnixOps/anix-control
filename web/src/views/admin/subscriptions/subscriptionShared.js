// Shared helpers of the 订阅分组 pages (UI U7): response readers for the
// subscription endpoints (legacy bodies and the panel envelope), the
// subscription formats, and the template link builder used by 复制链接 and
// the 复制合并订阅 fallback. Moved unchanged from the single Subscriptions.vue.

export const SUBSCRIPTION_FORMATS = Object.freeze([
  'auto', 'v2ray', 'clash', 'stash', 'egern', 'surge', 'loon', 'shadowrocket', 'quantumultx', 'json', 'base64json'
])

// Formats the preview endpoint renders (everything but the UA-detected one).
export const PREVIEW_FORMATS = Object.freeze(SUBSCRIPTION_FORMATS.filter(format => format !== 'auto'))

export const TEMPLATE_PROTOCOLS = Object.freeze(['vless', 'vmess', 'trojan', 'shadowsocks', 'hysteria2', 'tuic'])
export const TEMPLATE_TRANSPORTS = Object.freeze(['tcp', 'ws', 'grpc', 'h2', 'quic'])
export const TLS_FINGERPRINTS = Object.freeze(['chrome', 'firefox', 'safari', 'edge', 'random'])

export function readSubscriptionEnvelopeError(res) {
  const candidates = [res, res?.data]
  for (const candidate of candidates) {
    if (!candidate || typeof candidate !== 'object') continue
    if (!Object.prototype.hasOwnProperty.call(candidate, 'code')) continue
    if (Number(candidate.code) === 0) return null
    return candidate.msg || candidate.message || candidate.error || ''
  }
  return null
}

export function ensureSubscriptionSuccess(res, fallbackMessage) {
  const message = readSubscriptionEnvelopeError(res)
  if (message !== null) {
    throw new Error(message || fallbackMessage)
  }
  return res
}

export function readSubscriptionPayload(res, fallbackMessage) {
  const payload = ensureSubscriptionSuccess(res, fallbackMessage)
  if (!payload || typeof payload !== 'object') return null
  if (Object.prototype.hasOwnProperty.call(payload, 'code')) return payload.data ?? null
  if (payload.data && typeof payload.data === 'object' && Object.prototype.hasOwnProperty.call(payload.data, 'code')) {
    return payload.data.data ?? null
  }
  if (payload.data && typeof payload.data === 'object' && Object.prototype.hasOwnProperty.call(payload.data, 'data')) {
    return payload.data.data ?? null
  }
  return payload.data ?? payload
}

export function readSubscriptionList(res, fallbackMessage) {
  const payload = readSubscriptionPayload(res, fallbackMessage)
  return Array.isArray(payload) ? payload : []
}

export function subscriptionErrorText(error) {
  return error?.response?.data?.msg || error?.response?.data?.error || error?.message || ''
}

export function subscriptionFileExt(format) {
  if (format === 'auto' || format === 'ua') return 'txt'
  if (format === 'clash' || format === 'stash' || format === 'egern') return 'yaml'
  if (format === 'json' || format === 'sing-box') return 'json'
  if (format === 'surge') return 'conf'
  return 'txt'
}

// The administrator's own subscription link for one group and format.
export function groupSubscriptionUrl({ origin, token, groupId, format }) {
  if (!token) return ''
  const groupQuery = `groups=${groupId || ''}`
  if (!format || format === 'auto' || format === 'ua') {
    return `${origin}/s/${token}?${groupQuery}`
  }
  return `${origin}/s/${token}?type=${format}&${groupQuery}`
}

export function downloadText(content, filename) {
  const blob = new Blob([content], { type: 'text/plain' })
  const url = URL.createObjectURL(blob)
  const a = document.createElement('a')
  a.href = url
  a.download = filename
  a.click()
  URL.revokeObjectURL(url)
}

// generateNodeLink: a share link for one template, with a placeholder UUID
// (the real one is the user's). Used by 复制链接 and the fallback merge.
export function generateNodeLink(template) {
  const uuid = 'test-uuid-1234-5678-abcdef'

  if (template.type === 'vless') {
    const params = ['encryption=none']

    if (template.tls === 2) {
      params.push('security=reality')
      if (template.reality_public_key) params.push(`pbk=${template.reality_public_key}`)
      if (template.reality_short_id) params.push(`sid=${template.reality_short_id}`)
      if (template.server_name) params.push(`sni=${template.server_name}`)
      if (template.tls_fingerprint) params.push(`fp=${template.tls_fingerprint}`)
    } else if (template.tls === 1) {
      params.push('security=tls')
      if (template.server_name) params.push(`sni=${template.server_name}`)
    } else {
      params.push('security=none')
    }

    params.push(`type=${template.transport || 'tcp'}`)

    if (template.protocol_settings) {
      try {
        const settings = JSON.parse(template.protocol_settings)
        if (settings.flow) params.push(`flow=${settings.flow}`)
      } catch {
        // Unreadable settings: no flow.
      }
    }

    if (template.transport === 'tcp') {
      params.push('headerType=none')
    }

    return `vless://${uuid}@${template.server}:${template.port}?${params.join('&')}#${encodeURIComponent(template.name)}`
  }

  if (template.type === 'vmess') {
    const config = {
      v: '2',
      ps: template.name,
      add: template.server,
      port: template.port,
      id: uuid,
      aid: 0,
      scy: 'auto',
      net: template.transport || 'tcp',
      type: 'none',
      tls: template.tls === 1 ? 'tls' : ''
    }
    return `vmess://${btoa(JSON.stringify(config))}`
  }

  if (template.type === 'trojan') {
    const params = []
    if (template.server_name) params.push(`sni=${template.server_name}`)
    params.push(`type=${template.transport || 'tcp'}`)
    const paramStr = params.length ? `?${params.join('&')}` : ''
    return `trojan://${uuid}@${template.server}:${template.port}${paramStr}#${encodeURIComponent(template.name)}`
  }

  if (template.type === 'hysteria2' || template.type === 'hy2') {
    const params = []
    if (template.server_name) params.push(`sni=${template.server_name}`)
    const paramStr = params.length ? `?${params.join('&')}` : ''
    return `hy2://${uuid}@${template.server}:${template.port}${paramStr}#${encodeURIComponent(template.name)}`
  }

  return ''
}
