import { computed, ref } from 'vue'
import { listNodes, listRoutes } from '@/api/forwardV4'

// The forwarding inventory as the node list shows it (one node page for
// every machine, docs/design/node-unification): the regular nodes' relay
// role (their hops and hop errors, by `proxy-<id>`) and the forward hosts
// that are not regular nodes (Ansible hosts and Agent forward nodes not yet
// converted). `available` stays false while the forward API has not
// answered or is not there (no forward package, no permission): the page
// then shows no relay data and no 转发主机 tab. A late answer to an earlier
// load is dropped.
export function useForwardInventory() {
  const nodes = ref([])
  const routes = ref([])
  const canDelete = ref(null)
  const available = ref(false)
  const loading = ref(false)
  let sequence = 0

  async function load() {
    const mine = ++sequence
    loading.value = true
    try {
      const [nodeAnswer, routeAnswer] = await Promise.all([listNodes(), listRoutes()])
      if (mine !== sequence) return
      nodes.value = nodeAnswer.nodes
      canDelete.value = nodeAnswer.canDelete
      routes.value = routeAnswer.routes
      available.value = true
    } catch {
      if (mine !== sequence) return
      nodes.value = []
      routes.value = []
      available.value = false
    } finally {
      if (mine === sequence) loading.value = false
    }
  }

  // proxy-<id> → its inventory row, for the regular nodes in the list.
  const byRef = computed(() => new Map(nodes.value.map(node => [node.node_ref, node])))
  // The machines that are not regular nodes.
  const hosts = computed(() => nodes.value.filter(node => node.kind === 'forward'))
  // Of those, the Agent forward nodes waiting for 转为常规节点.
  const unconverted = computed(() => hosts.value.filter(node => node.record?.transport !== 'NODE_TRANSPORT_ANSIBLE'))

  return { nodes, routes, canDelete, available, loading, byRef, hosts, unconverted, load }
}
