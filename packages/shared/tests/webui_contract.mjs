import assert from 'node:assert/strict'
import { access, readdir, readFile } from 'node:fs/promises'
import path from 'node:path'
import { fileURLToPath } from 'node:url'

// The WebUI bundle is optional: only a package whose manifest template
// declares `webui` ships one (packages/shared/build_package.py). This checks
// every such bundle against its manifest, and that no other package keeps a
// webui/ directory the builder would refuse.

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

async function exists(target) {
  try {
    await access(target)
    return true
  } catch {
    return false
  }
}

const packageNames = (await readdir(packagesRoot, { withFileTypes: true }))
  .filter(entry => entry.isDirectory())
  .map(entry => entry.name)
  .sort()

const checked = []
for (const packageID of packageNames) {
  const packageRoot = path.join(packagesRoot, packageID)
  let manifest
  try {
    manifest = JSON.parse(await readFile(path.join(packageRoot, 'manifest.template.json'), 'utf8'))
  } catch {
    continue
  }

  if (!('webui' in manifest)) {
    assert.ok(!(await exists(path.join(packageRoot, 'webui'))), `${packageID} keeps a webui/ directory but its manifest declares no webui`)
    assert.equal(manifest.frontend_sha256, '', `${packageID} declares no webui, so frontend_sha256 must stay empty`)
    continue
  }

  const webui = manifest.webui
  assert.ok(webui?.bundle?.path && Array.isArray(webui.routes), `${packageID} webui must declare a bundle and routes`)
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
  checked.push(packageID)
}

assert.ok(checked.length > 0, 'no package declares a WebUI bundle')
console.log(`v4 WebUI package contract passed for ${checked.length} packages with a WebUI: ${checked.join(', ')}`)
