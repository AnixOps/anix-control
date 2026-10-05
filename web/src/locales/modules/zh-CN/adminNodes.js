// admin.nodes：节点列表与节点详情页（UI U7）。
import { AGENT_NAME } from '../../../constants/brand'

export default {
  title: '节点',
  subtitle: '为订阅提供服务的代理节点。打开节点查看协议、凭据、部署和日志。',
  addNode: '添加节点',
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
    load: '负载',
    loadValue: 'CPU {cpu} · 内存 {memory}',
    traffic: '累计流量',
    monthlyQuota: '月流量限额',
    quotaExceeded: '本月超限',
    lastHeartbeat: '最后心跳',
    connection: '连接方式',
    certificate: '证书',
    never: '从未',
    empty: '还没有节点',
    emptyDescription: `${AGENT_NAME} 用注册密钥启动后，节点会自动出现在这里；也可以手动添加。`,
    loadFailed: '无法加载节点'
  },
  actions: {
    open: '打开节点',
    manageProtocols: '管理协议',
    syncReload: '同步并重载',
    logs: '查看日志',
    deployParents: '部署父节点',
    edit: '编辑',
    delete: '删除',
    authKey: '注册密钥'
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
      apikey_stream: 'API 密钥流',
      legacy: '旧版',
      third_party: '第三方',
      offline: '离线'
    },
    connectionHint: {
      mtls_stream: '用 Agent 证书认证的 Agent Control 流',
      apikey_stream: '用节点 API 密钥认证的 Agent Control 流',
      legacy: '旧版 REST、WebSocket 或 Clean Agent 通道；agent_control.mtls 设为 required 后会被拒绝',
      third_party: '只看到第三方节点软件（UniProxy 或 v2board gRPC），不是 Agent',
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
    titleCreate: '添加节点',
    titleEdit: '编辑节点',
    createDescription: `手动添加节点。配置了注册密钥的 ${AGENT_NAME} 会自动注册节点。`,
    create: '添加节点',
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
  authKey: {
    title: '注册密钥',
    label: '注册密钥',
    hint: `把这个密钥写进 ${AGENT_NAME} 的 config.json，节点启动时会自动注册到控制面。同一密钥可注册任意多个节点。`,
    noKey: '还没有生成密钥',
    copy: '复制密钥',
    copyConfig: '复制配置',
    configTitle: `${AGENT_NAME} config.json`,
    configHint: '把它粘贴到 Agent 的 config.json。生成密钥后，配置会带上上面的密钥。',
    editSettings: '修改连接设置',
    registeredCount: '已有 {count} 个节点用这个密钥注册',
    generate: '生成密钥',
    generateAnother: '生成新密钥',
    hiddenKey: '已隐藏（********）',
    hiddenHint: '密钥只在生成时显示一次。生成新密钥后即可复制；已有密钥仍可继续使用。',
    shownOnce: '请现在复制：离开本页面后不再显示。',
    defaultName: '面板密钥 {date}'
  },
  deploy: {
    title: '部署父节点',
    summaryTitle: '{count} 个父节点',
    summaryText: '为 config/deploy/ansible/nodes 生成清单、group_vars 和命令。SSH 密码和私钥路径只保留在本页面。',
    warning: '生成的文件配合 config/deploy/ansible/nodes/deploy_v2bx.yml 使用。子节点如有需要请单独部署。',
    settingsTitle: '连接',
    hostsTitle: '主机',
    outputTitle: '生成的文件',
    loading: '正在读取父节点凭据…',
    empty: '本页没有父节点。',
    commandsLabel: '命令',
    authModes: {
      password: '密码',
      key: '私钥'
    },
    fields: {
      panelApiHost: '控制面 API 地址',
      grpcHost: 'gRPC 地址',
      grpcServerName: 'gRPC 服务器名称',
      amd64BinaryPath: 'AMD64 二进制路径',
      arm64BinaryPath: 'ARM64 二进制路径',
      coreType: '核心',
      grpcUseTLS: 'gRPC 使用 TLS',
      pluginSupervisorEnabled: '插件 Supervisor 灰度',
      pluginRoot: '插件状态目录',
      pluginSocketDir: '插件 Socket 目录',
      pluginOfficialPublicKey: '官方插件公钥'
    },
    pluginSupervisorHint: '数据面保持不变，只让这个 Agent 执行官方签名软件包的生命周期操作。',
    pluginSupervisorControlRequired: '插件 Supervisor 灰度需要 gRPC 地址使用 TLS，或使用回环 gRPC 主机。',
    pluginSupervisorKeyRequired: '复制插件 Supervisor 灰度配置前，请填写官方插件公钥。',
    pluginSupervisorKeyInvalid: '官方插件公钥必须是 Base64 编码的 Ed25519 公钥。',
    table: {
      alias: '别名',
      sshHost: 'SSH 主机',
      port: 'SSH 端口',
      user: 'SSH 用户',
      arch: '架构',
      authMode: 'SSH 登录方式',
      authValue: '密码或私钥路径'
    },
    placeholders: {
      password: 'sshpass 使用；只保留在本页面。',
      privateKey: '~/.ssh/id_ed25519'
    }
  },
  install: {
    title: '复制安装命令',
    description: '在节点 {node} 上用一条命令安装并接入 Agent。',
    intro: '生成一个绑定这个节点的一次性注册令牌，以及每个下载源的安装命令。以 root（sudo）在节点上粘贴运行：脚本安装 Agent、清理这台机器上的旧转发运行时，并等待 Agent 完成注册。再次运行同一条命令是安全的。',
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
    description: `安装并连接这个节点的 ${AGENT_NAME}。`,
    registration: 'Agent 注册',
    connection: '连接设置',
    connectionHint: '只影响生成的文件，不会保存。',
    ansible: 'Ansible',
    ansibleRoot: '生成用 Ansible 部署这个节点的清单、group_vars 和命令。',
    ansibleChild: 'Ansible 部署助手只覆盖父节点。这个子节点请单独部署。',
    installCommand: '一条命令安装',
    installCommandText: '复制一条命令，在节点上粘贴运行，即可安装 Agent 并完成注册（一次性令牌）。',
    openInstall: '复制安装命令',
    openHelper: '打开部署助手'
  },
  credentials: {
    title: '凭据',
    description: `${AGENT_NAME} 用这个节点的 API 密钥登录控制面。`,
    nodeId: '节点 ID',
    apiKey: 'API 密钥',
    apiKeyHidden: '已隐藏。读取会记入审计日志。',
    apiKeyShown: '已在下方显示（遮罩）。',
    apiKeyHelp: '已遮罩，可显示或复制。回到本分区时需要重新读取。',
    reveal: '读取 API 密钥',
    noKey: '服务器没有返回密钥',
    revealFailed: '无法读取 API 密钥：{message}',
    auditFooter: '每次读取节点凭据都会以“显示”记入审计日志。共享密钥不在这里显示。',
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
      visual: '表单'
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
    wireguard: {
      sections: {
        access: 'WireGuard 接入',
        relay: '双机中转',
        networkPolicy: '入口网络路径（可选）'
      },
      fields: {
        cidr: 'Peer CIDR',
        serverAddress: '入口接口地址',
        serverPrivateKey: '服务端私钥',
        serverPublicKey: '服务端公钥',
        mtu: 'MTU',
        dns: 'DNS 服务器',
        allowedIps: 'Allowed IPs',
        role: '节点角色',
        tunnelType: '入口到出口隧道',
        wssCompat: 'WSS 兼容模式',
        wssPath: 'WSS 路径',
        wssSecure: '校验出口证书',
        wssServerName: 'WSS 服务名称（SNI）',
        wssCaFile: '入口机 WSS CA 证书文件',
        wssCertFile: '出口机 WSS 证书文件',
        wssKeyFile: '出口机 WSS 私钥文件',
        relayServer: '出口中转主机',
        relayServerPort: '出口中转端口',
        tunPort: 'GOST TUN 端口',
        tunName: 'GOST TUN 名称',
        entryTunAddress: '入口 TUN 地址',
        exitTunAddress: '出口 TUN 地址',
        outboundIface: '出口网卡',
        exitNat: '启用出口 NAT',
        routingTable: '路由表',
        routingPriority: '路由优先级',
        networkPolicyEnabled: '启用多线路故障转移',
        networkPath: '网络路径',
        pathName: '路径名称',
        pathInterface: '网卡',
        pathSource: '源 IP',
        pathGateway: '网关',
        pathPriority: '优先级（越小越优先）',
        healthInterval: '探测间隔（秒）',
        healthTimeout: '探测超时（秒）',
        failureThreshold: '失败阈值',
        failbackDelay: '主线恢复等待（秒）'
      },
      actions: {
        generateKeypair: '生成密钥对',
        addNetworkPath: '添加网络路径',
        removeNetworkPath: '移除路径 {index}'
      },
      values: {
        entry: '国内入口',
        exit: '海外出口'
      },
      hints: {
        wssCompat: '默认使用 QUIC。WSS 会校验出口证书，只作为 UDP 中转受阻或不稳定时的兼容模式。',
        networkPolicy: '仅接管到出口中转 IP 的连接，并为每个源 IP 建立独立回程路由。普通节点无需启用；出口主机必须填写 IP 地址。'
      }
    },
    enable: '在节点上运行',
    show: '在订阅中显示'
  },
  statusText: {
    pending: '待激活',
    online: '在线',
    offline: '离线',
    disabled: '已停用',
    unknown: '未知'
  },
  tlsModes: {
    none: '无 TLS',
    standard: '标准 TLS',
    reality: 'Reality（推荐）'
  },
  transports: {
    tcp: 'TCP',
    ws: 'WebSocket',
    grpc: 'gRPC',
    quic: 'QUIC',
    h2: 'HTTP/2'
  },
  messages: {
    requiredFields: '请填写必填字段',
    saveFailed: '无法保存：{message}',
    syncSuccess: '节点“{name}”已接收同步',
    syncFailed: '同步失败：{message}',
    nodeCreated: '已添加节点 {name}',
    nodeSaved: '已保存节点 {name}',
    nodeDeleted: '已删除节点 {name}',
    nodeDisabled: '已停用节点 {name}',
    nodeEnabled: '已启用节点 {name}',
    protocolCreated: '已添加协议 {type} :{port}',
    protocolSaved: '已保存协议 {type} :{port}',
    protocolDeleted: '已删除协议 {type} :{port}',
    generateFailed: '生成失败：{message}',
    deployLoadFailed: '无法读取父节点凭据',
    copied: '已复制',
    copyFailedManual: '无法自动复制，请选中文本后手动复制。',
    invalidJsonDetail: 'JSON 格式无效：{message}',
    quotaExceededBanner: '本页有 {count} 个节点本月流量已超限。仅作标记，不会自动限制。'
  }
}
