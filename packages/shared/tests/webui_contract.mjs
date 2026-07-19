import assert from 'node:assert/strict'
import { readdir, readFile } from 'node:fs/promises'
import path from 'node:path'
import { fileURLToPath } from 'node:url'

const here = path.dirname(fileURLToPath(import.meta.url))
const packagesRoot = path.resolve(here, '..', '..')
const versionToken = '__ANIXOPS_PACKAGE_VERSION__'

function extensionHost() {
  return {
    defineComponent: value => value,
    h: (type, props, children) => ({ type, props, children }),
    onMounted: () => {},
    readonly: value => value,
    ref: value => ({ value }),
    request: async () => ({}),
  }
}

function moduleURL(source) {
  return `data:text/javascript;base64,${Buffer.from(source, 'utf8').toString('base64')}`
}

const packageNames = (await readdir(packagesRoot, { withFileTypes: true }))
  .filter(entry => entry.isDirectory())
  .map(entry => entry.name)
  .sort()

for (const packageID of packageNames) {
  const packageRoot = path.join(packagesRoot, packageID)
  let manifest
  try {
    manifest = JSON.parse(await readFile(path.join(packageRoot, 'manifest.template.json'), 'utf8'))
  } catch {
    continue
  }

  const webui = manifest.webui
  if (!webui?.bundle?.path || !Array.isArray(webui.routes)) {
    continue
  }
  const source = await readFile(path.join(packageRoot, webui.bundle.path), 'utf8')
  assert.ok(source.includes(versionToken), `${packageID} WebUI must declare the release-version token`)

  const materialized = source.replaceAll(versionToken, manifest.version)
  assert.ok(!materialized.includes(versionToken), `${packageID} WebUI must materialize the release version`)
  const extension = await import(moduleURL(materialized))
  assert.deepEqual(extension.anixopsExtension, {
    webuiApiVersion: 'anixops.webui/v1',
    pluginId: packageID,
    version: manifest.version,
    bundle: { path: webui.bundle.path },
  }, `${packageID} WebUI identity must match its manifest`)

  for (const route of webui.routes) {
    const factory = extension[route.export]
    assert.equal(typeof factory, 'function', `${packageID} WebUI is missing ${route.export}`)
    const component = factory(extensionHost())
    assert.ok(
      (typeof component === 'object' && component !== null) || typeof component === 'function',
      `${packageID} WebUI factory ${route.export} must return a component`,
    )
  }
}

console.log('v4 WebUI package contract passed')
