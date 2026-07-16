import { createHash } from 'node:crypto'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { createMemoryHistory, createRouter } from 'vue-router'
import { createAdminExtensionRuntime, loadVerifiedBundleModule } from '@/extensions/runtime'

const HASH = 'a'.repeat(64)
const MACHINE_TELEMETRY_HASH = 'b'.repeat(64)

const CoreLayout = { template: '<router-view />' }
const CorePage = { template: '<div>Core page</div>' }
const ExtensionPage = { template: '<div>Extension page</div>' }

function createTestRouter() {
  return createRouter({
    history: createMemoryHistory(),
    routes: [{
      path: '/admin',
      name: 'admin',
      component: CoreLayout,
      children: [{ path: 'dashboard', name: 'admin-dashboard', component: CorePage }],
    }],
  })
}

function catalogEntry(overrides = {}) {
  return {
    plugin_id: 'example',
    plugin_name: 'Example',
    publisher: 'AnixOps',
    version: '1.2.3',
    api_version: 'v1',
    installation_id: 1,
    state: 'healthy',
    bundle: { path: 'webui/index.mjs', sha256: HASH },
    permissions: ['example.view'],
    menus: [{
      id: 'example.main',
      parent: 'services',
      label: 'Example',
      icon: 'box',
      route: '/admin/extensions/example',
      permission: 'example.view',
      order: 100,
    }],
    routes: [{
      id: 'example.main',
      path: '/admin/extensions/example',
      export: 'default',
      permission: 'example.view',
    }],
    config_schema: { type: 'object' },
    ...overrides,
  }
}

function localModule(overrides = {}) {
  return {
    anixopsExtension: {
      pluginId: 'example',
      version: '1.2.3',
      bundle: { path: 'webui/index.mjs', sha256: HASH },
    },
    default: ExtensionPage,
    ...overrides,
  }
}

function createRuntime(fetchExtensions, resolveModule = () => async () => localModule(), extra = {}) {
  return createAdminExtensionRuntime({
    fetchExtensions,
    resolveModule,
    refreshIntervalMs: 0,
    ...extra,
  })
}

function sha256Hex(source) {
  return createHash('sha256').update(source).digest('hex')
}

function digestArrayBuffer(data) {
  const digest = createHash('sha256').update(Buffer.from(data)).digest()
  return digest.buffer.slice(digest.byteOffset, digest.byteOffset + digest.byteLength)
}

function stubBrowserBundleLoader(source) {
  Object.defineProperty(URL, 'createObjectURL', {
    configurable: true,
    value: vi.fn(() => `data:text/javascript;charset=utf-8,${encodeURIComponent(source)}`),
  })
  Object.defineProperty(URL, 'revokeObjectURL', {
    configurable: true,
    value: vi.fn(),
  })
}

