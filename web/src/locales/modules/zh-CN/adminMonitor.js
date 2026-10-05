export default {
  adminMonitor: {
    title: '流量与监控',
    subtitle: '节点的实时状态、按用户的流量、节点延迟和转发路径。',
    sectionsLabel: '流量与监控分区',
    sections: {
      live: '实时节点',
      traffic: '用户流量'
    },
    range: {
      label: '时间范围',
      '1h': '1 小时',
      '24h': '24 小时',
      '7d': '7 天',
      '30d': '30 天'
    },
    refresh: '刷新',
    live: {
      connection: {
        connecting: '连接中…',
        connected: '实时更新中',
        disconnected: '已断开',
        reconnecting: '{seconds} 秒后重连…'
      },
      overview: {
        label: '节点概况',
        totalNodes: '节点总数',
        onlineNodes: '在线',
        offlineNodes: '离线',
        pendingNodes: '待激活',
        upload: '累计上传 {value}',
        download: '累计下载 {value}'
      },
      table: {
        label: '节点实时状态',
        name: '名称',
        host: '地址',
        status: '状态',
        cpu: 'CPU',
        memory: '内存',
        disk: '磁盘',
        users: '在线用户',
        uptime: '运行时间'
      },
      status: {
        online: '在线',
        offline: '离线',
        pending: '待激活'
      },
      traffic: {
        view: '查看流量',
        openNode: '打开节点页面',
        title: '{name} 的流量',
        description: '上传和下载随时间的变化。协议、凭据和日志在节点页面。'
      },
      waiting: '正在等待第一份节点快照',
      waitingDescription: '连接建立后，节点的状态会实时出现在这里。',
      empty: '还没有节点',
      emptyDescription: '添加节点后，它的 CPU、内存和在线用户会实时显示在这里。',
      offlineTitle: '实时连接已断开',
      offlineDescription: '无法连接实时监控，稍后会自动重连。',
      reconnectNow: '立即重连',
      unsupported: '当前浏览器不支持 WebSocket'
    },
    traffic: {
      user: '用户',
      allUsers: '全部用户',
      showing: '正在查看 {user}',
      clearUser: '查看全部用户',
      summary: {
        label: '流量汇总',
        total: '区间总流量',
        peak: '峰值小时',
        latestReport: '最近上报',
        latestReportHint: '节点最后一次写入流量日志',
        noReport: '暂无流量上报记录'
      },
      chart: {
        title: '每小时流量',
        label: '{user}在{range}内的每小时流量',
        summary: '共 {total}，峰值 {peak}（{hour}）',
        summaryEmpty: '所选区间没有流量',
        series: '流量',
        hour: '时间',
        value: '流量'
      },
      empty: '所选区间暂无流量数据',
      emptyWithLatest: '最近一次上报是 {time}，请检查节点进程或上报链路。',
      emptyNever: '还没有任何节点流量上报记录。',
      loadFailed: '无法加载流量',
      ranking: {
        title: '用户流量排行',
        description: '选择一位用户，在上方图表中查看他的流量。',
        label: '用户流量排行',
        rank: '排名',
        user: '用户',
        traffic: '流量',
        empty: '所选区间暂无用户流量',
        loadFailed: '无法加载用户流量排行'
      }
    }
  }
}
