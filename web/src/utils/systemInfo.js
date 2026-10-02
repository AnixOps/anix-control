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

// aboutRows: the version and build details shown by the account menu's 关于
// dialog and by 系统设置 → 关于, so both always list the same rows.
// `frontend` carries the web build's VITE_APP_BUILD_CODE / _TIME.
export function aboutRows(t, info, frontend = {}) {
  const data = info || {}
  const frontendBuildCode = frontend.buildCode || ''
  const frontendBuildTime = frontend.buildTime || ''
  const buildCode = data.build_code || frontendBuildCode
  const out = []
  const version = formatVersion(data.version || '', buildCode)
  if (version) out.push({ key: 'version', label: t('shell.about.version'), value: version })
  if (data.build_time) out.push({ key: 'backend-build', label: t('shell.about.backendBuild'), value: data.build_time })
  if (data.commit && data.commit !== 'unknown') out.push({ key: 'commit', label: t('shell.about.commit'), value: data.commit })
  if (frontendBuildCode) out.push({ key: 'frontend-build', label: t('shell.about.frontendBuild'), value: frontendBuildCode })
  if (frontendBuildTime) out.push({ key: 'frontend-time', label: t('shell.about.frontendTime'), value: frontendBuildTime })
  return out
}

export const FRONTEND_BUILD = Object.freeze({
  buildCode: import.meta.env.VITE_APP_BUILD_CODE || '',
  buildTime: import.meta.env.VITE_APP_BUILD_TIME || ''
})
