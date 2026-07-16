import assert from 'node:assert/strict'

import create, { anixopsExtension } from '../webui/index.mjs'

assert.deepEqual(anixopsExtension, {
  webuiApiVersion: 'anixops.webui/v1',
  pluginId: 'gost-mesh',
  version: '1.0.0',
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
      plugin_id: 'gost-mesh',
      version: '1.0.0',
      summary: { tunnels: 1, ready: 1, reconciling: 0, rollback_required: 0 },
      tunnels: [{
        id: 'gost-wss-one',
        name: 'cn-standard-wss',
        entry_node: 'cn-standard-entry',
        exit_node: 'hk-nat-one',
        protocol: 'wss',
        listen: '0.0.0.0:443',
        upstream: 'hk-nat-one:8443',
        ready: true,
      }],
    }
  },
})

assert.equal(typeof component.setup, 'function')
const render = component.setup()
assert.equal(typeof mounted, 'function')
await mounted()

assert.deepEqual(requests, [[
  '/api/v3/plugins/gost-mesh/status?limit=200',
  { method: 'GET' },
]])

const rendered = JSON.stringify(render())
assert.match(rendered, /cn-standard-wss/)
assert.match(rendered, /cn-standard-entry/)
assert.match(rendered, /hk-nat-one/)
assert.match(rendered, /wss/)
assert.match(rendered, /Ready/)

console.log('gost-mesh WebUI smoke test passed')
