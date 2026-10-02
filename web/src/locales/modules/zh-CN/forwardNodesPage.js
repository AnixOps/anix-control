// 转发节点（UI U7）：NodeX 节点、Ansible 机器、本地运行时与 NodeX 运行时
// 共用的页头、运行方式切换、详情页与运行时页面文案。
export default {
  forwardNodesPage: {
    title: '转发节点',
    modes: {
      label: '运行方式'
    },
    descriptions: {
      nodex: 'NodeX 模式的有状态中继与出口节点，以及旧版端口规则。',
      local: '面板主机上的 Ansible 执行器：无状态，不依赖 NodeX 控制面。',
      nodexRuntime: 'NodeX 控制面的连接配置、探测结果和最近的 gost 任务。'
    },
    nodex: {
      tableLabel: 'NodeX 节点',
      typeFilter: '节点类型',
      statusFilter: '在线状态',
      onlineNote: '“在线”只表示 host:port 能建立 TCP 连接，不代表 gost 或 NodeX 已就绪。',
      columns: {
        name: '节点',
        type: '类型',
        status: '状态',
        managementApi: '管理 API',
        regionIsp: '区域 / ISP',
        latency: '延迟',
        connections: '当前连接',
        traffic: '上传 / 下载',
        weight: '权重 / 最大连接',
        lastCheck: '最近检查',
        uptime: '在线率',
        result: '最近结果'
      },
      summary: {
        relay: '中继',
        exit: '出口',
        online: '在线',
        upload: '上传',
        download: '下载'
      }
    },
    rules: {
      tableLabel: '旧版规则',
      userFilterLabel: '按用户 ID 筛选',
      apply: '筛选',
      columns: {
        name: '规则',
        ingress: '入口',
        egress: '出口',
        owner: '归属',
        limits: '限制',
        traffic: '流量',
        status: '状态'
      }
    },
    detail: {
      sections: '节点详情',
      tabs: {
        overview: '概览',
        config: '配置',
        danger: '危险操作'
      },
      runtime: '运行',
      actions: '操作',
      identity: '节点',
      endpoints: '地址与端口',
      capacity: '容量',
      editConfig: '编辑配置',
      tokenHidden: '已设置（已隐藏）',
      tokenNone: '未设置',
      lastResult: '最近结果',
      noResult: '尚未检查',
      loadFailed: '无法加载节点',
      notFound: '找不到这个节点，它可能已被删除。',
      dangerFooter: '删除后无法撤销。依赖它的转发关系会失效。',
      disableFooter: '禁用后它不再承载新的转发任务，可以随时重新启用。'
    },
    ansibleDetail: {
      loadFailed: '无法加载 Ansible 机器',
      notFound: '找不到这台机器，它可能已被删除。',
      dangerFooter: '删除后无法撤销。它将从 Ansible 执行机群中移除。'
    },
    runtimePage: {
      commandsTitle: '排查命令',
      copyGroup: '复制 {label} 命令',
      copied: '已复制',
      doctorTitle: 'Doctor 输出',
      statusLoadFailed: '无法加载运行时状态',
      emptyStatus: '还没有状态',
      emptyJobs: '暂无运行时任务',
      emptyJobsDescription: '转发、隧道或节点变化时，面板写入的运行时任务会出现在这里。',
      columns: {
        job: '任务',
        target: '对象',
        status: '状态',
        time: '时间',
        message: '结果'
      }
    }
  }
}
