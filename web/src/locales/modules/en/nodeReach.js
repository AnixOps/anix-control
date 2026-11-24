// Node reachability: what the vantages in each region see. Same keys as
// zh-CN/nodeReach.js.
export default {
  nodeReach: {
    column: 'Reachability',
    title: 'Reachability probe',
    description: 'Every 10 minutes, vantage nodes connect to the enabled protocol ports of other nodes (connection only) and compare domestic and overseas results to distinguish node failures from regional unreachability.',
    loading: 'Loading reachability',
    loadFailed: 'Could not load reachability',
    saveFailed: 'Could not save the vantage',
    noAnswer: 'No result, or the last result is older than 30 minutes.',
    role: 'Vantage',
    roles: { none: 'Not a vantage', domestic: 'Domestic vantage', overseas: 'Overseas vantage' },
    roleHint: {
      none: 'This node does not probe other nodes; it is only probed.',
      active: "The Agent of this node probes other nodes every 10 minutes.",
      waiting: "Set as a vantage. Probing starts once the Agent is connected and upgraded to a supported version."
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
