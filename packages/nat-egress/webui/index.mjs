const PLUGIN_ID = 'nat-egress'
const VERSION = '__ANIXOPS_PACKAGE_VERSION__'
const STATUS_PATH = '/api/v3/plugins/nat-egress/status?limit=200'

export const anixopsExtension = {
  webuiApiVersion: 'anixops.webui/v1',
  pluginId: PLUGIN_ID,
  version: VERSION,
  bundle: { path: 'webui/index.mjs' },
}

function count(value) {
  return Number.isFinite(Number(value)) ? String(value) : '0'
}

function stateLabel(exit) {
  if (exit?.rollback_required) return 'Rollback'
  if (exit?.degraded) return 'Degraded'
  if (exit?.ready) return 'Ready'
  return 'Pending'
}

export default function create(host) {
  const { defineComponent, h, ref, onMounted, request } = host
  return defineComponent({
    name: 'NatEgressExtension',
    setup() {
      const loading = ref(true)
      const error = ref('')
      const value = ref({ summary: {}, exits: [] })
      onMounted(async () => {
        try {
          value.value = await request(STATUS_PATH, { method: 'GET' })
        } catch (err) {
          error.value = err?.message || 'Failed to load NAT egress status'
        } finally {
          loading.value = false
        }
      })
      return () => {
        if (loading.value) {
          return h('section', { class: 'control-page nat-egress-extension' }, [
            h('h1', 'NAT Egress'),
            h('p', { role: 'status' }, 'Loading egress nodes...'),
          ])
        }
        if (error.value) {
          return h('section', { class: 'control-page nat-egress-extension' }, [
            h('h1', 'NAT Egress'),
            h('p', { class: 'error', role: 'alert' }, error.value),
          ])
        }
        const summary = [
          ['Exits', count(value.value.summary?.exits)],
          ['Ready', count(value.value.summary?.ready)],
          ['Degraded', count(value.value.summary?.degraded)],
          ['Rollback', count(value.value.summary?.rollback_required)],
        ]
        const rows = (value.value.exits || []).map(exit => h('tr', { key: exit.id }, [
          h('td', exit.name || exit.id),
          h('td', exit.node_name || '-'),
          h('td', exit.egress_interface || '-'),
          h('td', exit.public_ip || '-'),
          h('td', exit.policy_table == null ? '-' : String(exit.policy_table)),
          h('td', exit.health || '-'),
          h('td', stateLabel(exit)),
        ]))
        return h('section', { class: 'control-page nat-egress-extension' }, [
          h('header', [
            h('h1', 'NAT Egress'),
            h('p', 'Signed Agent runtime status for nftables masquerade, policy routing, stable cleanup state, and marked egress health. Production signing remains gated by the pinned Agent revision and namespace evidence.'),
          ]),
          h('dl', { class: 'metric-grid' }, summary.flatMap(([label, metric]) => [
            h('dt', label),
            h('dd', metric),
          ])),
          h('table', { class: 'data-table' }, [
            h('thead', [h('tr', ['Exit', 'Node', 'Interface', 'Public IP', 'Policy Table', 'Health', 'State'].map(label => h('th', label)))]),
            h('tbody', rows.length ? rows : [h('tr', { key: 'empty' }, [h('td', { colspan: 7, class: 'empty-state' }, 'No NAT egress exits')])]),
          ]),
        ])
      }
    },
  })
}
