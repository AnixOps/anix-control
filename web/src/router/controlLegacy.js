// The built-in packages' status (it replaced 插件中心, /admin/plugins).
export const BUILTIN_PACKAGES_PATH = '/admin/system/packages'

export function resolveLegacyControlRedirect(to) {
  const query = { ...(to?.query || {}) }
  delete query.tab

  return {
    path: BUILTIN_PACKAGES_PATH,
    query,
    hash: to?.hash || '',
  }
}
