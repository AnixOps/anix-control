export const LEGACY_DEPLOYMENT_TABS = new Set([
  'assignments',
  'scopes',
  'topologies',
  'operations',
])

export function resolveLegacyControlRedirect(to) {
  const query = { ...(to?.query || {}) }
  const tab = typeof query.tab === 'string' ? query.tab : ''
  delete query.tab

  return {
    path: LEGACY_DEPLOYMENT_TABS.has(tab) ? '/admin/deployments' : '/admin/plugins',
    query,
    hash: to?.hash || '',
  }
}
