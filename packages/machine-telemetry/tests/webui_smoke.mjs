import assert from 'node:assert/strict'

import create, { anixopsExtension } from '../webui/index.mjs'

assert.deepEqual(anixopsExtension, {
  webuiApiVersion: 'anixops.webui/v1',
  pluginId: 'machine-telemetry',
  version: '__ANIXOPS_PACKAGE_VERSION__',
  bundle: { path: 'webui/index.mjs' },
})

let mounted
const requests = []

function h(type, props, children) {
  if (arguments.length === 2 && (Array.isArray(props) || typeof props === 'string')) {
    children = props
    props = {}
  }
  return { type, props: props || {}, children }
}

const component = create({
  defineComponent: value => value,
  h,
  ref: value => ({ value }),
  onMounted: callback => { mounted = callback },
  request: async (path, options) => {
    requests.push([path, options])
    return {
      plugin_id: 'machine-telemetry',
      version: '1.1.0',
      summary: { total: 1, online: 1, offline: 0, unhealthy: 0 },
      nodes: [{
        id: 7,
        name: 'edge-one',
        host: '192.0.2.7',
        status: 'enabled',
        online: true,
        runtime_healthy: true,
        cpu_usage: 12.5,
        memory_usage: 40,
        disk_usage: 70,
        uptime: 3660,
        online_users: 3,
      }],
    }
  },
})

assert.equal(typeof component.setup, 'function')
const render = component.setup()
assert.equal(typeof mounted, 'function')
await mounted()

assert.deepEqual(requests, [[
  '/api/v3/plugins/machine-telemetry/status?limit=200',
  { method: 'GET' },
]])

const rendered = JSON.stringify(render())
assert.match(rendered, /edge-one/)
assert.match(rendered, /12\.5%/)
assert.match(rendered, /Online/)
assert.match(rendered, /1h 1m/)

console.log('machine-telemetry WebUI smoke test passed')
