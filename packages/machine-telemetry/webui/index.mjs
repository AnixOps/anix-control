const PLUGIN_ID = 'machine-telemetry'
const PLUGIN_VERSION = '1.0.0'
const BUNDLE_PATH = 'webui/index.mjs'
const STATUS_PATH = '/api/v3/plugins/machine-telemetry/status?limit=200'

// The signed manifest owns the bundle digest. Embedding that digest here would
// require the module to contain its own SHA-256 and create a circular identity.
export const anixopsExtension = Object.freeze({
  webuiApiVersion: 'anixops.webui/v1',
  pluginId: PLUGIN_ID,
  version: PLUGIN_VERSION,
  bundle: Object.freeze({ path: BUNDLE_PATH }),
})

function responseData(payload) {
  if (!payload || typeof payload !== 'object' || Array.isArray(payload)) {
    return null
  }
  const value = payload.data && typeof payload.data === 'object' ? payload.data : payload
  return value && typeof value === 'object' && !Array.isArray(value) ? value : null
}

function finiteNumber(value) {
  return typeof value === 'number' && Number.isFinite(value) ? value : null
}

function percent(value) {
  const number = finiteNumber(value)
  return number === null ? 'n/a' : `${number.toFixed(1)}%`
}

function uptime(value) {
  const number = finiteNumber(value)
  if (number === null || number < 0) {
    return 'n/a'
  }
  const seconds = Math.floor(number)
  const days = Math.floor(seconds / 86400)
  const hours = Math.floor((seconds % 86400) / 3600)
  const minutes = Math.floor((seconds % 3600) / 60)
  return days > 0 ? `${days}d ${hours}h` : hours > 0 ? `${hours}h ${minutes}m` : `${minutes}m`
}

function stateLabel(node) {
  if (node?.status === 'disabled' || node?.status === 3) {
    return 'Disabled'
  }
  if (!node?.online) {
    return 'Offline'
  }
  return node.runtime_healthy === false ? 'Unhealthy' : 'Online'
}

export default function create(host) {
  if (!host || typeof host !== 'object') {
    throw new Error('AnixOps WebUI host is required')
  }
  const { defineComponent, h, ref, onMounted, request } = host
  if ([defineComponent, h, ref, onMounted, request].some(value => typeof value !== 'function')) {
    throw new Error('AnixOps WebUI host does not implement anixops.webui/v1')
  }

  return defineComponent({
    name: 'MachineTelemetryExtension',
    setup() {
      const loading = ref(true)
      const error = ref('')
      const status = ref(null)

      async function refreshTelemetry() {
        loading.value = true
        error.value = ''
        try {
          const value = responseData(await request(STATUS_PATH, { method: 'GET' }))
          if (!value || !Array.isArray(value.nodes) || !value.summary) {
            throw new Error('invalid response')
          }
          status.value = value
        } catch (requestError) {
          status.value = null
          error.value = requestError instanceof Error ? requestError.message : 'request failed'
        } finally {
          loading.value = false
        }
      }

      onMounted(refreshTelemetry)

      return () => {
        if (loading.value) {
          return h('section', { class: 'control-page machine-telemetry-extension' }, [
            h('h1', 'Machine Telemetry'),
            h('p', { role: 'status' }, 'Loading telemetry...'),
          ])
        }
        if (error.value) {
          return h('section', { class: 'control-page machine-telemetry-extension' }, [
            h('h1', 'Machine Telemetry'),
            h('p', { role: 'alert' }, `Telemetry unavailable: ${error.value}`),
          ])
        }
        const value = status.value || { summary: {}, nodes: [] }
        const summaryRows = [
          ['Total', value.summary.total ?? 0],
          ['Online', value.summary.online ?? 0],
          ['Offline', value.summary.offline ?? 0],
          ['Unhealthy', value.summary.unhealthy ?? 0],
        ]
        return h('section', { class: 'control-page machine-telemetry-extension' }, [
          h('div', { class: 'page-header' }, [
            h('div', [
              h('h1', 'Machine Telemetry'),
              h('p', { class: 'description-cell' }, `Observed package ${PLUGIN_VERSION}`),
            ]),
            h('button', { type: 'button', class: 'btn btn-secondary', onClick: refreshTelemetry }, 'Refresh'),
          ]),
          h('div', { class: 'stats-grid' }, summaryRows.map(([label, count]) => h('div', { class: 'stat-item', key: label }, [
            h('span', { class: 'stat-label' }, label),
            h('strong', { class: 'stat-value' }, String(count)),
          ]))),
          h('div', { class: 'table-container' }, [
            h('table', { class: 'data-table' }, [
              h('thead', [h('tr', [
                h('th', 'Node'), h('th', 'State'), h('th', 'CPU'), h('th', 'Memory'), h('th', 'Disk'), h('th', 'Uptime'), h('th', 'Users'),
              ])]),
              h('tbody', value.nodes.length > 0
                ? value.nodes.map(node => h('tr', { key: node.id }, [
                  h('td', [h('strong', node.name || `Node ${node.id}`), h('div', { class: 'description-cell' }, node.host || '')]),
                  h('td', stateLabel(node)),
                  h('td', percent(node.cpu_usage)),
                  h('td', percent(node.memory_usage)),
                  h('td', percent(node.disk_usage)),
                  h('td', uptime(node.uptime)),
                  h('td', String(node.online_users ?? 0)),
                ]))
                : [h('tr', { key: 'empty' }, [h('td', { colspan: 7, class: 'empty-state' }, 'No telemetry')])]),
            ]),
          ]),
        ])
      }
    },
  })
}
