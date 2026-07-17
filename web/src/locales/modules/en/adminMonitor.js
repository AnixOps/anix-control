export default {
  adminMonitor: {
    title: 'Real-Time Monitor',
    subtitle: 'Live node status via WebSocket',
    connecting: 'Connecting...',
    connected: 'Connected',
    disconnected: 'Disconnected',
    reconnecting: 'Reconnecting in {seconds}s...',
    overview: {
      title: 'Overview',
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
      upload: 'Upload',
      download: 'Download',
      uptime: 'Uptime'
    },
    status: {
      online: 'Online',
      offline: 'Offline',
      pending: 'Pending'
    },
    empty: 'No nodes to display.',
    errors: {
      connectFailed: 'Failed to connect to monitor WebSocket',
      unsupported: 'WebSocket is not supported in this browser'
    }
  }
}
