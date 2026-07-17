import { computed, defineComponent, h, onErrorCaptured, onMounted, readonly, ref, shallowRef } from 'vue'
import { getKernelExtensions } from '@/api/kernel'
import { useUserStore } from '@/stores/user'
import { getLocalExtensionModuleLoader } from './registry'

const ADMIN_ROUTE_NAME = 'admin'
const ENVELOPE_VERSION = 'v1'
const REFRESH_INTERVAL_MS = 30_000
const PLUGIN_ID_PATTERN = /^[a-z0-9](?:[a-z0-9._-]{0,118}[a-z0-9])?$/
const VERSION_PATTERN = /^[A-Za-z0-9](?:[A-Za-z0-9._+-]{0,118}[A-Za-z0-9])?$/
const EXPORT_PATTERN = /^(?:default|[A-Za-z_$][A-Za-z0-9_$]*)$/
const SHA256_PATTERN = /^[a-fA-F0-9]{64}$/
const PERMISSION_PATTERN = /^[a-z0-9][a-z0-9._:-]{0,119}$/
const MENU_PARENTS = new Set(['services', 'operations', 'system'])
const EXTENSION_STATES = new Set(['enabled', 'healthy'])
const WEBUI_API_VERSION = 'anixops.webui/v1'

const ICON_LABELS = Object.freeze({
  activity: 'AC',
  box: 'EX',
  gauge: 'GA',
  network: 'NW',
  plug: 'PL',
  server: 'SV',
  settings: 'ST',
  shield: 'SH'
})

function runtimeError(code, message, pluginID = '') {
  return { code, message, plugin_id: pluginID }
}

function runtimeSkip(code, message, pluginID) {
  return { ...runtimeError(code, message, pluginID), status: 'skipped' }
}

function requireString(value, field, maxLength = 160) {
  if (typeof value !== 'string' || value.length === 0 || value.length > maxLength || value !== value.trim() || /[\u0000-\u001f\u007f]/.test(value)) {
    throw new Error(`${field} is invalid`)
  }
  return value
}

function isSafeBundlePath(path) {
  if (typeof path !== 'string' || path.length === 0 || path.length > 240 || path.startsWith('/') || path.includes('\\') || path.includes('?') || path.includes('#') || path.includes('%')) {
    return false
  }
  const segments = path.split('/')
  if (segments[0] !== 'webui' || segments.some(segment => !segment || segment === '.' || segment === '..' || !/^[A-Za-z0-9._-]+$/.test(segment))) {
    return false
  }
  return /\.m?js$/.test(segments.at(-1))
}

function bundleFileName(path) {
  return typeof path === 'string' ? path.split('/').at(-1) : ''
}

function safeURLBase() {
  if (typeof window !== 'undefined' && window.location?.origin) {
    return window.location.origin
  }
  return 'http://localhost'
}

function decodePathSegment(value) {
  try {
    return decodeURIComponent(value)
  } catch {
    return ''
  }
}

function isSafeBundleURL(value, pluginID, version, sha256, bundlePath) {
  if (typeof value !== 'string' || value.length === 0 || value.length > 520 || value.includes('\\') || value.startsWith('//')) {
    return false
  }
  const base = safeURLBase()
  let parsed
  try {
    parsed = new URL(value, base)
  } catch {
    return false
  }
  if (parsed.origin !== base || parsed.search || parsed.hash || !parsed.pathname.startsWith('/api/v3/extensions/')) {
    return false
  }
  const segments = parsed.pathname.split('/').filter(Boolean).map(decodePathSegment)
  return segments.length === 8 &&
    segments[0] === 'api' &&
    segments[1] === 'v3' &&
    segments[2] === 'extensions' &&
    segments[3] === pluginID &&
    segments[4] === version &&
    segments[5] === 'webui' &&
    segments[6] === sha256.toLowerCase() &&
    segments[7] === bundleFileName(bundlePath)
}

function isNamespacedID(pluginID, value) {
  return typeof value === 'string' && value.startsWith(`${pluginID}.`) && PERMISSION_PATTERN.test(value)
}

