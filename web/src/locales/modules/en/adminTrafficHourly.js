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
    user: {
      label: 'User',
      all: 'All users'
    },
    ranking: {
      title: 'User Traffic Ranking',
      hint: 'Click a row to view that user in the chart above',
      user: 'User',
      traffic: 'Traffic',
      empty: 'No user traffic in the selected range'
    },
    summary: {
      total: 'Range total',
      peak: 'Peak hour',
      peakAt: 'Peak time',
      latestReport: 'Latest report',
      latestReportHint: 'Last traffic log written by a node',
      noReport: 'No traffic reports yet'
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
    emptyWithLatest: 'Latest report was {time}. Check node processes or the reporting path.',
    emptyNever: 'No node traffic report has been recorded yet.',
    messages: {
      fetchFailed: 'Failed to load hourly traffic',
      rankingFetchFailed: 'Failed to load user traffic ranking'
    }
  }
}
