// Network topology (src/views/admin/Topology.vue).
export default {
  topology: {
    nav: 'Topology',
    title: 'Network topology',
    description: 'Every device and the lines between them. Select a device or a line to see where it goes.',
    loadFailed: 'The topology did not load',
    emptyTitle: 'No devices yet',
    emptyDescription: 'Once the first device joins, its place and lines are drawn here.',
    openNodes: 'Add a device',
    close: 'Close',
    unplaced: '{n} devices have no known place. Put a region in the name or the tags (JP, Tokyo) to map them:',
    view: { label: 'View', map: 'World map', graph: 'Diagram' },
    map: { zoomIn: 'Zoom in', zoomOut: 'Zoom out', fit: 'Fit the devices', world: 'Whole world' },
    roles: { entry: 'Entry', relay: 'Relay', exit: 'Exit', direct: 'Direct' },
    status: { online: 'Online', offline: 'Offline', pending: 'Pending', disabled: 'Disabled' },
    tone: { ok: 'Healthy', warn: 'Needs attention', bad: 'Down', off: 'Paused' },
    legend: { label: 'Legend', line: 'Line', egress: 'Domain egress' },
    metrics: {
      devices: 'Devices online',
      devicesDetail: 'Online / all',
      lines: 'Lines',
      linesDetail: '{n} need attention',
      places: 'Places',
      placesDetail: '{n} not placed',
      users: 'Users online',
      usersDetail: 'Sum of what the devices report'
    },
    lines: { title: 'Lines', empty: 'No relayed lines yet. Users reach every device directly.' },
    line: { kind: 'Kind', route: 'Relayed line', egress: 'Domain egress', traffic: 'Traffic, last 24 hours', open: 'Open the line' },
    device: {
      place: 'Place',
      noPlace: 'Unknown',
      role: 'Role',
      host: 'Address',
      usersLabel: 'Users online',
      users: '{n} online',
      lines: 'Lines through it ({n})',
      noLines: 'No line uses it; users reach it directly.',
      open: 'Open the device'
    }
  }
}
