// The node page's exit policy section on the protocols tab: whether the node
// refuses mainland-China destinations as an exit. Same keys as
// zh-CN/nodeExitPolicy.js.
export default {
  nodeExitPolicy: {
    title: 'Exit policy',
    description: 'What this node refuses when clients leave to the internet through it.',
    loading: 'Loading the exit policy',
    loadFailed: 'Could not load the exit policy',
    blockMainland: 'Refuse mainland-China destinations',
    blockMainlandHelp: 'Off by default. The node refuses every connection to a mainland-China address, so mainland traffic sent here by mistake fails instead of leaving from abroad. Needs an Agent and kernel of 5.0.2 or later; older ones ignore it.',
    blocked: '{n} connections refused, the last at {date}.',
    none: 'No refused connection reported.',
    saveFailed: 'Could not save the exit policy'
  }
}
