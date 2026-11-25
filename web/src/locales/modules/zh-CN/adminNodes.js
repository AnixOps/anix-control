// admin.nodes：节点列表与节点详情页（UI U7）。
import { AGENT_NAME } from '../../../constants/brand'

export default {
  title: '节点',
  subtitle: '为订阅提供服务的代理节点。',
  confirm: {
    deleteNodeTitle: '删除节点 {name}？',
    deleteNodeMessage: '将移除节点及其凭据。此操作无法撤销。',
    deleteNodeAction: '删除节点',
    disableNodeTitle: '停用节点 {name}？',
    disableNodeMessage: '订阅将不再提供该节点，其 Agent 证书将被吊销。',
    disableNodeAction: '停用节点',
    deleteProtocolTitle: '删除协议 {type} :{port}？',
    deleteProtocolMessage: '节点 {name} 将不再提供该协议。此操作无法撤销。',
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
    searchPlaceholder: '名称或地址',
    group: '按节点组筛选',
    allGroups: '全部节点组'
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
    compatibility: {
      warning: '版本需关注',
      incompatible: '版本不兼容',
      problems: {
        agent_protocol_too_old: 'Agent 协议修订号低于主控最低要求，请升级 Agent。',
        agent_protocol_too_new: 'Agent 协议修订号高于主控，新功能未启用，请升级主控。',
        version_line_mismatch: 'Agent 与主控的主次版本号不一致，请升级至同一版本线。',
        kernel_too_old: 'anx-node 低于该 Agent 的最低要求，请升级内核。'
      }
    },
    kernelUnknown: '未上报',
    kernelUnknownHint: '由在线节点的 Agent 上报；旧版 Agent 不上报。',
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
    empty: '暂无节点',
    emptyDescription: '点击“接入节点”获取安装命令。',
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
    unavailable: '服务状态不可用：需启用机器遥测插件并授予 API 权限',
    notEnabled: '此节点未启用服务采集'
  },
  move: {
    action: '移动到节点组',
    titleOne: '移动节点 {name}',
    titleMany: '移动 {count} 个节点',
    message: '节点组的默认设置仅用于新接入的节点，移入的节点保持原有设置。',
    target: '目标节点组',
    none: '未分组',
    relays: '该节点组配置了中继，节点将经中继连接主控，约一分钟内生效。',
    moved: '已将 {count} 个节点移至“{group}”',
    removed: '已将 {count} 个节点移出节点组'
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
    loadFailed: '无法加载节点',
    notFound: '节点 #{id} 不存在',
    notFoundHint: '节点可能已被删除。',
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
    never: '暂无心跳',
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
    quotaFooter: '超出月流量限额仅作标记，不自动限制。',
    overQuota: '本月流量已超限。仅作标记，不自动限制。',
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
    noRecord: '暂无 Agent 记录',
    noRecordHint: '该节点未注册 Agent，可在“部署”中安装。',
    connectionLabel: '连接方式',
    transport: '最近通道',
    lastSeen: '最近出现',
    neverSeen: '从未',
    certificateLabel: '证书',
    notAfter: '有效期至',
    renewAfter: '续期时间',
    revokedAt: '吊销时间',
    revokeReason: '吊销原因',
    overdueNotice: 'Agent 未按时续期证书（应于 {date} 后续期），证书仍有效。',
    revokedNotice: '该 Agent 证书已吊销，须重新注册。',
    footer: '数据来自 Agent 连接清单。',
    connection: {
      mtls_stream: 'mTLS 流',
      offline: '离线'
    },
    connectionHint: {
      mtls_stream: '使用 Agent 证书认证的控制流',
      offline: '最近五分钟内无连接'
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
    parentHint: '用于组成多级中转链路。转发流量同时计入本节点及各级上级节点。',
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
      rate: '用量按此倍率计算。',
      sort: '数值小者靠前。',
      monthlyLimit: '留空表示不限。',
      monthlyResetDay: '1 到 28。'
    }
  },
  install: {
    title: '换到新机器',
    description: '将节点 {node} 迁移至另一台机器。',
    intro: '生成绑定该节点的一次性令牌及安装命令，在新机器上以 root 运行。新机器将取代原机器，命令可重复运行。',
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
    once: '令牌仅显示一次、仅可使用一次，{time} 失效。',
    mirror: '下载源',
    mirrors: {
      control: '控制面',
      cn: '国内镜像',
      github: 'GitHub',
      controlHelp: '脚本和 Agent 从本控制面下载。',
      cnHelp: '脚本从本控制面下载，Agent 从国内镜像下载；校验和来自控制面或 GitHub。',
      githubHelp: '脚本和 Agent 从 GitHub Releases 下载，令牌仍向本控制面注册。'
    },
    commandLabel: '在节点上以 root 运行',
    copy: '复制命令',
    fallback: '该下载源未配置：{note}',
    signed: '安装脚本已签名，可使用 /install.sh.sig 及官方发布公钥校验。',
    unsigned: '本控制面无安装脚本签名。如需校验，请使用 GitHub Releases 中的 agent-install.sh 及其 .sig。',
    legacy: '安装时将删除旧转发运行时（nftables 表 inet v2b_forward、ip v2b_forward、ip anixops_forward 及 v2forward-agent 服务），并列出删除内容。',
    failed: '无法生成安装命令'
  },

  deploySection: {
    title: '部署',
    description: `管理该节点的 ${AGENT_NAME}。新机器请在“节点 › 接入节点”中添加。`,
    move: '换到新机器',
    moveText: '迁移至另一台机器，或在证书失效后重新接入。',
    moveAction: '生成安装命令',
    certificate: '证书',
    certificateText: '节点使用证书连接面板，到期前自动续期。',
    certificateGone: '证书已失效，Agent 无法连接。请通过“换到新机器”重新接入。',
    certificateNone: '该节点暂无 Agent 证书。请通过“换到新机器”接入。',
    uninstall: '卸载',
    uninstallText: '在节点上运行。加 --purge 同时删除身份和数据。',
    uninstallLabel: '在节点上以 root 运行',
    copy: '复制命令'
  },
  traffic: {
    title: '流量',
    description: '经过该节点的用户流量，已按流量倍率计算。',
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
      summary: '上传 {up}，下载 {down}。峰值{bucket}：{peak}，开始于 {time}。',
      summaryEmpty: '无流量。',
      bucketHour: '一小时',
      bucketDay: '一天',
      time: '时间'
    },
    empty: '该时间范围内无流量',
    emptyHour: '该时段无上报。小时数据随流量日志保留期限清理。',
    emptyDay: '该时段无记录。',
    loadFailed: '无法加载流量',
    noteHour: '按小时（UTC 整点）汇总，按本地时区显示。',
    noteDay: '按天（Control 主机日历日）汇总，不受流量日志保留期限影响。'
  },
  rotate: {
    title: 'Agent 凭据',
    row: '轮换 Agent 凭据',
    rowHint: '吊销该节点 Agent 的凭据，并签发一次性凭据用于重新注册。',
    footer: '仅限超级管理员。轮换及原因记入审计日志，凭据本身不记录。',
    action: '轮换凭据…',
    superOnly: '仅超级管理员可轮换凭据。',
    disabledHint: '该节点已停用，请先启用。',
    confirm: {
      title: '轮换 {name} 的凭据？',
      message: 'Agent 须重新注册。仅最后一次轮换的凭据有效。',
      action: '轮换凭据',
      impact: '影响',
      revoke: '吊销该节点全部 Agent 证书、注册记录和转发链路证书。运行中的 Agent 将在下次心跳时断开。',
      issue: '签发新的一次性凭据，仅显示一次。未使用的旧凭据随即失效。',
      offline: '无需连接节点，离线时也可轮换。',
      reason: '原因',
      reasonHelp: '选填，记入审计日志。{count} / {max} 字节（汉字占 3 字节）。',
      reasonTooLong: '原因过长：{count} / {max} 字节。',
      reasonPlaceholder: '例如：主机硬盘失窃',
      ttl: '凭据有效期'
    },
    result: {
      title: '{name} 的新凭据',
      description: '仅显示一次，关闭后无法再次查看。',
      warning: '请按密码妥善保管。凭据在使用或过期前可用于以该节点身份注册 Agent，请勿分享。',
      label: '一次性凭据',
      copy: '复制凭据',
      expires: '有效期至 {time}',
      remaining: '剩余 {remaining}',
      expired: '凭据已过期，请重新轮换。',
      single: '仅可使用一次。',
      revoked: '已吊销',
      certificates: 'Agent 证书',
      enrollments: '注册记录',
      linkCertificates: '转发链路证书',
      next: '使用该凭据在节点上重新注册 Agent，步骤见指南。',
      guide: '轮换节点凭据（指南）',
      done: '完成'
    },
    errors: {
      super_admin_required: '仅超级管理员可轮换凭据。',
      node_not_found: '节点不存在。',
      node_disabled: '节点已停用，请先启用。',
      agent_pki_disabled: '内置 Agent CA 未开启（module_runtime.ca_kek），Control 无法签发凭据。',
      failed: '无法轮换凭据：{message}'
    }
  },
  credentials: {
    title: '凭据',
    description: `${AGENT_NAME} 使用 mTLS 证书连接控制面，节点无 API 密钥。`,
    nodeId: '节点 ID',
    protocolSecrets: '协议密钥',
    protocolSecretsRow: '打开协议',
    protocolSecretsFooter: '私钥和密码显示为 ********，保留即沿用原值。'
  },
  danger: {
    title: '危险区',
    description: '以下操作将改变订阅内容。',
    disable: '停用节点',
    disableHint: '订阅不再提供该节点，其 Agent 证书将被吊销。',
    disableAction: '停用…',
    enable: '启用节点',
    enableHint: '节点恢复为待激活，下次心跳后显示在线。',
    enableAction: '启用',
    delete: '删除节点',
    deleteHint: '移除节点及其协议和凭据，需输入节点名称确认。',
    deleteAction: '删除…'
  },
  protocols: {
    title: '协议',
    description: '该节点提供的协议，修改于下次同步时下发。',
    tableLabel: '{name} 的协议',
    add: '添加协议',
    empty: '暂无协议',
    emptyDescription: '添加协议后，订阅方可使用该节点。',
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
    stale: '最近上报于 {time}，数据可能已过时。',
    unsupported: '此节点无法上报服务',
    unsupportedReason: '原因：{reason}',
    unsupportedNoReason: '节点需要 systemd 和 cgroup v2。',
    waiting: '正在等待首次上报',
    waitingDescription: '启用采集后，节点约每分钟上报一次。',
    empty: '无符合筛选规则的服务',
    emptyDescription: '请在设置中修改包含或排除规则。',
    disabled: {
      title: '此节点未启用',
      description: '默认关闭。启用后仅采集 systemd 服务的单元名称、状态、CPU 和内存。',
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
      enabledHelp: '只读，不启动、停止或重启服务。',
      include: '包含规则',
      includeHelp: '每行一条，例如 nginx*.service。留空表示全部。',
      exclude: '排除规则',
      excludeHelp: '每行一条，匹配的服务不显示。',
      invalidGlob: '规则无效：{glob}',
      tooManyGlobs: '最多 {max} 条规则',
      save: '保存',
      saved: '服务设置已保存，将于节点下次接收配置时生效。',
      saveFailed: '无法保存服务设置',
      conflict: '设置已被其他操作修改，已重新加载，请检查后再保存。',
      noInstallation: '机器遥测未安装 Agent 组件，请先在插件中安装。'
    }
  },
  logs: {
    title: '日志',
    description: '节点运行日志，按时间倒序。',
    tableLabel: '{name} 的日志',
    refresh: '刷新日志',
    empty: '暂无日志',
    emptyDescription: '节点上报后将在此显示。',
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
    pickHint: '选择模板，下一步可修改全部参数。',
    portSpec: '端口 {port}',
    recommended: '推荐',
    change: '更换',
    fixed: '{value}（该协议固定）',
    blank: {
      name: '自定义',
      description: '从空白配置开始'
    },
    sections: {
      basics: '基本信息',
      connection: '传输与安全',
      publish: '发布'
    },
    enableHint: '关闭后保留配置，不在节点上运行。',
    showHint: '用户订阅中包含此入站。',
    maskedSecretsHint: '已保存的密钥显示为 ********，保留即沿用原值，填写新值则替换。',
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
      copy: '复制'
    },
    jsonStatus: {
      valid: 'JSON 有效',
      invalid: 'JSON 无效'
    },
    iwan: {
      title: 'Panabit iWAN',
      unencrypted: '不加密',
      unencryptedHint: 'iWAN 无加密和完整性校验，仅适用于可信线路。',
      fields: {
        pool: '地址池',
        mtu: 'MTU',
        dnsUpstream: 'DNS 上游',
        allowEncrypt: '允许 XOR 掩码',
        allowClientToClient: '允许用户互访',
        fixedAddresses: '固定地址'
      },
      hints: {
        pool: 'IPv4 网段（/8 至 /30），首个地址为网关。',
        dnsUpstream: '网关 DNS 上游地址，IP 或 IP:端口。',
        allowEncrypt: 'XOR 掩码不是加密。',
        fixedAddresses: '每行一条“用户 id=地址”，地址须在地址池内且非网关。',
        login: '登录名为用户 id，口令为 UUID 去除连字符后的前 16 位。iwand 配置可在订阅页以 iWAN 格式获取。'
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
      security: '安全设置',
      anxCover: '回落目标',
      anxEntry: '用户入口',
      anxTuning: '握手调优',
      anxAdvanced: '高级设置'
    },
    groupIntros: {
      anxCover: '非 ANX 连接转发至此站点。',
      anxEntry: '仅用户直连本节点时需要，中转节点保持默认。',
      anxTuning: '选填，默认使用通用预设。'
    },
    none: '无',
    needsAttention: '有提示',
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
      psk: 'PSK',
      version: '版本',
      mode: '模式',
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
      dest_stack: 'TLS 栈（dest_stack）',
      dest_behavior: 'dest 行为（dest_behavior）',
      dest_behaviors: '按 dest 的行为（dest_behaviors）',
      wg_listen_port: 'UDP 端口（外衣 wg）',
      bare_listen_port: 'TCP 端口（外衣 bare）',
      wire_gen: '对外发布的握手代',
      min_wire_gen: '接受的最低握手代'
    },
    hints: {
      anxServerNames: '逗号分隔，客户端使用第一个；须为回落目标支持的域名。',
      anxDest: '非 ANX 连接转发地址，格式 host:port。',
      anxAllowNoSni: '同时接受无 SNI 的连接，开启后订阅不下发 SNI。需 anx-node alpha.7 以上。',
      anxShapePlain: '仅用于互通调试。',
      anxMaxStreams: '单会话并发流数上限，默认 256。',
      anxWireGen: '填 2 后订阅公钥带 "{\'@\'}2"，客户端使用第 2 代握手。须在 anx-node 支持且客户端均已升级后启用，旧客户端无法使用。默认 1。',
      anxMinWireGen: '填 2 后拒绝第 1 代握手。须在发布代为 2 且 Stats auth_gen1 不再增长后启用。默认 1。需 anx-node 支持第 2 代。',
      anxDestBehavior: '粘贴该 dest 完整的 "dest_behavior": {…} 行或 JSON 对象，须包含 session_tickets 与 flight_records。留空使用栈预设。',
      anxDestBehaviorMissing: '告警：部分 dest 的 dest_behavior 缺少 session_tickets 或 flight_records，请粘贴完整行（值为 "none" 视为已填）。',
      anxDestBehaviors: '多个 dest 时使用：JSON 对象，键为 dest 项（"host:port"），值为对 dest_behavior 的覆盖。需新版 anx-node。',
      anxDestBehaviorsStale: '部分条目的站点已不在 dest 列表中，保留但不下发。',
      anxWgListenPort: '在此 UDP 端口提供外衣 wg，订阅带 wg_port。须在防火墙放行。留空关闭。需新版 anx-node。',
      anxBareListenPort: '在此 TCP 端口提供外衣 bare，须开启无 SNI 模式，订阅带 bare_port。须在防火墙放行。留空关闭。需新版 anx-node。',
      serverKey: '2022 系列加密必填，切换时自动生成。',
      snellPsk: '所有用户共用，用户另以 UUID 登录。仅 Surge 格式订阅提供本协议。需 Agent 协议修订 3 及以上。',
      snellVersion: '1 至 6。版本 4 节点兼容版本 5 客户端；混淆仅用于版本 1 至 4，模式仅用于版本 6，版本 6 的 PSK 至少 12 字节。',
      privateKey: '留空则保存时自动生成密钥对。',
      publicKey: '下发给客户端的公钥，留空则由私钥推导。'
    }
  },
  tabs: {
    label: '节点分区',
    nodes: '节点',
    groups: '分组'
  },
  networkGroups: {
    title: '网络分组',
    summary: '组网区域。节点可同时属于一个节点组和一个组网区域。',
    manage: '管理组网',
    empty: '暂无组网区域',
    loadFailed: '无法加载组网区域',
    columns: {
      name: '区域',
      cidr: '地址段',
      nodes: '节点',
      state: '状态'
    }
  },
  // 接入节点: one entry for a new machine (docs/design/node-onboarding).
  join: {
    title: '接入节点',
    description: '在目标机器上运行安装命令，接入所选节点组。',
    checking: '正在检查接入条件…',
    loading: '正在准备…',
    loadFailed: '无法准备接入：{message}',
    retry: '重试',
    recheck: '重新检查',
    group: '节点组',
    newGroup: '新建节点组',
    requiresApproval: '新节点需确认',
    defaultCreated: '已创建默认节点组“{name}”：VLESS Reality 协议，随机端口，已加入默认订阅分组。',
    blocked: {
      title: '无法接入节点',
      description: '请先解决以下问题。'
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
      public_url: '将 agent_install.public_url 设为面板 https 地址（例如 https://panel.example.com）后重启面板。使用反向代理时，须将代理加入 server.trusted_proxies。',
      panel_certificate: '面板证书未就绪，原因见页面顶部横幅。处理后点击“重新申请”，或在服务器上运行 anix-control tls retry。',
      grpc: '设置 domain 或开启 grpc.enabled；节点连接地址与面板不同时，设置 agent_install.grpc_target。然后重启面板。',
      grpc_certificate: 'gRPC 证书须由公共证书机构签发。设置 domain 后自动申请（使用 CDN 时，将 grpc.public_name 设为仅解析的域名）；原因见页面顶部横幅。',
      ca: '将 module_runtime.ca_kek（环境变量 ANIX_CONTROL_MODULE_RUNTIME_CA_KEK）设为 32 字节密钥（openssl rand -base64 32）后重启面板。',
      agent_release: '将 agent_install.agent_version 设为已发布的 Agent 版本（例如 v4.2.0），或将发行包放入 agent_install.artifact_dir，然后重启面板。',
      script_signature: '将 agent_install.signature_file 指向本版本的 agent-install.sh.sig，或留空。'
    },
    command: {
      title: '安装命令',
      mirror: '下载源',
      mirrors: {
        control: '面板',
        cn: '国内镜像',
        github: 'GitHub',
        controlHelp: '脚本和 Agent 从本面板下载。',
        cnHelp: '脚本从本面板下载，Agent 从国内镜像下载；校验和来自面板或 GitHub。',
        githubHelp: '脚本和 Agent 从 GitHub Releases 下载，节点仍接入本面板。'
      },
      commandLabel: '在目标机器上以 root 运行',
      copy: '复制命令',
      fallback: '该下载源未配置：{note}',
      expires: '令牌于 {time} 到期，可用于接入多台机器。',
      neverExpires: '令牌长期有效，可用于接入多台机器。',
      expired: '令牌已过期。请在“节点组 › 编辑”中设置新有效期或重置令牌。',
      history: '命令包含令牌，将留存于目标机器的 shell 历史。',
      reset: '重置令牌',
      resetConfirmTitle: '重置“{name}”的令牌？',
      resetConfirmMessage: '已复制的命令将全部失效，已接入节点不受影响。',
      resetDone: '令牌已重置，请使用新命令。',
      uninstall: '卸载命令',
      uninstallHelp: '在节点上运行。加 --purge 同时删除身份和数据。',
      forbidden: '仅超级管理员可查看接入命令。',
      failed: '无法读取接入命令：{message}'
    },
    nodes: {
      title: '已接入的机器',
      waiting: '等待机器接入…',
      waitingHint: '运行上方命令后，机器将在此显示。',
      joined: '{name} 已接入',
      online: '在线',
      connecting: '等待上线',
      more: '另有 {count} 台更早接入的机器，请在节点列表中查看。'
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
    pendingHint: '待确认。确认前不下发配置和用户。',
    approved: '已批准',
    rejected: '已拒绝',
    approve: '批准',
    reject: '拒绝',
    banner: '{count} 台机器待确认，确认前不下发配置和用户。',
    view: '查看',
    rejectTitle: '拒绝节点 {name}？',
    rejectMessage: '节点将被停用，证书将被吊销。',
    approvedToast: '已批准节点 {name}',
    rejectedToast: '已拒绝节点 {name}',
    failed: '无法处理：{message}'
  },
  // 节点组 (the tab and its sheet).
  groups: {
    title: '节点组',
    summary: '节点组用于归类节点，并提供接入命令和新节点的默认设置。可在节点列表中移动节点。',
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
    tokenExpiredEdit: '令牌已过期，请在“编辑”中设置新有效期。',
    approvalOn: '“{name}”的新节点需确认。',
    approvalOff: '“{name}”的新节点无需确认。',
    empty: '暂无节点组',
    emptyDescription: '点击“接入节点”，将自动创建默认节点组。',
    loadFailed: '无法加载节点组',
    created: '已创建节点组 {name}',
    saved: '已保存节点组 {name}',
    deleted: '已删除节点组 {name}',
    deleteTitle: '删除节点组 {name}？',
    deleteMessage: '接入令牌立即失效，已接入节点不受影响。',
    deleteAction: '删除节点组',
    deleteInUse: '组内仍有节点，请先将其移出。',
    nameTaken: '节点组名称已存在。',
    caKekMissing: '未设置 module_runtime.ca_kek，无法创建节点组。'
  },
  groupForm: {
    titleCreate: '新建节点组',
    titleEdit: '编辑节点组',
    createDescription: '默认设置仅用于之后接入的节点。',
    defaults: '新节点默认值',
    defaultsIntro: '仅用于之后接入的节点，已在组内或移入的节点不受影响。',
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
      requireApproval: '新节点需确认'
    },
    placeholders: {
      name: '香港',
      tags: '香港,IEPL,高速'
    },
    help: {
      rate: '用量按此倍率计算。',
      sort: '数值小者靠前。',
      monthlyLimit: '留空表示不限。',
      resetDay: '1 到 28。',
      maxNodes: '仅限制接入，0 表示不限。',
      requireApproval: '确认前不下发配置和用户。'
    },
    expiry: {
      never: '长期有效',
      '7d': '7 天',
      '30d': '30 天',
      '90d': '90 天',
      '365d': '1 年',
      keep: '保持不变（{time} 到期）',
      expired: '已过期（请重新设置）',
      expiredError: '令牌已过期，请选择新有效期。'
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
    anxA2: 'A2 外衣',
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
    transportUnsupported: '{type} 不支持 {value} 传输',
    tlsUnsupported: '{type} 不支持“{value}”',
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
    copyFailedManual: '无法自动复制，请手动复制。',
    invalidJsonDetail: 'JSON 格式无效：{message}',
    quotaExceededBanner: '本页 {count} 个节点本月流量已超限，仅作标记，不自动限制。'
  }
}
