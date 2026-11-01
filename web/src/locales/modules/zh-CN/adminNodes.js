// admin.nodes：节点列表与节点详情页（UI U7）。
import { AGENT_NAME } from '../../../constants/brand'

export default {
  title: '节点',
  subtitle: '为订阅提供服务的代理节点。打开节点查看协议、凭据、部署和日志。',
  confirm: {
    deleteNodeTitle: '删除节点 {name}？',
    deleteNodeMessage: '节点和它的凭据会被移除。此操作无法撤销。',
    deleteNodeAction: '删除节点',
    disableNodeTitle: '停用节点 {name}？',
    disableNodeMessage: '订阅将不再提供这个节点，已签发给它的 Agent 证书会被吊销。',
    disableNodeAction: '停用节点',
    deleteProtocolTitle: '删除协议 {type} :{port}？',
    deleteProtocolMessage: '节点 {name} 将不再提供这个协议。此操作无法撤销。',
    deleteProtocolAction: '删除协议'
  },
  stats: {
    total: '全部',
    online: '在线',
    offline: '离线',
    pending: '待激活'
  },
  filters: {
    label: '按状态筛选',
    search: '搜索节点',
    searchPlaceholder: '名称或地址'
  },
  table: {
    label: '节点',
    id: 'ID',
    name: '名称',
    address: '地址',
    status: '状态',
    runtimeHealthy: '运行时正常',
    runtimeUnhealthy: '运行时异常',
    parent: '上级节点',
    protocols: '协议数',
    agentVersion: 'Agent 版本',
    kernelVersion: '内核版本',
    kernelUnknown: '未上报',
    kernelUnknownHint: '节点在线时由 Agent 上报 anx-node 版本；旧版 Agent 不上报，升级 Agent 后显示。',
    load: '负载',
    loadValue: 'CPU {cpu} · 内存 {memory}',
    traffic: '累计流量',
    monthlyQuota: '月流量限额',
    quotaExceeded: '本月超限',
    lastHeartbeat: '最后心跳',
    lastSeen: '最后在线',
    nodeGroup: '节点组',
    connection: '连接方式',
    certificate: '证书',
    serviceStatus: '服务状态',
    never: '从未',
    empty: '还没有节点',
    emptyDescription: '点“接入节点”，在机器上运行一条命令，节点会自动出现在这里。',
    loadFailed: '无法加载节点'
  },
  // The 服务状态 column: the machine-telemetry services summary of a node.
  serviceStatus: {
    states: {
      healthy: '{active}/{total} 正常',
      partial: '{active}/{total} 运行中',
      failed: '{failed} 个失败',
      empty: '无服务',
      waiting: '等待上报',
      unsupported: '不支持',
      stale: '数据过期'
    },
    counts: '共 {total} 个服务：{active} 个运行中，{failed} 个失败',
    loading: '正在加载服务状态',
    unavailable: '服务状态不可用：需要已启用的机器遥测插件和它的 API 权限',
    notEnabled: '此节点未启用服务采集'
  },
  actions: {
    open: '打开节点',
    manageProtocols: '管理协议',
    syncReload: '同步并重载',
    upgradeKernel: '升级内核',
    logs: '查看日志',
    edit: '编辑',
    delete: '删除'
  },
  detail: {
    title: '节点详情',
    back: '节点',
    loading: '正在加载节点…',
    loadFailed: '无法加载这个节点',
    notFound: '节点 #{id} 不存在',
    notFoundHint: '它可能已被删除。请返回节点列表。',
    sections: '节点分区',
    sectionNames: {
      overview: '概览',
      protocols: '协议',
      credentials: '凭据',
      deploy: '部署',
      traffic: '流量',
      diagnostics: '诊断',
      reality: '目标站',
      logs: '日志',
      services: '服务',
      danger: '危险区'
    }
  },
  overview: {
    health: '健康',
    traffic: '流量',
    config: '设置',
    never: '还没有心跳',
    os: '操作系统',
    serverIp: '服务器 IP',
    uptime: '运行时间',
    onlineUsers: '在线用户',
    cpu: 'CPU',
    memory: '内存',
    disk: '磁盘',
    totalUpload: '累计上传',
    totalDownload: '累计下载',
    unlimited: '不限 · 本月已用 {used}',
    resetDay: '每月 {day} 日',
    quotaFooter: '超出月流量限额的节点只做标记，不会自动限制。',
    overQuota: '这个节点本月流量已超限。仅作标记，不会自动限制。',
    runtimeError: '节点报告运行时异常：{message}',
    rootNode: '无（落地节点）',
    registered: '添加方式',
    autoRegistered: `${AGENT_NAME} 自动注册`,
    manual: '手动添加',
    createdAt: '创建时间'
  },
  agent: {
    title: 'Agent 连接',
    retry: '重试',
    loading: '正在加载 Agent 连接…',
    unavailable: '暂不可用',
    loadFailed: '无法加载 Agent 连接',
    noRecord: '还没有 Agent 记录',
    noRecordHint: '该节点还没有注册 Agent，可在“部署”页安装。',
    connectionLabel: '连接方式',
    transport: '最近通道',
    lastSeen: '最近出现',
    neverSeen: '从未',
    certificateLabel: '证书',
    notAfter: '有效期至',
    renewAfter: '续期时间',
    revokedAt: '吊销时间',
    revokeReason: '吊销原因',
    overdueNotice: 'Agent 应在 {date} 之后续期证书。证书仍然有效，但 Agent 没有按时续期。',
    revokedNotice: '该 Agent 的证书已被吊销，Agent 必须重新注册。',
    footer: '数据来自 Agent 连接清单。实时连接只有持有该连接的 Control 进程才知道。',
    connection: {
      mtls_stream: 'mTLS 流',
      offline: '离线'
    },
    connectionHint: {
      mtls_stream: '用 Agent 证书认证的 Agent Control 流',
      offline: '最近五分钟内没有任何连接'
    },
    certificate: {
      valid: '有效',
      overdue: '续期逾期',
      revoked: '已吊销',
      expired: '已过期',
      none: '无证书',
      until: '至 {date}',
      expiredOn: '{date} 过期',
      revokedOn: '{date} 吊销'
    }
  },
  form: {
    titleEdit: '编辑节点',
    save: '保存更改',
    required: '必填',
    parentNone: '无（作为根 / 落地节点）',
    parentHint: '选择上级节点可组成多级中转链路；本节点转发的流量会同时计入自己和每一级上级节点。',
    fields: {
      name: '名称',
      address: '地址',
      tags: '标签',
      rate: '流量倍率',
      sort: '排序',
      status: '状态',
      parent: '上级节点',
      monthlyLimit: '月流量限额',
      monthlyResetDay: '每月重置日'
    },
    placeholders: {
      name: 'hk-01',
      address: 'IP 或域名',
      tags: '香港,IEPL,高速'
    },
    help: {
      rate: '用量按这个倍率计算。',
      sort: '数字小的排在前面。',
      monthlyLimit: '留空表示不限。',
      monthlyResetDay: '1 到 28。'
    }
  },
  install: {
    title: '换到新机器',
    description: '用一条命令把节点 {node} 装到另一台机器上。',
    intro: '生成一个绑定这个节点的一次性令牌，以及每个下载源的安装命令。以 root（sudo）在新机器上粘贴运行：脚本安装 Agent、清理这台机器上的旧转发运行时，并等待 Agent 完成接入。再次运行同一条命令是安全的。新机器上的节点会取代原来的机器。',
    ttl: '令牌有效期',
    ttlOptions: {
      hour: '1 小时（默认）',
      sixHours: '6 小时',
      day: '24 小时',
      week: '7 天（最长）'
    },
    generate: '生成安装命令',
    regenerate: '重新生成',
    commandTitle: '安装命令',
    once: '令牌只在这里显示一次，只能使用一次，{time} 失效。关闭后无法再次查看。',
    mirror: '下载源',
    mirrors: {
      control: '控制面',
      cn: '国内镜像',
      github: 'GitHub',
      controlHelp: '脚本和 Agent 从这个控制面下载。',
      cnHelp: '脚本从这个控制面下载，Agent 从国内镜像下载；校验和始终来自控制面或 GitHub。',
      githubHelp: '脚本和 Agent 从 GitHub Releases 下载；令牌仍向这个控制面注册。'
    },
    commandLabel: '在节点上以 root 运行',
    copy: '复制命令',
    fallback: '这个下载源尚未配置：{note}',
    signed: '安装脚本已签名：可用 /install.sh.sig 和官方发布公钥校验后再运行。',
    unsigned: '这个控制面没有安装脚本的发布签名；需要校验时请使用 GitHub Releases 中的 agent-install.sh 及其 .sig。',
    legacy: '安装时会删除这台机器上的旧转发运行时（nftables 表 inet v2b_forward、ip v2b_forward、ip anixops_forward 和 v2forward-agent 服务），并在结束时列出删除的内容。',
    failed: '无法生成安装命令'
  },

  deploySection: {
    title: '部署',
    description: `管理这个节点的 ${AGENT_NAME}。新机器不在这里添加：到“节点 › 接入节点”。`,
    move: '换到新机器',
    moveText: '把这个节点移到另一台机器，或在证书失效后重新接入。生成一条一次性命令，在新机器上以 root 运行。',
    moveAction: '生成安装命令',
    certificate: '证书',
    certificateText: '节点用证书连接面板，Agent 会在到期前自动续期。',
    certificateGone: '证书已失效，这个节点的 Agent 连不上。用“换到新机器”重新接入。',
    certificateNone: '这个节点还没有 Agent 证书。用“换到新机器”接入一台机器。',
    uninstall: '卸载',
    uninstallText: '在这台节点机器上运行。加 --purge 会同时删除它的身份和数据。',
    uninstallLabel: '在节点上以 root 运行',
    copy: '复制命令'
  },
  traffic: {
    title: '流量',
    description: '这个节点上的用户经过它的流量，已按节点流量倍率计算。',
    range: '时间范围',
    ranges: { '24h': '24 小时', '7d': '7 天', '30d': '30 天', '90d': '90 天', '1y': '1 年' },
    rangeNames: {
      '24h': '最近 24 小时',
      '7d': '最近 7 天',
      '30d': '最近 30 天',
      '90d': '最近 90 天',
      '1y': '最近 1 年'
    },
    refresh: '刷新',
    summaryLabel: '流量合计',
    upload: '上传',
    download: '下载',
    total: '合计',
    window: '{from} 至 {to}',
    chart: {
      title: '流量走势',
      label: '{node} 的流量，{range}',
      summary: '上传 {up}，下载 {down}。最忙的{bucket}：{peak}，开始于 {time}。',
      summaryEmpty: '没有流量。',
      bucketHour: '一小时',
      bucketDay: '一天',
      time: '时间'
    },
    empty: '这个时间范围内没有流量',
    emptyHour: '这些小时里没有收到上报。小时数据只保留到流量日志的保留期限。',
    emptyDay: '这些天里没有记录。',
    loadFailed: '无法加载流量',
    noteHour: '按小时（UTC 整点）汇总流量日志，按你的时区显示。清理流量日志之前的时间没有数据。',
    noteDay: '按天（Control 主机的日历日）读取每日统计，不受流量日志保留期限影响。'
  },
  rotate: {
    title: 'Agent 凭据',
    row: '轮换 Agent 凭据',
    rowHint: '吊销这个节点的 Agent 持有的凭据，并签发一个一次性凭据让它重新注册。',
    footer: '仅限超级管理员。轮换连同你填写的原因会记入审计日志，凭据本身不会。',
    action: '轮换凭据…',
    superOnly: '只有超级管理员可以轮换凭据。',
    disabledHint: '这个节点已停用。请先启用它，再轮换凭据。',
    confirm: {
      title: '轮换 {name} 的凭据？',
      message: 'Agent 需要重新注册。可以随时再次轮换，只有最后一个凭据有效。',
      action: '轮换凭据',
      impact: '会发生什么',
      revoke: '这个节点所有 Agent 证书、注册记录和转发链路证书都会被吊销。正在运行的 Agent 会在下一次心跳时断开连接。',
      issue: '签发一个新的一次性凭据，只显示一次。之前没用过的凭据随即失效。',
      offline: '不会联系节点，所以节点离线时也能轮换：它的 Agent 下次连接时才会发现。',
      reason: '原因',
      reasonHelp: '可选，记入审计日志。{count} / {max} 字节（一个汉字占 3 字节）。',
      reasonTooLong: '原因太长：{count} / {max} 字节。',
      reasonPlaceholder: '例如：主机的硬盘被盗',
      ttl: '凭据有效期'
    },
    result: {
      title: '{name} 的新凭据',
      description: '只显示一次。关闭此对话框后无法再次查看。',
      warning: '请像密码一样保管。在它被使用或过期之前，任何拿到它的人都能以这个节点的身份注册 Agent：不要分享，也不要粘贴到聊天、工单或文件里。',
      label: '一次性凭据',
      copy: '复制凭据',
      expires: '有效期至 {time}',
      remaining: '还剩 {remaining}',
      expired: '这个凭据已过期。请再次轮换以获取新的凭据。',
      single: '只能使用一次。',
      revoked: '已吊销',
      certificates: 'Agent 证书',
      enrollments: '注册记录',
      linkCertificates: '转发链路证书',
      next: '把这个凭据交给机器，让 Agent 重新注册。指南里有已有身份的节点重新注册的步骤。',
      guide: '轮换节点凭据（指南）',
      done: '完成'
    },
    errors: {
      super_admin_required: '只有超级管理员可以轮换凭据。',
      node_not_found: '这个节点已不存在。',
      node_disabled: '这个节点已停用。请先启用它。',
      agent_pki_disabled: '内置 Agent CA 未开启（module_runtime.ca_kek），Control 无法签发凭据。',
      failed: '无法轮换凭据：{message}'
    }
  },
  credentials: {
    title: '凭据',
    description: `${AGENT_NAME} 用 mTLS 证书登录控制面，节点没有 API 密钥。`,
    nodeId: '节点 ID',
    protocolSecrets: '协议密钥',
    protocolSecretsRow: '打开协议',
    protocolSecretsFooter: '协议设置里的私钥和密码显示为 ********；保留 ******** 即沿用已保存的值。'
  },
  danger: {
    title: '危险区',
    description: '这些操作会改变订阅提供的内容。',
    disable: '停用节点',
    disableHint: '订阅不再提供它，它的 Agent 证书会被吊销。',
    disableAction: '停用…',
    enable: '启用节点',
    enableHint: '节点回到待激活，下一次心跳后显示在线。',
    enableAction: '启用',
    delete: '删除节点',
    deleteHint: '移除节点、它的协议和凭据。需要输入节点名称确认。',
    deleteAction: '删除…'
  },
  protocols: {
    title: '协议',
    description: '这个节点提供的协议。改动在下一次同步时下发到节点。',
    tableLabel: '{name} 的协议',
    add: '添加协议',
    empty: '还没有协议',
    emptyDescription: '添加协议或从模板开始，订阅才能使用这个节点。',
    loadFailed: '无法加载协议',
    enabled: '启用',
    disabled: '停用',
    listed: '显示',
    hidden: '隐藏',
    columns: {
      type: '协议',
      port: '端口',
      transport: '传输',
      tls: 'TLS',
      status: '状态',
      show: '订阅中'
    }
  },
  services: {
    title: '服务',
    description: '此节点的 systemd 服务（只读）：状态、最近 10 分钟的 CPU 和内存。',
    tableLabel: '{name} 的服务',
    refresh: '刷新服务',
    settings: '设置',
    loadFailed: '无法加载服务',
    totals: "总计 {total} {'|'} 失败 {failed} {'|'} 每 10 分钟更新一次",
    observedAt: '上报于 {time}',
    stale: '最近一次上报在 {time}。表格可能已过时：节点或其采集器可能已离线。',
    unsupported: '此节点无法上报服务',
    unsupportedReason: '原因：{reason}',
    unsupportedNoReason: '节点需要 systemd 和 cgroup v2。',
    waiting: '正在等待首次上报',
    waitingDescription: '启用采集后，节点大约每分钟发送一次服务表。',
    empty: '没有符合节点筛选规则的服务',
    emptyDescription: '请在设置中修改包含或排除规则。',
    disabled: {
      title: '此节点未启用',
      description: '服务表默认关闭。启用后会采集此节点 systemd 服务的单元名称、状态、CPU 和内存，不采集其他内容。',
      enable: '为此节点启用'
    },
    filters: {
      label: '按状态筛选',
      all: '全部',
      search: '搜索服务',
      searchPlaceholder: '单元名称'
    },
    states: {
      active: '运行中',
      failed: '失败',
      inactive: '未运行',
      activating: '启动中',
      deactivating: '停止中',
      reloading: '重新加载中'
    },
    columns: {
      name: '服务',
      state: '状态',
      cpuAvg: 'CPU（10 分钟平均）',
      cpuPeak: 'CPU 峰值',
      memory: '内存',
      memoryPeak: '内存峰值'
    },
    form: {
      title: '服务设置',
      description: '节点 {name}',
      enabled: '采集此节点的服务',
      enabledHelp: '只读：服务表不会启动、停止或重启任何服务。',
      include: '包含规则',
      includeHelp: '每行一条，例如 nginx*.service。留空则包含所有服务。',
      exclude: '排除规则',
      excludeHelp: '每行一条。匹配的服务不会显示。',
      invalidGlob: '规则无效：{glob}',
      tooManyGlobs: '最多 {max} 条规则',
      save: '保存',
      saved: '服务设置已保存，节点会在下次接收配置时应用。',
      saveFailed: '无法保存服务设置',
      conflict: '设置已在别处修改，已重新加载最新设置，请检查后再次保存。',
      noInstallation: '机器遥测没有 Agent 安装。请先在插件中安装。'
    }
  },
  logs: {
    title: '日志',
    description: '节点上报的运行日志，最新的在前。',
    tableLabel: '{name} 的日志',
    refresh: '刷新日志',
    empty: '这个节点还没有日志',
    emptyDescription: '节点上报日志后会显示在这里。',
    loadFailed: '无法加载日志',
    filters: {
      allLevels: '全部级别',
      search: '搜索日志',
      sourcePlaceholder: '来源',
      searchPlaceholder: '内容、来源或 trace ID'
    },
    levels: {
      debug: '调试',
      info: '信息',
      warning: '警告',
      error: '错误'
    },
    columns: {
      time: '时间',
      level: '级别',
      source: '来源',
      message: '内容',
      fields: '结构化字段'
    }
  },
  protocolForm: {
    titleCreate: '添加协议',
    titleEdit: '编辑协议',
    description: '节点 {name}',
    create: '添加协议',
    save: '保存协议',
    templateLibrary: '从模板开始',
    maskedSecretsHint: '已保存的密钥（私钥、密码、令牌）显示为 ********。保留 ******** 即沿用原值，填写新值则替换。',
    modeLabel: '编辑方式',
    jsonLabel: '协议 JSON',
    tabs: {
      visual: '表单',
      json: '高级 JSON'
    },
    fields: {
      type: '协议',
      port: '监听端口',
      tls: 'TLS',
      transport: '传输',
      settings: '协议设置（JSON）',
      tlsSettings: 'TLS 设置（JSON）',
      realitySettings: 'Reality 设置（JSON）',
      transportSettings: '传输设置（JSON）'
    },
    jsonActions: {
      format: '格式化',
      copy: '复制',
      fromTemplate: '从模板加载'
    },
    templatePicker: {
      title: '从模板加载',
      description: '选择一个模板，编辑器中的 JSON 会被替换。',
      label: '模板',
      placeholder: '选择模板',
      required: '请先选择要加载的模板。',
      apply: '加载模板'
    },
    jsonStatus: {
      valid: 'JSON 有效',
      invalid: 'JSON 无效'
    },
    iwan: {
      title: 'Panabit iWAN',
      unencrypted: '不加密',
      unencryptedHint: 'iWAN 协议没有加密和完整性校验，流量可被线路上的设备看到和篡改，只适合可信线路（规格 §9）。',
      fields: {
        pool: '地址池',
        mtu: 'MTU',
        dnsUpstream: 'DNS 上游',
        allowEncrypt: '允许 XOR 掩码',
        allowClientToClient: '允许用户互访',
        fixedAddresses: '固定地址'
      },
      hints: {
        pool: 'IPv4 网段（/8 到 /30），第一个地址是网关。',
        dnsUpstream: '网关 DNS 转发到的地址，IP 或 IP:端口。',
        allowEncrypt: 'XOR 只防随手查看，不是加密。',
        fixedAddresses: '每行一个“用户 id=地址”，地址须在地址池内且不是网关。',
        login: '用户登录名是面板用户 id，口令是 UUID 去掉连字符后的前 16 位；用户在订阅页选择 iWAN 格式可拿到 iwand 配置。'
      }
    },
    wireguard: {
      sections: {
        access: 'WireGuard 接入'
      },
      fields: {
        cidr: 'Peer CIDR',
        serverAddress: '接口地址',
        serverPrivateKey: '服务端私钥',
        serverPublicKey: '服务端公钥',
        mtu: 'MTU',
        dns: 'DNS 服务器',
        allowedIps: 'Allowed IPs'
      },
      actions: {
        generateKeypair: '生成密钥对'
      }
    },
    enable: '在节点上运行',
    show: '在订阅中显示'
  },
  // 协议表单字段 (protocolFields.js): 3x-ui 风格, 按协议/传输/TLS 显示。
  protocolFields: {
    groups: {
      protocol: '协议设置',
      transport: '传输设置',
      security: '安全设置'
    },
    none: '无',
    generate: '随机生成',
    labels: {
      server_names: '允许的 SNI',
      alpn: 'ALPN',
      flow: '流控 (Flow)',
      method: '加密方式',
      server_key: '服务端密钥',
      up_mbps: '上行带宽 (Mbps)',
      down_mbps: '下行带宽 (Mbps)',
      obfs: '混淆',
      obfs_password: '混淆密码',
      congestion_control: '拥塞控制',
      path: '路径',
      host: 'Host',
      serviceName: 'gRPC 服务名',
      server_name: 'SNI（服务器名称）',
      fingerprint: 'uTLS 指纹',
      allowInsecure: '允许不安全证书',
      dest: '目标站 (Dest)',
      short_id: 'Short ID',
      private_key: '私钥',
      public_key: '公钥',
      allow_no_sni: '无 SNI 模式',
      shape_plain: '关闭流量整形',
      max_streams: '单会话最大流数',
      wg_listen_port: 'WireGuard 外形 UDP 端口（外衣 wg）'
    },
    hints: {
      anxServerNames: '用逗号分隔，客户端使用第一个；必须是伪装目标站真实支持的域名。ANX 用户密钥由 UUID 派生，服务器公钥由节点上的 Agent 上报，订阅选择 AnixOps Net 格式即可拿到 anx:// 链接。',
      anxDest: '非 ANX 连接（含主动探测）原样转发到这里，格式 host:port。',
      anxAllowNoSni: '同时接受不带 SNI 的连接（客户端直连 IP，适合按备案拦截 SNI 的国内入口）；开启后订阅不再下发 SNI。需要 alpha.7 之后的新版 anx-node。',
      anxShapePlain: '默认使用 VisionLike 整形（抹掉 TLS-in-TLS 特征）；只在互通调试时关闭。',
      anxMaxStreams: '一个 ANX 会话里同时打开的流数上限，留空为 256。',
      anxWgListenPort: '在这个 UDP 端口同时提供外衣 wg（外形像 WireGuard），给用户连国内入口用，与无 SNI 的 A2 互为备份；订阅会带上 wg_port，客户端先试它再回落 A2。需要在防火墙放行该 UDP 端口。留空为关闭。需要带外衣 wg 的新版 anx-node。',
      serverKey: '2022 系列加密的服务端密钥；留空则按节点自动派生。',
      privateKey: '留空则保存时自动生成密钥对；已保存的私钥显示为 ********。',
      publicKey: '订阅下发给客户端的公钥；留空则由私钥推出。'
    }
  },
  tabs: {
    label: '节点分区',
    nodes: '节点',
    groups: '节点组'
  },
  // 接入节点: one entry for a new machine (docs/design/node-onboarding).
  join: {
    title: '接入节点',
    description: '在要接入的机器上运行一条命令，它会自动成为所选节点组里的节点。',
    checking: '正在检查面板能否接入节点…',
    loading: '正在准备…',
    loadFailed: '无法准备接入：{message}',
    retry: '重试',
    recheck: '重新检查',
    group: '节点组',
    newGroup: '新建节点组',
    requiresApproval: '新节点需要你确认',
    defaultCreated: '已为你建好默认节点组“{name}”：一个 VLESS Reality 协议，端口随机，已加入默认订阅分组。之后可在“节点组”里修改。',
    blocked: {
      title: '现在还不能接入节点',
      description: '先解决下面的问题再复制命令，否则命令会在节点上失败。'
    },
    checks: {
      public_url: '面板地址',
      panel_certificate: '面板证书',
      grpc: 'gRPC 地址',
      grpc_certificate: 'gRPC 证书',
      ca: '节点证书的签发密钥',
      agent_release: 'Agent 版本',
      script_signature: '安装脚本签名'
    },
    fixes: {
      public_url: '把 agent_install.public_url 设为节点访问面板的 https 地址（例如 https://panel.example.com），然后重启面板。经反向代理时，还要把代理加入 server.trusted_proxies。',
      panel_certificate: '节点通过 https 下载安装脚本，不接受不受信任的证书。页面顶部的横幅写明了证书没有就绪的原因；处理后在横幅里点“重新申请”（或在服务器上运行 anix-control tls retry）。',
      grpc: '设置 domain（会开启 gRPC 监听），或开启 grpc.enabled；节点连的地址不同于面板地址时，再设置 agent_install.grpc_target。然后重启面板。',
      grpc_certificate: '节点用系统自带的证书机构校验 gRPC 证书，不接受其他证书。设置 domain（面板在 CDN 后面时，再把 grpc.public_name 设为一个只解析不代理的域名），面板会自动申请；页面顶部的横幅写明了还没就绪的原因。',
      ca: '把 module_runtime.ca_kek（环境变量 ANIX_CONTROL_MODULE_RUNTIME_CA_KEK）设为 32 字节密钥，可用 openssl rand -base64 32 生成，然后重启面板。',
      agent_release: '把 agent_install.agent_version 设为已发布的 Agent 版本号（例如 v4.2.0），或把 Agent 发行包放进 agent_install.artifact_dir，然后重启面板。',
      script_signature: '把 agent_install.signature_file 指向本版本的 agent-install.sh.sig，或留空。'
    },
    command: {
      title: '安装命令',
      mirror: '下载源',
      mirrors: {
        control: '面板',
        cn: '国内镜像',
        github: 'GitHub',
        controlHelp: '脚本和 Agent 从这个面板下载。',
        cnHelp: '脚本从这个面板下载，Agent 从国内镜像下载；校验和始终来自面板或 GitHub。',
        githubHelp: '脚本和 Agent 从 GitHub Releases 下载；节点仍向这个面板接入。'
      },
      commandLabel: '在要接入的机器上以 root 运行',
      copy: '复制命令',
      fallback: '这个下载源还没配置：{note}',
      expires: '令牌 {time} 到期。同一条命令可以接入任意多台机器。',
      neverExpires: '令牌长期有效。同一条命令可以接入任意多台机器。',
      expired: '令牌已过期：到“节点组 › 编辑”设置新的有效期，或重置令牌，才能继续接入。',
      history: '命令里含有令牌，运行后会留在那台机器的 shell 历史里。',
      reset: '重置令牌',
      resetConfirmTitle: '重置“{name}”的令牌？',
      resetConfirmMessage: '之前复制的命令会全部失效；已经接入的节点不受影响。',
      resetDone: '令牌已重置，请使用新的命令。',
      uninstall: '卸载命令',
      uninstallHelp: '在节点上运行。加 --purge 会同时删除它的身份和数据。',
      forbidden: '只有超级管理员可以查看接入命令。请用超级管理员账号登录。',
      failed: '无法读取接入命令：{message}'
    },
    nodes: {
      title: '已接入的机器',
      waiting: '等待机器接入…',
      waitingHint: '在机器上运行上面的命令，它会出现在这里。',
      joined: '{name} 已接入',
      online: '在线',
      connecting: '等待上线',
      more: '还有 {count} 台更早接入的机器，请到节点列表里查看。'
    },
    quick: {
      name: '节点组名称',
      namePlaceholder: '香港',
      create: '创建节点组',
      failed: '无法创建节点组：{message}'
    }
  },
  approval: {
    pending: '待确认',
    pendingHint: '这个节点还在等你确认：确认前不会收到配置和用户。',
    approved: '已批准',
    rejected: '已拒绝',
    approve: '批准',
    reject: '拒绝',
    banner: '有 {count} 台机器等你确认。确认前它们不会收到配置和用户。',
    view: '查看',
    rejectTitle: '拒绝节点 {name}？',
    rejectMessage: '节点会被停用，它的证书会被吊销。',
    approvedToast: '已批准节点 {name}',
    rejectedToast: '已拒绝节点 {name}',
    failed: '无法处理：{message}'
  },
  // 节点组 (the tab and its sheet).
  groups: {
    summary: '节点组决定新节点的默认设置、协议和接入令牌。',
    add: '新建节点组',
    table: {
      label: '节点组',
      name: '名称',
      nodes: '节点',
      requireApproval: '需确认',
      token: '接入令牌',
      rate: '流量倍率',
      sort: '排序'
    },
    nodesLive: '{count} 台',
    nodesCount: '{live} 台节点，{pending} 台待确认',
    nodesPending: '{count} 待确认',
    toggleApproval: '“{name}”的新节点需要确认',
    tokenNever: '长期有效',
    tokenUntil: '{date} 到期',
    tokenExpired: '已过期',
    tokenExpiredEdit: '这个节点组的令牌已过期。请先在“编辑”里设置新的有效期。',
    approvalOn: '“{name}”的新节点从现在起需要确认。',
    approvalOff: '“{name}”的新节点不再需要确认。',
    empty: '还没有节点组',
    emptyDescription: '点“接入节点”，面板会建好默认节点组并给出命令。',
    loadFailed: '无法加载节点组',
    created: '已创建节点组 {name}',
    saved: '已保存节点组 {name}',
    deleted: '已删除节点组 {name}',
    deleteTitle: '删除节点组 {name}？',
    deleteMessage: '它的接入令牌立即失效。已接入的节点不受影响。',
    deleteAction: '删除节点组',
    deleteInUse: '组内还有节点：请先删除或迁走它们。',
    nameTaken: '已有同名的节点组。',
    caKekMissing: '还没有设置 module_runtime.ca_kek，无法创建节点组。'
  },
  groupForm: {
    titleCreate: '新建节点组',
    titleEdit: '编辑节点组',
    createDescription: '节点组的设置会复制到之后接入的每个节点。',
    create: '创建节点组',
    save: '保存更改',
    saveFailed: '无法保存：{message}',
    joining: '接入',
    fields: {
      name: '名称',
      rate: '流量倍率',
      sort: '排序',
      tags: '标签',
      monthlyLimit: '月流量限额',
      resetDay: '每月重置日',
      tokenExpiry: '令牌有效期',
      maxNodes: '节点数上限',
      requireApproval: '新节点需要我确认'
    },
    placeholders: {
      name: '香港',
      tags: '香港,IEPL,高速'
    },
    help: {
      rate: '用量按这个倍率计算。',
      sort: '数字小的排在前面。',
      monthlyLimit: '留空表示不限。',
      resetDay: '1 到 28。',
      maxNodes: '0 表示不限。',
      requireApproval: '确认前，节点不会收到配置和用户。'
    },
    expiry: {
      never: '长期有效',
      '7d': '7 天',
      '30d': '30 天',
      '90d': '90 天',
      '365d': '1 年',
      keep: '保持不变（{time} 到期）',
      expired: '已过期（请重新设置）',
      expiredError: '令牌已过期，请选择新的有效期。'
    }
  },
  statusText: {
    pending: '待激活',
    online: '在线',
    offline: '离线',
    disabled: '已停用',
    unknown: '未知'
  },
  tlsModes: {
    anxA2: 'A2 借用握手（REALITY 式）',
    none: '无 TLS',
    standard: '标准 TLS',
    reality: 'Reality（推荐）'
  },
  transports: {
    tcp: 'TCP',
    ws: 'WebSocket',
    grpc: 'gRPC',
    quic: 'QUIC',
    h2: 'HTTP/2',
    httpupgrade: 'HTTPUpgrade',
    udp: 'UDP'
  },
  messages: {
    transportUnsupported: '{type} 不能使用 {value} 传输',
    tlsUnsupported: '{type} 不能使用「{value}」',
    requiredFields: '请填写必填字段',
    saveFailed: '无法保存：{message}',
    syncSuccess: '节点“{name}”已接收同步',
    syncFailed: '同步失败：{message}',
    nodeSaved: '已保存节点 {name}',
    nodeDeleted: '已删除节点 {name}',
    nodeDisabled: '已停用节点 {name}',
    nodeEnabled: '已启用节点 {name}',
    protocolCreated: '已添加协议 {type} :{port}',
    protocolSaved: '已保存协议 {type} :{port}',
    protocolDeleted: '已删除协议 {type} :{port}',
    generateFailed: '生成失败：{message}',
    copied: '已复制',
    copyFailedManual: '无法自动复制，请选中文本后手动复制。',
    invalidJsonDetail: 'JSON 格式无效：{message}',
    quotaExceededBanner: '本页有 {count} 个节点本月流量已超限。仅作标记，不会自动限制。'
  }
}
