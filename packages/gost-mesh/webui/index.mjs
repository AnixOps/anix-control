const PLUGIN_ID = 'gost-mesh'
const VERSION = '1.0.0'
const STATUS_PATH = '/api/v3/plugins/gost-mesh/status?limit=200'

export const anixopsExtension = {
  webuiApiVersion: 'anixops.webui/v1',
  pluginId: PLUGIN_ID,
  version: VERSION,
  bundle: { path: 'webui/index.mjs' },
}

function count(value) {
  return Number.isFinite(Number(value)) ? String(value) : '0'
}

function stateLabel(tunnel) {
  if (tunnel?.cleanup_pending) return 'Cleanup required'
  if (tunnel?.rollback_required) return 'Rollback'
  if (tunnel?.reconciling) return 'Reconciling'
  if (tunnel?.health === 'unhealthy' || tunnel?.health === 'degraded') return 'Degraded'
  if (tunnel?.ready) return 'Ready'
  return 'Pending'
}

export default function create(host) {
  const { defineComponent, h, ref, onMounted, request } = host
  return defineComponent({
    name: 'GostMeshExtension',
    setup() {
      const loading = ref(true)
      const error = ref('')
      const value = ref({ summary: {}, tunnels: [] })
      onMounted(async () => {
        try {
          value.value = await request(STATUS_PATH, { method: 'GET' })
        } catch (err) {
          error.value = err?.message || 'Failed to load GOST mesh status'
        } finally {
          loading.value = false
        }
      })
      return () => {
        if (loading.value) {
          return h('section', { class: 'control-page gost-mesh-extension' }, [
            h('h1', 'GOST Mesh'),
            h('p', { role: 'status' }, 'Loading tunnel mesh...'),
          ])
        }
        if (error.value) {
          return h('section', { class: 'control-page gost-mesh-extension' }, [
            h('h1', 'GOST Mesh'),
            h('p', { class: 'error', role: 'alert' }, error.value),
          ])
        }
        const summary = [
          ['Tunnels', count(value.value.summary?.tunnels)],
          ['Ready', count(value.value.summary?.ready)],
          ['Reconciling', count(value.value.summary?.reconciling)],
          ['Cleanup', count(value.value.summary?.cleanup_pending)],
          ['Rollback', count(value.value.summary?.rollback_required)],
        ]
        const rows = (value.value.tunnels || []).map(tunnel => h('tr', { key: tunnel.id }, [
          h('td', tunnel.name || tunnel.id),
          h('td', tunnel.entry_node || '-'),
          h('td', tunnel.exit_node || '-'),
          h('td', tunnel.transport || tunnel.protocol || '-'),
          h('td', tunnel.listen || '-'),
          h('td', tunnel.upstream || '-'),
          h('td', stateLabel(tunnel)),
        ]))
        return h('section', { class: 'control-page gost-mesh-extension' }, [
          h('header', [
            h('h1', 'GOST Mesh'),
            h('p', 'Signed GOST v3.2.6 WSS/QUIC runtime status. Canary approval is still required before production traffic.'),
          ]),
          h('dl', { class: 'metric-grid' }, summary.flatMap(([label, metric]) => [
            h('dt', label),
            h('dd', metric),
          ])),
          h('table', { class: 'data-table' }, [
            h('thead', [h('tr', ['Tunnel', 'Entry', 'Exit', 'Protocol', 'Listen', 'Upstream', 'State'].map(label => h('th', label)))]),
            h('tbody', rows.length ? rows : [h('tr', { key: 'empty' }, [h('td', { colspan: 7, class: 'empty-state' }, 'No GOST tunnels')])]),
          ]),
        ])
      }
    },
  })
}
