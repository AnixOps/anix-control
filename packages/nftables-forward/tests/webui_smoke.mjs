import assert from 'node:assert/strict'

import create, { anixopsExtension } from '../webui/index.mjs'

assert.deepEqual(anixopsExtension, {
  webuiApiVersion: 'anixops.webui/v1',
  pluginId: 'nftables-forward',
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
      plugin_id: 'nftables-forward',
      version: '1.2.0',
      summary: { rules: 1, ready: 1, reconciling: 0, rollback_required: 0 },
      rules: [{
        id: 7,
        name: 'dedicated-shanghai-443',
        node_id: 11,
        node_name: 'cn-dedicated-one',
        topology: 'cn-entry -> target',
        protocol: 'tcp+udp',
        listen: '0.0.0.0:443',
        target: '198.51.100.20:443',
        desired_revision: 4,
        observed_revision: 4,
        enabled: true,
      }],
    }
  },
})

assert.equal(typeof component.setup, 'function')
const render = component.setup()
assert.equal(typeof mounted, 'function')
await mounted()

assert.deepEqual(requests, [[
  '/api/v3/plugins/nftables-forward/status?limit=200',
  { method: 'GET' },
]])

const rendered = JSON.stringify(render())
assert.match(rendered, /dedicated-shanghai-443/)
assert.match(rendered, /cn-dedicated-one/)
assert.match(rendered, /tcp\+udp/)
assert.match(rendered, /4\/4/)
assert.match(rendered, /Ready/)

console.log('nftables-forward WebUI smoke test passed')
