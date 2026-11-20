import { ref } from 'vue'
import { getKernelReach } from '@/api/kernel'

// Reachability by region for the node list's 可达性 column and the node
// page: one call, after the list has loaded. `status` is 'idle', 'loading',
// 'ready' or 'unavailable'; `nodes` maps a node id to its status and
// `vantages` a node id to its region.
export function useNodeReach() {
  const nodes = ref(new Map())
  const vantages = ref(new Map())
  const status = ref('idle')
  let sequence = 0

  function apply(answer) {
    nodes.value = new Map((answer?.nodes || []).map(node => [Number(node.node_id), node]))
    vantages.value = new Map((answer?.vantages || []).map(v => [Number(v.node_id), v]))
  }

  async function load() {
    const mine = ++sequence
    status.value = 'loading'
    try {
      const answer = await getKernelReach()
      if (mine !== sequence) return
      apply(answer)
      status.value = 'ready'
    } catch {
      if (mine !== sequence) return
      apply(null)
      status.value = 'unavailable'
    }
  }

  return { nodes, vantages, status, load, apply }
}

// The tones of the statuses: a word, never only a colour.
export const REACH_TONES = {
  ok: 'success',
  region_unreachable: 'danger',
  overseas_unreachable: 'warning',
  unreachable: 'danger',
  unknown: 'neutral'
}
