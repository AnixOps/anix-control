export default {
  adminDashboard: {
    title: '仪表盘',
    subtitle: '用户、节点、流量与工单概况及待处理事项。',
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
      ordersDetail: '共 {total} · 已完成 {paid}',
      backup: '上次验证通过的备份',
      backupNone: '暂无',
      backupAge: '{age}前',
      backupTaken: '备份于 {date}',
      backupStale: '已超过 {hours} 小时',
      backupUnsupported: '备份需要 SQLite',
      backupUnknown: '备份状态未加载'
    },
    traffic: {
      title: '24 小时流量',
      description: '所有用户每小时的流量。',
      open: '流量与监控',
      label: '最近 24 小时的每小时流量',
      summary: '共 {total}，峰值 {peak}（{hour}）',
      summaryEmpty: '最近 24 小时无流量',
      empty: '最近 24 小时无流量',
      emptyDescription: '节点上报流量后按小时显示。',
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
      description: '离线节点、待回复工单、流量上报中断，以及证书与发布流程告警。',
      allClear: '一切正常',
      allClearDescription: '无离线节点，无待回复工单。',
      offlineNode: '{name} 离线',
      lastSeen: '最后心跳 {time}',
      neverSeen: '从未上报心跳',
      moreOffline: '还有 {count} 个离线节点',
      openTickets: '{count} 个工单等待回复',
      openTicketsHint: '最早的一个创建于 {time}',
      pendingOrders: '{count} 个订单待支付',
      trafficStale: '流量上报已停止',
      trafficStaleHint: '最近上报：{time}。请检查节点进程或上报链路。',
      view: '告警状态',
      viewActive: '进行中',
      viewResolved: '已解决',
      summary: '{count} 条进行中的告警',
      summaryCritical: '{count} 条进行中，{critical} 条严重',
      alertsFailed: '无法加载证书和发布流程告警',
      noResolved: '暂无已解决的告警',
      noResolvedDescription: '显示最近 30 天内已恢复的告警。',
      resolvedAt: '{date} 已解决',
      kinds: {
        agentCertificate: {
          ending: '{name} 的 Agent 证书即将到期',
          ended: '{name} 的 Agent 证书已过期',
          endingHint: '{date} 到期，Agent 未续期。请确认 Agent 正在运行且可连接 Control。',
          endedHint: '{date} 已到期，Agent 无法连接。请使用新的安装命令重新注册。'
        },
        linkCertificate: {
          ending: '{name} 的转发链路证书即将到期',
          ended: '{name} 的转发链路证书已过期',
          endingHint: '{date} 到期，随 Agent 证书续期。请检查 Agent。',
          endedHint: '{date} 已到期。请恢复 Agent 连接或重新注册。'
        },
        ca: {
          module: '模块',
          forwardLink: '转发链路',
          ending: '当前{ca} CA 即将到期',
          ended: '当前{ca} CA 已到期',
          staged: '{date} 到期。新 CA 已就绪，将自动接替签发。',
          notStaged: '{date} 到期，新 CA 未就绪。请立即轮换 CA。'
        },
        nodeSecretsStalled: {
          title: '{table} 的凭据拆分停在 {phase} 阶段',
          hint: '自 {date} 起无操作。请校验并完成，或关闭此提醒。'
        },
        nodeSecretsInterrupted: {
          title: '{table} 凭据拆分收尾中断',
          hint: '开始于 {date}。请重新运行收尾。'
        },
        builtinPackageInvalid: {
          title: '内置包 {name} 校验失败',
          hint: '未安装，相关页面和接口不可用。请安装经过校验的发布程序。'
        },
        builtinPackageFailed: {
          title: '内置包 {name} 未在运行',
          hint: '相关页面和接口不可用。请根据 Control 日志排查后重启 Control。'
        },
        panelCertificate: {
          ending: '{name} 的证书即将到期',
          endingHint: '{date} 到期，自动续期失败。请在服务器上运行 anix-control tls status 查看原因。',
          endingFilesHint: '{date} 到期。请替换为续期后的证书文件，面板将自动重新加载。',
          failed: '面板无可用的 {name} 证书',
          renewalFailing: '{name} 的证书续期失败',
          hint: '面板正在使用临时证书。请在服务器上运行 anix-control tls status 查看原因。'
        },
        backupVerifyFailed: {
          title: '备份 {name} 未通过验证',
          hint: '创建于 {date}，不可用于还原。请在服务器上运行 anix-control backup create 并查看 Control 日志。'
        },
        backupStale: {
          title: '最近 {hours} 小时内无验证通过的备份',
          hint: '最近验证通过的备份创建于 {date}。请查看 Control 日志中的备份任务或立即备份。',
          never: '暂无验证通过的备份。请查看 Control 日志中的备份任务或立即备份。'
        },
        exitMainland: {
          title: '{name} 拒绝了访问中国大陆目标的连接（累计 {n} 次）',
          hint: '最近一次：{date}。请检查入口与客户端分流，大陆流量不应到达该出口。'
        },
        crossBorderDown: {
          title: '跨境链路 {name} 自 {date} 起不通',
          hint: '请在“节点 → 过境应急”切换至第一档（REALITY），仍不可达时切换至第二档（XHTTP）。参见 docs/guide/cross-border-standby.md。',
          switched: '流量已切换至备用通道 {tier}。链路恢复后请在“过境应急”切回原链路。'
        },
        crossBorderWide: {
          title: '{total} 条跨境链路中 {n} 条同时不可达',
          hint: '多条链路同时不可达。请按人工预案全部切换至第一档或第二档，并记录现象。'
        }
      },
      nodesFailed: '无法加载节点状态',
      ticketsFailed: '无法加载工单'
    },
    activity: {
      title: '最近操作',
      description: '来自审计日志。',
      open: '审计日志',
      empty: '暂无操作记录',
      emptyDescription: '显示管理员操作记录。',
      loadFailed: '无法加载审计日志',
      actor: '{name}',
      system: '系统'
    }
  }
}
