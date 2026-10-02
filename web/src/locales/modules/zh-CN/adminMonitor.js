export default {
  adminMonitor: {
    title: '流量与监控',
    subtitle: '节点的实时状态、按用户的流量、节点延迟和转发路径。',
    sectionsLabel: '流量与监控分区',
    sections: {
      live: '实时节点',
      traffic: '用户流量',
      latency: '节点延迟',
      forward: '转发'
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
    },
    latency: {
      target: '探测目标',
      chart: {
        title: '延迟趋势',
        label: '{target}在{range}内的延迟',
        summary: '平均 {avg}，P95 最高 {p95}',
        summaryEmpty: '所选区间没有采样',
        avg: '平均',
        p95: 'P95',
        max: '最大',
        time: '时间',
        unit: '{value} ms'
      },
      noTargets: '还没有探测目标',
      noTargetsDescription: '转发、隧道或节点激活后，探测目标会自动出现。',
      noData: '所选区间没有采样数据',
      noDataDescription: '探测器按设定的间隔采样，较早的数据会按保留期清理。',
      loadFailed: '无法加载延迟',
      targets: {
        title: '探测目标',
        description: '每个节点探测目标的最新采样。选择一个查看趋势。',
        label: '节点探测目标',
        name: '目标',
        address: '地址',
        latest: '最新延迟',
        loss: '丢包率',
        status: '状态',
        lastSample: '最近采样',
        loadFailed: '无法加载探测目标'
      },
      online: '在线',
      offline: '离线'
    },
    forward: {
      ingress: {
        title: '多入口对比',
        description: '同一个转发经由不同隧道入口的延迟与丢包。',
        forward: '转发',
        selectForward: '选择转发',
        label: '转发入口对比',
        tunnel: '隧道',
        ingress: '入口',
        ingressIp: '入口 IP',
        avgRtt: '平均 RTT',
        loss: '丢包率',
        status: '状态',
        noForwards: '还没有可对比的转发',
        noForwardsDescription: '转发开始探测后会出现在这里。',
        noRows: '这个转发暂无入口数据',
        loadFailed: '无法加载入口对比'
      },
      topology: {
        title: '转发拓扑',
        description: '中转、出口与代理节点之间的路径，以及各节点的延迟。',
        label: '转发拓扑图',
        summary: '{nodes} 个节点，{edges} 条路径',
        legend: '图例',
        relay: '中转',
        exit: '出口',
        proxy: '代理',
        offline: '离线',
        latency: '{value} ms',
        empty: '还没有可展示的拓扑',
        emptyDescription: '添加中转或出口节点后，路径会出现在这里。',
        loadFailed: '无法加载拓扑'
      },
      jobs: {
        title: '运行时作业',
        description: '最近 50 个转发运行时作业。',
        label: '转发运行时作业',
        action: '动作',
        backend: '后端',
        status: '状态',
        createdAt: '创建时间',
        empty: '还没有运行时作业',
        loadFailed: '无法加载运行时作业',
        states: {
          pending: '等待中',
          running: '运行中',
          success: '成功',
          failed: '失败'
        }
      },
      online: '在线',
      offline: '离线'
    }
  }
}
