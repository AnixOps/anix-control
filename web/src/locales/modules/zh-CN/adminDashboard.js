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
      description: '离线节点、待回复的工单、停止的流量上报，以及证书和发布流程告警。',
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
      view: '告警状态',
      viewActive: '进行中',
      viewResolved: '已解决',
      summary: '{count} 条进行中的告警',
      summaryCritical: '{count} 条进行中，{critical} 条严重',
      alertsFailed: '无法加载证书和发布流程告警',
      noResolved: '没有已解决的告警',
      noResolvedDescription: '最近 30 天内已恢复的告警会列在这里。',
      resolvedAt: '{date} 已解决',
      kinds: {
        agentCertificate: {
          ending: '{name} 的 Agent 证书即将到期',
          ended: '{name} 的 Agent 证书已过期',
          endingHint: '{date} 到期。Agent 没有续期：请确认它在运行并能连上 Control。',
          endedHint: '{date} 已到期。Agent 无法连接：请用新的安装命令重新注册。'
        },
        linkCertificate: {
          ending: '{name} 的转发链路证书即将到期',
          ended: '{name} 的转发链路证书已过期',
          endingHint: '{date} 到期。它随 Agent 证书一起续期：请检查 Agent。',
          endedHint: '{date} 已到期。请让 Agent 恢复连接，或重新注册。'
        },
        moduleCertificate: {
          ending: '模块 {name} 的证书即将到期',
          ended: '模块 {name} 的证书已过期',
          endingHint: '{date} 到期。请确认模块在运行并能连上 Control。',
          endedHint: '{date} 已到期。请重启模块、重新注册，或吊销其注册。'
        },
        ca: {
          module: '模块',
          forwardLink: '转发链路',
          ending: '当前{ca} CA 即将到期',
          ended: '当前{ca} CA 已到期',
          staged: '{date} 到期。下一个 CA 已就绪，会自动接替签发。',
          notStaged: '{date} 到期。尚无下一个 CA：请立即轮换 CA。'
        },
        nodeSecretsStalled: {
          title: '{table} 的凭据拆分停在 {phase} 阶段',
          hint: '自 {date} 起没有操作。请校验并完成它，或关闭此提醒。'
        },
        nodeSecretsInterrupted: {
          title: '{table} 的凭据拆分在收尾时被中断',
          hint: '{date} 开始。再运行一次收尾即可继续。'
        },
        identityImport: {
          title: '身份导入已停滞',
          hint: '自 {date} 起没有进展。请继续导入或开始切换。'
        },
        identityCutover: {
          title: '身份切换尚未收尾',
          hint: '身份服务自 {date} 起处理登录。请完成收尾，或回滚。'
        }
      },
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
