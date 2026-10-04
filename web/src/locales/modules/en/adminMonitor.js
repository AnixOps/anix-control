export default {
  adminMonitor: {
    title: 'Traffic & Monitoring',
    subtitle: 'Live node status, traffic by user, node latency and forward paths.',
    sectionsLabel: 'Traffic and monitoring sections',
    sections: {
      live: 'Live nodes',
      traffic: 'User traffic'
    },
    range: {
      label: 'Time range',
      '1h': '1 h',
      '24h': '24 h',
      '7d': '7 d',
      '30d': '30 d'
    },
    refresh: 'Refresh',
    live: {
      connection: {
        connecting: 'Connecting…',
        connected: 'Live',
        disconnected: 'Disconnected',
        reconnecting: 'Reconnecting in {seconds} s…'
      },
      overview: {
        label: 'Node overview',
        totalNodes: 'Nodes',
        onlineNodes: 'Online',
        offlineNodes: 'Offline',
        pendingNodes: 'Pending',
        upload: '{value} uploaded',
        download: '{value} downloaded'
      },
      table: {
        label: 'Live node status',
        name: 'Name',
        host: 'Address',
        status: 'Status',
        cpu: 'CPU',
        memory: 'Memory',
        disk: 'Disk',
        users: 'Users online',
        uptime: 'Uptime'
      },
      status: {
        online: 'Online',
        offline: 'Offline',
        pending: 'Pending'
      },
      waiting: 'Waiting for the first node snapshot',
      waitingDescription: 'Node status shows here in real time once connected.',
      empty: 'No nodes yet',
      emptyDescription: 'Once you add a node, its CPU, memory and online users show here live.',
      offlineTitle: 'The live connection dropped',
      offlineDescription: 'Couldn’t reach the live monitor. It reconnects on its own.',
      reconnectNow: 'Reconnect now',
      unsupported: 'This browser doesn’t support WebSocket'
    },
    traffic: {
      user: 'User',
      allUsers: 'All users',
      showing: 'Showing {user}',
      clearUser: 'Show all users',
      summary: {
        label: 'Traffic summary',
        total: 'Traffic in range',
        peak: 'Peak hour',
        latestReport: 'Last report',
        latestReportHint: 'When a node last wrote a traffic log',
        noReport: 'No traffic reports yet'
      },
      chart: {
        title: 'Hourly traffic',
        label: 'Hourly traffic of {user} over {range}',
        summary: '{total} in total, peak {peak} ({hour})',
        summaryEmpty: 'No traffic in the selected range',
        series: 'Traffic',
        hour: 'Hour',
        value: 'Traffic'
      },
      empty: 'No traffic data in the selected range',
      emptyWithLatest: 'The last report was {time}. Check the node processes and the report path.',
      emptyNever: 'No node has reported traffic yet.',
      loadFailed: 'Couldn’t load traffic',
      ranking: {
        title: 'Top users by traffic',
        description: 'Pick a user to see their traffic in the chart above.',
        label: 'Users by traffic',
        rank: 'Rank',
        user: 'User',
        traffic: 'Traffic',
        empty: 'No user traffic in the selected range',
        loadFailed: 'Couldn’t load the user ranking'
      }
    }
  }
}
