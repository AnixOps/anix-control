import assert from 'node:assert/strict'

import create, { anixopsExtension } from '../webui/index.mjs'

assert.deepEqual(anixopsExtension, {
  webuiApiVersion: 'anixops.webui/v1',
  pluginId: 'nat-egress',
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
      plugin_id: 'nat-egress',
      version: '1.0.0',
      summary: { exits: 1, ready: 1, degraded: 0, rollback_required: 0 },
      exits: [{
        id: 'nat-hk-one',
        name: 'hk-nat-one',
        node_name: 'hk-egress-01',
        egress_interface: 'eth0',
        public_ip: '203.0.113.20',
        policy_table: 100,
        health: '23ms',
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
  '/api/v3/plugins/nat-egress/status?limit=200',
  { method: 'GET' },
]])

const rendered = JSON.stringify(render())
assert.match(rendered, /hk-nat-one/)
assert.match(rendered, /hk-egress-01/)
assert.match(rendered, /203\.0\.113\.20/)
assert.match(rendered, /23ms/)
assert.match(rendered, /Ready/)

console.log('nat-egress WebUI smoke test passed')
