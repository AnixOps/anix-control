// 转发页面里的入口高可用（L2）：DNS 服务商页面、路由编辑器的绑定选择和
// 路由详情的「入口高可用」卡片。与 en/forwardDns.js 键相同。
export default {
  forwardDns: {
    nav: 'DNS',
    superOnly: '仅超级管理员可添加、修改或删除 DNS 服务商。',
    page: {
      title: 'DNS 服务商',
      description: '用于将已绑定的入口主机名指向健康入口节点的 DNS 账号。',
      add: '添加服务商',
      readOnly: '只读。仅超级管理员可添加、修改或删除。',
      loadFailed: '无法加载 DNS 服务商',
      emptyTitle: '暂无 DNS 服务商',
      emptyDescription: '添加 DNS 服务账号后，可在路由编辑器中绑定入口主机名。',
      defaultEndpoint: '服务商默认',
      missing: '缺少：{names}',
      bindingCount: '{n} 个绑定',
      unused: '未使用',
      note: '凭据加密保存，不再显示，此处仅列出名称。',
      edit: '编辑…',
      editSuperOnly: '编辑（仅超级管理员）',
      delete: '删除…',
      deleteSuperOnly: '删除（仅超级管理员）',
      deleteInUse: '删除（{n} 个绑定正在使用）',
      columns: {
        name: '服务商',
        bindings: '绑定',
        endpoint: '接口地址',
        credentials: '凭据',
        updated: '更新时间'
      }
    },
    sheet: {
      createTitle: '添加 DNS 服务商',
      editTitle: '编辑 {name}',
      description: '凭据仅授予目标区域的写入权限。',
      name: '名称',
      namePlaceholder: 'cloudflare-main',
      kind: '服务商类型',
      kindFixed: '服务商类型创建后不可修改。',
      settings: '设置',
      credentials: '凭据',
      credentialsHelp: '加密保存，之后不再显示。',
      credentialsKeep: '已保存的凭据显示为 ********，保持不变即沿用，输入新值则替换。',
      stored: '已保存，保持不变即沿用。',
      create: '添加服务商',
      save: '保存'
    },
    kindHelp: {
      DNS_PROVIDER_KIND_CLOUDFLARE: '具有该区域 Zone → DNS → Edit 和 Zone → Zone → Read 权限的 API 令牌。',
      DNS_PROVIDER_KIND_ALIDNS: 'RAM 用户的 AccessKey，仅授予该域名的 alidns 记录操作。',
      DNS_PROVIDER_KIND_DNSPOD: 'CAM 子用户的 SecretId 和 SecretKey，仅授予 dnspod 记录操作。',
      DNS_PROVIDER_KIND_HUAWEICLOUD: 'IAM 用户的访问密钥（AK/SK），仅授予 dns 区域与记录集操作。',
      DNS_PROVIDER_KIND_WEBHOOK: '每次变更以 HMAC-SHA256 签名后 POST 至指定 HTTPS 接口。'
    },
    config: {
      endpoint: 'API 地址',
      url: 'Webhook 地址'
    },
    configHelp: {
      endpoint: '选填，地域 API 主机。',
      url: 'https:// 地址。'
    },
    configPlaceholder: {
      DNS_PROVIDER_KIND_CLOUDFLARE: { endpoint: 'api.cloudflare.com' },
      DNS_PROVIDER_KIND_ALIDNS: { endpoint: 'alidns.aliyuncs.com' },
      DNS_PROVIDER_KIND_DNSPOD: { endpoint: 'dnspod.tencentcloudapi.com' },
      DNS_PROVIDER_KIND_HUAWEICLOUD: { endpoint: 'dns.myhuaweicloud.com' },
      DNS_PROVIDER_KIND_WEBHOOK: { url: 'https://dns-hook.example.com/anixops' }
    },
    credential: {
      api_token: 'API 令牌',
      access_key_id: 'AccessKey ID',
      access_key_secret: 'AccessKey Secret',
      secret_id: 'SecretId',
      secret_key: 'SecretKey',
      access_key: '访问密钥（AK）',
      secret: '签名密钥'
    },
    delete: {
      title: '删除服务商 {name}？',
      message: '将删除其凭据，已发布的记录保持不变。',
      confirm: '删除服务商'
    },
    mode: {
      DNS_BINDING_MODE_DDNS: 'DDNS',
      DNS_BINDING_MODE_CNAME: 'CNAME'
    },
    modeHint: {
      DNS_BINDING_MODE_DDNS: '直接写入入口主机名记录。',
      DNS_BINDING_MODE_CNAME: '写入托管名称，入口主机名需一次性 CNAME 至该名称。'
    },
    binding: {
      hostnameHelp: '客户端访问各入口的统一名称。可绑定 DNS 服务商或自行维护记录。',
      enable: '指向健康入口',
      enableHelp: '入口节点故障或恢复时更新 A/AAAA 记录。',
      bound: '已绑定 DNS 服务商',
      provider: 'DNS 服务商',
      providerPlaceholder: '选择服务商',
      noProviders: '暂无 DNS 服务商。',
      manageProviders: '管理 DNS 服务商',
      zone: '区域',
      zoneHelp: '记录所在区域（Zone）。',
      mode: '模式',
      recordName: '托管名称',
      recordNamePlaceholder: 'r1.ha.example.net',
      recordNameHelp: '区域内由 Control 写入的名称。',
      record: '记录',
      ddnsRecord: '记录：',
      recordTypes: '记录类型',
      recordTypesHelp: '取消类型将删除对应记录。',
      type: {
        A: 'IPv4 地址',
        AAAA: 'IPv6 地址'
      },
      ttl: 'TTL',
      seconds: '秒',
      ttlHelp: '建议设为服务商允许的最小值（默认 60）。',
      vantage: '拨测节点标签',
      vantageHelp: '选填，格式 key=value（例如 vantage=cn）。带此标签的节点拨测各入口地址，多数不通的地址将从记录移除，恢复后重新加入。',
      vantageFormat: '格式为 key=value，例如 vantage=cn',
      paused: '暂停',
      pausedHelp: '保留已发布记录，不做变更。',
      cnameInstruction: '在 {host} 的 DNS 中创建 CNAME：{host} → 托管名称。',
      cnameTarget: 'CNAME 目标',
      copy: '复制',
      immutable: '服务商、区域、名称和模式绑定后不可修改。如需更换，请在路由详情中解除绑定后重新绑定。',
      mismatch: '此绑定写入 {name}，DDNS 模式下不可修改。请在路由详情中解除绑定后重新绑定。',
      needsHostname: '请先填写入口主机名。',
      required: '必填',
      saveBlocked: 'DNS 绑定未完成：{fields}。请补全或关闭绑定。',
      fieldsSeparator: '、',
      incompleteShort: 'DNS 绑定未完成',
      outsideZone: '名称须在该区域内。',
      typeRequired: '至少选择一种记录类型。',
      ttlRange: '1 至 {max} 秒。',
      saveFailed: '路由已保存，但 DNS 绑定未保存：{message}'
    },
    card: {
      title: '入口高可用',
      description: '入口主机名的 DNS 记录跟随健康的入口节点。',
      loadFailed: '无法加载 DNS 状态',
      edit: '编辑绑定',
      unbind: '解除绑定…',
      unbindSuperOnly: '仅超级管理员可解除绑定',
      unboundHost: '{host} 未绑定 DNS 服务商，记录须自行维护。',
      unboundNoHost: '此路由无入口主机名，请在编辑器中填写后绑定。',
      bind: '绑定 DNS',
      setHostname: '填写入口主机名',
      lastError: '最近错误',
      nextAttempt: '下次尝试：{when}',
      hostname: '入口主机名',
      mode: '模式',
      zone: '区域',
      ttl: 'TTL',
      ttlValue: '{n} 秒',
      published: '发布时间',
      evaluated: '评估时间',
      never: '尚未发布',
      records: '记录',
      type: '类型',
      publishedValues: '已发布',
      desiredValues: '期望',
      none: '无',
      keepPublished: '无健康入口：保留已发布',
      entries: '入口节点',
      noAddress: '无公网地址',
      unreachable: '拨测不通：{addresses}（{n} 个拨测节点），已从记录移除',
      inRotation: '轮换中',
      outOfRotation: '已移出',
      joining: '健康 {n}/3，即将加入',
      leaving: '异常 {n}/3，即将移出',
      timing: '每 10 秒评估：连续 3 次异常移出，连续 3 次健康加入。每 30 秒刷新。'
    },
    unbind: {
      title: '解除 {host} 的绑定？',
      description: 'Control 将不再维护该主机名的记录。',
      purge: '同时删除 Control 发布的记录',
      purgeHelp: '不勾选则保留记录。',
      purgeFailedHint: '绑定已保留。可重试，或不删除记录解除绑定。',
      confirm: '解除绑定'
    },
    state: {
      ok: '正常',
      pending: '等待发布',
      degraded: '降级',
      error: '错误',
      rate_limited: '限流中',
      paused: '已暂停',
      unbound: '未绑定',
      route_missing: '路由不存在',
      hostname_mismatch: '主机名不一致'
    },
    stateHelp: {
      ok: '记录即为健康入口的地址。',
      pending: '未发布：绑定后尚无健康入口。',
      degraded: '无健康入口，保留最后发布的记录。',
      error: '服务商拒绝或失败，将退避重试。',
      rate_limited: '服务商调用额度已用尽，变更将稍后进行。',
      paused: '绑定或路由已暂停或受限，不做变更。',
      unbound: '未绑定 DNS 服务商。',
      route_missing: '路由已删除，请解除绑定。',
      hostname_mismatch: 'DDNS 模式下入口主机名与绑定名称不一致，请解除绑定后重新绑定。'
    },
    reason: {
      healthy: '健康',
      converging: '收敛中',
      not_in_inventory: '不在清单中',
      no_address: '无公网地址',
      never_reported: '从未上报',
      report_stale: '上报过期',
      offline: '离线',
      hop_error: '跳错误',
      upstreams_down: '上游全部不可用',
      vantage_unreachable: '拨测全部不通'
    },
    codes: {
      secret_store_unavailable: '未配置凭据加密密钥（module_runtime.ca_kek）。',
      provider_in_use: '此服务商正被路由绑定使用，请先解除绑定。',
      binding_exists: '此路由，或此服务商、区域和名称，已有绑定。',
      name_taken: '已有同名服务商。',
      unknown_route: '路由未保存。',
      unknown_provider: 'DNS 服务商不存在。',
      entry_hostname_required: '路由无入口主机名。',
      hostname_mismatch: 'DDNS 模式下记录名须为路由入口主机名。',
      immutable: '服务商、区域、名称和模式不可修改，请解除绑定后重新绑定。'
    },
    errors: {
      dns_purge_failed: 'DNS 服务商未能删除记录。'
    },
    toast: {
      created: '已添加 {name}',
      saved: '已保存 {name}',
      deleted: '已删除 {name}',
      unbound: '已解除 {host} 的绑定'
    }
  }
}
