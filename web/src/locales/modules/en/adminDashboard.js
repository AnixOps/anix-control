export default {
  adminDashboard: {
    title: 'Dashboard',
    subtitle: 'System overview and real-time metrics',
    stats: {
      totalUsers: 'Total Users',
      todayNewUsers: 'New today',
      activeUsers: 'Active Users',
      expiredBanned: 'Expired: {expired} | Banned: {banned}',
      activeNodes: 'Active Nodes',
      onlineUsers: 'Online',
      monthlyIncome: 'Monthly Income',
      incomeSummary: 'Today: ¥{today} | Total: ¥{total}'
    },
    orders: {
      title: 'Orders',
      total: 'Total',
      pending: 'Pending',
      completed: 'Completed'
    },
    traffic: {
      title: 'Traffic',
      totalUsed: 'Total Used',
      today: 'Today'
    },
    cache: {
      cachedAt: 'Cached at {time}'
    },
    actions: {
      refresh: 'Refresh',
      refreshing: 'Refreshing...'
    },
    messages: {
      fetchFailed: 'Failed to load dashboard data:'
    }
  }
}