describe('admin extension runtime', () => {
  afterEach(() => {
    vi.unstubAllGlobals()
    vi.restoreAllMocks()
    localStorage.clear()
  })

  it('registers only a signed-catalog entry backed by a matching local module', async () => {
    const router = createTestRouter()
    const resolveModule = vi.fn(() => async () => localModule())
    const runtime = createRuntime(async () => [catalogEntry()], resolveModule)

    const result = await runtime.refresh(router)

    expect(resolveModule).toHaveBeenCalledWith('example')
    expect(result.errors).toEqual([])
    expect(result.pluginIDs).toEqual(['example'])
    expect(runtime.menus.value).toEqual([expect.objectContaining({
      label: 'Example',
      icon: 'EX',
      to: '/admin/extensions/example',
    })])
    expect(router.getRoutes().map(route => route.path)).toContain('/admin/extensions/example')
    expect(router.resolve('/admin/extensions/example').matched.at(-1).meta).toMatchObject({
      extension: true,
      extensionPluginID: 'example',
      extensionPermission: 'example.view',
    })
  })

  it('loads the bundled machine-telemetry reference module from the controlled registry', async () => {
    const router = createTestRouter()
    const runtime = createRuntime(
      async () => [catalogEntry({
        plugin_id: 'machine-telemetry',
        plugin_name: 'Machine Telemetry',
        version: '1.0.0',
        bundle: { path: 'webui/index.mjs', sha256: MACHINE_TELEMETRY_HASH },
        permissions: ['machine-telemetry.view'],
        menus: [{
          id: 'machine-telemetry.main',
          parent: 'services',
          label: 'Machine Telemetry',
          icon: 'activity',
          route: '/admin/extensions/machine-telemetry',
          permission: 'machine-telemetry.view',
          order: 100,
        }],
        routes: [{
          id: 'machine-telemetry.main',
          path: '/admin/extensions/machine-telemetry',
          export: 'default',
          permission: 'machine-telemetry.view',
        }],
      })],
      pluginID => pluginID === 'machine-telemetry'
        ? () => import('@/extensions/modules/machine-telemetry/index.js')
        : null,
    )

    const result = await runtime.refresh(router)

    expect(result.errors).toEqual([])
    expect(result.pluginIDs).toEqual(['machine-telemetry'])
    expect(router.getRoutes().map(route => route.path)).toContain('/admin/extensions/machine-telemetry')
    expect(runtime.menus.value[0]).toMatchObject({
      label: 'Machine Telemetry',
      to: '/admin/extensions/machine-telemetry',
      permission: 'machine-telemetry.view',
    })
  })

  it('loads a verified same-origin bundle URL without the local module registry', async () => {
    const router = createTestRouter()
    const resolveModule = vi.fn()
    const loadBundleModule = vi.fn(async () => localModule())
    const runtime = createRuntime(
      async () => [catalogEntry({
        bundle: {
          path: 'webui/index.mjs',
          sha256: HASH,
          url: `/api/v3/extensions/example/1.2.3/webui/${HASH}/index.mjs`,
        },
      })],
      resolveModule,
      { loadBundleModule },
    )

    const result = await runtime.refresh(router)

    expect(resolveModule).not.toHaveBeenCalled()
    expect(loadBundleModule).toHaveBeenCalledWith(expect.objectContaining({
      pluginID: 'example',
      bundle: expect.objectContaining({
        url: `/api/v3/extensions/example/1.2.3/webui/${HASH}/index.mjs`,
      }),
    }))
    expect(result.errors).toEqual([])
    expect(result.pluginIDs).toEqual(['example'])
    expect(router.getRoutes().map(route => route.path)).toContain('/admin/extensions/example')
  })

  it('materializes an import-free WebUI factory with a plugin-scoped host API', async () => {
    const router = createTestRouter()
    let host
    const factoryModule = {
      anixopsExtension: {
        pluginId: 'example',
        version: '1.2.3',
        webuiApiVersion: 'anixops.webui/v1',
        bundle: { path: 'webui/index.mjs' },
      },
      default: injectedHost => {
        host = injectedHost
        return ExtensionPage
      },
    }
    const runtime = createRuntime(async () => [catalogEntry()], () => async () => factoryModule)

    const result = await runtime.refresh(router)

    expect(result.errors).toEqual([])
    expect(host).toMatchObject({ pluginID: 'example', version: '1.2.3' })
    expect(typeof host.defineComponent).toBe('function')
    expect(typeof host.request).toBe('function')

    const fetchMock = vi.fn(async () => ({
      ok: true,
      status: 200,
      json: async () => ({ data: { health: 'ok' } }),
    }))
    vi.stubGlobal('fetch', fetchMock)
    localStorage.setItem('token', 'factory-token')
    await expect(host.request('/api/v3/plugins/example/status?limit=1')).resolves.toEqual({ health: 'ok' })
    expect(fetchMock).toHaveBeenCalledWith('/api/v3/plugins/example/status?limit=1', {
      method: 'GET',
      credentials: 'same-origin',
      headers: { Accept: 'application/json', Authorization: 'Bearer factory-token' },
      body: undefined,
    })
    await expect(host.request('/api/v3/plugins/other/status')).rejects.toThrow('outside the plugin API namespace')
    await expect(host.request('https://evil.example/status')).rejects.toThrow('outside the plugin API namespace')
  })

  it('verifies a fetched bundle digest in the browser before importing it', async () => {
    const source = 'export const ok = true; export default { name: "RemoteExtension" };'
    const sha256 = sha256Hex(source)
    const fetchMock = vi.fn(async () => ({ ok: true, text: async () => source }))
    vi.stubGlobal('fetch', fetchMock)
    vi.stubGlobal('crypto', { subtle: { digest: vi.fn(async (_algorithm, data) => digestArrayBuffer(data)) } })
    stubBrowserBundleLoader(source)
    localStorage.setItem('token', 'bundle-token')

    const module = await loadVerifiedBundleModule({
      bundle: {
        url: `/api/v3/extensions/example/1.2.3/webui/${sha256}/index.mjs`,
        sha256,
      },
    })

    expect(module.ok).toBe(true)
    expect(fetchMock).toHaveBeenCalledWith(`/api/v3/extensions/example/1.2.3/webui/${sha256}/index.mjs`, {
      credentials: 'same-origin',
      headers: {
        Accept: 'text/javascript, application/javascript',
        Authorization: 'Bearer bundle-token',
      },
    })
  })

  it('rejects a fetched bundle when the browser digest does not match', async () => {
    const source = 'export default { name: "TamperedExtension" };'
    vi.stubGlobal('fetch', vi.fn(async () => ({ ok: true, text: async () => source })))
    vi.stubGlobal('crypto', { subtle: { digest: vi.fn(async (_algorithm, data) => digestArrayBuffer(data)) } })
    stubBrowserBundleLoader(source)

    await expect(loadVerifiedBundleModule({
      bundle: {
        url: `/api/v3/extensions/example/1.2.3/webui/${HASH}/index.mjs`,
        sha256: HASH,
      },
    })).rejects.toThrow('digest does not match')
  })

  it.each([
    'https://evil.example/plugin.mjs',
    '//evil.example/plugin.mjs',
    '/webui/index.mjs',
    'webui/../index.mjs',
    'webui/index.mjs?token=secret',
    'webui/%2e%2e/index.mjs',
  ])('rejects non-artifact bundle path %s before resolving any module', async path => {
    const router = createTestRouter()
    const resolveModule = vi.fn()
    const runtime = createRuntime(async () => [catalogEntry({ bundle: { path, sha256: HASH } })], resolveModule)

    const result = await runtime.refresh(router)

    expect(resolveModule).not.toHaveBeenCalled()
    expect(result.errors).toEqual([expect.objectContaining({ code: 'invalid_extension', plugin_id: 'example' })])
    expect(router.getRoutes().map(route => route.path)).not.toContain('/admin/extensions/example')
    expect(router.hasRoute('admin-dashboard')).toBe(true)
  })

  it('rejects bundle URLs that are not verified same-origin asset URLs', async () => {
    const router = createTestRouter()
    const resolveModule = vi.fn()
    const loadBundleModule = vi.fn()
    const runtime = createRuntime(
      async () => [catalogEntry({ bundle: { path: 'webui/index.mjs', sha256: HASH, url: 'https://evil.example/plugin.mjs' } })],
      resolveModule,
      { loadBundleModule },
    )

    const result = await runtime.refresh(router)

    expect(resolveModule).not.toHaveBeenCalled()
    expect(loadBundleModule).not.toHaveBeenCalled()
    expect(result.errors).toEqual([expect.objectContaining({ code: 'invalid_extension', plugin_id: 'example' })])
  })

  it('rejects a catalog identity that does not match the compiled local module', async () => {
    const router = createTestRouter()
    const runtime = createRuntime(
      async () => [catalogEntry()],
      () => async () => localModule({
        anixopsExtension: {
          pluginId: 'example',
          version: '9.9.9',
          bundle: { path: 'webui/index.mjs', sha256: HASH },
        },
      }),
    )

    const result = await runtime.refresh(router)

    expect(result.errors[0].message).toContain('identity does not match')
    expect(result.pluginIDs).toEqual([])
    expect(router.hasRoute('admin-dashboard')).toBe(true)
  })

  it('isolates an invalid plugin while registering another valid extension', async () => {
    const router = createTestRouter()
    const invalid = catalogEntry({
      plugin_id: 'broken',
      plugin_name: 'Broken',
      permissions: ['broken.view'],
      menus: [{ id: 'broken.main', parent: 'services', label: 'Broken', icon: 'box', route: '/admin/dashboard', permission: 'broken.view', order: 1 }],
      routes: [{ id: 'broken.main', path: '/admin/dashboard', export: 'default', permission: 'broken.view' }],
    })
    const runtime = createRuntime(async () => [invalid, catalogEntry()])

    const result = await runtime.refresh(router)

    expect(result.pluginIDs).toEqual(['example'])
    expect(result.errors).toEqual([expect.objectContaining({ plugin_id: 'broken' })])
    expect(router.hasRoute('admin-dashboard')).toBe(true)
    expect(router.getRoutes().map(route => route.path)).toContain('/admin/extensions/example')
  })

  it('removes menus and routes when an extension disappears from the enabled catalog', async () => {
    const router = createTestRouter()
    let catalog = [catalogEntry()]
    const runtime = createRuntime(async () => catalog)

    await runtime.refresh(router)
    expect(router.getRoutes().map(route => route.path)).toContain('/admin/extensions/example')

    catalog = []
    await runtime.refresh(router)

    expect(runtime.menus.value).toEqual([])
    expect(router.getRoutes().map(route => route.path)).not.toContain('/admin/extensions/example')
    expect(router.hasRoute('admin-dashboard')).toBe(true)
  })

  it('handles package WebUI install, disable, update and rollback catalog transitions', async () => {
    const router = createTestRouter()
    const hashV1 = '1'.repeat(64)
    const hashV2 = '2'.repeat(64)
    const entry = (version, sha256, label) => catalogEntry({
      version,
      bundle: {
        path: 'webui/index.mjs',
        sha256,
        url: `/api/v3/extensions/example/${version}/webui/${sha256}/index.mjs`,
      },
      menus: [{
        id: 'example.main',
        parent: 'services',
        label,
        icon: 'box',
        route: '/admin/extensions/example',
        permission: 'example.view',
        order: 100,
      }],
    })
    let catalog = []
    const loadBundleModule = vi.fn(async extension => localModule({
      anixopsExtension: {
        pluginId: extension.pluginID,
        version: extension.version,
        bundle: {
          path: extension.bundle.path,
          sha256: extension.bundle.sha256,
        },
      },
    }))
    const runtime = createRuntime(async () => catalog, () => null, { loadBundleModule })

    await runtime.refresh(router)
    expect(runtime.menus.value).toEqual([])
    expect(router.getRoutes().map(route => route.path)).not.toContain('/admin/extensions/example')

    catalog = [entry('1.0.0', hashV1, 'Example v1')]
    let result = await runtime.refresh(router)
    expect(result.pluginIDs).toEqual(['example'])
    expect(runtime.menus.value[0].label).toBe('Example v1')
    expect(router.getRoutes().map(route => route.path)).toContain('/admin/extensions/example')

    catalog = []
    result = await runtime.refresh(router)
    expect(result.pluginIDs).toEqual([])
    expect(runtime.menus.value).toEqual([])
    expect(router.getRoutes().map(route => route.path)).not.toContain('/admin/extensions/example')

    catalog = [entry('2.0.0', hashV2, 'Example v2')]
    result = await runtime.refresh(router)
    expect(result.errors).toEqual([])
    expect(runtime.menus.value[0].label).toBe('Example v2')
    expect(loadBundleModule).toHaveBeenLastCalledWith(expect.objectContaining({ version: '2.0.0' }))

    catalog = [entry('1.0.0', hashV1, 'Example v1')]
    result = await runtime.refresh(router)
    expect(result.errors).toEqual([])
    expect(runtime.menus.value[0].label).toBe('Example v1')
    expect(loadBundleModule).toHaveBeenLastCalledWith(expect.objectContaining({ version: '1.0.0' }))
    expect(router.getRoutes().map(route => route.path)).toContain('/admin/extensions/example')
  })

  it('fails closed on a catalog outage without removing core routes', async () => {
    const router = createTestRouter()
    let fail = false
    const runtime = createRuntime(async () => {
      if (fail) {
        throw new Error('offline')
      }
      return [catalogEntry()]
    })

    await runtime.refresh(router)
    fail = true
    const result = await runtime.refresh(router)

    expect(result.errors).toEqual([expect.objectContaining({ code: 'catalog_unavailable' })])
    expect(runtime.menus.value).toEqual([])
    expect(router.getRoutes().map(route => route.path)).not.toContain('/admin/extensions/example')
    expect(router.resolve('/admin/dashboard').matched.at(-1).name).toBe('admin-dashboard')
  })

  it('rejects duplicate plugin IDs and never executes either local loader', async () => {
    const router = createTestRouter()
    const resolveModule = vi.fn(() => async () => localModule())
    const runtime = createRuntime(async () => [catalogEntry(), catalogEntry({ installation_id: 2 })], resolveModule)

    const result = await runtime.refresh(router)

    expect(resolveModule).not.toHaveBeenCalled()
    expect(result.errors).toHaveLength(2)
    expect(result.pluginIDs).toEqual([])
  })
})
