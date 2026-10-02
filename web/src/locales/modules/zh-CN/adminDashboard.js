export default {
  adminDashboard: {
    title: '仪表盘',
    subtitle: '用户、节点、流量与工单的现状，以及需要你处理的事项。',
    updatedAt: '数据更新于 {time}',
    refresh: '刷新',
    loadFailed: '无法加载仪表盘',
    metrics: {
      label: '关键指标',
      users: '用户',
      usersToday: '今日 +{count}',
      usersDetail: '活跃 {active} · 已到期 {expired} · 已封禁 {banned}',
      nodes: '在线节点',
      nodesDetail: '在线用户 {count}',
      traffic: '今日流量',
      trafficDetail: '累计 {total}',
      tickets: '待处理工单',
      ticketsDetail: '已回复 {count}',
      ticketsUnknown: '工单列表未加载',
      revenue: '本月收入',
      revenueDetail: '今日 {today} · 累计 {total}',
      orders: '待支付订单',
      ordersDetail: '共 {total} · 已完成 {paid}'
    },
    traffic: {
      title: '24 小时流量',
      description: '所有用户每小时的流量。',
      open: '流量与监控',
      label: '最近 24 小时的每小时流量',
      summary: '共 {total}，峰值 {peak}（{hour}）',
      summaryEmpty: '最近 24 小时没有流量',
      empty: '最近 24 小时没有流量',
      emptyDescription: '节点上报流量后，这里会按小时显示。',
      loadFailed: '无法加载流量',
      hour: '时间',
      value: '流量',
      series: '流量'
    },
    commerce: {
      title: '收入与订单'
    },
    alerts: {
      title: '需要处理',
      description: '离线节点、待回复的工单和停止的流量上报。',
      allClear: '一切正常',
      allClearDescription: '没有离线节点，也没有待回复的工单。',
      offlineNode: '{name} 离线',
      lastSeen: '最后心跳 {time}',
      neverSeen: '从未上报心跳',
      moreOffline: '还有 {count} 个离线节点',
      openTickets: '{count} 个工单等待回复',
      openTicketsHint: '最早的一个创建于 {time}',
      pendingOrders: '{count} 个订单待支付',
      trafficStale: '流量上报已停止',
      trafficStaleHint: '最近一次上报是 {time}，请检查节点进程或上报链路。',
      nodesFailed: '无法加载节点状态',
      ticketsFailed: '无法加载工单'
    },
    activity: {
      title: '最近操作',
      description: '来自审计日志。',
      open: '审计日志',
      empty: '还没有操作记录',
      emptyDescription: '管理员的操作会记录在这里。',
      loadFailed: '无法加载审计日志',
      actor: '{name}',
      system: '系统'
    }
  }
}
