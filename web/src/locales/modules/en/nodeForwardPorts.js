// The node page's forward ports section on the protocols tab: the port range
// the forward planner allocates from on the node, and its reserved ports.
// Same keys as zh-CN/nodeForwardPorts.js.
export default {
  nodeForwardPorts: {
    title: 'Forward ports',
    description: 'Forward routes listen on this node on ports from this range; reserved ports are never taken. Saving replans every route through the node.',
    loading: 'Loading the forward port settings',
    loadFailed: 'Could not load the forward port settings',
    first: 'Port range from',
    last: 'Port range to',
    sourceOwn: 'This is the node\'s own range.',
    sourceBuiltIn: 'The node has no range of its own and uses the built-in {first}–{last}. Change it and save to keep it as the node\'s own.',
    sourceNone: 'The node has no range of its own. Enter both ends and save.',
    bothEnds: 'Enter both ends of the range',
    reversed: 'The first port cannot be above the last',
    reserved: 'Reserved ports',
    reservedHelp: 'Separated by commas or spaces. SSH (22) and the node\'s own protocol ports are always reserved.',
    reservedInvalid: 'Reserved ports must be whole numbers from 1 to 65535',
    save: 'Save',
    saved: 'Saved; the routes through this node were replanned.'
  }
}
