export default {
  adminTrafficHourly: {
    title: 'Hourly Traffic',
    subtitle: 'Traffic trend aggregated by hour',
    range: {
      label: 'Time range',
      last24h: 'Last 24 hours',
      last7d: 'Last 7 days',
      last30d: 'Last 30 days'
    },
    summary: {
      total: 'Range total',
      peak: 'Peak hour',
      peakAt: 'Peak time'
    },
    chart: {
      seriesName: 'Traffic',
      yAxis: 'Traffic'
    },
    actions: {
      refresh: 'Refresh',
      refreshing: 'Refreshing...'
    },
    empty: 'No traffic data in the selected range',
    messages: {
      fetchFailed: 'Failed to load hourly traffic'
    }
  }
}
