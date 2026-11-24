// The node page's forward ports section on the protocols tab: the port range
// the forward planner allocates from on the node, and its reserved ports.
// Same keys as zh-CN/nodeForwardPorts.js.
export default {
  nodeForwardPorts: {
    title: 'Forward ports',
    description: 'Listening ports for forward routes on this node, excluding reserved ports. Saving replans routes through the node.',
    loading: 'Loading the forward port settings',
    loadFailed: 'Could not load the forward port settings',
    first: 'Port range from',
    last: 'Port range to',
    sourceOwn: 'This is the node\'s own range.',
    sourceBuiltIn: 'No range set; the built-in {first}–{last} applies. Save to set it for this node.',
    sourceNone: 'No range set. Enter both ends.',
    bothEnds: 'Enter both ends of the range',
    reversed: 'The first port cannot be above the last',
    reserved: 'Reserved ports',
    reservedHelp: 'Separated by commas or spaces. SSH (22) and the node\'s own protocol ports are always reserved.',
    reservedInvalid: 'Reserved ports must be whole numbers from 1 to 65535',
    save: 'Save',
    saved: 'Saved. Routes through this node were replanned.'
  }
}