function isNamespacedRoutePath(pluginID, path) {
  const root = `/admin/extensions/${pluginID}`
  if (path === root) {
    return true
  }
  if (!path.startsWith(`${root}/`) || path.includes('?') || path.includes('#') || path.includes('%') || path.includes('\\')) {
    return false
  }
  return path.slice(root.length + 1).split('/').every(segment => /^[a-z0-9][a-z0-9-]{0,62}$/.test(segment) || /^:[a-z][a-zA-Z0-9_]{0,62}$/.test(segment))
}

function normalizeExtension(raw) {
  if (!raw || typeof raw !== 'object' || Array.isArray(raw)) {
    throw new Error('extension entry must be an object')
  }

  const pluginID = requireString(raw.plugin_id, 'plugin_id', 120)
  if (!PLUGIN_ID_PATTERN.test(pluginID) || pluginID.includes('..')) {
    throw new Error('plugin_id is not a controlled module ID')
  }
  if (raw.publisher !== 'AnixOps') {
    throw new Error('publisher is not trusted')
  }
  if (raw.api_version !== ENVELOPE_VERSION) {
    throw new Error('api_version is unsupported')
  }
  const version = requireString(raw.version, 'version', 120)
  if (!VERSION_PATTERN.test(version)) {
    throw new Error('version is invalid')
  }
  if (!EXTENSION_STATES.has(raw.state)) {
    throw new Error('extension is not enabled')
  }
  if (!Number.isSafeInteger(raw.installation_id) || raw.installation_id <= 0) {
    throw new Error('installation_id is invalid')
  }
  const configSchema = raw.config_schema ?? {}
  if (!configSchema || typeof configSchema !== 'object' || Array.isArray(configSchema)) {
    throw new Error('config_schema must be an object')
  }
  if (!raw.bundle || !isSafeBundlePath(raw.bundle.path) || !SHA256_PATTERN.test(raw.bundle.sha256 || '')) {
    throw new Error('bundle identity is invalid')
  }
  const bundleSHA256 = raw.bundle.sha256.toLowerCase()
  const bundleURL = raw.bundle.url || ''
  if (bundleURL && !isSafeBundleURL(bundleURL, pluginID, version, bundleSHA256, raw.bundle.path)) {
    throw new Error('bundle URL is not a verified same-origin asset')
  }

  if (!Array.isArray(raw.permissions)) {
    throw new Error('permissions must be an array')
  }
  const permissions = new Set()
  for (const permission of raw.permissions) {
    if (!isNamespacedID(pluginID, permission)) {
      throw new Error('permission is outside the plugin namespace')
    }
    permissions.add(permission)
  }

  if (!Array.isArray(raw.routes) || raw.routes.length === 0) {
    throw new Error('routes must be a non-empty array')
  }
  const routeIDs = new Set()
  const routePaths = new Set()
  const routes = raw.routes.map(route => {
    if (!route || typeof route !== 'object' || !isNamespacedID(pluginID, route.id) || routeIDs.has(route.id)) {
      throw new Error('route id is invalid or duplicated')
    }
    if (!isNamespacedRoutePath(pluginID, route.path) || routePaths.has(route.path)) {
      throw new Error('route path is invalid or duplicated')
    }
    const exportName = route.export || 'default'
    if (!EXPORT_PATTERN.test(exportName)) {
      throw new Error('route export is invalid')
    }
    if (!permissions.has(route.permission)) {
      throw new Error('route permission was not declared')
    }
    routeIDs.add(route.id)
    routePaths.add(route.path)
    return {
      id: route.id,
      path: route.path,
      exportName,
      permission: route.permission
    }
  })

  if (!Array.isArray(raw.menus)) {
    throw new Error('menus must be an array')
  }
  const menuIDs = new Set()
  const menus = raw.menus.map(menu => {
    if (!menu || typeof menu !== 'object' || !isNamespacedID(pluginID, menu.id) || menuIDs.has(menu.id)) {
      throw new Error('menu id is invalid or duplicated')
    }
    if (!MENU_PARENTS.has(menu.parent) || !routePaths.has(menu.route) || !permissions.has(menu.permission)) {
      throw new Error('menu target or permission is invalid')
    }
    const order = menu.order ?? 1000
    if (!Number.isSafeInteger(order) || order < -10_000 || order > 10_000) {
      throw new Error('menu order is invalid')
    }
    menuIDs.add(menu.id)
    return {
      id: menu.id,
      parent: menu.parent,
      label: requireString(menu.label, 'menu label', 80),
      icon: ICON_LABELS[menu.icon] || 'EX',
      route: menu.route,
      permission: menu.permission,
      order
    }
  })

  return {
    pluginID,
    pluginName: requireString(raw.plugin_name, 'plugin_name', 160),
    version,
    installationID: raw.installation_id,
    configSchema,
    bundle: {
      path: raw.bundle.path,
      sha256: bundleSHA256,
      url: bundleURL
    },
    permissions: [...permissions],
    routes,
    menus
  }
}

