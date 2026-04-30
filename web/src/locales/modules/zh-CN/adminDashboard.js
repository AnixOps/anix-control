export default {
  adminDashboard: {
    title: '仪表盘',
    subtitle: '系统概览与实时指标',
    stats: {
      totalUsers: '总用户数',
      todayNewUsers: '今日新增',
      activeUsers: '活跃用户',
      expiredBanned: '过期: {expired} | 封禁: {banned}',
      activeNodes: '活跃节点',
      onlineUsers: '在线用户',
      monthlyIncome: '月收入',
      incomeSummary: '今日: ¥{today} | 总计: ¥{total}'
    },
    orders: {
      title: '订单概览',
      total: '总订单',
      pending: '待支付',
      completed: '已完成'
    },
    traffic: {
      title: '流量统计',
      totalUsed: '总用量',
      today: '今日流量'
    },
    cache: {
      cachedAt: '缓存时间: {time}'
    },
    actions: {
      refresh: '刷新',
      refreshing: '刷新中...'
    },
    messages: {
      fetchFailed: '加载仪表盘数据失败:'
    }
  }
}
