// The node page's exit policy section on the protocols tab: whether the node
// refuses mainland-China destinations as an exit. Same keys as
// zh-CN/nodeExitPolicy.js.
export default {
  nodeExitPolicy: {
    title: 'Exit policy',
    description: 'Destinations this node refuses.',
    loading: 'Loading the exit policy',
    loadFailed: 'Could not load the exit policy',
    blockMainland: 'Refuse mainland-China destinations',
    blockMainlandHelp: 'Off by default. When on, connections to mainland-China addresses are refused. Requires Agent and kernel 5.0.2 or later.',
    blocked: '{n} connections refused, the last at {date}.',
    none: 'No refused connection reported.',
    saveFailed: 'Could not save the exit policy'
  }
}
