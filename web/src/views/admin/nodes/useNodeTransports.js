import { ref } from 'vue'
import { getKernelAgentTransports } from '@/api/kernel'
import { indexTransports, nodeRef } from './agentConnection'

// Loads the Agent connection and certificate of some proxy nodes without
// holding anything up: the node list and the node page render from the v2
// answer and call this afterwards. `status` is 'idle', 'loading', 'ready' or
// 'error'; `entries` maps `proxy-<id>` to the inventory row. A late answer to
// an earlier request (another page, another node) is dropped.
export function useNodeTransports() {
  const entries = ref(new Map())
  const status = ref('idle')
  let sequence = 0

  async function load(ids) {
    const mine = ++sequence
    const names = (Array.isArray(ids) ? ids : []).filter(id => id !== undefined && id !== null && id !== '').map(nodeRef)
    if (!names.length) {
      entries.value = new Map()
      status.value = 'idle'
      return
    }
    status.value = 'loading'
    try {
      const inventory = await getKernelAgentTransports({ nodes: names })
      if (mine !== sequence) return
      entries.value = indexTransports(inventory)
      status.value = 'ready'
    } catch (error) {
      if (mine !== sequence) return
      console.error('Failed to load the Agent connections:', error)
      entries.value = new Map()
      status.value = 'error'
    }
  }

  function reset() {
    sequence += 1
    entries.value = new Map()
    status.value = 'idle'
  }

  return { entries, status, load, reset }
}
