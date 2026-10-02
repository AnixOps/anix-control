export default {
  adminDashboard: {
    title: 'Dashboard',
    subtitle: 'Users, nodes, traffic and tickets at a glance, and what needs you.',
    updatedAt: 'Data from {time}',
    refresh: 'Refresh',
    loadFailed: 'Couldn’t load the dashboard',
    metrics: {
      label: 'Key metrics',
      users: 'Users',
      usersToday: '+{count} today',
      usersDetail: '{active} active · {expired} expired · {banned} banned',
      nodes: 'Nodes online',
      nodesDetail: '{count} users online',
      traffic: 'Traffic today',
      trafficDetail: '{total} in total',
      tickets: 'Open tickets',
      ticketsDetail: '{count} answered',
      ticketsUnknown: 'Tickets didn’t load',
      revenue: 'Revenue this month',
      revenueDetail: '{today} today · {total} in total',
      orders: 'Orders awaiting payment',
      ordersDetail: '{total} in total · {paid} completed'
    },
    traffic: {
      title: 'Traffic, last 24 hours',
      description: 'Hourly traffic of all users.',
      open: 'Traffic & Monitoring',
      label: 'Hourly traffic over the last 24 hours',
      summary: '{total} in total, peak {peak} ({hour})',
      summaryEmpty: 'No traffic in the last 24 hours',
      empty: 'No traffic in the last 24 hours',
      emptyDescription: 'Hourly traffic shows here once nodes report it.',
      loadFailed: 'Couldn’t load traffic',
      hour: 'Hour',
      value: 'Traffic',
      series: 'Traffic'
    },
    commerce: {
      title: 'Revenue and orders'
    },
    alerts: {
      title: 'Needs attention',
      description: 'Offline nodes, tickets waiting for a reply and stalled traffic reports.',
      allClear: 'All clear',
      allClearDescription: 'No node is offline and no ticket is waiting.',
      offlineNode: '{name} is offline',
      lastSeen: 'Last heartbeat {time}',
      neverSeen: 'Never sent a heartbeat',
      moreOffline: '{count} more offline nodes',
      openTickets: '{count} tickets waiting for a reply',
      openTicketsHint: 'The oldest was opened {time}',
      pendingOrders: '{count} orders awaiting payment',
      trafficStale: 'Traffic reports have stopped',
      trafficStaleHint: 'The last report was {time}. Check the node processes and the report path.',
      nodesFailed: 'Couldn’t load node status',
      ticketsFailed: 'Couldn’t load tickets'
    },
    activity: {
      title: 'Recent activity',
      description: 'From the audit log.',
      open: 'Audit log',
      empty: 'No activity yet',
      emptyDescription: 'Administrator actions are recorded here.',
      loadFailed: 'Couldn’t load the audit log',
      actor: '{name}',
      system: 'System'
    }
  }
}
