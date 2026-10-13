export const LEGACY_DEPLOYMENT_TABS = new Set([
  'assignments',
  'scopes',
  'topologies',
  'operations',
])

// The built-in packages' status (it replaced 插件中心, /admin/plugins).
export const BUILTIN_PACKAGES_PATH = '/admin/system/packages'

export function resolveLegacyControlRedirect(to) {
  const query = { ...(to?.query || {}) }
  const tab = typeof query.tab === 'string' ? query.tab : ''
  delete query.tab

  return {
    path: LEGACY_DEPLOYMENT_TABS.has(tab) ? '/admin/deployments' : BUILTIN_PACKAGES_PATH,
    query,
    hash: to?.hash || '',
  }
}
