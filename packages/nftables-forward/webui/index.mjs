const PLUGIN_ID = 'nftables-forward'
const PLUGIN_VERSION = '1.0.0'
const BUNDLE_PATH = 'webui/index.mjs'
const STATUS_PATH = '/api/v3/plugins/nftables-forward/status?limit=200'

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

function count(value) {
  return typeof value === 'number' && Number.isFinite(value) ? value : 0
}

function stateLabel(rule) {
  if (rule?.rollback_required) {
    return 'Rollback required'
  }
  if (rule?.observed_revision !== rule?.desired_revision) {
    return 'Reconciling'
  }
  return rule?.enabled === false ? 'Disabled' : 'Ready'
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
    name: 'NftablesForwardExtension',
    setup() {
      const loading = ref(true)
      const error = ref('')
      const status = ref(null)

      async function refreshForwarding() {
        loading.value = true
        error.value = ''
        try {
          const value = responseData(await request(STATUS_PATH, { method: 'GET' }))
          if (!value || !Array.isArray(value.rules) || !value.summary) {
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

      onMounted(refreshForwarding)

      return () => {
        if (loading.value) {
          return h('section', { class: 'control-page nftables-forward-extension' }, [
            h('h1', 'nftables Forward'),
            h('p', { role: 'status' }, 'Loading forwarding rules...'),
          ])
        }
        if (error.value) {
          return h('section', { class: 'control-page nftables-forward-extension' }, [
            h('h1', 'nftables Forward'),
            h('p', { role: 'alert' }, `Forwarding status unavailable: ${error.value}`),
          ])
        }
        const value = status.value || { summary: {}, rules: [] }
        const summaryRows = [
          ['Rules', count(value.summary.rules)],
          ['Ready', count(value.summary.ready)],
          ['Reconciling', count(value.summary.reconciling)],
          ['Rollback', count(value.summary.rollback_required)],
        ]
        return h('section', { class: 'control-page nftables-forward-extension' }, [
          h('div', { class: 'page-header' }, [
            h('div', [
              h('h1', 'nftables Forward'),
              h('p', { class: 'description-cell' }, `Dedicated TCP/UDP package ${PLUGIN_VERSION}`),
            ]),
            h('button', { type: 'button', class: 'btn btn-secondary', onClick: refreshForwarding }, 'Refresh'),
          ]),
          h('div', { class: 'stats-grid' }, summaryRows.map(([label, value]) => h('div', { class: 'stat-item', key: label }, [
            h('span', { class: 'stat-label' }, label),
            h('strong', { class: 'stat-value' }, String(value)),
          ]))),
          h('div', { class: 'table-container' }, [
            h('table', { class: 'data-table' }, [
              h('thead', [h('tr', [
                h('th', 'Rule'), h('th', 'Node'), h('th', 'Protocol'), h('th', 'Listen'), h('th', 'Target'), h('th', 'Revision'), h('th', 'State'),
              ])]),
              h('tbody', value.rules.length > 0
                ? value.rules.map(rule => h('tr', { key: rule.id || `${rule.node_id}:${rule.listen}` }, [
                  h('td', [h('strong', rule.name || `Rule ${rule.id}`), h('div', { class: 'description-cell' }, rule.topology || '')]),
                  h('td', rule.node_name || `Node ${rule.node_id ?? 'n/a'}`),
                  h('td', rule.protocol || 'tcp+udp'),
                  h('td', rule.listen || ''),
                  h('td', rule.target || ''),
                  h('td', `${rule.observed_revision ?? 0}/${rule.desired_revision ?? 0}`),
                  h('td', stateLabel(rule)),
                ]))
                : [h('tr', { key: 'empty' }, [h('td', { colspan: 7, class: 'empty-state' }, 'No nftables forwarding rules')])]),
            ]),
          ]),
        ])
      }
    },
  })
}
