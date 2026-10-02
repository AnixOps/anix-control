// readSystemInfo accepts the legacy body and the panel envelope
// ({ code, msg, data }) of GET /api/v2/admin/system/info.
export function readSystemInfo(res) {
  if (!res || typeof res !== 'object') return {}
  const payload = Object.prototype.hasOwnProperty.call(res, 'code') ? res.data : (res.data ?? res)
  return payload && typeof payload === 'object' ? payload : {}
}

// formatVersion: "v2.1.0 #202607090001"; a version that already carries a
// build code is shown as it is.
export function formatVersion(version, buildCode) {
  if (!version) return ''
  if (version.includes('#')) return `v${version}`
  return buildCode ? `v${version} #${buildCode}` : `v${version}`
}
