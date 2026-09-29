export default {
  adminMonitor: {
    title: 'Real-Time Monitor',
    subtitle: 'Live node status via WebSocket',
    connecting: 'Connecting...',
    connected: 'Connected',
    disconnected: 'Disconnected',
    reconnecting: 'Reconnecting in {seconds}s...',
    overview: {
      totalNodes: 'Total Nodes',
      onlineNodes: 'Online',
      offlineNodes: 'Offline',
      pendingNodes: 'Pending',
      totalUpload: 'Total Upload',
      totalDownload: 'Total Download'
    },
    nodeTable: {
      title: 'Nodes',
      name: 'Name',
      host: 'Host',
      status: 'Status',
      cpu: 'CPU',
      memory: 'Memory',
      disk: 'Disk',
      users: 'Users',
      uptime: 'Uptime'
    },
    status: {
      online: 'Online',
      offline: 'Offline',
      pending: 'Pending'
    },
    empty: 'No nodes to display.',
    errors: {
      unsupported: 'WebSocket is not supported in this browser'
    }
  }
}