function filterAuthorizedExtension(extension, hasPermission) {
  const grants = new Map(extension.permissions.map(permission => {
    try {
      return [permission, hasPermission(permission) === true]
    } catch {
      return [permission, false]
    }
  }))
  const routes = extension.routes.filter(route => grants.get(route.permission) === true)
  const routePaths = new Set(routes.map(route => route.path))

  return {
    ...extension,
    permissions: extension.permissions.filter(permission => grants.get(permission) === true),
    routes,
    menus: extension.menus.filter(menu => grants.get(menu.permission) === true && routePaths.has(menu.route))
  }
}

function currentUserHasPermission(permission) {
  try {
    return useUserStore().hasPermission(permission) === true
  } catch {
    return false
  }
}

function validateLocalModule(extension, module) {
  if (!module || typeof module !== 'object') {
    throw new Error('local extension module did not load')
  }
  const descriptor = module.anixopsExtension
  if (!descriptor || typeof descriptor !== 'object' || descriptor.pluginId !== extension.pluginID || descriptor.version !== extension.version) {
    throw new Error('local extension module identity does not match the catalog')
  }
  if (!descriptor.bundle || descriptor.bundle.path !== extension.bundle.path) {
    throw new Error('local extension bundle identity does not match the signed catalog')
  }
  if (descriptor.bundle.sha256 !== undefined && (typeof descriptor.bundle.sha256 !== 'string' || descriptor.bundle.sha256.toLowerCase() !== extension.bundle.sha256)) {
    throw new Error('local extension optional bundle digest does not match the signed catalog')
  }
  if (descriptor.webuiApiVersion !== undefined && descriptor.webuiApiVersion !== WEBUI_API_VERSION) {
    throw new Error('local extension WebUI API version is unsupported')
  }
  for (const route of extension.routes) {
    const component = module[route.exportName]
    if ((typeof component !== 'object' || component === null) && typeof component !== 'function') {
      throw new Error(`local extension export ${route.exportName} is missing`)
    }
  }
  return module
}

function isPluginAPIPath(pluginID, value) {
  if (typeof value !== 'string' || value.length === 0 || value.length > 420 || value.includes('\\') || value.includes('%') || value.includes('#')) {
    return false
  }
  let parsed
  try {
    parsed = new URL(value, safeURLBase())
  } catch {
    return false
  }
  const namespace = `/api/v3/plugins/${pluginID}`
  return parsed.origin === safeURLBase() &&
    (parsed.pathname === namespace || parsed.pathname.startsWith(`${namespace}/`)) &&
    parsed.pathname.split('/').every(segment => segment !== '.' && segment !== '..')
}

