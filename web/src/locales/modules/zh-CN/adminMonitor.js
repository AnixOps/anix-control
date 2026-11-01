export default {
  adminMonitor: {
    title: '流量与监控',
    subtitle: '节点的实时状态、按用户的流量、节点延迟和转发路径。',
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
        description: '节点按目标域名上报每个用户的流量，用户可以在“访问统计”页面看到自己的记录。默认关闭，关闭后立即停止记录。',
        enabled: '记录访问统计',
        enabledHint: '节点需要支持 destinations.v1 的 anix-agent。',
        retention: '保留',
        retentionUnit: '天',
        retentionHelp: '7 到 180 天。每个活跃用户每天约占 25–50 KB。',
        save: '保存',
        saved: '访问统计设置已保存',
        saveFailed: '保存设置失败',
        loadFailed: '设置加载失败'
      },
      off: '访问统计已关闭，不再记录新数据；仍显示已保留的记录。'
    },
    categories: {
      qos: {
        title: 'QoS：优先级与限速',
        description: '按流量类型设置优先级：线路忙时低优先级（默认是下载类）让路，空闲时照样跑满。还可以对指定域名按每个用户限速，例如 Steam 下载。设置随节点配置下发，约一分钟内生效，需要支持 qos 的 anixcore。',
        enabled: '启用 QoS',
        enabledHint: '关闭后节点不再区分优先级，也不按类别限速。',
        priorities: '各类型的优先级',
        priority: { high: '高', normal: '普通', low: '低' },
        defaultEgress: '节点下载带宽',
        defaultEgressHelp: '节点出口带宽，用于节点级调度；0 表示只在用户限速内调度。',
        minShare: '低优先级保底',
        minShareHelp: '线路忙时低优先级至少保留的带宽比例，1–90%。',
        nodeEgress: '单个节点的下载带宽',
        nodeEgressHelp: '留空使用上面的默认值。',
        useDefault: '默认',
        caps: '按域名限速（每个用户）',
        capsHelp: '匹配的流量对每个用户合计限速，包括子域名。',
        capName: '名称',
        capLimit: '每用户上限',
        capLimitHelp: '0 表示不限速，只设优先级。',
        capPriority: '优先级',
        capDomains: '域名',
        capDomainsHelp: '每行一个，或用逗号、空格分隔。',
        addCap: '添加限速',
        removeCap: '删除这条限速',
        saved: 'QoS 设置已保存，约一分钟内下发到节点',
        saveFailed: '保存 QoS 设置失败',
        loadFailed: 'QoS 设置加载失败'
      },
      test: {
        title: '测试分类',
        description: '按当前规则判定一个域名、地址或网段属于哪一类。',
        domain: '域名、地址或网段',
        protocol: '协议（可选）',
        run: '判定',
        result: '{target} 属于“{category}”',
        failed: '判定失败'
      },
      rules: {
        title: '自定义规则',
        description: '自定义规则优先于远程规则源和内置规则，立即生效，历史统计也按新规则重新归类。把域名指向“网站”可以把它移出内置类型。',
        label: '自定义分类规则',
        add: '添加规则',
        edit: '编辑规则',
        emptyTitle: '还没有自定义规则',
        emptyDescription: '目前按内置规则和远程规则源分类。',
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
        deleteMessage: '删除后该目标回到远程规则源或内置规则的类型。',
        deleteConfirm: '删除规则',
        deleted: '规则已删除',
        deleteFailed: '删除规则失败'
      },
      sources: {
        title: '远程规则源',
        description: '定时从网址拉取域名和网段列表，整份列表归入一个类型。支持纯域名/地址列表、v2fly domain-list-community（domain:、full:）、Clash 规则（DOMAIN-SUFFIX、IP-CIDR）和 Clash rule-provider。拉取失败时保留上一次的列表。',
        label: '远程规则源',
        add: '添加规则源',
        edit: '编辑规则源',
        emptyTitle: '还没有远程规则源',
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
        deleteMessage: '它拉取的列表一起删除，相关目标回到其他规则的类型。',
        deleteConfirm: '删除规则源',
        deleted: '规则源已删除',
        deleteFailed: '删除规则源失败'
      },
      builtin: {
        title: '内置规则',
        description: '内置在 Control 中的规则，随版本更新；自定义规则和远程规则源可以覆盖它们。',
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
