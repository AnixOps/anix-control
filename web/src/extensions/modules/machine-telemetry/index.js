export const anixopsExtension = Object.freeze({
  pluginId: 'machine-telemetry',
  version: '1.0.0',
  webuiApiVersion: 'anixops.webui/v1',
  bundle: { path: 'webui/index.mjs' },
})

export default function createMachineTelemetryExtension(host) {
  const { computed, defineComponent, h, onMounted, ref, request } = host

  return defineComponent({
    name: 'MachineTelemetryExtension',
    setup() {
      const loading = ref(true)
      const error = ref('')
      const status = ref(null)
      const nodes = computed(() => status.value?.nodes || [])

      async function refresh() {
        loading.value = true
        error.value = ''
        try {
          status.value = await request('/api/v3/plugins/machine-telemetry/status?limit=200')
        } catch (requestError) {
          error.value = requestError instanceof Error ? requestError.message : 'Telemetry unavailable'
        } finally {
          loading.value = false
        }
      }

      onMounted(refresh)

      return () => h('section', { class: 'control-page machine-telemetry-extension', 'aria-busy': String(loading.value) }, [
        h('div', { class: 'page-header' }, [
          h('div', [h('h1', 'Machine Telemetry')]),
          h('button', { class: 'btn btn-secondary', type: 'button', disabled: loading.value, onClick: refresh }, loading.value ? 'Loading...' : 'Refresh'),
        ]),
        error.value ? h('div', { class: 'alert alert-error', role: 'alert' }, error.value) : null,
        status.value ? h('div', { class: 'stats-grid' }, [
          ['Total', status.value.summary?.total ?? 0],
          ['Online', status.value.summary?.online ?? 0],
          ['Offline', status.value.summary?.offline ?? 0],
          ['Unhealthy', status.value.summary?.unhealthy ?? 0],
        ].map(([label, value]) => h('div', { class: 'stat-item', key: label }, [
          h('span', { class: 'stat-label' }, label),
          h('strong', { class: 'stat-value' }, String(value)),
        ]))) : null,
        h('div', { class: 'table-container' }, [
          h('table', { class: 'data-table' }, [
            h('thead', [h('tr', [
              h('th', 'Node'), h('th', 'State'), h('th', 'CPU'), h('th', 'Memory'), h('th', 'Disk'), h('th', 'Users'),
            ])]),
            h('tbody', nodes.value.length > 0
              ? nodes.value.map(node => h('tr', { key: node.id }, [
                h('td', [h('strong', node.name), h('div', { class: 'description-cell' }, node.host)]),
                h('td', node.online ? (node.runtime_healthy ? 'Online' : 'Unhealthy') : 'Offline'),
                h('td', `${Number(node.cpu_usage || 0).toFixed(1)}%`),
                h('td', `${Number(node.memory_usage || 0).toFixed(1)}%`),
                h('td', `${Number(node.disk_usage || 0).toFixed(1)}%`),
                h('td', String(node.online_users || 0)),
              ]))
              : [h('tr', { key: 'empty' }, [h('td', { colspan: 6, class: 'empty-state' }, loading.value ? 'Loading...' : 'No telemetry')])]),
          ]),
        ]),
      ])
    },
  })
}