function extensionHost(extension) {
  async function request(path, options = {}) {
    if (!isPluginAPIPath(extension.pluginID, path)) {
      throw new Error('extension request is outside the plugin API namespace')
    }
    const method = String(options.method || 'GET').toUpperCase()
    if (!['GET', 'POST', 'PUT', 'PATCH', 'DELETE'].includes(method)) {
      throw new Error('extension request method is not allowed')
    }
    const headers = { Accept: 'application/json' }
    const authorization = readAuthorizationHeader()
    if (authorization) {
      headers.Authorization = authorization
    }
    let body
    if (options.body !== undefined) {
      body = typeof options.body === 'string' ? options.body : JSON.stringify(options.body)
      if (new TextEncoder().encode(body).byteLength > 1_048_576) {
        throw new Error('extension request body exceeds 1 MiB')
      }
      headers['Content-Type'] = 'application/json'
    }
    const response = await fetch(path, { method, credentials: 'same-origin', headers, body })
    let payload = null
    try {
      payload = await response.json()
    } catch {
      // The gateway contract is JSON-only; preserve a useful status failure.
    }
    if (!response.ok) {
      throw new Error(payload?.error?.message || `extension request failed with status ${response.status}`)
    }
    return payload?.data ?? payload
  }

  return Object.freeze({
    pluginID: extension.pluginID,
    version: extension.version,
    computed,
    defineComponent,
    h,
    onMounted,
    ref,
    readonly,
    request
  })
}

function extensionRouteComponent(extension, module, route) {
  const exported = module[route.exportName]
  if (module.anixopsExtension?.webuiApiVersion !== WEBUI_API_VERSION) {
    return exported
  }
  const component = exported(extensionHost(extension))
  if ((typeof component !== 'object' || component === null) && typeof component !== 'function') {
    throw new Error(`extension factory ${route.exportName} did not return a component`)
  }
  return component
}

function formatAuthorizationHeader(token) {
  const normalized = typeof token === 'string' ? token.trim() : ''
  if (!normalized) {
    return ''
  }
  return /^Bearer\s+/i.test(normalized) ? normalized : `Bearer ${normalized}`
}

function readAuthorizationHeader() {
  try {
    return formatAuthorizationHeader(localStorage.getItem('token') || '')
  } catch {
    return ''
  }
}

function toHex(buffer) {
  return [...new Uint8Array(buffer)].map(value => value.toString(16).padStart(2, '0')).join('')
}

export async function loadVerifiedBundleModule(extension) {
  if (!extension?.bundle?.url) {
    throw new Error('extension bundle URL is required')
  }
  if (typeof fetch !== 'function') {
    throw new Error('extension bundle fetch is unavailable')
  }
  if (!globalThis.crypto?.subtle) {
    throw new Error('extension bundle digest verification is unavailable')
  }
  const headers = { Accept: 'text/javascript, application/javascript' }
  const authorization = readAuthorizationHeader()
  if (authorization) {
    headers.Authorization = authorization
  }
  const response = await fetch(extension.bundle.url, {
    credentials: 'same-origin',
    headers
  })
  if (!response?.ok) {
    throw new Error(`extension bundle request failed with status ${response?.status || 0}`)
  }
  const source = await response.text()
  const digest = await globalThis.crypto.subtle.digest('SHA-256', new TextEncoder().encode(source))
  if (toHex(digest) !== extension.bundle.sha256) {
    throw new Error('extension bundle digest does not match the signed catalog')
  }
  if (typeof Blob !== 'function' || !URL?.createObjectURL) {
    throw new Error('extension module loader is unavailable')
  }
  const objectURL = URL.createObjectURL(new Blob([source], { type: 'text/javascript' }))
  try {
    return await import(/* @vite-ignore */ objectURL)
  } finally {
    URL.revokeObjectURL(objectURL)
  }
}

function extensionRouteHost(component) {
  return defineComponent({
    name: 'AdminExtensionRouteHost',
    setup() {
      const failed = ref(false)
      onErrorCaptured(() => {
        failed.value = true
        return false
      })
      return () => failed.value
        ? h('section', { class: 'empty-state', role: 'alert' }, 'Extension unavailable')
        : h(component)
    }
  })
}

function snapshot(menus, errors, skipped, plugins, extensions) {
  return {
    menus: [...menus.value],
    errors: [...errors.value],
    skipped: [...skipped.value],
    pluginIDs: [...plugins.value],
    extensions: [...extensions.value]
  }
}

