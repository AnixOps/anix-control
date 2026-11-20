// Node reachability: what the vantages in each region see. Same keys as
// zh-CN/nodeReach.js.
export default {
  nodeReach: {
    column: 'Reachability',
    title: 'Reachability probe',
    description: 'Every 10 minutes the vantage nodes connect from their region to the enabled protocol ports of the other nodes (connections only, no data) and compare domestic and overseas answers, to tell a node that is down from one that is unreachable from some regions only.',
    loading: 'Loading reachability',
    loadFailed: 'Could not load reachability',
    saveFailed: 'Could not save the vantage',
    noAnswer: 'No answer yet, or the last one is older than 30 minutes.',
    role: 'Vantage',
    roles: { none: 'Not a vantage', domestic: 'Domestic vantage', overseas: 'Overseas vantage' },
    roleHint: {
      none: 'This node does not probe the others; it is only probed.',
      active: "This node's Agent probes the other nodes every 10 minutes.",
      waiting: "Set as a vantage; it starts once this node's Agent is connected and runs a version with the reachability probe."
    },
    vantage: { domestic: 'Domestic vantage', overseas: 'Overseas vantage' },
    region: { domestic: 'Domestic', overseas: 'Overseas' },
    view: { yes: '{region}: reachable', no: '{region}: unreachable', none: '{region}: no answer' },
    status: {
      ok: 'OK',
      region_unreachable: 'Domestic unreachable',
      overseas_unreachable: 'Overseas unreachable',
      unreachable: 'Unreachable',
      unknown: 'No answer'
    }
  }
}
