export default {
  adminTrafficHourly: {
    title: '小时流量统计',
    subtitle: '按小时聚合的流量趋势',
    range: {
      label: '时间范围',
      last24h: '最近 24 小时',
      last7d: '最近 7 天',
      last30d: '最近 30 天'
    },
    user: {
      label: '用户',
      all: '全部用户'
    },
    ranking: {
      title: '用户流量排行',
      hint: '点击某行可在上方图表中查看该用户',
      user: '用户',
      traffic: '流量',
      empty: '所选区间暂无用户流量'
    },
    summary: {
      total: '区间总流量',
      peak: '峰值小时',
      peakAt: '峰值时间'
    },
    chart: {
      seriesName: '流量',
      yAxis: '流量'
    },
    actions: {
      refresh: '刷新',
      refreshing: '刷新中...'
    },
    empty: '所选区间暂无流量数据',
    messages: {
      fetchFailed: '加载小时流量失败'
    }
  }
}