export function createAdminExtensionRuntime(options = {}) {
  const fetchExtensions = options.fetchExtensions || getKernelExtensions
  const resolveModule = options.resolveModule || getLocalExtensionModuleLoader
  const loadBundleModule = options.loadBundleModule || loadVerifiedBundleModule
  const hasPermission = typeof options.hasPermission === 'function' ? options.hasPermission : currentUserHasPermission
  const now = options.now || Date.now
  const refreshInterval = options.refreshIntervalMs ?? REFRESH_INTERVAL_MS
  const menus = shallowRef([])
  const errors = shallowRef([])
  const skipped = shallowRef([])
  const plugins = shallowRef([])
  const extensions = shallowRef([])
  const routeRemovers = new Map()
  let initialized = false
  let lastAttempt = 0
  let refreshPromise = null
  let generation = 0

  function removeExtensionRoutes() {
    for (const remove of routeRemovers.values()) {
      try {
        remove()
      } catch {
        // A router-owned remover is idempotent; a broken extension must not
        // interfere with cleanup of other extensions or core records.
      }
    }
    routeRemovers.clear()
    menus.value = []
    skipped.value = []
    plugins.value = []
    extensions.value = []
  }

  async function prepareExtension(raw, duplicateIDs) {
    let extension
    try {
      extension = normalizeExtension(raw)
      if (duplicateIDs.has(extension.pluginID)) {
        throw new Error('plugin_id is duplicated in the extension catalog')
      }
      extension = filterAuthorizedExtension(extension, hasPermission)
      if (extension.routes.length === 0) {
        return {
          skipped: runtimeSkip(
            'forbidden',
            'extension has no routes allowed by the current permission grants',
            extension.pluginID
          )
        }
      }
      let module
      if (extension.bundle.url) {
        module = await loadBundleModule(extension)
      } else {
        const loader = resolveModule(extension.pluginID)
        if (typeof loader !== 'function') {
          throw new Error('plugin is not present in the local extension registry')
        }
        module = await loader()
      }
      return { extension, module: validateLocalModule(extension, module) }
    } catch (error) {
      return {
        error: runtimeError(
          'invalid_extension',
          error instanceof Error ? error.message : 'extension validation failed',
          extension?.pluginID || (typeof raw?.plugin_id === 'string' ? raw.plugin_id : '')
        )
      }
    }
  }

  async function performRefresh(router, refreshGeneration) {
    lastAttempt = now()
    let catalog
    try {
      catalog = await fetchExtensions()
      if (!Array.isArray(catalog)) {
        throw new Error('extension catalog must be an array')
      }
    } catch (error) {
      if (refreshGeneration === generation) {
        removeExtensionRoutes()
        errors.value = [runtimeError('catalog_unavailable', error instanceof Error ? error.message : 'extension catalog is unavailable')]
        initialized = true
      }
      return snapshot(menus, errors, skipped, plugins, extensions)
    }

    const counts = new Map()
    for (const item of catalog) {
      if (typeof item?.plugin_id === 'string') {
        counts.set(item.plugin_id, (counts.get(item.plugin_id) || 0) + 1)
      }
    }
    const duplicateIDs = new Set([...counts].filter(([, count]) => count > 1).map(([id]) => id))
    const prepared = await Promise.all(catalog.map(item => prepareExtension(item, duplicateIDs)))
    if (refreshGeneration !== generation) {
      return snapshot(menus, errors, skipped, plugins, extensions)
    }

    removeExtensionRoutes()
    const nextErrors = prepared.flatMap(item => item.error ? [item.error] : [])
    const nextSkipped = prepared.flatMap(item => item.skipped ? [item.skipped] : [])
    const occupiedPaths = new Set(
      typeof router?.getRoutes === 'function'
        ? router.getRoutes().map(route => route.path)
        : []
    )
    const nextMenus = []
    const nextPlugins = []
    const nextExtensions = []

    for (const candidate of prepared.filter(item => item.extension && item.module)) {
      const { extension, module } = candidate
      const pluginRemovers = []
      try {
        if (typeof router?.addRoute !== 'function' || typeof router?.hasRoute !== 'function' || !router.hasRoute(ADMIN_ROUTE_NAME)) {
          throw new Error('admin router is not ready for extension routes')
        }
        for (const route of extension.routes) {
          if (occupiedPaths.has(route.path)) {
            throw new Error(`route path ${route.path} is already registered`)
          }
        }
        for (const route of extension.routes) {
          const component = extensionRouteComponent(extension, module, route)
          const routeName = `extension:${extension.pluginID}:${route.id}`
          const remove = router.addRoute(ADMIN_ROUTE_NAME, {
            path: route.path.slice('/admin/'.length),
            name: routeName,
            component: extensionRouteHost(component),
            meta: {
              requiresAuth: true,
              requiresAdmin: true,
              extension: true,
              extensionPluginID: extension.pluginID,
              extensionPermission: route.permission
            }
          })
          pluginRemovers.push(remove)
          occupiedPaths.add(route.path)
        }
        routeRemovers.set(extension.pluginID, () => {
          for (const remove of pluginRemovers.reverse()) {
            remove()
          }
        })
        nextPlugins.push(extension.pluginID)
        nextExtensions.push(Object.freeze({
          pluginID: extension.pluginID,
          pluginName: extension.pluginName,
          version: extension.version,
          installationID: extension.installationID,
          configSchema: extension.configSchema
        }))
        nextMenus.push(...extension.menus.map(menu => ({
          pluginID: extension.pluginID,
          id: menu.id,
          parent: menu.parent,
          label: menu.label,
          icon: menu.icon,
          to: menu.route,
          permission: menu.permission,
          order: menu.order
        })))
      } catch (error) {
        for (const remove of pluginRemovers.reverse()) {
          try {
            remove()
          } catch {
            // Keep cleanup local to the rejected plugin.
          }
        }
        nextErrors.push(runtimeError('route_registration_failed', error instanceof Error ? error.message : 'extension route registration failed', extension.pluginID))
      }
    }

    nextMenus.sort((left, right) => left.order - right.order || left.label.localeCompare(right.label) || left.id.localeCompare(right.id))
    menus.value = nextMenus
    errors.value = nextErrors
    skipped.value = nextSkipped
    plugins.value = nextPlugins.sort()
    extensions.value = nextExtensions.sort((left, right) => left.pluginID.localeCompare(right.pluginID))
    initialized = true
    return snapshot(menus, errors, skipped, plugins, extensions)
  }

  function refresh(router) {
    if (refreshPromise) {
      return refreshPromise
    }
    const refreshGeneration = ++generation
    refreshPromise = performRefresh(router, refreshGeneration).finally(() => {
      if (refreshGeneration === generation) {
        refreshPromise = null
      }
    })
    return refreshPromise
  }

  function ensure(router) {
    if (initialized && now() - lastAttempt < refreshInterval) {
      return Promise.resolve(snapshot(menus, errors, skipped, plugins, extensions))
    }
    return refresh(router)
  }

  function reset() {
    generation += 1
    refreshPromise = null
    initialized = false
    lastAttempt = 0
    removeExtensionRoutes()
    errors.value = []
  }

  return {
    menus: readonly(menus),
    errors: readonly(errors),
    skipped: readonly(skipped),
    plugins: readonly(plugins),
    extensions: readonly(extensions),
    ensure,
    refresh,
    reset
  }
}

const adminExtensionRuntime = createAdminExtensionRuntime()

export const adminExtensionMenus = adminExtensionRuntime.menus
export const adminExtensionErrors = adminExtensionRuntime.errors
export const adminExtensionSkipped = adminExtensionRuntime.skipped
export const adminExtensionPluginIDs = adminExtensionRuntime.plugins
export const adminExtensions = adminExtensionRuntime.extensions

export function ensureAdminExtensions(router) {
  return adminExtensionRuntime.ensure(router)
}

export function refreshAdminExtensions(router) {
  return adminExtensionRuntime.refresh(router)
}

export function resetAdminExtensions() {
  adminExtensionRuntime.reset()
}
