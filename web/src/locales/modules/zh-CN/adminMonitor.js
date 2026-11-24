export default {
  adminMonitor: {
    title: '流量与监控',
    subtitle: '节点实时状态、用户流量、节点延迟与转发路径。',
    sectionsLabel: '流量与监控分区',
    sections: {
      live: '实时节点',
      traffic: '用户流量',
      destinations: '访问统计',
      categories: '流量分类'
    },
    range: {
      label: '时间范围',
      '1h': '1 小时',
      '24h': '24 小时',
      '7d': '7 天',
      '30d': '30 天'
    },
    destinations: {
      user: '用户',
      allUsers: '全部用户',
      settings: {
        title: '访问统计',
        description: '节点按目标域名上报各用户流量，用户可在“访问统计”页查看本人记录。默认关闭，关闭后立即停止记录。',
        enabled: '记录访问统计',
        enabledHint: '节点需要支持 destinations.v1 的 anix-agent。',
        retention: '保留',
        retentionUnit: '天',
        retentionHelp: '7–180 天。每个活跃用户每天约 25–50 KB。',
        save: '保存',
        saved: '访问统计设置已保存',
        saveFailed: '保存设置失败',
        loadFailed: '设置加载失败'
      },
      off: '访问统计已关闭，不再记录新数据，已保留的记录仍显示。'
    },
    categories: {
      qos: {
        title: 'QoS：优先级与限速',
        description: '按流量类型设置优先级：线路繁忙时低优先级流量（默认为下载类）让行，空闲时不受限制；可对指定域名按用户限速。设置随节点配置下发，约一分钟内生效，需 anixcore 支持 qos。',
        enabled: '启用 QoS',
        enabledHint: '关闭后节点不区分优先级，也不按类别限速。',
        priorities: '各类型的优先级',
        priority: { high: '高', normal: '普通', low: '低' },
        defaultEgress: '节点下载带宽',
        defaultEgressHelp: '节点出口带宽，用于节点级调度；0 表示仅在用户限速内调度。',
        minShare: '低优先级保底',
        minShareHelp: '线路繁忙时低优先级保留的最低带宽比例，1–90%。',
        nodeEgress: '单个节点的下载带宽',
        nodeEgressHelp: '留空使用上面的默认值。',
        useDefault: '默认',
        caps: '按域名限速（每个用户）',
        capsHelp: '对每个用户的匹配流量（含子域名）合计限速。',
        capName: '名称',
        capLimit: '每用户上限',
        capLimitHelp: '0 表示不限速，只设优先级。',
        capPriority: '优先级',
        capDomains: '域名',
        capDomainsHelp: '每行一个，或以逗号、空格分隔。',
        addCap: '添加限速',
        removeCap: '删除此限速',
        saved: 'QoS 设置已保存，约一分钟内下发至节点',
        saveFailed: '保存 QoS 设置失败',
        loadFailed: 'QoS 设置加载失败'
      },
      test: {
        title: '测试分类',
        description: '按当前规则判定域名、地址或网段的类别。',
        domain: '域名、地址或网段',
        protocol: '协议（可选）',
        run: '判定',
        result: '{target} 属于“{category}”',
        failed: '判定失败'
      },
      rules: {
        title: '自定义规则',
        description: '自定义规则优先于远程规则源与内置规则，立即生效，历史统计按新规则重新归类。',
        label: '自定义分类规则',
        add: '添加规则',
        edit: '编辑规则',
        emptyTitle: '暂无自定义规则',
        emptyDescription: '当前按内置规则与远程规则源分类。',
        loadFailed: '规则加载失败',
        kind: '类型',
        value: '值',
        category: '归入',
        note: '备注',
        kinds: { domain: '域名', network: '网段', protocol: '协议' },
        valueHelp: {
          domain: '如 example.com，同时匹配它的子域名。',
          network: '如 203.0.113.0/24 或单个地址。',
          protocol: '节点嗅探到的协议，如 bittorrent。'
        },
        saved: '规则已保存',
        saveFailed: '保存规则失败',
        deleteTitle: '删除规则 {value}？',
        deleteMessage: '删除后该目标恢复为远程规则源或内置规则的类型。',
        deleteConfirm: '删除规则',
        deleted: '规则已删除',
        deleteFailed: '删除规则失败'
      },
      sources: {
        title: '远程规则源',
        description: '定时拉取域名与网段列表，整份列表归入同一类型。支持纯域名/地址列表、v2fly domain-list-community、Clash 规则与 Clash rule-provider。拉取失败时保留上次列表。',
        label: '远程规则源',
        add: '添加规则源',
        edit: '编辑规则源',
        emptyTitle: '暂无远程规则源',
        emptyDescription: '例如 https://raw.githubusercontent.com/v2fly/domain-list-community/master/data/netflix 归入“视频”。',
        name: '名称',
        url: '网址',
        category: '归入',
        enabled: '启用',
        refreshHours: '更新间隔',
        refreshUnit: '小时',
        refreshHelp: '1 到 168 小时。',
        entries: '条目',
        fetchedAt: '上次拉取',
        never: '尚未拉取',
        status: '状态',
        ok: '正常',
        failed: '失败',
        disabled: '已停用',
        refresh: '立即拉取',
        refreshed: '已拉取 {count} 条',
        refreshFailed: '拉取失败：{error}',
        saved: '规则源已保存，将在 10 分钟内拉取',
        saveFailed: '保存规则源失败',
        deleteTitle: '删除规则源 {name}？',
        deleteMessage: '已拉取的列表将一并删除，相关目标恢复为其他规则的类型。',
        deleteConfirm: '删除规则源',
        deleted: '规则源已删除',
        deleteFailed: '删除规则源失败'
      },
      builtin: {
        title: '内置规则',
        description: 'Control 内置规则，随版本更新，可被自定义规则与远程规则源覆盖。',
        counts: '{domains} 个域名 · {networks} 个网段 · {protocols} 个协议',
        show: '查看',
        hide: '收起'
      },
      cancel: '取消',
      save: '保存',
      loadFailed: '分类规则加载失败'
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
        description: '上传与下载随时间的变化。'
      },
      waiting: '正在等待第一份节点快照',
      waitingDescription: '连接建立后实时显示节点状态。',
      empty: '暂无节点',
      emptyDescription: '添加节点后实时显示 CPU、内存与在线用户。',
      offlineTitle: '实时连接已断开',
      offlineDescription: '实时监控连接失败，将自动重连。',
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
        latestReportHint: '节点最近写入流量日志的时间',
        noReport: '暂无流量上报记录'
      },
      chart: {
        title: '每小时流量',
        label: '{user}在{range}内的每小时流量',
        summary: '共 {total}，峰值 {peak}（{hour}）',
        summaryEmpty: '所选区间无流量',
        series: '流量',
        hour: '时间',
        value: '流量'
      },
      empty: '所选区间暂无流量数据',
      emptyWithLatest: '最近上报：{time}。请检查节点进程或上报链路。',
      emptyNever: '暂无节点流量上报记录。',
      loadFailed: '无法加载流量',
      ranking: {
        title: '用户流量排行',
        description: '选择用户以在上方图表中查看其流量。',
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
